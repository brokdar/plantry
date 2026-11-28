// Package substitution swaps one ingredient of one cook session (HELP-3,
// HELP-7). The swap is a hidden variant of the session's recipe (archived,
// unlisted, in the recipe's variant group) that the session and its plate
// components point at, so nutrition, the shopping list, cook mode and every
// planner fact follow with no new read path. Undo is a food event of kind
// substitution on the session.
package substitution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/foodevent"
	"github.com/jaltszeimer/plantry/backend/internal/domain/gates"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/units"
)

// maxOptions is how many substitutes one ingredient offers.
const maxOptions = 3

const leafPageSize = 500

// HiddenVariant marks a food as the hidden variant of a batch substitution.
type HiddenVariant struct {
	FoodID int64
	// BaseFoodID is the recipe the variant was made from; 0 when it is gone.
	BaseFoodID int64
	// SessionID is the session using it; 0 when none does.
	SessionID int64
}

// Store is the port for the hidden variants and the session re-pointing. Every
// method joins the unit of work carried by ctx.
type Store interface {
	// Hidden returns the row of food id; ok is false when it is not hidden.
	Hidden(ctx context.Context, foodID int64) (HiddenVariant, bool, error)
	PutHidden(ctx context.Context, v HiddenVariant) error
	// ClearSession frees the session's slot on its hidden variant.
	ClearSession(ctx context.Context, sessionID int64) error
	// UnusedHidden returns the hidden variants nothing points at.
	UnusedHidden(ctx context.Context) ([]int64, error)
	// SetSessionFood points the session and its plate components at a food.
	SetSessionFood(ctx context.Context, sessionID, foodID int64) error
	// DeleteHidden removes the hidden-variant row of a food (it becomes an
	// ordinary recipe).
	DeleteHidden(ctx context.Context, foodID int64) error
	// CountSwap counts one more swap of from for to in recipe base and
	// reports true exactly once, at the second time.
	CountSwap(ctx context.Context, baseID, fromID, toID int64) (bool, error)
}

// Recipes is the recipe-change side the permanent saves use
// (recipechange.Service).
type Recipes interface {
	ReplaceLines(ctx context.Context, id int64, children []food.FoodComponent, changes []string) error
	PutChanges(ctx context.Context, id int64, changes []string) error
	ConfirmedDates(ctx context.Context, id int64) ([]string, error)
}

// Service applies and undoes substitutions.
type Service struct {
	foods    food.Repository
	sessions plate.SessionRepository
	store    Store
	recipes  Recipes
	events   *foodevent.Service
	// matcher and origins add a table substitute the catalogue lacks; both
	// nil leaves the catalogue as it is.
	matcher Matcher
	origins food.OriginStore
	// household is the diet, allergens and never items an option must fit;
	// nil offers every option.
	household func(ctx context.Context) (gates.Household, error)
	// Now is the clock for the past-plate guard; time.Now unless a test sets it.
	Now func() time.Time
}

// NewService creates a Service.
func NewService(foods food.Repository, sessions plate.SessionRepository, store Store, recipes Recipes, events *foodevent.Service) *Service {
	return &Service{foods: foods, sessions: sessions, store: store, recipes: recipes, events: events, Now: time.Now}
}

var _ foodevent.Restorer = (*Service)(nil)

// Matcher is the matching path (food.Matcher): it finds the external food a
// name stands for.
type Matcher interface {
	Match(ctx context.Context, req food.MatchRequest) (*food.MatchResult, error)
}

// WithCatalogue lets Options add a table substitute that is not in the
// catalogue yet, the way an import does (MATCH-3): matched, then created
// with its origin.
func (s *Service) WithCatalogue(m Matcher, origins food.OriginStore) *Service {
	s.matcher, s.origins = m, origins
	return s
}

// WithHousehold makes Options leave out what the household cannot eat: a
// diet conflict, an allergen or a never item (SUB-3).
func (s *Service) WithHousehold(h func(ctx context.Context) (gates.Household, error)) *Service {
	s.household = h
	return s
}

// Option is one offered substitute.
type Option struct {
	FoodID int64
	Name   string
	Ratio  float64
	// Amount is the substitute's amount for the sessions asked, zero for a
	// to-taste line.
	Amount units.Quantity
	NoteEN string
	NoteDE string
}

