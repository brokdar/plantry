// Package recipechange saves the assistant's recipe changes (spec VAR-1,
// VAR-2, VAR-4): a filled variant in the recipe's variant group, or an edit
// of the recipe itself. Every proposed ingredient line goes through the
// matching path (P3) and the gates (GATE-1, GATE-3) first; a block saves
// nothing. Each save carries a changes list ("what changed and why") and is
// a recipe_content food event, so Undo puts the recipe back or removes the
// variant.
package recipechange

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/diet"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/foodevent"
	"github.com/jaltszeimer/plantry/backend/internal/domain/gates"
	"github.com/jaltszeimer/plantry/backend/internal/domain/importer"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
)

// Proposal is a changed recipe as the model proposes it: ingredient lines as
// text ("200 g coconut yoghurt"), steps, and the changes list.
type Proposal struct {
	Name        string   `json:"name"`
	Ingredients []string `json:"ingredients"`
	Steps       []string `json:"steps"`
	Changes     []string `json:"changes"`
}

// Line is one ingredient line and its match.
type Line struct {
	Text     string `json:"text"`
	Matched  bool   `json:"matched"`
	FoodID   int64  `json:"food_id,omitempty"`
	FoodName string `json:"food_name,omitempty"`
	// Reason is the matcher's reason for an unmatched line.
	Reason string `json:"reason,omitempty"`
}

// Check is the matching and gate outcome of a recipe.
type Check struct {
	Lines  []Line          `json:"lines"`
	Blocks []gates.Finding `json:"blocks"`
	Flags  []gates.Finding `json:"flags"`
}

// Blocked reports whether a gate blocks the recipe (an unmatched line is a
// block too).
func (c Check) Blocked() bool { return len(c.Blocks) > 0 }

// Result is the outcome of a save. Food is nil when the check blocked it.
type Result struct {
	Food *food.Food `json:"-"`
	// FoodID is the saved recipe, 0 when the check blocked it.
	FoodID  int64          `json:"food_id"`
	Changes []string       `json:"changes"`
	Undo    foodevent.Undo `json:"undo"`
	Check   Check          `json:"check"`
}

// Content is the visible part of a recipe for a before/after diff.
type Content struct {
	Name        string   `json:"name"`
	Ingredients []string `json:"ingredients"`
	Steps       []string `json:"steps"`
}

// Preview is an in-place edit before it is saved.
type Preview struct {
	Before Content `json:"before"`
	After  Content `json:"after"`
	// ConfirmedDates are the dates of confirmed plates from today on that
	// hold the recipe: their shopping changes (VAR-2).
	ConfirmedDates []string `json:"confirmed_dates"`
	Check          Check    `json:"check"`
	// plan is the matching the preview ran, for EditPreviewed.
	plan planned
}

// Matcher is the matching path (food.Matcher).
type Matcher interface {
	Match(ctx context.Context, req food.MatchRequest) (*food.MatchResult, error)
}

// Store keeps the changes lists and reads the confirmed plates. Every
// method joins the unit of work carried by ctx.
type Store interface {
	Changes(ctx context.Context, foodID int64) ([]string, error)
	// PutChanges stores the list; an empty list removes it.
	PutChanges(ctx context.Context, foodID int64, changes []string) error
	ConfirmedDates(ctx context.Context, foodID int64, today time.Time) ([]string, error)
}

// ProfileReader reads the household profile.
type ProfileReader interface {
	Get(ctx context.Context) (*profile.Profile, error)
}

// Service saves recipe changes.
type Service struct {
	foods   food.Repository
	origins food.OriginStore
	matcher Matcher
	store   Store
	profile ProfileReader
	events  *foodevent.Service
	llm     llm.Resolver
	runs    *airun.Recorder
	// Now is the clock of the confirmed-plates read; time.Now unless a test
	// sets it.
	Now func() time.Time
}

// NewService creates a Service.
func NewService(foods food.Repository, origins food.OriginStore, matcher Matcher, store Store, prof ProfileReader, events *foodevent.Service) *Service {
	return &Service{foods: foods, origins: origins, matcher: matcher, store: store, profile: prof, events: events, Now: time.Now}
}

var _ foodevent.Restorer = (*Service)(nil)

// Changes returns the changes list of food id (nil when it has none).
func (s *Service) Changes(ctx context.Context, id int64) ([]string, error) {
	return s.store.Changes(ctx, id)
}

