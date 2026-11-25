// Package chefcanary is the paid canary of the Chef's attribute labelling
// (A3, delivery I7): it labels a library with a real provider, logs every
// call in the run log of the database it runs on, and compares the result
// with the DATA-1 labels (internal/chefsets). cmd/plantry-chef-canary runs
// it; tests drive it with the fake provider.
package chefcanary

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/chefsets"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/flavor"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

// Config is one canary run.
type Config struct {
	// DB is a migrated database; its unlabelled recipes are labelled.
	DB       *sql.DB
	Set      chefsets.Set
	Client   llm.Client
	Provider string
	Model    string
	// Prices are ai.price_overrides for the cost (nil: the built-in table).
	Prices map[string]airun.Price
	Logger *slog.Logger
}

// FieldScore is how often the labelled value of a field agrees with DATA-1.
type FieldScore struct {
	Field string `json:"field"`
	Agree int    `json:"agree"`
	Total int    `json:"total"`
}

// Report is a canary run's outcome.
type Report struct {
	// Recipes are the recipes of the set found in the database (compared).
	Recipes int `json:"recipes"`
	// Runs and Failed count the chef_attributes runs this canary made.
	Runs         int     `json:"runs"`
	Failed       int     `json:"failed"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	CostUSD      float64 `json:"cost_usd"`
	// Unpriced: the model has no price, CostUSD is 0.
	Unpriced bool         `json:"unpriced"`
	Fields   []FieldScore `json:"fields"`
	// Misses list each disagreement as "field recipe: got X, label Y".
	Misses []string `json:"misses"`
}

// Seed stores the set's ingredients as leaves and its recipes as composed
// foods, unless the database already holds recipes.
func Seed(ctx context.Context, db *sql.DB, s chefsets.Set) error {
	foods := sqlite.NewFoodRepo(db)
	n, err := foods.CountComposedFoods(ctx)
	if err != nil || n > 0 {
		return err
	}
	leaves := map[string]int64{}
	for _, in := range s.Ingredients {
		src := food.SourceManual
		f := &food.Food{Kind: food.KindLeaf, Source: &src, Name: in.Name, Kcal100g: &in.Kcal, Protein100g: &in.Protein, Fat100g: &in.Fat, Carbs100g: &in.Carbs}
		if err := foods.Create(ctx, f); err != nil {
			return fmt.Errorf("seed ingredient %s: %w", in.Name, err)
		}
		leaves[in.Name] = f.ID
	}
	for _, name := range slices.Sorted(maps.Keys(s.Foods)) {
		sf := s.Foods[name]
		if sf.Leaf {
			continue
		}
		role := food.Role(sf.Role)
		f := &food.Food{Kind: food.KindComposed, Name: name, Role: &role, ReferencePortions: &sf.Portions, Tags: sf.Tags}
		for i, ing := range sf.Ingredients {
			f.Children = append(f.Children, food.FoodComponent{ChildID: leaves[ing.Name], Amount: ing.Grams, Unit: "g", Grams: ing.Grams, SortOrder: i})
		}
		if err := foods.Create(ctx, f); err != nil {
			return fmt.Errorf("seed recipe %s: %w", name, err)
		}
	}
	return nil
}

// Run labels every unlabelled recipe of the database, one call per batch
// (a failed batch is counted and the rest go on), then compares the
// recipes of the set with their labels.
func Run(ctx context.Context, cfg Config) (Report, error) {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	runs := sqlite.NewAIRunRepo(cfg.DB)
	rec := airun.NewRecorder(runs, logger, time.Now)
	foods := sqlite.NewFoodRepo(cfg.DB)
	store := sqlite.NewFoodAttributeRepo(cfg.DB)
	nutrition := food.NewNutritionResolver(foods)
	labeler := flavor.NewLabeler(store, foods, nutrition, sqlite.NewTxRunner(cfg.DB),
		llm.StaticResolver(rec.Meter(cfg.Client, cfg.Provider), cfg.Model), rec)

	before, err := lastRunID(ctx, runs)
	if err != nil {
		return Report{}, err
	}
	ids, err := store.UnlabelledRecipes(ctx)
	if err != nil {
		return Report{}, err
	}
	for len(ids) > 0 {
		n := min(flavor.LabelBatch, len(ids))
		if err := labeler.Label(ctx, ids[:n]); err != nil {
			logger.WarnContext(ctx, "canary.batch failed", "food_ids", ids[:n], "error", err)
		}
		ids = ids[n:]
	}

	var rep Report
	rows, err := runs.ListRuns(ctx, airun.RunQuery{Kind: airun.KindChefAttributes, Limit: 200})
	if err != nil {
		return Report{}, err
	}
	for _, r := range rows {
		if r.ID <= before {
			continue
		}
		rep.Runs++
		if r.Status != airun.StatusOK {
			rep.Failed++
		}
		rep.InputTokens += r.Tokens.Input
		rep.OutputTokens += r.Tokens.Output
		if c := airun.Cost(r.Model, r.Provider, r.Tokens, cfg.Prices); c != nil {
			rep.CostUSD += *c
		} else {
			rep.Unpriced = true
		}
	}
	if err := compare(ctx, cfg.Set, foods, flavor.NewService(store, foods, nutrition, nil), &rep); err != nil {
		return Report{}, err
	}
	return rep, nil
}

func lastRunID(ctx context.Context, runs *sqlite.AIRunRepo) (int64, error) {
	rows, err := runs.ListRuns(ctx, airun.RunQuery{Limit: 1})
	if err != nil || len(rows) == 0 {
		return 0, err
	}
	return rows[0].ID, nil
}

// compare scores the labels of the set's recipes found in the database by
// name. A field counts as agreeing only when the assistant set it (source
// auto) and it matches the label: heroes when the first labelled hero is
// one of them, richness within one step (richness_exact: equal), taste per
// axis within one step, the rest equal.
func compare(ctx context.Context, set chefsets.Set, foods *sqlite.FoodRepo, svc *flavor.Service, rep *Report) error {
	list, err := foods.List(ctx, food.ListQuery{Kind: food.KindComposed, Limit: 10000})
	if err != nil {
		return err
	}
	scores := map[string]*FieldScore{}
	score := func(field string, ok bool, name string, got, want any) {
		s := scores[field]
		if s == nil {
			s = &FieldScore{Field: field}
			scores[field] = s
		}
		s.Total++
		if ok {
			s.Agree++
		} else {
			rep.Misses = append(rep.Misses, fmt.Sprintf("%s %s: got %v, label %v", field, name, got, want))
		}
	}
	for _, f := range list.Items {
		sf, ok := set.Foods[f.Name]
		if !ok || sf.Leaf {
			continue
		}
		rep.Recipes++
		v, err := svc.Get(ctx, f.ID)
		if err != nil {
			return err
		}
		got, want := v.Values, sf.Attrs
		auto := func(fl flavor.Field) bool { return v.Sources[fl] == flavor.SourceAuto }
		str := func(fl flavor.Field, g, w string) { score(string(fl), auto(fl) && g == w, f.Name, g, w) }
		str(flavor.FieldCuisine, got.Cuisine, want.Cuisine)
		str(flavor.FieldKind, got.Kind, want.Kind)
		str(flavor.FieldTexture, got.Texture, want.Texture)
		str(flavor.FieldTemperature, got.Temperature, want.Temperature)
		str(flavor.FieldColour, got.Colour, want.Colour)
		str(flavor.FieldTechnique, got.Technique, want.Technique)
		str(flavor.FieldFormat, got.Format, want.Format)
		score("heroes", auto(flavor.FieldHeroes) && len(want.Heroes) > 0 && slices.Contains(got.Heroes, want.Heroes[0]), f.Name, got.Heroes, want.Heroes)
		score("saucy", auto(flavor.FieldSaucy) && got.Saucy != nil && *got.Saucy == want.Saucy, f.Name, deref(got.Saucy), want.Saucy)
		r := -9
		if got.Richness != nil {
			r = *got.Richness
		}
		score("richness", auto(flavor.FieldRichness) && abs(r-want.Richness) <= 1, f.Name, r, want.Richness)
		score("richness_exact", auto(flavor.FieldRichness) && r == want.Richness, f.Name, r, want.Richness)
		t := flavor.Taste{Salt: -9, Acid: -9, Sweet: -9, Bitter: -9, Umami: -9, Heat: -9}
		if got.Taste != nil {
			t = *got.Taste
		}
		w := want.Taste
		for _, axis := range []struct {
			name      string
			got, want int
		}{{"salt", t.Salt, w.Salt}, {"acid", t.Acid, w.Acid}, {"sweet", t.Sweet, w.Sweet}, {"bitter", t.Bitter, w.Bitter}, {"umami", t.Umami, w.Umami}, {"heat", t.Heat, w.Heat}} {
			score("taste."+axis.name, auto(flavor.FieldTaste) && abs(axis.got-axis.want) <= 1, f.Name, axis.got, axis.want)
		}
	}
	for _, s := range scores {
		rep.Fields = append(rep.Fields, *s)
	}
	sort.Slice(rep.Fields, func(i, j int) bool { return rep.Fields[i].Field < rep.Fields[j].Field })
	slices.Sort(rep.Misses)
	return nil
}

func deref[T any](p *T) any {
	if p == nil {
		return nil
	}
	return *p
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// Summary renders the report as text.
func (r Report) Summary() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "recipes compared: %d, runs: %d (failed %d), tokens in/out: %d/%d, cost: ", r.Recipes, r.Runs, r.Failed, r.InputTokens, r.OutputTokens)
	if r.Unpriced {
		sb.WriteString("unknown (unpriced model)\n")
	} else {
		fmt.Fprintf(&sb, "$%.4f\n", r.CostUSD)
	}
	for _, f := range r.Fields {
		pct := 0.0
		if f.Total > 0 {
			pct = 100 * float64(f.Agree) / float64(f.Total)
		}
		fmt.Fprintf(&sb, "  %-15s %3d/%3d  %5.1f%%\n", f.Field, f.Agree, f.Total, pct)
	}
	return sb.String()
}
