package gates

import (
	"regexp"
	"strings"
)

var (
	kidneyKw = []string{"kidneybohne", "kidney bean", "kidney-bohne", "rote bohne", "red bean", "weisse bohne", "white bean", "cannellini", "weisse riesenbohne"}
	cannedKw = []string{"dose", "canned", "tinned", "aus der dose", "gekocht", "vorgekocht", "cooked", "glas", "abgetropft", "drained", "konserve"}
	driedRe  = regexp.MustCompile(`getrocknet|trocken|dried|\bdry\b|\broh\w*|\braw\b|ungekocht|uncooked`)
	// soakDurRe cuts a soak duration out of a step, so that "12 Std.
	// einweichen" does not count as boiling time.
	soakDurRe  = regexp.MustCompile(`\d+(?:[.,]\d+)?\s*(?:min\w*|std\.?|stunden?|h|hours?|hr)\s*(?:lang\s+)?(?:ein|ueber\s*nacht\s*)?(?:weichen|eingeweicht|soak\w*)|soak\w*[^.;]*?\d+(?:[.,]\d+)?\s*(?:min\w*|std\.?|stunden?|h|hours?|hr)\b`)
	boilWord   = `(?:kochen|boil\w*|aufkoch\w*|durchkoch\w*|sprudelnd\w*|kochend\w*)`
	durUnit    = `(min\w*|std\.?|stunden?|h|hours?|hr)\b`
	durBefore  = regexp.MustCompile(`(\d+(?:[.,]\d+)?)\s*` + durUnit + `\.?\s*(?:lang\s+)?(?:\w+\s+){0,3}?` + boilWord)
	durAfter   = regexp.MustCompile(boilWord + `\s+(?:\w+\s+){0,3}?(?:for\s+|fuer\s+|ca\.?\s+|mindestens\s+)?(\d+(?:[.,]\d+)?)\s*` + durUnit)
	soakKw     = []string{"einweichen", "eingeweicht", "soak", "ueber nacht", "overnight"}
	slowKw     = []string{"slow cooker", "slowcooker", "schongarer", "crockpot", "crock pot", "multikocher"}
	boilKw     = []string{"sprudelnd", "aufkoch", "boil", "durchkoch", "hart kochen", "kochen", "kochend"}
	simmerOnly = regexp.MustCompile(`koecheln|simmer|ziehen lassen`)
)

// hazards: kidney beans, garlic and herbs in oil, canning.
func (c *checker) hazards() {
	c.kidneyBeans()
	c.garlicOil()
	c.canning()
}

// boilMinutes is the longest duration a step pairs with a boil word, soak
// durations left out.
func boilMinutes(s string) float64 {
	s = soakDurRe.ReplaceAllString(s, " ")
	best := 0.0
	for _, re := range []*regexp.Regexp{durBefore, durAfter} {
		for _, m := range re.FindAllStringSubmatch(s, -1) {
			v := parseNum(m[1])
			if strings.HasPrefix(m[2], "std") || strings.HasPrefix(m[2], "stunde") || m[2] == "h" || strings.HasPrefix(m[2], "hour") || m[2] == "hr" {
				v *= 60
			}
			best = max(best, v)
		}
	}
	return best
}

// kidneyBeans blocks dried kidney beans without a hard boil of at least 10
// minutes (phytohaemagglutinin; BfR "Rohe Bohnen sind giftig" and FDA Bad Bug
// Book: boil 10 minutes; a slow cooker does not reach it). A bean line counts
// as dried when it says so, or when a step soaks it or uses a slow cooker;
// otherwise it is assumed canned or cooked.
func (c *checker) kidneyBeans() {
	for i, n := range c.names {
		if !containsAny(n, kidneyKw) {
			continue
		}
		dried := driedRe.MatchString(n)
		if !dried && containsAny(n, cannedKw) {
			continue
		}
		for _, s := range c.steps {
			if containsAny(s, soakKw) || containsAny(s, slowKw) && containsAny(s, []string{"bohne", "bean"}) {
				dried = true
			}
		}
		if !dried {
			continue
		}
		boiled := false
		for _, s := range c.steps {
			if containsAny(s, boilKw) && !simmerOnly.MatchString(s) && !containsAny(s, slowKw) && boilMinutes(s) >= 10 {
				boiled = true
			}
		}
		if !boiled {
			c.block("kidney_beans_unboiled", i, -1, "Dried kidney beans must be boiled hard for at least 10 minutes.", "name", c.r.Ingredients[i].Name)
		}
		return
	}
}