// Variant saves p as a filled variant of recipe baseID in its variant group
// (no confirmation needed, VAR-4). A blocked check saves nothing: Result.Food
// is nil and Result.Check says why.
func (s *Service) Variant(ctx context.Context, baseID int64, p Proposal) (Result, error) {
	base, err := s.recipe(ctx, baseID)
	if err != nil {
		return Result{}, err
	}
	pl, err := s.plan(ctx, base, p)
	if err != nil || pl.check.Blocked() {
		return Result{Check: pl.check}, err
	}
	var out *food.Food
	undo, err := s.events.Recorded(ctx, func(ctx context.Context) error {
		children, err := s.children(ctx, pl, food.Origin{Kind: food.OriginImport, Ref: fmt.Sprintf("variant:%d", baseID)})
		if err != nil {
			return err
		}
		if base.VariantGroupID == nil {
			group, err := s.foods.CreateVariantGroup(ctx, base.Name)
			if err != nil {
				return fmt.Errorf("create variant group: %w", err)
			}
			base.VariantGroupID = &group
			if err := s.foods.Update(ctx, base); err != nil {
				return fmt.Errorf("assign recipe to variant group: %w", err)
			}
		}
		v := *base
		v.ID, v.Archived, v.Favorite = 0, false, false
		v.Name = strings.TrimSpace(p.Name)
		v.Children = children
		v.Instructions = instructions(p.Steps)
		v.Tags = slices.Clone(base.Tags)
		v.SourceURL, v.ImagePath = nil, nil
		if err := s.foods.Create(ctx, &v); err != nil {
			return fmt.Errorf("create variant: %w", err)
		}
		if err := s.store.PutChanges(ctx, v.ID, p.Changes); err != nil {
			return err
		}
		out = &v
		after, err := s.snapshot(ctx, &v)
		if err != nil {
			return err
		}
		return s.events.Record(ctx, foodevent.Event{
			Kind: foodevent.KindRecipeContent, SubjectID: v.ID, Action: foodevent.ActionCreate, After: after,
		})
	})
	if err != nil {
		return Result{}, err
	}
	return Result{Food: out, FoodID: out.ID, Changes: p.Changes, Undo: undo, Check: pl.check}, nil
}

// PreviewEdit shows an in-place edit of recipe id before it is saved: the
// diff, the confirmed plates from today on and the check. It writes nothing.
func (s *Service) PreviewEdit(ctx context.Context, id int64, p Proposal) (Preview, error) {
	base, err := s.recipe(ctx, id)
	if err != nil {
		return Preview{}, err
	}
	pl, err := s.plan(ctx, base, p)
	if err != nil {
		return Preview{}, err
	}
	dates, err := s.store.ConfirmedDates(ctx, id, s.Now())
	if err != nil {
		return Preview{}, err
	}
	if dates == nil {
		dates = []string{}
	}
	return Preview{
		Before:         contentOf(base),
		After:          Content{Name: strings.TrimSpace(p.Name), Ingredients: p.Ingredients, Steps: p.Steps},
		ConfirmedDates: dates,
		Check:          pl.check,
		plan:           pl,
	}, nil
}

// Edit changes recipe id in place (only on the user's explicit request,
// VAR-2). A blocked check saves nothing.
func (s *Service) Edit(ctx context.Context, id int64, p Proposal) (Result, error) {
	base, err := s.recipe(ctx, id)
	if err != nil {
		return Result{}, err
	}
	pl, err := s.plan(ctx, base, p)
	if err != nil {
		return Result{}, err
	}
	return s.edit(ctx, base, p, pl)
}

// EditPreviewed saves the edit PreviewEdit returned pv for, with the same
// matching, so the saved lines are the ones previewed and nothing is matched
// twice.
func (s *Service) EditPreviewed(ctx context.Context, id int64, p Proposal, pv Preview) (Result, error) {
	base, err := s.recipe(ctx, id)
	if err != nil {
		return Result{}, err
	}
	return s.edit(ctx, base, p, pv.plan)
}

func (s *Service) edit(ctx context.Context, base *food.Food, p Proposal, pl planned) (Result, error) {
	id := base.ID
	if pl.check.Blocked() {
		return Result{Check: pl.check}, nil
	}
	var out *food.Food
	undo, err := s.events.Recorded(ctx, func(ctx context.Context) error {
		before, err := s.snapshot(ctx, base)
		if err != nil {
			return err
		}
		children, err := s.children(ctx, pl, food.Origin{Kind: food.OriginImport, Ref: fmt.Sprintf("edit:%d", id)})
		if err != nil {
			return err
		}
		next := *base
		next.Name = strings.TrimSpace(p.Name)
		next.Children = children
		next.Instructions = instructions(p.Steps)
		if err := s.foods.Update(ctx, &next); err != nil {
			return fmt.Errorf("edit recipe: %w", err)
		}
		if err := s.store.PutChanges(ctx, id, p.Changes); err != nil {
			return err
		}
		out = &next
		after, err := s.snapshot(ctx, &next)
		if err != nil {
			return err
		}
		return s.events.Record(ctx, foodevent.Event{
			Kind: foodevent.KindRecipeContent, SubjectID: id, Action: foodevent.ActionUpdate, Before: before, After: after,
		})
	})
	if err != nil {
		return Result{}, err
	}
	return Result{Food: out, FoodID: out.ID, Changes: p.Changes, Undo: undo, Check: pl.check}, nil
}

