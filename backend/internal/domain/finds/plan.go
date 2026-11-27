package finds

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/discovery"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/importer"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
)

// SessionLength is how long a planning session stays open after its last
// trigger (FIND-1, P11).
const SessionLength = 30 * time.Minute

// exploreTag is planning.ExploreTag: the planner places an untried recipe
// with it first while the window's new-dish quota is open, newest first.
const exploreTag = "explore"

// maxStaged bounds the ready finds read for one generation.
const maxStaged = 50

// Library turns a ready find into a recipe and back (C3).
type Library interface {
	// Leaf is the catalogue leaf of a matched candidate (food.EnsureLeaf).
	Leaf(ctx context.Context, c food.Candidate, o food.Origin) (int64, error)
	Finalize(ctx context.Context, in importer.FinalizeInput) (*importer.FinalizedFood, error)
	Create(ctx context.Context, f *food.Food) error
	Delete(ctx context.Context, id int64) error
	// StartTrial puts a recipe that entered the library on trial (NEW-1).
	StartTrial(ctx context.Context, id int64) error
	// Unplan removes the window's draft plates that eat foodID; a confirmed
	// one refuses (domain.ErrInUse).
	Unplan(ctx context.Context, foodID int64, windowStart time.Time) error
	// OnDraft reports whether a draft plate of the window eats foodID.
	OnDraft(ctx context.Context, foodID int64, windowStart time.Time) (bool, error)
}

// Planning is what the planning side of finds reads (C3).
type Planning struct {
	Library Library
	ChefOn  func(ctx context.Context) (bool, error)
	// NewDishes is the window's new-dish count (chef.new_dishes_per_week).
	NewDishes func(ctx context.Context) (int, error)
	// Dish names the cuisine the planning finds for the window starting on
	// day look for (the day's discovery cuisine); planningDishes makes each
	// a different main.
	Dish func(ctx context.Context, day time.Time) (string, error)
	Now  func() time.Time
}

func (s *Service) chefOn(ctx context.Context) (bool, error) {
	if s.d.Planning.ChefOn == nil {
		return false, nil
	}
	return s.d.Planning.ChefOn(ctx)
}

// Touch opens the planning session, or keeps it open for SessionLength
// (generate and regenerate call it; the plan page through Opened).
func (s *Service) Touch(ctx context.Context) error {
	on, err := s.chefOn(ctx)
	if err != nil || !on {
		return err
	}
	return s.d.Store.TouchSession(ctx, s.d.Planning.Now().Add(SessionLength))
}

// InSession reports whether a planning session is open now.
func (s *Service) InSession(ctx context.Context) (bool, error) {
	until, err := s.d.Store.SessionUntil(ctx)
	if err != nil {
		return false, err
	}
	return s.d.Planning.Now().Before(until), nil
}

// Opened is the plan page opening a window: a window with drafts or the next
// window opens a session; the next window also prefetches its new dishes
// (FIND-1a).
func (s *Service) Opened(ctx context.Context, windowStart time.Time, drafts, next bool) error {
	if !drafts && !next {
		return nil
	}
	if err := s.Touch(ctx); err != nil {
		return err
	}
	if !next {
		return nil
	}
	return s.Prefetch(ctx, windowStart)
}

// Prefetch starts the finds the window's new dishes still need, only during
// a planning session (P11): new dishes less the searching ones and the ready
// ones that can fill a lunch or dinner main (FIND-3: ready finds first; one
// that cannot stays ready for later and does not hold a new search back).
func (s *Service) Prefetch(ctx context.Context, windowStart time.Time) error {
	on, err := s.chefOn(ctx)
	if err != nil || !on {
		return err
	}
	if open, err := s.InSession(ctx); err != nil || !open {
		return err
	}
	want, err := s.d.Planning.NewDishes(ctx)
	if err != nil {
		return err
	}
	have, err := s.openFitting(ctx)
	if err != nil {
		return err
	}
	if want <= have {
		return nil
	}
	cuisine, err := s.d.Planning.Dish(ctx, windowStart)
	if err != nil {
		return err
	}
	if clean(cuisine) == "" {
		// No cuisine in the household's scope (FIND-15): nothing to search.
		return nil
	}
	return s.request(ctx, planningDishes(clean(cuisine), want-have, windowStart))
}