// Options returns up to three substitutes for the leaf from, taken from the
// curated table: only foods in the catalogue with nutrition (P3), in table
// order. sessionIDs name the batches the amounts are for; the sessions the
// swap can apply to (their recipe has a line of from and they are not past)
// are returned as applicable, in the order asked. When none can, it is
// domain.ErrInvalidInput.
func (s *Service) Options(ctx context.Context, fromID int64, sessionIDs []int64) ([]Option, []int64, error) {
	from, err := s.leaf(ctx, fromID)
	if err != nil {
		return nil, nil, err
	}
	var grams float64
	applicable := []int64{}
	var lastErr error
	for _, id := range sessionIDs {
		line, g, err := s.line(ctx, id, fromID)
		if err == nil {
			err = s.guardPast(ctx, id)
		}
		if errors.Is(err, domain.ErrInvalidInput) {
			lastErr = err
			continue
		}
		if err != nil {
			return nil, nil, err
		}
		applicable = append(applicable, id)
		if !line.ToTaste {
			grams += g
		}
	}
	if len(applicable) == 0 {
		if lastErr == nil {
			lastErr = fmt.Errorf("%w: no cook session given", domain.ErrInvalidInput)
		}
		return nil, nil, lastErr
	}
	rows := food.SubstitutesOf(from.Name)
	if len(rows) == 0 {
		return []Option{}, applicable, nil
	}
	leaves, err := s.leaves(ctx)
	if err != nil {
		return nil, nil, err
	}
	var h *gates.Household
	if s.household != nil {
		hh, err := s.household(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("household: %w", err)
		}
		h = &hh
	}
	out := []Option{}
	for _, row := range rows {
		if len(out) == maxOptions {
			break
		}
		if h != nil && !fitsHousehold(food.SynonymName(row.To), 0, *h) {
			continue
		}
		to := catalogueLeaf(leaves, row.To)
		if to == nil {
			if to, err = s.provision(ctx, row.To); err != nil {
				return nil, nil, err
			}
		}
		if to == nil || to.ID == fromID || (h != nil && !fitsHousehold(to.Name, to.ID, *h)) {
			continue
		}
		opt := Option{FoodID: to.ID, Name: to.Name, Ratio: row.Ratio, NoteEN: row.NoteEN, NoteDE: row.NoteDE}
		if grams > 0 {
			opt.Amount = units.Display(grams*row.Ratio, "g")
		}
		out = append(out, opt)
	}
	return out, applicable, nil
}

// Applied is the outcome of Apply.
type Applied struct {
	Undo foodevent.Undo
	// OfferPermanent is true once, when the same swap in the same recipe is
	// applied the second time.
	OfferPermanent bool
	// ConfirmedDates are the dates of confirmed plates from today on that
	// hold the recipe: "Always in this recipe" changes their shopping.
	ConfirmedDates []string
}