// ReplaceLines swaps the ingredient lines of recipe id in place and adds
// add to its changes list: one recipe_content food event, so Undo puts the
// recipe back (the substitution's "Always in this recipe", HELP-7). The lines
// are stored foods already, so no matching or gate runs.
func (s *Service) ReplaceLines(ctx context.Context, id int64, children []food.FoodComponent, add []string) error {
	base, err := s.recipe(ctx, id)
	if err != nil {
		return err
	}
	before, err := s.snapshot(ctx, base)
	if err != nil {
		return err
	}
	changes, err := s.store.Changes(ctx, id)
	if err != nil {
		return err
	}
	next := *base
	next.Children = children
	if err := s.foods.Update(ctx, &next); err != nil {
		return fmt.Errorf("edit recipe: %w", err)
	}
	if err := s.store.PutChanges(ctx, id, append(changes, add...)); err != nil {
		return err
	}
	after, err := s.snapshot(ctx, &next)
	if err != nil {
		return err
	}
	return s.events.Record(ctx, foodevent.Event{
		Kind: foodevent.KindRecipeContent, SubjectID: id, Action: foodevent.ActionUpdate, Before: before, After: after,
	})
}

// PutChanges stores the changes list of food id; an empty list removes it.
func (s *Service) PutChanges(ctx context.Context, id int64, changes []string) error {
	return s.store.PutChanges(ctx, id, changes)
}

// ConfirmedDates are the dates of confirmed plates from today on that hold
// recipe id (or a hidden variant of it).
func (s *Service) ConfirmedDates(ctx context.Context, id int64) ([]string, error) {
	dates, err := s.store.ConfirmedDates(ctx, id, s.Now())
	if dates == nil {
		dates = []string{}
	}
	return dates, err
}

// CheckRecipe runs the gates on stored recipe id ("Check this recipe"). It
// needs no model: every line is a food already; a leaf without nutrition is
// an unmatched line (P3).
func (s *Service) CheckRecipe(ctx context.Context, id int64) (Check, error) {
	base, err := s.recipe(ctx, id)
	if err != nil {
		return Check{}, err
	}
	h, _, err := s.household(ctx)
	if err != nil {
		return Check{}, err
	}
	r := gateRecipe(base, base.Name, stepTexts(base.Instructions))
	chk := Check{Lines: make([]Line, 0, len(base.Children))}
	for _, c := range base.Children {
		matched := c.ChildKind == food.KindComposed
		if !matched {
			leaf, err := s.foods.Get(ctx, c.ChildID)
			if err != nil {
				return Check{}, err
			}
			matched = leaf.Kcal100g != nil
		}
		ln := Line{Text: lineText(c), Matched: matched, FoodID: c.ChildID, FoodName: c.ChildName}
		if !matched {
			ln.Reason = food.ReasonNutrition
		}
		chk.Lines = append(chk.Lines, ln)
		r.Ingredients = append(r.Ingredients, gates.Ingredient{FoodID: c.ChildID, Name: c.ChildName, Grams: c.Grams, ToTaste: c.ToTaste, Matched: matched})
	}
	res := gates.Check(r, h)
	chk.Blocks, chk.Flags = nonNil(res.Blocks), nonNil(res.Flags)
	return chk, nil
}

// Restore puts recipe id back to before (the recipe_content kind's
// foodevent.Restorer): nil removes the variant the change created.
func (s *Service) Restore(ctx context.Context, id int64, before json.RawMessage) error {
	if before == nil {
		if err := s.foods.Delete(ctx, id); err != nil {
			if errors.Is(err, domain.ErrInUse) {
				return fmt.Errorf("%w: the variant is in use", domain.ErrPlanChangeStale)
			}
			return err
		}
		return nil
	}
	var snap contentSnap
	if err := json.Unmarshal(before, &snap); err != nil {
		return fmt.Errorf("recipe content snapshot of food %d: %w", id, err)
	}
	f, err := s.foods.Get(ctx, id)
	if err != nil {
		return fmt.Errorf("%w: the recipe of the change is gone", domain.ErrPlanChangeStale)
	}
	f.Name = snap.Name
	f.Children = snap.Children
	f.Instructions = instructions(snap.Steps)
	if err := s.foods.Update(ctx, f); err != nil {
		return err
	}
	return s.store.PutChanges(ctx, id, snap.Changes)
}