// openFitting counts the searching planning finds and the ready finds that
// can fill a lunch or dinner main cell.
func (s *Service) openFitting(ctx context.Context) (int, error) {
	searching := StateSearching
	open, err := s.d.Store.List(ctx, &searching, maxStaged)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, f := range open {
		if f.Trigger == TriggerPlanning {
			n++
		}
	}
	k, err := s.kitchen(ctx)
	if err != nil {
		return 0, err
	}
	state := StateReady
	ready, err := s.d.Store.List(ctx, &state, maxStaged)
	if err != nil {
		return 0, err
	}
	for _, f := range ready {
		if cellUnfit(f.Draft, k) == "" {
			n++
		}
	}
	return n, nil
}

// kitchen is the household's kitchen (time allowances for cellUnfit).
func (s *Service) kitchen(ctx context.Context) (profile.Kitchen, error) {
	if s.d.Household == nil {
		return profile.Kitchen{}, nil
	}
	h, err := s.d.Household(ctx)
	return h.Kitchen, err
}

// ThemeSet starts n finds for a theme the user set on a future window
// (FIND-1d): outside a session too. D3 calls it.
func (s *Service) ThemeSet(ctx context.Context, theme string, n int) error {
	on, err := s.chefOn(ctx)
	if err != nil || !on {
		return err
	}
	return s.request(ctx, planningDishes(clean(theme), n, s.now()))
}

