package discovery

import (
	"context"
	"fmt"
	"strings"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/importer"
)

// Line annotates one draft ingredient, in the draft's order (D14).
type Line struct {
	// FoodID is the local leaf the line matched, known or not; the review
	// preselects it (D18).
	FoodID *int64 `json:"food_id,omitempty"`
	// Known: the matched leaf is used by at least one of the user's recipes.
	// A line is new when it has no match or the leaf is unused.
	Known bool `json:"known"`
	// Candidate is the matched leaf's name.
	Candidate string `json:"candidate,omitempty"`
}

// PerPortion is the card's macros, computed from the matched leaves (D15).
type PerPortion struct {
	Kcal    float64 `json:"kcal"`
	Protein float64 `json:"protein"`
	Fat     float64 `json:"fat"`
	Carbs   float64 `json:"carbs"`
}

// Annotations is what annotations_json stores for a suggestion.
type Annotations struct {
	Lines             []Line     `json:"lines"`
	KnownShare        float64    `json:"known_share"`
	PerPortion        PerPortion `json:"per_portion"`
	NutritionComplete bool       `json:"nutrition_complete"`
	// Missing names the lines and leaves that keep the nutrition incomplete.
	Missing []string `json:"missing"`
	// Why and Cuisine come from the research agent, not from Annotate.
	Why     string `json:"why,omitempty"`
	Cuisine string `json:"cuisine,omitempty"`
}

// LeafMatcher is food.Resolver's local step.
type LeafMatcher interface {
	LocalMatch(ctx context.Context, name, lang string) (*food.Candidate, error)
}

// UsedLeaves says which leaves a composed food uses (FoodRepo).
type UsedLeaves interface {
	LeafIDsUsedInRecipes(ctx context.Context, ids []int64) (map[int64]bool, error)
}

// DraftNutrition is food.NutritionResolver over an unsaved recipe.
type DraftNutrition interface {
	PerPortionOfDraft(ctx context.Context, lines []food.DraftLine, portions float64) (food.Detailed, error)
}

// Annotator marks a draft's lines known or new against the local catalogue
// and computes its macros. It works locally only: no FDC, no OFF, no LLM.
type Annotator struct {
	Matcher   LeafMatcher
	Used      UsedLeaves
	Nutrition DraftNutrition
}

// Annotate annotates every ingredient of d.
func (a *Annotator) Annotate(ctx context.Context, d *importer.Draft) (*Annotations, error) {
	out := &Annotations{Lines: make([]Line, len(d.Ingredients)), Missing: []string{}}
	lines := make([]food.DraftLine, len(d.Ingredients))
	var matched []int64
	for i, ing := range d.Ingredients {
		name := lineName(ing)
		lines[i] = food.DraftLine{Name: name, Amount: ing.Amount, Unit: ing.Unit, PortionGrams: ing.PortionGrams}
		if ing.Name == "" {
			continue
		}
		c, err := a.Matcher.LocalMatch(ctx, ing.Name, d.Language)
		if err != nil {
			return nil, fmt.Errorf("match %q: %w", ing.Name, err)
		}
		if c == nil || c.ExistingID == nil {
			continue
		}
		id := *c.ExistingID
		out.Lines[i] = Line{FoodID: &id, Candidate: c.Name}
		lines[i].FoodID = id
		matched = append(matched, id)
	}

	used, err := a.Used.LeafIDsUsedInRecipes(ctx, matched)
	if err != nil {
		return nil, fmt.Errorf("leaves used in recipes: %w", err)
	}
	known := 0
	for i := range out.Lines {
		if id := out.Lines[i].FoodID; id != nil && used[*id] {
			out.Lines[i].Known = true
			known++
		}
	}
	if n := len(out.Lines); n > 0 {
		out.KnownShare = float64(known) / float64(n)
	}

	det, err := a.Nutrition.PerPortionOfDraft(ctx, lines, d.ReferencePortions)
	if err != nil {
		return nil, fmt.Errorf("draft nutrition: %w", err)
	}
	out.PerPortion = PerPortion{Kcal: det.Macros.Kcal, Protein: det.Macros.Protein, Fat: det.Macros.Fat, Carbs: det.Macros.Carbs}
	out.NutritionComplete = det.Complete
	for _, m := range det.Missing {
		out.Missing = append(out.Missing, m.Name)
	}
	return out, nil
}

func lineName(ing importer.DraftIngredient) string {
	if ing.Name != "" {
		return ing.Name
	}
	return strings.TrimSpace(ing.RawText)
}