var (
	oilStoredRe = regexp.MustCompile(`(?:in|mit)\s+(?:oliven|sonnenblumen|raps)?oel\s+(?:einlegen|eingelegt|ansetzen|aufbewahr|lagern)|(?:cover|submerge|immerse|store|keep|preserve)\w*\b.*\bin\s+(?:olive\s+)?oil|mit oel bedeck|in oel bedeck|ansetzen|infus`)
	storeWord   = regexp.MustCompile(`aufbewahr|lagern|\bstore\b|\bstoring\b|keep in|haltbar|einlegen|kuehlschrank|fridge|refrigerat|\bglas\b|\bjar\b`)
)

var (
	garlicHerbKw = []string{"knoblauch", "garlic", "kraeuter", "herb", "rosmarin", "rosemary", "basilikum", "basil", "thymian", "thyme", "oregano", "chili", "peperoni"}
	infusedKw    = []string{"knoblauchoel", "garlic oil", "kraeuteroel", "herb oil", "infused oil", "knoblauch in oel", "garlic in oil", "herbs in oil", "kraeuter in oel", "rosmarinoel", "chilioel"}
)

// garlicOil blocks garlic or herbs infused in oil that is stored: botulism
// risk (BfR; FDA: garlic-in-oil must be refrigerated and used within days,
// or acidified, which a home recipe cannot verify). Frying garlic in oil is
// not an infusion: the recipe needs an "in oil" storing step or an infused
// oil ingredient, plus keep days or storage wording.
func (c *checker) garlicOil() {
	infusion, at := false, -1
	for i, n := range c.names {
		if containsAny(n, infusedKw) {
			infusion, at = true, i
		}
	}
	step := -1
	for si, s := range c.steps {
		if containsAny(s, garlicHerbKw) && oilStoredRe.MatchString(s) {
			infusion, step = true, si
		}
	}
	if !infusion {
		return
	}
	stored := c.r.KeepDays > 0
	for _, s := range c.steps {
		if storeWord.MatchString(s) && strings.Contains(s, "oel") {
			stored = true
		}
	}
	if stored {
		c.block("garlic_herbs_in_oil", at, step, "Garlic or herbs stored in oil can grow botulism; make it fresh and use it at once.")
	}
}

var (
	canAlways = []string{"einwecken", "einmachen", "canning", "fermentier", "konservier", "haltbar machen", "sterilisier", "preserving", "water bath", "wasserbad einkochen"}
	canJar    = regexp.MustCompile(`glaeser|\bglas\b|\bjar\b|\bjars\b|mason|weck`)
	canMaybe  = []string{"einkochen", "preserve", "pickle", "pickling", "einlegen in"}
)

// canning blocks canning, preserving and fermenting: no tested ratio can be
// checked here. "einkochen" also means reducing a sauce, so it only counts
// with a jar in the same step.
func (c *checker) canning() {
	for si, s := range c.steps {
		if containsAny(s, canAlways) || containsAny(s, canMaybe) && canJar.MatchString(s) {
			c.block("canning", -1, si, "Canning, preserving or fermenting needs a tested recipe.")
			return
		}
	}
	for i, n := range c.names {
		if containsAny(n, []string{"eingemacht", "eingewecktes"}) {
			c.block("canning", i, -1, "Canning, preserving or fermenting needs a tested recipe.")
			return
		}
	}
}
