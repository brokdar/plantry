package chefcanary

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/chefsets"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/diet"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/gates"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/slot"
	"github.com/jaltszeimer/plantry/backend/internal/domain/substitution"
)

// SubsConfig is the subs mode: "no X" in a recipe context through the S1
// substitution service. It is rule-based, so it costs nothing and needs no
// provider.
type SubsConfig struct {
	// DB is the database whose catalogue the substitutes come from. It is
	// copied (VACUUM INTO) and the cases run on the copy, so it stays as it is.
	DB    *sql.DB
	Cases []chefsets.SubCase
}

// Sane amount ratios: a substitute's grams over the removed food's (the
// table has dried herbs at 0.33 and a stock cube at 25).
const (
	minRatio = 0.05
	maxRatio = 30
)

// scratchCopy copies the database next to the temp dir and opens it.
func scratchCopy(ctx context.Context, db *sql.DB) (*sql.DB, func(), error) {
	dir, err := os.MkdirTemp("", "chef-canary-*")
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	path := filepath.Join(dir, "scratch.db")
	if _, err := db.ExecContext(ctx, `VACUUM INTO ?`, path); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("copy database: %w", err)
	}
	cp, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	cp.SetMaxOpenConns(1)
	return cp, func() { _ = cp.Close(); cleanup() }, nil
}

// RunSubs asks the substitution service for the options of each case and
// checks them by rule: at least one option, none the removed food, at least
// one that fits the diet, allergens and never items, every amount sane.
func RunSubs(ctx context.Context, cfg SubsConfig) (ModeReport, error) {
	cp, cleanup, err := scratchCopy(ctx, cfg.DB)
	if err != nil {
		return ModeReport{}, err
	}
	defer cleanup()
	foods := sqlite.NewFoodRepo(cp)
	sessions := sqlite.NewCookSessionRepo(cp)
	svc := substitution.NewService(foods, sessions, sqlite.NewSubstitutionRepo(cp), nil, nil)
	slotID, err := scratchSlot(ctx, cp)
	if err != nil {
		return ModeReport{}, err
	}
	leaves, err := leafIDs(ctx, foods)
	if err != nil {
		return ModeReport{}, err
	}

	rep := ModeReport{Mode: "subs", Cases: len(cfg.Cases)}
	unsafeOffered := 0
	for _, c := range cfg.Cases {
		res, unsafe, err := runSub(ctx, c, foods, sessions, svc, leaves, slotID)
		if err != nil {
			return ModeReport{}, fmt.Errorf("case %s: %w", c.ID, err)
		}
		unsafeOffered += unsafe
		rep.Results = append(rep.Results, res)
		rep.Ran++
	}
	pass := 0
	for _, r := range rep.Results {
		if r.OK {
			pass++
		}
	}
	rep.add(Metric{Name: "pass_rate", Value: pct(pass, rep.Ran), Unit: "%", Note: fmt.Sprintf("%d/%d", pass, rep.Ran)}.target(">= 90 %", true, 90))
	rep.add(Metric{Name: "options_unsafe_for_household", Value: float64(unsafeOffered), Note: "offered options that break a diet, allergen or never rule (a case passes while one safe option is offered)"})
	return rep, nil
}

// leafIDs maps lower-cased leaf names to ids.
func leafIDs(ctx context.Context, foods *sqlite.FoodRepo) (map[string]int64, error) {
	out := map[string]int64{}
	for offset := 0; ; offset += 500 {
		page, err := foods.List(ctx, food.ListQuery{Kind: food.KindLeaf, Limit: 500, Offset: offset})
		if err != nil {
			return nil, err
		}
		for _, f := range page.Items {
			out[strings.ToLower(f.Name)] = f.ID
		}
		if len(page.Items) < 500 {
			return out, nil
		}
	}
}

