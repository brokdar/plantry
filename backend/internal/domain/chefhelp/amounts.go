package chefhelp

import (
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/units"
)

// kitchenAmount renders an amount of the named ingredient as a cook measures
// it: grams and ml rounded as units.Display does, small volumes as spoons in
// halves, a large volume of a solid (cheese, vegetables, dry grains) in
// grams, counts in whole pieces, other units in halves.
func kitchenAmount(amount float64, unit, name string) string {
	base, u := units.ToBase(amount, unit)
	if u == "ml" && base > 60 {
		if g, ok := food.SolidGrams(name, base); ok {
			q := units.Display(g, "g")
			return number(q.Amount) + " " + q.Unit
		}
	}
	switch {
	case u == "ml" && base < 15:
		return spoon(base/5, "tsp")
	case u == "ml" && base <= 60:
		return spoon(base/15, "tbsp")
	case u == "g" || u == "ml":
		q := units.Display(amount, unit)
		return number(q.Amount) + " " + q.Unit
	case u == "tsp" || u == "tbsp":
		return spoon(amount, u)
	}
	if units.IsCount(units.Normalize(unit)) {
		return number(math.Max(1, math.Round(amount))) + " " + unit
	}
	return strings.TrimSpace(number(halves(amount)) + " " + unit)
}

func spoon(v float64, u string) string { return number(halves(v)) + " " + u }

// halves rounds to the nearest half, never to zero.
func halves(v float64) float64 { return math.Max(0.5, math.Round(v*2)/2) }

// quantityRE finds a number in a help line: 12, 1.5, 1,5, 1/2, ½, 1½.
var quantityRE = regexp.MustCompile(`\d+\s?[½¼¾⅓⅔]|[½¼¾⅓⅔]|\d+/\d+|\d+(?:[.,]\d+)?`)

// notAmountRE is what follows a number that is a time, temperature, size or
// count of repeats, not an ingredient amount.
var notAmountRE = regexp.MustCompile(`(?i)^\s*(?:[-–]\s*\d+(?:[.,]\d+)?\s*)?(?:°|º|degrees?\b|grad\b|min|sec|s\b|h\b|hours?\b|std|stunde|x\b|times\b|mal\b|cm\b|mm\b|inch|%|c\b|f\b)`)

var fractions = map[rune]float64{'½': .5, '¼': .25, '¾': .75, '⅓': 1.0 / 3, '⅔': 2.0 / 3}

func parseQuantity(tok string) float64 {
	tok = strings.Join(strings.Fields(tok), "")
	var whole float64
	r := []rune(tok)
	if f, ok := fractions[r[len(r)-1]]; ok {
		if len(r) > 1 {
			whole, _ = strconv.ParseFloat(string(r[:len(r)-1]), 64)
		}
		return whole + f
	}
	if a, b, ok := strings.Cut(tok, "/"); ok {
		x, _ := strconv.ParseFloat(a, 64)
		y, _ := strconv.ParseFloat(b, 64)
		if y == 0 {
			return x
		}
		return x / y
	}
	v, _ := strconv.ParseFloat(strings.Replace(tok, ",", ".", 1), 64)
	return v
}

// quantities are the amounts a help line states, excluding times,
// temperatures and step references.
func quantities(line string) []float64 {
	var out []float64
	for _, q := range quantitiesIn(line) {
		out = append(out, q.v)
	}
	return out
}

type quantity struct {
	v    float64
	rest string // the text after the number
}

func quantitiesIn(line string) []quantity {
	var out []quantity
	for _, m := range quantityRE.FindAllStringIndex(line, -1) {
		before := strings.ToLower(line[:m[0]])
		if strings.HasSuffix(before, "step ") || strings.HasSuffix(before, "schritt ") || strings.HasSuffix(before, "#") {
			continue
		}
		if notAmountRE.MatchString(line[m[1]:]) {
			continue
		}
		out = append(out, quantity{parseQuantity(line[m[0]:m[1]]), line[m[1]:]})
	}
	return out
}

// allowedNumbers are the amounts a help answer may state: the batch's
// listed amounts and the numbers of the recipe's own steps.
func allowedNumbers(recipe *food.Food, portions float64) []float64 {
	scale := 1.0
	if recipe.ReferencePortions != nil && *recipe.ReferencePortions > 0 {
		scale = portions / *recipe.ReferencePortions
	}
	var out []float64
	for _, c := range recipe.Children {
		if !c.ToTaste {
			out = append(out, parseQuantity(strings.Fields(kitchenAmount(c.Amount*scale, c.Unit, c.ChildName))[0]))
		}
	}
	for _, st := range recipe.Instructions {
		out = append(out, quantities(st.Text)...)
	}
	return out
}

var smallSpoonRE = regexp.MustCompile(`(?i)^\s*(?:tsp|teaspoons?|tbsp|tablespoons?|tl\b|el\b|prise)`)

// amountUnitRE is a measuring unit right after a number: only such numbers
// (and numbers before a listed ingredient word) are amounts; times, ranges,
// pieces and levels are not checked.
var amountUnitRE = regexp.MustCompile(`(?i)^\s*(?:kg|g|gr|gramm?|ml|l|liters?|litres?|tsp|teaspoons?|tbsp|tablespoons?|tl|el|cups?|tassen?|prise)\b`)

// namedAfter reports whether the text after a number starts with one of the
// ingredient words (a plural or German ending still counts).
func namedAfter(rest string, names []string) bool {
	rest = strings.ToLower(strings.TrimSpace(rest))
	for _, n := range names {
		if len([]rune(n)) >= 3 && strings.HasPrefix(rest, strings.ToLower(n)) {
			return true
		}
	}
	return false
}

// ingredientWords are the words of the recipe's ingredient names.
func ingredientWords(recipe *food.Food) []string {
	var out []string
	for _, c := range recipe.Children {
		out = append(out, strings.Fields(c.ChildName)...)
	}
	return out
}

// amountsListed reports whether every amount in the line is one of the
// listed amounts (HELP-2: amounts are copied, never computed). One spoon or
// less of a spoon passes: a small fix such as "1 tsp sugar" is not in the recipe.
func amountsListed(line string, allowed []float64, names ...string) bool {
	for _, q := range quantitiesIn(line) {
		if !amountUnitRE.MatchString(q.rest) && !namedAfter(q.rest, names) {
			continue
		}
		if q.v <= 1 && smallSpoonRE.MatchString(q.rest) {
			continue
		}
		ok := false
		for _, a := range allowed {
			if math.Abs(a-q.v) < 1e-6 {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	return true
}