// request starts an autonomous find per dish; no provider starts none.
func (s *Service) request(ctx context.Context, dishes []string) error {
	for _, dish := range dishes {
		_, err := s.Request(ctx, dish, TriggerPlanning)
		if errors.Is(err, llm.ErrProviderMissing) {
			return nil
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// Stage adds up to n ready finds, oldest first, to the library as untried
// recipes placed in the window starting on windowStart, before a generation
// (FIND-3). It returns their food ids (Settle returns the unused ones) and a
// note per ready find left for a later week because it cannot fill a lunch
// or dinner main. Of two ready finds from one source page only the first is
// placed.
func (s *Service) Stage(ctx context.Context, n int, windowStart time.Time) ([]int64, []string, error) {
	if n <= 0 {
		return nil, nil, nil
	}
	on, err := s.chefOn(ctx)
	if err != nil || !on {
		return nil, nil, err
	}
	k, err := s.kitchen(ctx)
	if err != nil {
		return nil, nil, err
	}
	state := StateReady
	ready, err := s.d.Store.List(ctx, &state, maxStaged)
	if err != nil {
		return nil, nil, err
	}
	slices.Reverse(ready) // List is newest first
	var ids []int64
	var notes []string
	urls := map[string]bool{}
	for i := range ready {
		f := &ready[i]
		if why := cellUnfit(f.Draft, k); why != "" {
			name := f.Dish
			if f.Draft != nil {
				name = f.Draft.Name
			}
			notes = append(notes, fmt.Sprintf("The ready new dish %s waits for a later week: %s.", name, why))
			continue
		}
		if len(ids) == n {
			continue
		}
		u := discovery.NormalizeURL(f.SourceURL)
		if f.SourceURL != "" && urls[u] {
			continue
		}
		id, err := s.place(ctx, f, windowStart)
		if err != nil {
			// The find stays ready; the run plans without it.
			s.d.Logger.WarnContext(ctx, "finds.stage skipped", "find_id", f.ID, "error", err)
			continue
		}
		urls[u] = true
		ids = append(ids, id)
	}
	return ids, notes, nil
}

// Settle returns the staged finds the generation did not place to ready.
func (s *Service) Settle(ctx context.Context, windowStart time.Time, staged []int64, placed func(foodID int64) bool) error {
	fs, err := s.d.Store.Placed(ctx, windowStart)
	if err != nil {
		return err
	}
	var errs []error
	for _, f := range fs {
		if f.FoodID == nil || !slices.Contains(staged, *f.FoodID) || placed(*f.FoodID) {
			continue
		}
		errs = append(errs, s.unplace(ctx, f, false))
	}
	return errors.Join(errs...)
}

// Activity is one entry of a window's "What the assistant did" list (PRO-5).
type Activity struct {
	Kind   string `json:"kind"` // find_placed
	FindID int64  `json:"find_id"`
	FoodID int64  `json:"food_id"`
	Name   string `json:"name"`
}

// ActivityKindFindPlaced: the planner placed a ready find.
const ActivityKindFindPlaced = "find_placed"

// Activities lists what the assistant did in the window starting on
// windowStart, at most budget lines (PRO-4); more counts the rest.
func (s *Service) Activities(ctx context.Context, windowStart time.Time, budget int) (list []Activity, more int, err error) {
	fs, err := s.d.Store.Placed(ctx, windowStart)
	if err != nil {
		return nil, 0, err
	}
	list = []Activity{}
	for _, f := range fs {
		// A regenerate may have dropped the recipe, and an accepted plate
		// makes the Undo fail: neither is something the assistant did now.
		if f.FoodID == nil {
			continue
		}
		on, err := s.d.Planning.Library.OnDraft(ctx, *f.FoodID, windowStart)
		if err != nil {
			return nil, 0, err
		}
		if !on {
			continue
		}
		a := Activity{Kind: ActivityKindFindPlaced, FindID: f.ID, Name: f.Dish}
		a.FoodID = *f.FoodID
		if f.Draft != nil && f.Draft.Name != "" {
			a.Name = f.Draft.Name
		}
		list = append(list, a)
	}
	budget = max(budget, 0)
	if len(list) > budget {
		return list[:budget], len(list) - budget, nil
	}
	return list, 0, nil
}

// UndoPlaced takes a placed find out of its window's drafts and the library;
// it is ready again.
func (s *Service) UndoPlaced(ctx context.Context, id int64) error {
	f, err := s.d.Store.Get(ctx, id)
	if err != nil {
		return err
	}
	if f.State != StateInLibrary || f.FoodID == nil {
		return fmt.Errorf("%w: find %d is not placed", domain.ErrNotFound, id)
	}
	return s.unplace(ctx, *f, true)
}

// unplace puts a placed find back to ready and deletes its recipe; withPlates
// first removes the recipe's draft plates in the window.
func (s *Service) unplace(ctx context.Context, f Find, withPlates bool) error {
	lib := s.d.Planning.Library
	return s.inTx(ctx, func(ctx context.Context) error {
		if withPlates {
			if f.PlacedOn == nil {
				return fmt.Errorf("%w: find %d is not placed", domain.ErrNotFound, f.ID)
			}
			start, err := time.Parse(time.DateOnly, *f.PlacedOn)
			if err != nil {
				return fmt.Errorf("find %d placed on %q: %w", f.ID, *f.PlacedOn, err)
			}
			if err := lib.Unplan(ctx, *f.FoodID, start); err != nil {
				return err
			}
		}
		if _, err := s.d.Store.Unplace(ctx, f.ID); err != nil {
			return err
		}
		return lib.Delete(ctx, *f.FoodID)
	})
}

func (s *Service) inTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if s.d.Tx == nil {
		return fn(ctx)
	}
	return s.d.Tx.InTx(ctx, fn)
}

// place writes a ready find as a recipe with its origin and marks it placed.
func (s *Service) place(ctx context.Context, f *Find, windowStart time.Time) (int64, error) {
	var id int64
	err := s.inTx(ctx, func(ctx context.Context) error {
		r, err := s.recipe(ctx, f)
		if err != nil {
			return err
		}
		if err := s.d.Planning.Library.Create(ctx, r); err != nil {
			return fmt.Errorf("create recipe: %w", err)
		}
		if err := s.d.Planning.Library.StartTrial(ctx, r.ID); err != nil {
			return err
		}
		if _, err := s.d.Store.Place(ctx, f.ID, r.ID, windowStart); err != nil {
			return err
		}
		id = r.ID
		return nil
	})
	return id, err
}

// recipe is a ready find's draft as a recipe: every line on its matched
// leaf (a ready find has no unmatched line, AC-7).
func (s *Service) recipe(ctx context.Context, f *Find) (*food.Food, error) {
	d := f.Draft
	if d == nil {
		return nil, fmt.Errorf("%w: find %d has no draft", domain.ErrInvalidInput, f.ID)
	}
	lib := s.d.Planning.Library
	origin := food.Origin{Kind: food.OriginFind, Ref: strconv.FormatInt(f.ID, 10)}
	in := importer.FinalizeInput{
		Name: d.Name, Role: d.Role, ReferencePortions: d.ReferencePortions,
		PrepMinutes: d.PrepMinutes, CookMinutes: d.CookMinutes, Tags: []string{exploreTag},
	}
	if in.Role == "" {
		in.Role = string(food.RoleMain) // a new dish for the plan is a main
	}
	if in.ReferencePortions <= 0 {
		in.ReferencePortions = 1
	}
	for i, step := range d.Instructions {
		in.Instructions = append(in.Instructions, importer.FinalizedInstruction{StepNumber: i + 1, Text: step})
	}
	for _, line := range d.Ingredients {
		m := line.Match
		if m == nil || m.Status != food.MatchMatched || m.Pick < 0 || m.Pick >= len(m.Candidates) {
			return nil, fmt.Errorf("%w: %q", domain.ErrImportUnmatchedLine, line.Name)
		}
		leaf, err := lib.Leaf(ctx, m.Candidates[m.Pick], origin)
		if err != nil {
			return nil, err
		}
		row := importer.FinalizedChild{
			Resolution: importer.ResolutionExisting, FoodID: leaf, ToTaste: line.ToTaste,
			Amount: line.Amount, Unit: line.Unit, PortionGrams: line.PortionGrams,
		}
		if !line.ToTaste && (row.Amount <= 0 || row.Unit == "") && m.Grams != nil {
			row.Amount, row.Unit, row.PortionGrams = *m.Grams, "g", nil
		}
		in.Children = append(in.Children, row)
	}
	out, err := lib.Finalize(ctx, in)
	if err != nil {
		return nil, err
	}
	role := food.Role(out.Role)
	r := &food.Food{
		Kind: food.KindComposed, Name: out.Name, Role: &role, ReferencePortions: &out.ReferencePortions,
		PrepMinutes: out.PrepMinutes, CookMinutes: out.CookMinutes, Tags: out.Tags,
		KeepDays: d.KeepDays, Freezable: d.Freezable,
	}
	if f.SourceURL != "" {
		r.SourceURL = &f.SourceURL
	}
	if d.Description != "" {
		r.Description = &d.Description
	}
	for _, c := range out.Children {
		r.Children = append(r.Children, food.FoodComponent{
			ChildID: c.ChildID, Amount: c.Amount, Unit: c.Unit, Grams: c.Grams,
			GramsSource: c.GramsSource, SortOrder: c.SortOrder, ToTaste: c.ToTaste,
		})
	}
	for _, st := range out.Instructions {
		r.Instructions = append(r.Instructions, food.Instruction{StepNumber: st.StepNumber, Text: st.Text})
	}
	return r, nil
}

// fillDishes are the ten searches of "Fill my library" (COLD-3), spread
// across roles and attributes; the pipeline adapts each to the household.
var fillDishes = []string{
	"quick weeknight chicken main",
	"vegetarian main with legumes",
	"light fish main",
	"hearty one-pot stew",
	"pasta main the whole family likes",
	"simple vegetable side",
	"side with rice or potatoes",
	"warming soup",
	"fresh salad",
	"mild vegetarian curry",
}

// FillLibrary starts the ten requested finds of "Fill my library"; each
// ready one enters the library on trial. The caller checks the Chef is on. Finds
// still searching from an earlier tap are returned with it; only the dishes
// without one are started, so a partial failure tops up to ten.
func (s *Service) FillLibrary(ctx context.Context) ([]Find, error) {
	searching := StateSearching
	open, err := s.d.Store.List(ctx, &searching, maxStaged)
	if err != nil {
		return nil, err
	}
	open = slices.DeleteFunc(open, func(f Find) bool { return f.Trigger != TriggerFill })
	out := open
	for _, dish := range fillDishes {
		if slices.ContainsFunc(open, func(f Find) bool { return f.Dish == clean(dish) }) {
			continue
		}
		f, err := s.Request(ctx, dish, TriggerFill)
		if err != nil {
			return out, err
		}
		out = append(out, *f)
	}
	return out, nil
}

// addToLibrary writes a ready find as a recipe on trial and marks it in the
// library, like Keep after the review.
func (s *Service) addToLibrary(ctx context.Context, f *Find) error {
	return s.inTx(ctx, func(ctx context.Context) error {
		r, err := s.recipe(ctx, f)
		if err != nil {
			return err
		}
		lib := s.d.Planning.Library
		if err := lib.Create(ctx, r); err != nil {
			return fmt.Errorf("create recipe: %w", err)
		}
		if err := lib.StartTrial(ctx, r.ID); err != nil {
			return err
		}
		_, err = s.d.Store.Decide(ctx, f.ID, StateInLibrary, nil, &r.ID)
		return err
	})
}