func runSub(ctx context.Context, c chefsets.SubCase, foods *sqlite.FoodRepo, sessions *sqlite.CookSessionRepo,
	svc *substitution.Service, leaves map[string]int64, slotID int64,
) (CaseResult, int, error) {
	res := CaseResult{ID: c.ID}
	recipe, removed, err := makeRecipe(ctx, foods, leaves, "subs "+c.ID, 1, c.Lines)
	if err != nil {
		return res, 0, err
	}
	sess := &plate.CookSession{FoodID: recipe.ID, CookDate: time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC), CookSlotID: slotID, Portions: 1, State: plate.StateConfirmed}
	if err := sessions.CreateSession(ctx, sess); err != nil {
		return res, 0, err
	}
	opts, _, err := svc.Options(ctx, removed, []int64{sess.ID})
	if err != nil && !isInvalid(err) {
		return res, 0, err
	}
	if len(opts) == 0 {
		res.Outcome, res.Detail = "no_option", fmt.Sprint(err)
		return res, 0, nil
	}
	h := gates.Household{Diet: diet.Class(c.Diet), Allergens: c.Allergens, NeverNames: c.Never}
	safe, unsafe, insane, same := 0, 0, 0, 0
	var names []string
	for _, o := range opts {
		names = append(names, o.Name)
		switch {
		case o.FoodID == removed:
			same++
		case o.Ratio < minRatio || o.Ratio > maxRatio || o.Amount.Amount <= 0:
			insane++
		}
		if fits(o.Name, h) {
			safe++
		} else {
			unsafe++
		}
	}
	res.OK = same == 0 && insane == 0 && safe > 0
	res.Outcome = map[bool]string{true: "pass", false: "fail"}[res.OK]
	res.Detail = fmt.Sprintf("options=%v safe=%d unsafe=%d removed_food_offered=%d amounts_not_sane=%d", slices.Clone(names), safe, unsafe, same, insane)
	return res, unsafe, nil
}

func isInvalid(err error) bool { return err != nil && errors.Is(err, domain.ErrInvalidInput) }

// fits runs the household gates over a recipe of just the substitute.
func fits(name string, h gates.Household) bool {
	r := gates.Recipe{Name: "check", Portions: 1, Ingredients: []gates.Ingredient{{Name: name, Grams: 100, Matched: true}}, Steps: []string{"Mix."}}
	for _, b := range gates.Check(r, h).Blocks {
		if b.Code == "never_item" || b.Code == "allergen" || b.Code == "diet_conflict" {
			return false
		}
	}
	return true
}

// makeRecipe stores a composed food of the lines (a missing leaf is created
// with placeholder nutrition) and returns it with the id of the first line's
// leaf. The recipe makes one portion of the lines' grams.
func makeRecipe(ctx context.Context, foods *sqlite.FoodRepo, leaves map[string]int64, name string, portions float64, lines []chefsets.SubLine) (*food.Food, int64, error) {
	src := food.SourceManual
	kcal, one := 100.0, 1.0
	recipe := &food.Food{Kind: food.KindComposed, Name: name, ReferencePortions: &portions}
	var first int64
	for i, ln := range lines {
		id, ok := catalogueID(leaves, ln.Name)
		if !ok {
			f := &food.Food{Kind: food.KindLeaf, Source: &src, Name: ln.Name, Kcal100g: &kcal, Protein100g: &one, Fat100g: &one, Carbs100g: &one}
			if err := foods.Create(ctx, f); err != nil {
				return nil, 0, err
			}
			id, leaves[strings.ToLower(ln.Name)] = f.ID, f.ID
		}
		if i == 0 {
			first = id
		}
		recipe.Children = append(recipe.Children, food.FoodComponent{ChildID: id, Amount: ln.Grams, Unit: "g", Grams: ln.Grams, SortOrder: i})
	}
	return recipe, first, foods.Create(ctx, recipe)
}

// scratchSlot returns a time slot of the copy, creating one when the database
// has none (a fresh canary database).
func scratchSlot(ctx context.Context, db *sql.DB) (int64, error) {
	repo := sqlite.NewSlotRepo(db)
	slots, err := repo.List(ctx, false)
	if err != nil {
		return 0, err
	}
	if len(slots) > 0 {
		return slots[0].ID, nil
	}
	s := &slot.TimeSlot{NameKey: "canary", Icon: "utensils", Active: true}
	return s.ID, repo.Create(ctx, s)
}

// catalogueID finds the leaf a line name stands for: the one of that name, else
// the one the substitution table treats as the same food (names in any
// language: Honig is the catalogue's Honey).
func catalogueID(leaves map[string]int64, name string) (int64, bool) {
	if id, ok := leaves[strings.ToLower(name)]; ok {
		return id, true
	}
	want := food.SubstitutesOf(name)
	if len(want) == 0 {
		return 0, false
	}
	for _, leaf := range slices.Sorted(maps.Keys(leaves)) {
		if slices.Equal(food.SubstitutesOf(leaf), want) {
			return leaves[leaf], true
		}
	}
	return 0, false
}
