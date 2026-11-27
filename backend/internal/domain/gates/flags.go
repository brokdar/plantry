package gates

import (
	"regexp"
	"strings"
)

// flagsAll runs the GATE-3 flags: shown, not blocking.
func (c *checker) flagsAll() {
	c.outliers()
	c.rawEgg()
	c.rareMuscle()
	c.sodium()
	if c.r.Versions > 0 && c.r.Versions < c.p.MinVersions {
		c.flag("few_versions", -1, -1, "Only "+itoa(c.r.Versions)+" versions were checked.",
			"versions", itoa(c.r.Versions), "wanted", itoa(c.p.MinVersions))
	}
}

// outlier groups: the most grams of one ingredient per portion that is still
// plausible. Upper bounds only (a garnish is small on purpose). Starting
// values from DGE portion guidance (meat/fish 150 g cooked, about 250 g raw
// at the high end of mains; dry rice/pasta 125 g at the high end; potatoes
// 400 g; vegetables 400 g; added fat 30 g) with margin.
var outlierGroups = []struct {
	code     string
	keywords []string
	maxG     float64
}{
	{"meat_fish", []string{"hack", "haehnchen", "huehnchen", "huhn", "pute", "schwein", "rind", "lamm", "lachs", "fisch", "chicken", "beef", "pork", "salmon", "steak", "filet"}, 350},
	{"grain", []string{"=reis", "-reis", "=rice", "nudel", "pasta", "spaghetti", "penne", "couscous", "bulgur"}, 200},
	{"potato", []string{"kartoffel", "potato"}, 600},
	{"fat", []string{"=oel", "-oel", "butter", "margarine", "schmalz", "=oil", "-oil"}, 40},
	{"sugar", []string{"zucker", "sugar"}, 80},
	{"salt", []string{"=salz", "=salt"}, 6},
}

// outliers flags an ingredient whose grams per portion exceed its group's
// band. Skipped without portions.
func (c *checker) outliers() {
	if c.r.Portions <= 0 {
		return
	}
	for i, in := range c.r.Ingredients {
		if in.Grams <= 0 || in.ToTaste {
			continue
		}
		per := in.Grams / float64(c.r.Portions)
		for _, g := range outlierGroups {
			if anyWord(c.ingWords[i], g.keywords) {
				if per > g.maxG {
					c.flag("amount_outlier", i, -1, in.Name+" is a lot per portion.",
						"name", in.Name, "group", g.code, "per_portion_g", itoa(int(per)), "max_g", itoa(int(g.maxG)))
				}
				break
			}
		}
	}
}

var (
	rawEggDishes = []string{"tiramisu", "mayonnaise", "mayo", "aioli", "carbonara", "mousse au chocolat", "schokoladenmousse", "chocolate mousse", "zabaione", "hollandaise", "eierlikoer", "eggnog", "cookie dough", "caesar"}
	eggKw        = []string{"=ei", "=eier", "eigelb", "eiweiss", "=egg", "=eggs", "egg yolk", "egg white"}
	rawEggRe     = regexp.MustCompile(`rohe[nrsm]?\s+(?:\w+\s+)?(?:ei|eier|eigelb|eiweiss|egg)|raw\s+(?:\w+\s+)?egg|lightly cooked|weichgekocht|weich gekocht|soft[- ]boiled|runny|fluessig\w* eigelb|pochiert|poached`)
)

// rawEgg flags a dish that is known to use raw or lightly cooked egg or says
// so in a step. A pasteurised egg ingredient removes the flag.
func (c *checker) rawEgg() {
	at := -1
	for i, ws := range c.ingWords {
		if anyWord(ws, eggKw) || containsAny(c.names[i], []string{"egg yolk", "egg white"}) {
			if containsAny(c.names[i], []string{"pasteuris", "pasteuriz"}) {
				return
			}
			at = i
		}
	}
	if at < 0 {
		return
	}
	name := strings.NewReplacer("ä", "ae", "ö", "oe", "ü", "ue").Replace(strings.ToLower(c.r.Name))
	hit := containsAny(name, rawEggDishes)
	step := -1
	for si, s := range c.steps {
		if rawEggRe.MatchString(s) {
			hit, step = true, si
		}
	}
	if hit {
		c.flag("raw_egg", at, step, "Raw or lightly cooked egg: not for pregnancy, small children or weak immunity.")
	}
}

// rareMuscle flags rare whole-muscle beef or lamb (safe by USDA when seared,
// but a risk for vulnerable eaters).
func (c *checker) rareMuscle() {
	if !c.hasWholeBeefLamb() {
		return
	}
	for si, s := range c.steps {
		if rare, w := doneness(s); rare {
			c.flag("rare_whole_muscle", -1, si, "Rare beef or lamb: not for pregnancy or weak immunity.", "wording", w)
			return
		}
	}
}

// sodiumMgPer100g, folded keyword to sodium in mg per 100 g. Sources: BLS
// 3.02 / USDA FoodData Central (table salt 39 300 mg; soy sauce about 5 500;
// fish sauce about 7 800; stock cubes and bouillon powder about 8 000; miso
// about 3 700).
var sodiumMgPer100g = []struct {
	keywords []string
	mg       float64
}{
	{[]string{"=salz", "=salt", "meersalz", "sea salt", "kochsalz"}, 39300},
	{[]string{"sojasauce", "soy sauce", "shoyu", "tamari"}, 5500},
	{[]string{"fischsauce", "fish sauce"}, 7800},
	{[]string{"bruehwuerfel", "bouillon", "stock cube", "bruehpulver", "bruehepulver"}, 8000},
	{[]string{"miso"}, 3700},
}

// sodium flags more than Params.MaxSodiumMgPerPortion from the listed salty
// ingredients (the WHO's 2 g sodium a day makes 2 g a portion a lot).
func (c *checker) sodium() {
	if c.r.Portions <= 0 {
		return
	}
	var total float64
	for i, in := range c.r.Ingredients {
		if in.Grams <= 0 || in.ToTaste {
			continue
		}
		for _, s := range sodiumMgPer100g {
			if anyWord(c.ingWords[i], s.keywords) || containsAny(c.names[i], multiWord(s.keywords)) {
				total += in.Grams * s.mg / 100
				break
			}
		}
	}
	if per := total / float64(c.r.Portions); per > c.p.MaxSodiumMgPerPortion {
		c.flag("high_sodium", -1, -1, "High in sodium per portion.", "sodium_mg", itoa(int(per)), "max_mg", itoa(int(c.p.MaxSodiumMgPerPortion)))
	}
}