// Apply swaps leaf from for leaf to in the session's recipe and returns the
// undo token. to must be one of the table's substitutes for from. Applying
// again on the same session builds on the current variant and replaces it.
func (s *Service) Apply(ctx context.Context, sessionID, fromID, toID int64) (Applied, error) {
	from, err := s.leaf(ctx, fromID)
	if err != nil {
		return Applied{}, err
	}
	to, err := s.leaf(ctx, toID)
	if err != nil {
		return Applied{}, err
	}
	ratio, ok := s.tableRatio(ctx, from.Name, to)
	if !ok || !hasNutrition(to) {
		return Applied{}, fmt.Errorf("%w: %s is not a substitute for %s", domain.ErrInvalidInput, to.Name, from.Name)
	}
	var out Applied
	undo, err := s.events.Recorded(ctx, func(ctx context.Context) error {
		sess, err := s.sessions.GetSession(ctx, sessionID)
		if err != nil {
			return err
		}
		if _, _, err := s.line(ctx, sessionID, fromID); err != nil {
			return err
		}
		if err := s.guardPast(ctx, sessionID); err != nil {
			return err
		}
		cur, err := s.foods.Get(ctx, sess.FoodID)
		if err != nil {
			return err
		}
		before, err := s.snapshot(ctx, sess.FoodID)
		if err != nil {
			return err
		}
		base := cur.ID
		if h, hidden, err := s.store.Hidden(ctx, cur.ID); err != nil {
			return err
		} else if hidden && h.BaseFoodID != 0 {
			base = h.BaseFoodID
		}
		variant, err := s.createVariant(ctx, cur, fromID, to, ratio)
		if err != nil {
			return err
		}
		if err := s.store.ClearSession(ctx, sessionID); err != nil {
			return err
		}
		if err := s.store.PutHidden(ctx, HiddenVariant{FoodID: variant.ID, BaseFoodID: base, SessionID: sessionID}); err != nil {
			return err
		}
		if err := s.store.SetSessionFood(ctx, sessionID, variant.ID); err != nil {
			return err
		}
		if err := s.dropUnused(ctx, cur.ID); err != nil {
			return err
		}
		if out.OfferPermanent, err = s.store.CountSwap(ctx, base, fromID, toID); err != nil {
			return err
		}
		if out.ConfirmedDates, err = s.recipes.ConfirmedDates(ctx, base); err != nil {
			return err
		}
		return s.record(ctx, sessionID, before, snap{FoodID: variant.ID})
	})
	if err != nil {
		return Applied{}, err
	}
	out.Undo = undo
	return out, nil
}

// hiddenOf returns the hidden variant the session points at; it is
// domain.ErrNotFound when the session has no substitution.
func (s *Service) hiddenOf(ctx context.Context, sessionID int64) (HiddenVariant, error) {
	sess, err := s.sessions.GetSession(ctx, sessionID)
	if err != nil {
		return HiddenVariant{}, err
	}
	h, hidden, err := s.store.Hidden(ctx, sess.FoodID)
	if err != nil {
		return HiddenVariant{}, err
	}
	if !hidden || h.BaseFoodID == 0 {
		return HiddenVariant{}, fmt.Errorf("%w: cook session %d has no substitution", domain.ErrNotFound, sessionID)
	}
	return h, nil
}

// Always makes the batch's swaps permanent in the recipe itself ("Always in
// this recipe"): the base recipe is edited in place to the hidden variant's
// lines (it may hold several stacked swaps), the session points back at it and
// the hidden variant is left unused. One undo token covers the edit and the
// re-pointing.
func (s *Service) Always(ctx context.Context, sessionID int64) (foodevent.Undo, error) {
	return s.events.Recorded(ctx, func(ctx context.Context) error {
		h, err := s.hiddenOf(ctx, sessionID)
		if err != nil {
			return err
		}
		base, err := s.foods.Get(ctx, h.BaseFoodID)
		if err != nil {
			return err
		}
		v, err := s.foods.Get(ctx, h.FoodID)
		if err != nil {
			return err
		}
		changes, _ := diffSwaps(base.Children, v.Children)
		if len(changes) == 0 {
			return fmt.Errorf("%w: the batch has no swap to keep", domain.ErrInvalidInput)
		}
		before, err := s.snapshot(ctx, h.FoodID)
		if err != nil {
			return err
		}
		children := make([]food.FoodComponent, len(v.Children))
		for i, c := range v.Children {
			c.ID, c.ParentID = 0, 0
			children[i] = c
		}
		if err := s.recipes.ReplaceLines(ctx, base.ID, children, changes); err != nil {
			return err
		}
		if err := s.store.ClearSession(ctx, sessionID); err != nil {
			return err
		}
		if err := s.store.SetSessionFood(ctx, sessionID, base.ID); err != nil {
			return err
		}
		if err := s.dropUnused(ctx, h.FoodID); err != nil {
			return err
		}
		return s.record(ctx, sessionID, before, snap{FoodID: base.ID})
	})
}

// Saved is a hidden variant turned into a visible one.
type Saved struct {
	FoodID int64
	Name   string
	Undo   foodevent.Undo
}

