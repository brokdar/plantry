package chefsets

import (
	"embed"
	"encoding/json"
	"fmt"
	"slices"
)

// The paid-canary sets (F1, SPEC section 10): find requests per theme and
// constraint, planted import errors, substitution cases and help questions.
// They are test data of cmd/plantry-chef-canary; no production code reads them.

//go:embed paid/*.json
var paidFS embed.FS

// FindRequest is one find the canary asks for. Constraint names the household
// setting the request carries: vegetarian (Diet), no_pork (Never), mild_child
// (RatherNot), max_30min (MaxMinutes) or german (Lang).
type FindRequest struct {
	ID         string   `json:"id"`
	Dish       string   `json:"dish"`
	Theme      string   `json:"theme"`
	Constraint string   `json:"constraint"`
	Lang       string   `json:"lang,omitempty"`
	Diet       string   `json:"diet,omitempty"`
	Never      []string `json:"never,omitempty"`
	Allergens  []string `json:"allergens,omitempty"`
	RatherNot  []string `json:"rather_not,omitempty"`
	MaxMinutes int      `json:"max_minutes,omitempty"`
}

// CheckIngredient is one ingredient line of an import-check recipe.
type CheckIngredient struct {
	Text   string  `json:"text"`
	Amount float64 `json:"amount"`
	Unit   string  `json:"unit"`
	Name   string  `json:"name"`
}

// CheckRecipe is a clean recipe as an import draft would hold it.
type CheckRecipe struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Language    string            `json:"language"`
	Portions    float64           `json:"portions"`
	Prep        int               `json:"prep"`
	Cook        int               `json:"cook"`
	Ingredients []CheckIngredient `json:"ingredients"`
	Steps       []string          `json:"steps"`
}

// Planted error kinds.
const (
	PlantAmountX10  = "amount_x10"
	PlantWrongUnit  = "wrong_unit"
	PlantDropStep   = "drop_step"
	PlantSwapIngr   = "swap_ingredient"
	PlantScaleServe = "scale_servings"
)

// Planted is one error planted into a copy of the base recipe. Which fields
// apply follows the kind: Ingredient (amount_x10, wrong_unit, swap_ingredient),
// ToUnit (wrong_unit), ToName (swap_ingredient), Step (drop_step), Factor
// (scale_servings).
type Planted struct {
	ID         string  `json:"id"`
	Base       string  `json:"base"`
	Kind       string  `json:"kind"`
	Ingredient int     `json:"ingredient,omitempty"`
	Step       int     `json:"step,omitempty"`
	ToUnit     string  `json:"to_unit,omitempty"`
	ToName     string  `json:"to_name,omitempty"`
	Factor     float64 `json:"factor,omitempty"`
}

// ImportCheckSet is the planted-error set of the import check: every base
// recipe is also a clean case, every Planted entry a case with one error.
// AllowedChanges are the change kinds the check may apply without it counting
// as a wrong fix (it fills metadata nothing else fills).
type ImportCheckSet struct {
	AllowedChanges []string      `json:"allowed_changes"`
	Recipes        []CheckRecipe `json:"recipes"`
	Planted        []Planted     `json:"planted"`
}

// SubLine is one recipe line of a substitution case.
type SubLine struct {
	Name  string  `json:"name"`
	Grams float64 `json:"grams"`
}

// SubCase is "no Remove" in a recipe context. The first line is the removed
// food. Diet, Allergens (gates codes) and Never constrain what may replace it.
type SubCase struct {
	ID        string    `json:"id"`
	Recipe    string    `json:"recipe"`
	Remove    string    `json:"remove"`
	Lines     []SubLine `json:"lines"`
	Diet      string    `json:"diet,omitempty"`
	Allergens []string  `json:"allergens,omitempty"`
	Never     []string  `json:"never,omitempty"`
}

// HelpRecipe is a recipe the help questions are asked about.
type HelpRecipe struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Portions    float64   `json:"portions"`
	Ingredients []SubLine `json:"ingredients"`
	Steps       []string  `json:"steps"`
}

// HelpQuestion is one cook-mode help question: explain a step, or a problem
// at a step.
type HelpQuestion struct {
	Recipe  string `json:"recipe"`
	Step    int    `json:"step"`
	Kind    string `json:"kind"`
	Problem string `json:"problem,omitempty"`
}