// Verify refuses an undo when recipe id's name, lines or steps are no longer
// what the change wrote (foodevent.Verifier): a manual edit since then must
// not be lost.
func (s *Service) Verify(ctx context.Context, id int64, after json.RawMessage) error {
	var want contentSnap
	if err := json.Unmarshal(after, &want); err != nil {
		return fmt.Errorf("recipe content snapshot of food %d: %w", id, err)
	}
	f, err := s.foods.Get(ctx, id)
	if err != nil {
		return fmt.Errorf("%w: the recipe of the change is gone", domain.ErrPlanChangeStale)
	}
	if want.Name != f.Name || !slices.Equal(want.Steps, stepTexts(f.Instructions)) ||
		!slices.EqualFunc(want.Children, f.Children, sameLine) {
		return fmt.Errorf("%w: the recipe was changed after the change", domain.ErrPlanChangeStale)
	}
	return nil
}

// sameLine compares what a manual edit changes of a line; the stored line
// also carries names and a grams source the snapshot leaves out.
func sameLine(a, b food.FoodComponent) bool {
	return a.ChildID == b.ChildID && a.Amount == b.Amount && a.Unit == b.Unit && a.Grams == b.Grams &&
		a.SortOrder == b.SortOrder && a.ToTaste == b.ToTaste
}

// contentSnap is the recipe_content kind's food event JSON of an edit.
type contentSnap struct {
	Name     string               `json:"name"`
	Children []food.FoodComponent `json:"children"`
	Steps    []string             `json:"steps"`
	Changes  []string             `json:"changes,omitempty"`
}

func (s *Service) snapshot(ctx context.Context, f *food.Food) (json.RawMessage, error) {
	changes, err := s.store.Changes(ctx, f.ID)
	if err != nil {
		return nil, err
	}
	children := make([]food.FoodComponent, len(f.Children))
	for i, c := range f.Children {
		c.ID, c.ParentID = 0, 0
		children[i] = c
	}
	return json.Marshal(contentSnap{Name: f.Name, Children: children, Steps: stepTexts(f.Instructions), Changes: changes})
}

func (s *Service) recipe(ctx context.Context, id int64) (*food.Food, error) {
	f, err := s.foods.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if f.Kind != food.KindComposed {
		return nil, fmt.Errorf("%w: only a recipe can be changed", domain.ErrInvalidInput)
	}
	return f, nil
}

// planned is a proposal after matching and the gates.
type planned struct {
	check  Check
	drafts []importer.DraftIngredient
	lines  []food.LineMatch
}

// plan matches the proposal's lines and runs the gates. It writes nothing:
// external foods are added to the catalogue only when the change is saved.
func (s *Service) plan(ctx context.Context, base *food.Food, p Proposal) (planned, error) {
	if len(p.Changes) == 0 {
		return planned{}, fmt.Errorf("%w: a change needs its changes list", domain.ErrInvalidInput)
	}
	h, lang, err := s.household(ctx)
	if err != nil {
		return planned{}, err
	}
	drafts := importer.ParseLines(p.Ingredients, lang)
	req := food.MatchRequest{Lang: lang, Requested: true, Ref: fmt.Sprintf("recipe:%d", base.ID)}
	for _, d := range drafts {
		req.Lines = append(req.Lines, importer.MatchLineOf(d))
	}
	var lines []food.LineMatch
	if len(req.Lines) > 0 {
		res, err := s.matcher.Match(ctx, req)
		if err != nil {
			return planned{}, fmt.Errorf("match lines: %w", err)
		}
		lines = res.Lines
	}
	r := gateRecipe(base, strings.TrimSpace(p.Name), p.Steps)
	chk := Check{Lines: make([]Line, 0, len(drafts))}
	for i, d := range drafts {
		m := lines[i]
		ln := Line{Text: d.RawText, Matched: m.Pick >= 0 && (d.ToTaste || m.Grams != nil), Reason: m.Reason}
		if ln.Text == "" {
			ln.Text = d.Name
		}
		var id int64
		if m.Pick >= 0 {
			c := m.Candidates[m.Pick]
			ln.FoodName = c.Name
			if c.ExistingID != nil {
				id = *c.ExistingID
			}
		}
		if !ln.Matched && ln.Reason == "" {
			ln.Reason = food.ReasonGrams
		}
		if ln.Matched {
			ln.Reason = ""
		}
		ln.FoodID = id
		chk.Lines = append(chk.Lines, ln)
		grams := 0.0
		if m.Grams != nil {
			grams = *m.Grams
		}
		r.Ingredients = append(r.Ingredients, gates.Ingredient{FoodID: id, Name: d.Name, Grams: grams, ToTaste: d.ToTaste, Matched: ln.Matched})
	}
	res := gates.Check(r, h)
	chk.Blocks, chk.Flags = nonNil(res.Blocks), nonNil(res.Flags)
	return planned{check: chk, drafts: drafts, lines: lines}, nil
}