// SaveVariant turns the session's hidden variant into a visible variant of
// its recipe ("Save as variant"): the hidden-variant row goes, it is
// unarchived and named "<recipe> with <substitutes>", and its changes list
// names every swap. The session keeps cooking it.
func (s *Service) SaveVariant(ctx context.Context, sessionID int64) (Saved, error) {
	var out Saved
	undo, err := s.events.Recorded(ctx, func(ctx context.Context) error {
		h, err := s.hiddenOf(ctx, sessionID)
		if err != nil {
			return err
		}
		base, err := s.foods.Get(ctx, h.BaseFoodID)
		if err != nil {
			return err
		}
		v, err := s.foods.Get(ctx, h.FoodID)
		if err != nil {
			return err
		}
		changes, targets := diffSwaps(base.Children, v.Children)
		if len(changes) == 0 {
			return fmt.Errorf("%w: the batch has no swap to keep", domain.ErrInvalidInput)
		}
		before := snap{FoodID: v.ID, Variant: &variantSnap{BaseFoodID: h.BaseFoodID, Name: v.Name}}
		if err := s.store.DeleteHidden(ctx, v.ID); err != nil {
			return err
		}
		v.Name = base.Name + " with " + strings.Join(targets, " and ")
		if err := s.foods.Update(ctx, v); err != nil {
			return fmt.Errorf("name variant: %w", err)
		}
		if _, err := s.foods.SetArchived(ctx, v.ID, false); err != nil {
			return fmt.Errorf("unarchive variant: %w", err)
		}
		if err := s.recipes.PutChanges(ctx, v.ID, changes); err != nil {
			return err
		}
		out.FoodID, out.Name = v.ID, v.Name
		return s.record(ctx, sessionID, before, snap{FoodID: v.ID})
	})
	if err != nil {
		return Saved{}, err
	}
	out.Undo = undo
	return out, nil
}

// diffSwaps lists the swaps that turn base lines into variant lines, one
// "<from> \u2192 <to>" entry each, and the names of the substitutes. Removed
// base lines pair in order with the variant's new or grown lines, so stacked
// swaps (A to B, then B to C) read as one swap (A to C).
func diffSwaps(base, variant []food.FoodComponent) (changes, targets []string) {
	baseGrams := map[int64]float64{}
	for _, c := range base {
		baseGrams[c.ChildID] = c.Grams
	}
	varIDs := map[int64]bool{}
	var to []string
	for _, c := range variant {
		varIDs[c.ChildID] = true
		if g, ok := baseGrams[c.ChildID]; !ok || g != c.Grams {
			to = append(to, c.ChildName)
		}
	}
	i := 0
	for _, c := range base {
		if varIDs[c.ChildID] {
			continue
		}
		if len(to) == 0 {
			changes = append(changes, c.ChildName+" removed")
			continue
		}
		t := to[min(i, len(to)-1)]
		i++
		changes = append(changes, c.ChildName+" \u2192 "+t)
		if !slices.Contains(targets, t) {
			targets = append(targets, t)
		}
	}
	return changes, targets
}

// guardPast refuses a session that is already cooked ("Done cooking") or has a
// plate dated before today: what was eaten stays as it was. Confirmed
// sessions of today and later stay allowed (the swap happens at the stove).
func (s *Service) guardPast(ctx context.Context, sessionID int64) error {
	sess, err := s.sessions.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if sess.CookedAt != nil {
		return fmt.Errorf("%w: cook session %d is already cooked", domain.ErrInvalidInput, sessionID)
	}
	links, err := s.sessions.ListLinks(ctx, sessionID)
	if err != nil {
		return err
	}
	now := s.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	for _, l := range links {
		if l.Cell.Date.Before(today) {
			return fmt.Errorf("%w: cook session %d has plates in the past", domain.ErrInvalidInput, sessionID)
		}
	}
	return nil
}

// Remove points the session back at its recipe and returns the undo token.
// It is domain.ErrNotFound when the session has no substitution.
func (s *Service) Remove(ctx context.Context, sessionID int64) (foodevent.Undo, error) {
	return s.events.Recorded(ctx, func(ctx context.Context) error {
		sess, err := s.sessions.GetSession(ctx, sessionID)
		if err != nil {
			return err
		}
		h, hidden, err := s.store.Hidden(ctx, sess.FoodID)
		if err != nil {
			return err
		}
		if !hidden || h.BaseFoodID == 0 {
			return fmt.Errorf("%w: cook session %d has no substitution", domain.ErrNotFound, sessionID)
		}
		before, err := s.snapshot(ctx, sess.FoodID)
		if err != nil {
			return err
		}
		if err := s.store.ClearSession(ctx, sessionID); err != nil {
			return err
		}
		if err := s.store.SetSessionFood(ctx, sessionID, h.BaseFoodID); err != nil {
			return err
		}
		return s.record(ctx, sessionID, before, snap{FoodID: h.BaseFoodID})
	})
}

