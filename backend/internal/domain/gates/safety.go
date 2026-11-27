package gates

import (
	"regexp"
	"strings"
)

// protein is a food group with a core-temperature minimum.
type protein struct {
	code     string
	keywords []string
	// minC is the stricter of USDA FSIS and BfR in °C:
	//   USDA FSIS Safe Minimum Internal Temperature chart: ground meat 71
	//   (160 F), poultry 74 (165 F), whole pork and fish 63 (145 F).
	//   BfR "Hinweise zum Schutz vor Lebensmittelinfektionen": 70 C core for
	//   at least 2 minutes for meat, poultry and fish.
	minC float64
}

// Order matters only for deterministic output. Mince is its own group so that
// "Rinderhack" is not whole-muscle beef.
var proteins = []protein{
	{"mince", []string{"hack", "mince", "ground beef", "ground pork", "ground turkey", "ground meat", "bolognese", "frikadelle", "bratwurst masse"}, 71},
	{"poultry", []string{"haehnchen", "huehnchen", "huehner", "huhn", "pute", "truthahn", "ente", "gans", "gefluegel", "chicken", "turkey", "duck", "poultry"}, 74},
	{"pork", []string{"schwein", "pork", "nacken", "kassler", "schnitzel"}, 70},
	{"fish", []string{
		"fisch", "lachs", "forelle", "kabeljau", "dorade", "seelachs", "thunfisch", "salmon", "cod", "trout", "tuna", "fish", "zander", "heilbutt",
		"scholle", "rotbarsch", "hering", "makrele", "sardine", "karpfen", "barsch", "saibling", "pangasius", "tilapia",
		"plaice", "redfish", "herring", "mackerel", "perch", "pollock",
	}, 70},
}

var notPoultry = strings.NewReplacer("huehnerbruehe", " ", "huehnerfond", " ")

// fishSauce is cut out of ingredient names before the protein check.
var notFish = strings.NewReplacer("fischsauce", " ", "fish sauce", " ", "fischstaebchen", " ", "fischfond", " ")

var wholeBeefLamb = []string{"rind", "beef", "steak", "lamm", "lamb", "roastbeef", "entrecote", "filet", "huefte"}

// proteinsIn lists the protein groups present in the recipe's ingredients.
func (c *checker) proteinsIn() []protein {
	var out []protein
	for _, p := range proteins {
		for i := range c.names {
			n := c.names[i]
			switch p.code {
			case "fish":
				n = notFish.Replace(n)
			case "poultry":
				n = notPoultry.Replace(n)
			}
			if p.code != "mince" && containsAny(n, proteins[0].keywords) {
				continue // "Haehnchenhack" and "Schweinehack" are mince
			}
			if containsAny(n, p.keywords) {
				out = append(out, p)
				break
			}
		}
	}
	return out
}

func (c *checker) hasWholeBeefLamb() bool {
	for _, n := range c.names {
		if containsAny(n, wholeBeefLamb) && !containsAny(n, proteins[0].keywords) && !containsAny(n, []string{"rinderbruehe", "rinderfond", "beef stock", "beef broth", "rinderbrühe"}) {
			return true
		}
	}
	return false
}

var (
	tempRe = regexp.MustCompile(`(\d{2,3})\s*(?:°|º|grad|degrees?)\s*(f\b|fahrenheit)?`)
	// negated doneness: "until no longer pink", "nicht mehr rosa".
	negDoneRe = regexp.MustCompile(`(no longer|not|nicht mehr|nicht|kein\w*|nichts mehr|ohne)\s+(?:\w+\s+)?(?:pink|rosa|blutig|rare|roh)`)
	// raw-ish doneness wording, en and de; "rosa pfeffer" is a spice.
	rareRe  = regexp.MustCompile(`medium[ -]rare|\brare\b|(?:cook\w* to|bis|until)\s+medium\s*(?:doneness|gar)\b|medium gebraten|medium gar|blutig|englisch gebraten|\brosa\b|\bpink\b|saignant|halb durch|halbroh|underdone`)
	spiceRe = regexp.MustCompile(`rosa pfeffer|rosapfeffer|rosa beeren|pink pepper\w*|rosa pfefferbeeren`)
)

var coreWords = []string{"kerntemp", "im kern", "core", "internal", "innentemp", "thermometer", "center", "centre", "kerntemperatur"}

// temperatures blocks a stated core temperature below the group's minimum and
// raw-ish doneness wording for mince, poultry and pork. Decision: oven or pan
// temperatures are not core temperatures, so only a step that talks about the
// core or a thermometer counts; fish is blocked by a stated temperature only,
// since a seared-rare fish steak is stated by cooks as a doneness, not a
// temperature. Whole-muscle beef and lamb "rare" is a flag (flagsAll).
func (c *checker) temperatures() {
	ps := c.proteinsIn()
	if len(ps) == 0 {
		return
	}
	for si, s := range c.steps {
		// The proteins this step names, or all of the recipe's.
		var mine []protein
		for _, p := range ps {
			if containsAny(s, p.keywords) {
				mine = append(mine, p)
			}
		}
		if len(mine) == 0 {
			mine = ps
		}
		if containsAny(s, coreWords) {
			for _, m := range tempRe.FindAllStringSubmatch(s, -1) {
				t := parseNum(m[1])
				if m[2] != "" {
					t = (t - 32) * 5 / 9
				}
				for _, p := range mine {
					if t+0.5 < p.minC { // 165 F is 73.9 C, 160 F 71.1 C
						c.block("core_temp_low", -1, si, "Core temperature "+m[1]+" is below the safe minimum for "+p.code+".",
							"group", p.code, "stated", m[1], "min", itoa(int(p.minC)))
						break
					}
				}
			}
		}
		if rare, group := doneness(s); rare {
			for _, p := range mine {
				if p.code != "fish" {
					c.block("rare_unsafe", -1, si, "A raw or rare finish is not safe for "+p.code+".", "group", p.code, "wording", group)
					break
				}
			}
		}
	}
}

// doneness finds raw-ish wording in a folded step, outside negations and
// spice names.
func doneness(s string) (bool, string) {
	s = spiceRe.ReplaceAllString(negDoneRe.ReplaceAllString(s, " "), " ")
	if m := rareRe.FindString(s); m != "" {
		return true, strings.TrimSpace(m)
	}
	return false, ""
}

// Keep days (cooked dishes): USDA FSIS "Leftovers and Food Safety" (3-4 days
// refrigerated), FSIS/BfR fish and rice (Bacillus cereus) shorter.
const (
	keepDefault = 4
	keepMeat    = 3 // mince and poultry
	keepFish    = 2
	keepRice    = 1
)

var riceKeywords = []string{"=reis", "-reis", "=rice", "-rice"}

func (c *checker) keepDays() {
	if c.r.KeepDays <= 0 {
		return
	}
	limit, group := keepDefault, "dish"
	ps := c.proteinsIn()
	for _, p := range ps {
		switch p.code {
		case "mince", "poultry":
			if keepMeat < limit {
				limit, group = keepMeat, p.code
			}
		case "fish":
			if keepFish < limit {
				limit, group = keepFish, "fish"
			}
		}
	}
	for _, ws := range c.ingWords {
		if anyWord(ws, riceKeywords) && keepRice < limit {
			limit, group = keepRice, "rice"
		}
	}
	if c.r.KeepDays > limit {
		c.block("keep_days", -1, -1, "Keeping it "+itoa(c.r.KeepDays)+" days is longer than is safe for "+group+".",
			"days", itoa(c.r.KeepDays), "max", itoa(limit), "group", group)
	}
}
