package gates

import (
	"fmt"
	"strings"
)

// band is an inclusive range of a ratio to flour weight (baker's percent
// divided by 100).
type band struct{ lo, hi float64 }

// bakeKind holds the bands of one kind of baked good. Sources: Ruhlman,
// "Ratio" (bread 5:3 flour:water, pound cake 1:1:1:1, pie dough 3:2:1,
// cookie 3:2:1), and Rose Levy Beranbaum / King Arthur baker's percentages
// for lean to wet doughs. Bands are deliberately wide: they catch a recipe
// that cannot work, not a style.
type bakeKind struct {
	code     string
	keywords []string // in the recipe name
	liquid   band     // water, milk and eggs
	fat      band
	sugar    band
	// leavening bands by type; checked only when the recipe has that type.
	freshYeast, dryYeast, powder, soda band
}

var bakeKinds = []bakeKind{
	{
		"bread",
		[]string{"brot", "bread", "broetchen", "bagel", "focaccia", "pizzateig", "ciabatta", "baguette"},
		band{0.5, 0.9},
		band{0, 0.15},
		band{0, 0.1},
		band{0.01, 0.06},
		band{0.003, 0.02},
		band{0.01, 0.08},
		band{0.003, 0.03},
	},
	{
		"cake",
		[]string{"kuchen", "cake", "torte", "muffin", "brownie", "biskuit", "gugelhupf", "ruehrteig"},
		band{0.6, 1.8},
		band{0.2, 1.1},
		band{0.3, 1.6},
		band{0.01, 0.06},
		band{0.003, 0.02},
		band{0.01, 0.08},
		band{0.003, 0.03},
	},
	{
		"pastry",
		[]string{"muerbeteig", "moerbeteig", "pastry", "plaetzchen", "cookie", "keks", "tarte", "quiche", "crumble", "blaetterteig"},
		band{0.05, 0.6},
		band{0.3, 0.9},
		band{0, 0.7},
		band{0.01, 0.06},
		band{0.003, 0.02},
		band{0.01, 0.08},
		band{0.003, 0.03},
	},
}

var (
	flourKw  = []string{"=mehl", "=flour", "-weizenmehl", "-dinkelmehl", "-roggenmehl", "-vollkornmehl", "-wheat flour"}
	liquidKw = []string{"wasser", "water", "milch", "milk", "=ei", "=eier", "=egg", "=eggs", "eigelb", "eiweiss"}
	fatKw    = []string{"butter", "margarine", "oel", "oil", "schmalz", "shortening"}
	sugarKw  = []string{"zucker", "sugar", "honig", "honey", "ahornsirup", "maple syrup"}
)

// baking checks the flour/liquid/fat/sugar/leavening ratios of bread, cake
// and pastry. A recipe is baked goods when its name says so and it has flour
// with a known weight; recipes without flour grams are not checked.
func (c *checker) baking() {
	name := strings.ToLower(c.r.Name)
	name = strings.NewReplacer("ä", "ae", "ö", "oe", "ü", "ue", "ß", "ss",
		"pfannkuchen", " ", "reibekuchen", " ", "kartoffelkuchen", " ", "fischkuchen", " ", "pancake", " ").Replace(name)
	var kind *bakeKind
	for i := range bakeKinds {
		if containsAny(name, bakeKinds[i].keywords) {
			kind = &bakeKinds[i]
			break
		}
	}
	if kind == nil {
		return
	}
	var flour, liquid, fat, sugar, fresh, dry, powder, soda float64
	for i, in := range c.r.Ingredients {
		g, ws, n := in.Grams, c.ingWords[i], c.names[i]
		if g <= 0 {
			continue
		}
		switch {
		case anyWord(ws, flourKw):
			flour += g
		case containsAny(n, []string{"backpulver", "baking powder"}):
			powder += g
		case containsAny(n, []string{"natron", "baking soda"}):
			soda += g
		case containsAny(n, []string{"trockenhefe", "dry yeast", "instant yeast", "instanthefe"}):
			dry += g
		case containsAny(n, []string{"hefe", "yeast"}):
			fresh += g
		case containsAny(n, []string{"buttermilch", "buttermilk"}) || anyWord(ws, liquidKw):
			liquid += g
		case anyWord(ws, fatKw):
			fat += g
		case anyWord(ws, sugarKw):
			sugar += g
		}
	}
	if flour <= 0 {
		return
	}
	check := func(what string, amount float64, b band) {
		r := amount / flour
		if amount <= 0 && b.lo > 0 && (what == "liquid" || what == "fat") {
			c.block("baking_ratio", -1, -1, "A "+kind.code+" needs "+what+".", "kind", kind.code, "part", what, "ratio", "0")
			return
		}
		if amount > 0 && (r < b.lo || r > b.hi) {
			c.block("baking_ratio", -1, -1, "The "+what+" to flour ratio of this "+kind.code+" is outside what works.",
				"kind", kind.code, "part", what, "ratio", fmtRatio(r), "min", fmtRatio(b.lo), "max", fmtRatio(b.hi))
		}
	}
	check("liquid", liquid, kind.liquid)
	check("fat", fat, kind.fat)
	check("sugar", sugar, kind.sugar)
	check("fresh_yeast", fresh, kind.freshYeast)
	check("dry_yeast", dry, kind.dryYeast)
	check("baking_powder", powder, kind.powder)
	check("baking_soda", soda, kind.soda)
}

func fmtRatio(r float64) string { return fmt.Sprintf("%.2f", r) }