// HelpSet is the help questions and their recipes.
type HelpSet struct {
	Recipes   []HelpRecipe   `json:"recipes"`
	Questions []HelpQuestion `json:"questions"`
}

func loadPaid(name string, into any) error {
	b, err := paidFS.ReadFile("paid/" + name)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, into); err != nil {
		return fmt.Errorf("chefsets: paid/%s: %w", name, err)
	}
	return nil
}

// LoadFinds returns the find requests, themes and constraints validated.
func LoadFinds() ([]FindRequest, error) {
	var f struct {
		Requests []FindRequest `json:"requests"`
	}
	if err := loadPaid("finds.json", &f); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, r := range f.Requests {
		if r.ID == "" || r.Dish == "" || r.Theme == "" || r.Constraint == "" || seen[r.ID] {
			return nil, fmt.Errorf("chefsets: find request %q is incomplete or repeated", r.ID)
		}
		seen[r.ID] = true
	}
	return f.Requests, nil
}

var plantKinds = []string{PlantAmountX10, PlantWrongUnit, PlantDropStep, PlantSwapIngr, PlantScaleServe}

// LoadImportCheck returns the planted-error set; every planted entry names an
// existing base recipe, an in-range line or step and what its kind needs.
func LoadImportCheck() (ImportCheckSet, error) {
	var s ImportCheckSet
	if err := loadPaid("importcheck.json", &s); err != nil {
		return s, err
	}
	base := map[string]CheckRecipe{}
	for _, r := range s.Recipes {
		if r.ID == "" || len(r.Ingredients) == 0 || len(r.Steps) < 2 || base[r.ID].ID != "" {
			return s, fmt.Errorf("chefsets: import-check recipe %q is incomplete or repeated", r.ID)
		}
		base[r.ID] = r
	}
	ids := map[string]bool{}
	for _, p := range s.Planted {
		r, ok := base[p.Base]
		bad := func(why string) error { return fmt.Errorf("chefsets: planted error %q: %s", p.ID, why) }
		switch {
		case !ok:
			return s, bad("unknown base recipe")
		case ids[p.ID]:
			return s, bad("repeated id")
		case !slices.Contains(plantKinds, p.Kind):
			return s, bad("unknown kind")
		}
		ids[p.ID] = true
		switch p.Kind {
		case PlantAmountX10, PlantWrongUnit, PlantSwapIngr:
			if p.Ingredient < 0 || p.Ingredient >= len(r.Ingredients) {
				return s, bad("ingredient out of range")
			}
			if (p.Kind == PlantWrongUnit && p.ToUnit == "") || (p.Kind == PlantSwapIngr && p.ToName == "") {
				return s, bad("missing replacement")
			}
		case PlantDropStep:
			if p.Step < 0 || p.Step >= len(r.Steps) {
				return s, bad("step out of range")
			}
		case PlantScaleServe:
			if p.Factor <= 1 {
				return s, bad("factor must exceed 1")
			}
		}
	}
	return s, nil
}

// LoadSubs returns the substitution cases.
func LoadSubs() ([]SubCase, error) {
	var f struct {
		Cases []SubCase `json:"cases"`
	}
	if err := loadPaid("subs.json", &f); err != nil {
		return nil, err
	}
	for _, c := range f.Cases {
		if c.ID == "" || len(c.Lines) == 0 || c.Lines[0].Name != c.Remove {
			return nil, fmt.Errorf("chefsets: substitution case %q must start with the removed food", c.ID)
		}
	}
	return f.Cases, nil
}

// LoadHelp returns the help set; every question names a recipe and a step of it.
func LoadHelp() (HelpSet, error) {
	var h HelpSet
	if err := loadPaid("help.json", &h); err != nil {
		return h, err
	}
	steps := map[string]int{}
	for _, r := range h.Recipes {
		steps[r.ID] = len(r.Steps)
	}
	for _, q := range h.Questions {
		if q.Step < 0 || q.Step >= steps[q.Recipe] || (q.Kind != "explain_step" && q.Kind != "went_wrong") {
			return h, fmt.Errorf("chefsets: help question %+v is invalid", q)
		}
	}
	return h, nil
}