// children turns the matched lines into recipe lines, adding external foods
// to the catalogue (P3, MATCH-3).
func (s *Service) children(ctx context.Context, pl planned, o food.Origin) ([]food.FoodComponent, error) {
	out := make([]food.FoodComponent, 0, len(pl.drafts))
	for i, d := range pl.drafts {
		m := pl.lines[i]
		id, _, err := food.EnsureLeaf(ctx, s.foods, s.origins, m.Candidates[m.Pick], o)
		if err != nil {
			return nil, err
		}
		c := food.FoodComponent{ChildID: id, Amount: d.Amount, Unit: d.Unit, SortOrder: i, GramsSource: food.GramsSourceDirect}
		switch {
		case d.ToTaste || m.Grams == nil:
			c.Amount, c.Unit, c.Grams, c.ToTaste = 1, "g", 1, true
		default:
			c.Grams = *m.Grams
			if c.Amount <= 0 || c.Unit == "" {
				c.Amount, c.Unit = c.Grams, "g"
			}
		}
		out = append(out, c)
	}
	return out, nil
}

// household is the part of the profile the gates read, and the recipe
// language. Allergens are the allergen codes the restriction chips name.
func (s *Service) household(ctx context.Context) (gates.Household, string, error) {
	p, err := s.profile.Get(ctx)
	if err != nil {
		return gates.Household{}, "", err
	}
	var h gates.Household
	switch p.DietType {
	case profile.DietVegan, profile.DietVegetarian, profile.DietPescetarian:
		h.Diet = diet.Class(p.DietType)
	}
	for _, code := range gates.AllergenCodes {
		for _, r := range p.DietaryRestrictions {
			if strings.Contains(strings.ToLower(r), code) {
				h.Allergens = append(h.Allergens, code)
				break
			}
		}
	}
	lang := "en"
	if strings.HasPrefix(p.Locale, "de") {
		lang = "de"
	}
	return h, lang, nil
}

func gateRecipe(base *food.Food, name string, steps []string) gates.Recipe {
	r := gates.Recipe{Name: name, Steps: steps, Portions: 1}
	if base.ReferencePortions != nil && *base.ReferencePortions >= 1 {
		r.Portions = int(*base.ReferencePortions)
	}
	if base.KeepDays != nil {
		r.KeepDays = *base.KeepDays
	}
	if base.Freezable != nil {
		r.Freezable = *base.Freezable
	}
	return r
}

func instructions(steps []string) []food.Instruction {
	out := make([]food.Instruction, 0, len(steps))
	for _, st := range steps {
		if st = strings.TrimSpace(st); st != "" {
			out = append(out, food.Instruction{StepNumber: len(out) + 1, Text: st})
		}
	}
	return out
}

func stepTexts(in []food.Instruction) []string {
	out := make([]string, len(in))
	for i, st := range in {
		out[i] = st.Text
	}
	return out
}

func contentOf(f *food.Food) Content {
	c := Content{Name: f.Name, Ingredients: make([]string, len(f.Children)), Steps: stepTexts(f.Instructions)}
	for i, ch := range f.Children {
		c.Ingredients[i] = lineText(ch)
	}
	return c
}

// lineText is a stored line as text ("200 g Coconut yoghurt").
func lineText(c food.FoodComponent) string {
	if c.ToTaste {
		return c.ChildName
	}
	return strings.TrimSpace(fmt.Sprintf("%s %s %s", formatAmount(c.Amount), c.Unit, c.ChildName))
}

func formatAmount(v float64) string {
	return strings.TrimSuffix(strings.TrimRight(fmt.Sprintf("%.2f", v), "0"), ".")
}

func nonNil(f []gates.Finding) []gates.Finding {
	if f == nil {
		return []gates.Finding{}
	}
	return f
}