// snap is the substitution kind's food event JSON: the food the session and
// its components point at, and its hidden-variant row when it has one.
type snap struct {
	FoodID  int64        `json:"food_id"`
	Variant *variantSnap `json:"variant,omitempty"`
}

type variantSnap struct {
	BaseFoodID int64 `json:"base_food_id"`
	// Name is set when the variant was saved as a visible one: undoing it
	// restores this name and hides the variant again.
	Name string `json:"name,omitempty"`
}

func (s *Service) snapshot(ctx context.Context, foodID int64) (snap, error) {
	out := snap{FoodID: foodID}
	h, hidden, err := s.store.Hidden(ctx, foodID)
	if err != nil {
		return snap{}, err
	}
	if hidden {
		out.Variant = &variantSnap{BaseFoodID: h.BaseFoodID}
	}
	return out, nil
}

func (s *Service) record(ctx context.Context, sessionID int64, before, after snap) error {
	b, _ := json.Marshal(before)
	a, _ := json.Marshal(after)
	return s.events.Record(ctx, foodevent.Event{
		Kind: foodevent.KindSubstitution, SubjectID: sessionID, Action: foodevent.ActionUpdate, Before: b, After: a,
	})
}

// Restore points session sessionID back at the state before (the
// substitution kind's foodevent.Restorer) and removes the variant it leaves
// unused.
func (s *Service) Restore(ctx context.Context, sessionID int64, before json.RawMessage) error {
	var b snap
	if err := json.Unmarshal(before, &b); err != nil {
		return fmt.Errorf("substitution snapshot of session %d: %w", sessionID, err)
	}
	if _, err := s.foods.Get(ctx, b.FoodID); err != nil {
		return fmt.Errorf("%w: the recipe of the change is gone", domain.ErrPlanChangeStale)
	}
	if err := s.store.ClearSession(ctx, sessionID); err != nil {
		return err
	}
	if b.Variant != nil {
		if err := s.store.PutHidden(ctx, HiddenVariant{FoodID: b.FoodID, BaseFoodID: b.Variant.BaseFoodID, SessionID: sessionID}); err != nil {
			return err
		}
		if b.Variant.Name != "" {
			if err := s.hideAgain(ctx, b.FoodID, b.Variant.Name); err != nil {
				return err
			}
		}
	}
	if err := s.store.SetSessionFood(ctx, sessionID, b.FoodID); err != nil {
		return err
	}
	return s.dropUnused(ctx)
}

// hideAgain undoes SaveVariant: the name comes back, the variant is archived
// and its changes list goes.
func (s *Service) hideAgain(ctx context.Context, id int64, name string) error {
	v, err := s.foods.Get(ctx, id)
	if err != nil {
		return fmt.Errorf("%w: the variant of the change is gone", domain.ErrPlanChangeStale)
	}
	v.Name = name
	if err := s.foods.Update(ctx, v); err != nil {
		return err
	}
	if _, err := s.foods.SetArchived(ctx, id, true); err != nil {
		return err
	}
	return s.recipes.PutChanges(ctx, id, nil)
}

// dropUnused deletes the unused hidden variants, except the keep ones (the
// food an undo of this change must still reach). A variant something still
// references stays.
func (s *Service) dropUnused(ctx context.Context, keep ...int64) error {
	ids, err := s.store.UnusedHidden(ctx)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if contains(keep, id) {
			continue
		}
		if err := s.foods.Delete(ctx, id); err != nil && !errors.Is(err, domain.ErrInUse) && !errors.Is(err, domain.ErrNotFound) {
			return err
		}
	}
	return nil
}

func contains(ids []int64, id int64) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}

