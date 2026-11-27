// Package gates checks a recipe against the household and the food-safety
// tables (spec §5.10 GATE-1 and GATE-3). It is pure: no I/O, no model, no
// clock. A block means the recipe is not saved or planned; a flag is shown
// and the recipe proceeds. Every finding carries a stable code, the
// ingredient or step it points at (-1 when it is about the whole recipe) and
// params for the UI's en/de text; Reason is a short English fallback.
package gates

import (
	"fmt"
	"strings"

	"github.com/jaltszeimer/plantry/backend/internal/domain/diet"
)

// Ingredient is one recipe line.
type Ingredient struct {
	FoodID  int64 // 0 when unmatched
	Name    string
	Grams   float64 // 0 when unknown
	ToTaste bool
	Matched bool
}

// Recipe is what the gates look at.
type Recipe struct {
	Name        string
	Portions    int
	Ingredients []Ingredient
	Steps       []string
	KeepDays    int // days the dish is meant to be kept; 0 when not stored
	Freezable   bool
	// Versions is how many sources a researched find was checked against;
	// 0 means not a researched find (no flag).
	Versions int
}

// Household is the part of the profile the gates need. Allergens are codes
// from AllergenCodes.
type Household struct {
	Diet         diet.Class // empty or omnivore: no diet conflict
	NeverFoodIDs []int64
	NeverNames   []string
	Allergens    []string
}

// Finding is one block or flag.
type Finding struct {
	Code       string            `json:"code"`
	Ingredient int               `json:"ingredient"` // index into Recipe.Ingredients, or -1
	Step       int               `json:"step"`       // index into Recipe.Steps, or -1
	Params     map[string]string `json:"params,omitempty"`
	Reason     string            `json:"reason"`
}

// Result holds the findings in a deterministic order.
type Result struct {
	Blocks []Finding
	Flags  []Finding
}

// Params are the tunable thresholds of the flags.
type Params struct {
	MaxSodiumMgPerPortion float64
	MinVersions           int
}

// DefaultParams are the starting values (tunable).
var DefaultParams = Params{MaxSodiumMgPerPortion: 2000, MinVersions: 3}

// Check runs every gate with DefaultParams.
func Check(r Recipe, h Household) Result { return CheckWith(r, h, DefaultParams) }

// CheckWith runs every gate.
func CheckWith(r Recipe, h Household, p Params) Result {
	c := &checker{r: r, h: h, p: p}
	c.prepare()
	c.completeness()
	c.household()
	c.temperatures()
	c.hazards()
	c.keepDays()
	c.baking()
	c.flagsAll()
	return c.res
}

type checker struct {
	r   Recipe
	h   Household
	p   Params
	res Result
	// names and steps are folded; ingWords the words of each folded name.
	names    []string
	steps    []string
	ingWords [][]string
}

func (c *checker) prepare() {
	for _, in := range c.r.Ingredients {
		n := diet.Fold(in.Name)
		c.names = append(c.names, n)
		c.ingWords = append(c.ingWords, words(n))
	}
	for _, s := range c.r.Steps {
		c.steps = append(c.steps, diet.Fold(s))
	}
}

func (c *checker) block(code string, ing, step int, reason string, kv ...string) {
	c.res.Blocks = append(c.res.Blocks, finding(code, ing, step, reason, kv))
}

func (c *checker) flag(code string, ing, step int, reason string, kv ...string) {
	c.res.Flags = append(c.res.Flags, finding(code, ing, step, reason, kv))
}

func finding(code string, ing, step int, reason string, kv []string) Finding {
	f := Finding{Code: code, Ingredient: ing, Step: step, Reason: reason}
	if len(kv) > 0 {
		f.Params = map[string]string{}
		for i := 0; i+1 < len(kv); i += 2 {
			f.Params[kv[i]] = kv[i+1]
		}
	}
	return f
}

func (c *checker) completeness() {
	if strings.TrimSpace(c.r.Name) == "" {
		c.block("missing_name", -1, -1, "The recipe has no name.")
	}
	if len(c.r.Ingredients) == 0 {
		c.block("missing_ingredients", -1, -1, "The recipe has no ingredients.")
	}
	if len(c.r.Steps) == 0 {
		c.block("missing_steps", -1, -1, "The recipe has no steps.")
	}
	if c.r.Portions <= 0 {
		c.block("missing_yield", -1, -1, "The recipe does not say how many portions it makes.")
	}
	for i, in := range c.r.Ingredients {
		if !in.Matched {
			c.block("unmatched_ingredient", i, -1, in.Name+" is not matched to a food.", "name", in.Name)
		}
	}
}

// household checks never items, allergens and the diet, with hidden
// ingredients expanded (hidden.go).
func (c *checker) household() {
	never := make([]string, 0, len(c.h.NeverNames))
	for _, n := range c.h.NeverNames {
		if f := diet.Fold(strings.TrimSpace(n)); f != "" {
			never = append(never, f)
		}
	}
	for i, in := range c.r.Ingredients {
		name := c.names[i]
		class := diet.Classify(in.Name)
		allergens := allergensOf(name)
		var implied []string
		for _, h := range hidden {
			if containsAny(name, h.keywords) {
				allergens = appendUnique(allergens, h.allergens...)
				class = lessStrict(class, h.class)
				implied = append(implied, h.implies...)
			}
		}
		if in.FoodID != 0 && containsID(c.h.NeverFoodIDs, in.FoodID) {
			c.block("never_item", i, -1, in.Name+" is on the never list.", "name", in.Name)
			continue
		}
		hit := false
		for _, n := range never {
			if strings.Contains(name, n) || impliesNever(implied, n) {
				c.block("never_item", i, -1, in.Name+" contains "+n+", which is on the never list.", "name", in.Name, "never", n)
				hit = true
				break
			}
		}
		if hit {
			continue
		}
		for _, a := range c.h.Allergens {
			a = diet.Fold(strings.TrimSpace(a))
			if containsStr(allergens, a) {
				c.block("allergen", i, -1, in.Name+" contains "+a+".", "name", in.Name, "allergen", a)
			}
		}
		if dietBlocks(c.h.Diet, class) {
			c.block("diet_conflict", i, -1, in.Name+" does not fit the diet.", "name", in.Name, "diet", string(c.h.Diet), "class", string(class))
		}
	}
}

// dietBlocks: a known class that is less restrictive than the household's
// diet. An empty diet or Unknown class never blocks.
func dietBlocks(d, class diet.Class) bool {
	if d == "" || !diet.Valid(d) || !diet.Valid(class) {
		return false
	}
	return !diet.Compatible(class, d)
}

// lessStrict returns the less restrictive of two classes, ignoring Unknown.
func lessStrict(a, b diet.Class) diet.Class {
	if !diet.Valid(a) {
		return b
	}
	if !diet.Valid(b) {
		return a
	}
	return diet.Max(a, b)
}

func containsID(ids []int64, id int64) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}

func itoa(n int) string { return fmt.Sprint(n) }

// impliesNever reports whether a hidden ingredient's implied term and a never
// name contain one another ("pinienkern" and "pinienkerne").
func impliesNever(implied []string, never string) bool {
	for _, t := range implied {
		if strings.Contains(never, t) || strings.Contains(t, never) {
			return true
		}
	}
	return false
}
