package chefhelp

import (
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/units"
)

// kitchenAmount renders an amount as a cook measures it: grams and ml
// rounded as units.Display does, small volumes as spoons in halves, counts
// and other units in halves.
func kitchenAmount(amount float64, unit string) string {
	base, u := units.ToBase(amount, unit)
	switch {
	case u == "ml" && base < 30 && base < 15:
		return spoon(base/5, "tsp")
	case u == "ml" && base < 30:
		return spoon(base/15, "tbsp")
	case u == "g" || u == "ml":
		q := units.Display(amount, unit)
		return number(q.Amount) + " " + q.Unit
	case u == "tsp" || u == "tbsp":
		return spoon(amount, u)
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
			out = append(out, parseQuantity(strings.Fields(kitchenAmount(c.Amount*scale, c.Unit))[0]))
		}
	}
	for _, st := range recipe.Instructions {
		out = append(out, quantities(st.Text)...)
	}
	return out
}

var smallSpoonRE = regexp.MustCompile(`(?i)^\s*(?:tsp|teaspoons?|tbsp|tablespoons?|tl\b|el\b|prise)`)

// amountsListed reports whether every amount in the line is one of the
// listed amounts (HELP-2: amounts are copied, never computed). One spoon or
// less of a spoon passes: a small fix such as "1 tsp sugar" is not in the recipe.
func amountsListed(line string, allowed []float64) bool {
	for _, q := range quantitiesIn(line) {
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