// createVariant stores a copy of recipe cur with the line of fromID replaced
// by to at ratio, archived and in cur's variant group.
func (s *Service) createVariant(ctx context.Context, cur *food.Food, fromID int64, to *food.Food, ratio float64) (*food.Food, error) {
	if cur.VariantGroupID == nil {
		group, err := s.foods.CreateVariantGroup(ctx, cur.Name)
		if err != nil {
			return nil, fmt.Errorf("create variant group: %w", err)
		}
		cur.VariantGroupID = &group
		if err := s.foods.Update(ctx, cur); err != nil {
			return nil, fmt.Errorf("assign recipe to variant group: %w", err)
		}
	}
	v := *cur
	v.ID, v.Archived, v.Favorite = 0, true, false
	v.Children = swapLine(cur.Children, fromID, to, ratio)
	v.Instructions = append([]food.Instruction(nil), cur.Instructions...)
	for i := range v.Instructions {
		v.Instructions[i].ID, v.Instructions[i].FoodID = 0, 0
	}
	v.Tags = append([]string(nil), cur.Tags...)
	if err := s.foods.Create(ctx, &v); err != nil {
		return nil, fmt.Errorf("create hidden variant: %w", err)
	}
	// Create does not write the archive flag.
	if _, err := s.foods.SetArchived(ctx, v.ID, true); err != nil {
		return nil, fmt.Errorf("archive hidden variant: %w", err)
	}
	return &v, nil
}

// swapLine replaces the line of fromID by to at ratio (grams per gram). When
// the recipe already has a line of to, the amounts add up in grams.
func swapLine(children []food.FoodComponent, fromID int64, to *food.Food, ratio float64) []food.FoodComponent {
	out := make([]food.FoodComponent, len(children))
	swapped := -1
	for i, c := range children {
		c.ID, c.ParentID = 0, 0
		if c.ChildID == fromID && swapped < 0 {
			c.ChildID, c.ChildName, c.ChildKind = to.ID, to.Name, food.KindLeaf
			c.Grams *= ratio
			c.Amount, c.Unit, c.GramsSource = c.Grams, "g", ""
			swapped = i
		}
		out[i] = c
	}
	if swapped < 0 {
		return out
	}
	merged := make([]food.FoodComponent, 0, len(out))
	for i, c := range out {
		if i != swapped && c.ChildID == to.ID && !c.ToTaste && !out[swapped].ToTaste {
			out[swapped].Grams += c.Grams
			out[swapped].Amount = out[swapped].Grams
		}
	}
	for i, c := range out {
		if i != swapped && c.ChildID == to.ID && !c.ToTaste && !out[swapped].ToTaste {
			continue
		}
		merged = append(merged, c)
	}
	return merged
}

// line returns the recipe line of leaf fromID in the session's recipe and its
// grams for the session's portions.
func (s *Service) line(ctx context.Context, sessionID, fromID int64) (food.FoodComponent, float64, error) {
	sess, err := s.sessions.GetSession(ctx, sessionID)
	if err != nil {
		return food.FoodComponent{}, 0, err
	}
	r, err := s.foods.Get(ctx, sess.FoodID)
	if err != nil {
		return food.FoodComponent{}, 0, err
	}
	ref := 1.0
	if r.ReferencePortions != nil && *r.ReferencePortions > 0 {
		ref = *r.ReferencePortions
	}
	for _, c := range r.Children {
		if c.ChildID == fromID {
			return c, c.Grams * sess.Portions / ref, nil
		}
	}
	return food.FoodComponent{}, 0, fmt.Errorf("%w: %s is not an ingredient of the session's recipe", domain.ErrInvalidInput, r.Name)
}

func (s *Service) leaf(ctx context.Context, id int64) (*food.Food, error) {
	f, err := s.foods.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if f.Kind != food.KindLeaf {
		return nil, fmt.Errorf("%w: food %d is not an ingredient", domain.ErrInvalidInput, id)
	}
	return f, nil
}

func (s *Service) leaves(ctx context.Context) ([]food.Food, error) {
	var out []food.Food
	for offset := 0; ; offset += leafPageSize {
		page, err := s.foods.List(ctx, food.ListQuery{Kind: food.KindLeaf, Limit: leafPageSize, Offset: offset})
		if err != nil {
			return nil, fmt.Errorf("list leaves: %w", err)
		}
		for _, f := range page.Items {
			if hasNutrition(&f) {
				out = append(out, f)
			}
		}
		if len(page.Items) < leafPageSize {
			return out, nil
		}
	}
}

// catalogueLeaf returns the first leaf that is a name of synonym entry id.
func catalogueLeaf(leaves []food.Food, id string) *food.Food {
	for i := range leaves {
		if food.IsSynonym(id, leaves[i].Name) {
			return &leaves[i]
		}
	}
	return nil
}

// tableRatio is the table's ratio for swapping the food named from for the
// leaf to: by its name, or as the leaf Options added for the row's entry.
func (s *Service) tableRatio(ctx context.Context, from string, to *food.Food) (float64, bool) {
	for _, row := range food.SubstitutesOf(from) {
		if food.IsSynonym(row.To, to.Name) {
			return row.Ratio, true
		}
		if s.origins == nil {
			continue
		}
		if ids, err := s.origins.OriginLeaves(ctx, substituteOrigin(row.To)); err == nil && slices.Contains(ids, to.ID) {
			return row.Ratio, true
		}
	}
	return 0, false
}

func hasNutrition(f *food.Food) bool {
	return f.Kcal100g != nil && f.Protein100g != nil && f.Fat100g != nil && f.Carbs100g != nil
}

// substituteOrigin is the origin of the leaf added for synonym entry id: it
// names the table substitute the leaf stands for whatever its USDA name is.
func substituteOrigin(id string) food.Origin {
	return food.Origin{Kind: food.OriginImport, Ref: "substitute:" + id}
}

// addedLeaf returns the leaf Options added earlier for synonym entry id, nil
// when there is none.
func (s *Service) addedLeaf(ctx context.Context, id string) (*food.Food, error) {
	ids, err := s.origins.OriginLeaves(ctx, substituteOrigin(id))
	if err != nil || len(ids) == 0 {
		return nil, err
	}
	f, err := s.foods.Get(ctx, ids[0])
	if errors.Is(err, domain.ErrNotFound) || (err == nil && !hasNutrition(f)) {
		return nil, nil
	}
	return f, err
}

// provision adds the catalogue leaf of synonym entry id (a table substitute
// the catalogue lacks) through the matching path, as an import does, and
// reuses the one added before. It returns nil when nothing is configured or
// the matching finds no food of its own (a pick that is another catalogue
// food is no substitute).
func (s *Service) provision(ctx context.Context, id string) (*food.Food, error) {
	name := food.SynonymName(id)
	if s.matcher == nil || s.origins == nil || name == "" {
		return nil, nil
	}
	if f, err := s.addedLeaf(ctx, id); f != nil || err != nil {
		return f, err
	}
	res, err := s.matcher.Match(ctx, food.MatchRequest{Lang: "en", Lines: []food.MatchLine{{Name: name, Amount: 100, Unit: "g"}}})
	if err != nil {
		return nil, fmt.Errorf("match substitute %s: %w", name, err)
	}
	if len(res.Lines) != 1 || res.Lines[0].Pick < 0 {
		return nil, nil
	}
	m := res.Lines[0]
	leafID, created, err := food.EnsureLeaf(ctx, s.foods, s.origins, m.Candidates[m.Pick], substituteOrigin(id))
	if err != nil {
		return nil, fmt.Errorf("add substitute %s: %w", name, err)
	}
	f, err := s.leaf(ctx, leafID)
	if err != nil || !hasNutrition(f) || (!created && !food.IsSynonym(id, f.Name)) {
		return nil, err
	}
	return f, nil
}

// fitsHousehold runs the household gates over a recipe of just the
// substitute.
func fitsHousehold(name string, foodID int64, h gates.Household) bool {
	r := gates.Recipe{Name: "substitute", Portions: 1, Ingredients: []gates.Ingredient{{FoodID: foodID, Name: name, Grams: 100, Matched: true}}, Steps: []string{"Mix."}}
	for _, b := range gates.Check(r, h).Blocks {
		if b.Code == "never_item" || b.Code == "allergen" || b.Code == "diet_conflict" {
			return false
		}
	}
	return true
}
