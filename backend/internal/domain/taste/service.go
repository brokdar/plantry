package taste

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/actor"
	"github.com/jaltszeimer/plantry/backend/internal/domain/flavor"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/foodevent"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// Signal is one taste_signals row: an attribute an event blamed.
type Signal struct {
	Kind    SignalKind
	Ref     Ref
	Weight  float64
	Share   float64
	At      time.Time
	FoodID  *int64
	PlateID *int64
}

// Store is the port the taste tables are read and written through. Calls
// join the unit of work in ctx.
type Store interface {
	AddSignal(ctx context.Context, s Signal) error
	// HasSignal reports whether a signal of kind about food exists.
	HasSignal(ctx context.Context, kind SignalKind, foodID int64) (bool, error)
	Items(ctx context.Context) ([]Item, error)
	// StapleFoods lists the recipes that are staples as of now under rule.
	StapleFoods(ctx context.Context, now time.Time, rule StapleRule) ([]int64, error)
	// Item returns one item, domain.ErrNotFound when missing.
	Item(ctx context.Context, id int64) (Item, error)
	// ItemByRef returns the item of ref in scope and window (zero for an
	// always item); ok false when there is none.
	ItemByRef(ctx context.Context, ref Ref, scope Scope, window time.Time) (Item, bool, error)
	// SaveItem inserts it (ID 0: a new id is set) or writes it under its ID,
	// re-creating a deleted row.
	SaveItem(ctx context.Context, it *Item) error
	DeleteItem(ctx context.Context, id int64) error
}

// Attributes reads a food's effective attributes (flavor.Service).
type Attributes interface {
	Get(ctx context.Context, id int64) (flavor.View, error)
}

// Foods resolves foods by id (with HiddenBaseID set).
type Foods interface {
	LookupByIDs(ctx context.Context, ids []int64) (map[int64]*food.Food, error)
}

// Recorder is the food change log (foodevent.Service).
type Recorder interface {
	Recorded(ctx context.Context, fn func(ctx context.Context) error) (foodevent.Undo, error)
	Record(ctx context.Context, e foodevent.Event) error
}

// History lists recent food events of a kind, newest first.
type History interface {
	Recent(ctx context.Context, kind foodevent.Kind, since time.Time, limit int64) ([]foodevent.Event, error)
}

// Deps are the service's dependencies. Now defaults to time.Now.
type Deps struct {
	Store      Store
	Attributes Attributes
	Foods      Foods
	Events     Recorder
	History    History
	Now        func() time.Time
}

// Service captures signals, keeps the taste items and answers the read API.
type Service struct{ d Deps }

// NewService creates a Service.
func NewService(d Deps) *Service {
	if d.Now == nil {
		d.Now = time.Now
	}
	return &Service{d: d}
}

var _ foodevent.Restorer = (*Service)(nil)

// ── Capture (TASTE-2) ───────────────────────────────────────────────────

// Rating records a plate rating that changed from old to status: loved and
// disliked are evidence on the plate's recipes; not eaten and an unchanged
// rating teach nothing.
func (s *Service) Rating(ctx context.Context, p *plate.Plate, old, status string) error {
	if status == old {
		return nil
	}
	switch status {
	case "loved":
		return s.plateSignal(ctx, SignalLoved, WeightLoved, p, nil)
	case "disliked":
		return s.plateSignal(ctx, SignalDisliked, WeightDisliked, p, nil)
	}
	return nil
}

// PlateChanged is the plate service's hook for each plate event it writes.
// Only the user's own create, update and delete teach (never an assistant's
// or the generator's, never an undo or a restore; the plate service skips
// changes made under plate.WithoutTaste): a plate added by hand is
// evidence for its recipes, a recipe removed from a draft plate (the plate
// deleted, or the recipe replaced) against them.
func (s *Service) PlateChanged(ctx context.Context, action plate.Action, before, after *plate.Plate) error {
	if actor.From(ctx).Kind != actor.User {
		return nil
	}
	switch action {
	case plate.ActionCreate, plate.ActionUpdate, plate.ActionDelete:
	default:
		return nil
	}
	added, removed := recipeDiff(before, after)
	if len(added) > 0 {
		if err := s.plateSignal(ctx, SignalPlateAdded, WeightPlateAdded, after, added); err != nil {
			return err
		}
	}
	if len(removed) > 0 && before.State == plate.StateDraft {
		return s.plateSignal(ctx, SignalPlateRemoved, WeightPlateRemoved, before, removed)
	}
	return nil
}

// Favourite records a favourite star: the first time a recipe is starred is
// evidence for it; unstarring is not a dislike, and starring again adds
// nothing.
func (s *Service) Favourite(ctx context.Context, foodID int64, on bool) error {
	if !on {
		return nil
	}
	seen, err := s.d.Store.HasSignal(ctx, SignalFavourite, foodID)
	if err != nil || seen {
		return err
	}
	return s.recipeSignal(ctx, SignalFavourite, WeightFavourite, foodID)
}

// Tip records an applied or dismissed tip on its flavour dimension; an
// ignored tip teaches nothing.
func (s *Service) Tip(ctx context.Context, dimension string, applied bool) error {
	kind, w := SignalTipDismissed, WeightTipDismissed
	if applied {
		kind, w = SignalTipApplied, WeightTipApplied
	}
	return s.apply(ctx, kind, w, []Blamed{{Ref{LevelFlavour, NormKey(dimension)}, 1}}, nil, nil)
}

// ReviewSwap records a recipe swapped in a review: evidence against the
// removed recipe and for the added one (0: none). Later PRs (C2) call it.
func (s *Service) ReviewSwap(ctx context.Context, removed, added int64) error {
	if removed != 0 {
		if err := s.recipeSignal(ctx, SignalReviewSwap, WeightReviewRemoved, removed); err != nil {
			return err
		}
	}
	if added != 0 {
		return s.recipeSignal(ctx, SignalReviewSwap, WeightReviewAdded, added)
	}
	return nil
}

// Not for me reasons.
const (
	ReasonTaste       = "taste"
	ReasonIngredients = "ingredients"
	ReasonTooLong     = "too_long"
)

// NotForMe records a "Not for me" on a recipe. Taste is evidence against
// the recipe's attributes; ingredients against the named hero ingredients
// (all the recipe's heroes when none is named); too long is time, not
// taste, and teaches nothing here; no reason is weak evidence. Later PRs
// (C2) call it.
func (s *Service) NotForMe(ctx context.Context, foodID int64, reason string, heroes []string) error {
	switch reason {
	case ReasonTooLong:
		return nil
	case ReasonTaste:
		return s.recipeSignal(ctx, SignalNotForMe, WeightNotForMe, foodID)
	case ReasonIngredients:
		if len(heroes) == 0 {
			a, err := s.attributes(ctx, foodID)
			if err != nil {
				return err
			}
			heroes = a.Heroes
		}
		blamed := make([]Blamed, 0, len(heroes))
		for _, h := range heroes {
			blamed = append(blamed, Blamed{Ref{LevelHero, NormKey(h)}, 1 / float64(len(heroes))})
		}
		return s.apply(ctx, SignalNotForMe, WeightNotForMe, blamed, &foodID, nil)
	}
	return s.recipeSignal(ctx, SignalNotForMe, WeightNotForMeOther, foodID)
}

// NotForMeIngredients records a "Not for me" with the ingredients reason
// on a find that never reached the library (C2): it has no attributes yet,
// so only the named ingredients teach.
func (s *Service) NotForMeIngredients(ctx context.Context, heroes []string) error {
	if len(heroes) == 0 {
		return nil
	}
	blamed := make([]Blamed, 0, len(heroes))
	for _, h := range heroes {
		blamed = append(blamed, Blamed{Ref{LevelHero, NormKey(h)}, 1 / float64(len(heroes))})
	}
	return s.apply(ctx, SignalNotForMe, WeightNotForMe, blamed, nil, nil)
}

// recipeDiff returns the recipes on after but not before (added) and on
// before but not after (removed); nil plates hold nothing.
func recipeDiff(before, after *plate.Plate) (added, removed []int64) {
	b, a := recipesOf(before), recipesOf(after)
	for _, id := range a {
		if !slices.Contains(b, id) {
			added = append(added, id)
		}
	}
	for _, id := range b {
		if !slices.Contains(a, id) {
			removed = append(removed, id)
		}
	}
	return added, removed
}

// recipesOf lists the plate's recipes in the main or side position.
func recipesOf(p *plate.Plate) []int64 {
	if p == nil {
		return nil
	}
	var out []int64
	for _, c := range p.Components {
		if c.IsComposed() && c.Position != plate.PositionExtra {
			out = append(out, c.FoodID)
		}
	}
	return out
}

// plateSignal blames a plate signal on the plate's recipes (only those of
// only, when given) by position (TASTE-3), as evidence shares (EvidenceShares).
func (s *Service) plateSignal(ctx context.Context, kind SignalKind, w float64, p *plate.Plate, only []int64) error {
	hasMain, sides := false, 0
	for _, c := range p.Components {
		switch {
		case !c.IsComposed() || c.Position == plate.PositionExtra:
		case c.Position == plate.PositionMain:
			hasMain = true
		default:
			sides++
		}
	}
	mainShare, sideShare := PlateShares(hasMain, sides)
	plateID := p.ID
	for _, c := range p.Components {
		if !c.IsComposed() || c.Position == plate.PositionExtra || (only != nil && !slices.Contains(only, c.FoodID)) {
			continue
		}
		share := sideShare
		if c.Position == plate.PositionMain {
			share = mainShare
		}
		a, err := s.attributes(ctx, c.FoodID)
		if err != nil {
			return err
		}
		foodID := c.FoodID
		if err := s.apply(ctx, kind, w, EvidenceShares(Blame(a, share)), &foodID, &plateID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) recipeSignal(ctx context.Context, kind SignalKind, w float64, foodID int64) error {
	a, err := s.attributes(ctx, foodID)
	if err != nil {
		return err
	}
	return s.apply(ctx, kind, w, EvidenceShares(Blame(a, 1)), &foodID, nil)
}

// attributes reads a recipe's attributes (none for a leaf); a hidden
// substitution variant teaches about its recipe (a recipe and its variants
// are one dish).
func (s *Service) attributes(ctx context.Context, id int64) (flavor.Attributes, error) {
	foods, err := s.d.Foods.LookupByIDs(ctx, []int64{id})
	if err != nil {
		return flavor.Attributes{}, err
	}
	f := foods[id]
	if f == nil {
		return flavor.Attributes{}, fmt.Errorf("%w: food %d", domain.ErrNotFound, id)
	}
	if f.Kind != food.KindComposed {
		return flavor.Attributes{}, nil // a leaf is no dish: nothing to blame
	}
	if f.HiddenBaseID != 0 {
		id = f.HiddenBaseID
	}
	v, err := s.d.Attributes.Get(ctx, id)
	if err != nil {
		return flavor.Attributes{}, fmt.Errorf("attributes of food %d: %w", id, err)
	}
	return v.Values, nil
}

// apply appends one signal per blamed ref and adds it to the ref's always
// item. A learned item whose polarity changes by it records a taste item
// food event (the "Recently learned" list and its Undo).
// It runs in one unit of work (joining the caller's).
func (s *Service) apply(ctx context.Context, kind SignalKind, w float64, blamed []Blamed, foodID, plateID *int64) error {
	now := s.d.Now().UTC().Truncate(time.Second)
	_, err := s.d.Events.Recorded(ctx, func(ctx context.Context) error {
		for _, b := range blamed {
			if b.Share <= 0 {
				continue
			}
			if err := s.d.Store.AddSignal(ctx, Signal{Kind: kind, Ref: b.Ref, Weight: w * b.Share, Share: b.Share, At: now, FoodID: foodID, PlateID: plateID}); err != nil {
				return err
			}
			if err := s.learn(ctx, b.Ref, w*b.Share, b.Share, now); err != nil {
				return err
			}
		}
		return nil
	})
	return err
}

func (s *Service) learn(ctx context.Context, ref Ref, w, share float64, now time.Time) error {
	it, ok, err := s.d.Store.ItemByRef(ctx, ref, ScopeAlways, time.Time{})
	if err != nil {
		return err
	}
	if !ok {
		it = Item{Level: ref.Level, Key: ref.Key, Scope: ScopeAlways, Source: SourceLearned, CreatedAt: now, UpdatedAt: now}
	}
	prev := it
	it.Evidence = it.Evidence.Faded(now.Sub(it.UpdatedAt)).Add(w, share)
	it.UpdatedAt = now
	if err := s.d.Store.SaveItem(ctx, &it); err != nil {
		return err
	}
	if it.Source != SourceLearned || prev.Effective(now) == it.Effective(now) {
		return nil
	}
	e := foodevent.Event{Kind: foodevent.KindTasteItem, SubjectID: it.ID, Action: foodevent.ActionUpdate}
	if !ok {
		e.Action = foodevent.ActionCreate
	} else if e.Before, err = snapshot(prev); err != nil {
		return err
	}
	if e.After, err = snapshot(it); err != nil {
		return err
	}
	return s.d.Events.Record(ctx, e)
}

// ── Statements and edits (TASTE-5) ─────────────────────────────────────

// State records a confirmed statement: the ref's item of scope (in the
// window starting at window for a window item) takes polarity with source
// statement and keeps its evidence. It is a taste item food event.
func (s *Service) State(ctx context.Context, ref Ref, polarity Polarity, scope Scope, window time.Time) (Item, foodevent.Undo, error) {
	if !ref.Level.Valid() || ref.Key == "" || !polarity.Valid() || (scope != ScopeAlways && scope != ScopeWindow) || (scope == ScopeWindow && window.IsZero()) {
		return Item{}, foodevent.Undo{}, fmt.Errorf("%w: taste statement", domain.ErrInvalidInput)
	}
	if scope == ScopeAlways {
		window = time.Time{}
	}
	var it Item
	undo, err := s.d.Events.Recorded(ctx, func(ctx context.Context) error {
		cur, ok, err := s.d.Store.ItemByRef(ctx, ref, scope, window)
		if err != nil {
			return err
		}
		now := s.d.Now().UTC().Truncate(time.Second)
		if !ok {
			cur = Item{Level: ref.Level, Key: ref.Key, Scope: scope, WindowStart: window, CreatedAt: now, UpdatedAt: now}
		}
		it = cur
		it.Evidence = cur.Evidence.Faded(now.Sub(cur.UpdatedAt))
		it.UpdatedAt = now
		it.Source, it.Polarity = SourceStatement, polarity
		return s.write(ctx, cur, &it, ok)
	})
	return it, undo, err
}

// ConfirmStatement is the user's tap on a statement chip (TASTE-8): scope
// always makes the statement permanent (and takes the window item the chat
// applied for the window instead); scope window keeps it for the window
// starting at window. The Undo covers both.
func (s *Service) ConfirmStatement(ctx context.Context, ref Ref, polarity Polarity, scope Scope, window time.Time) (Item, foodevent.Undo, error) {
	var it Item
	undo, err := s.d.Events.Recorded(ctx, func(ctx context.Context) error {
		if scope == ScopeAlways && !window.IsZero() {
			cur, ok, err := s.d.Store.ItemByRef(ctx, ref, ScopeWindow, window)
			if err != nil {
				return err
			}
			if ok {
				if err := s.d.Store.DeleteItem(ctx, cur.ID); err != nil {
					return err
				}
				before, err := snapshot(cur)
				if err != nil {
					return err
				}
				if err := s.d.Events.Record(ctx, foodevent.Event{Kind: foodevent.KindTasteItem, SubjectID: cur.ID, Action: foodevent.ActionDelete, Before: before}); err != nil {
					return err
				}
			}
		}
		var err error
		it, _, err = s.State(ctx, ref, polarity, scope, window)
		return err
	})
	return it, undo, err
}

// SetPolarity sets an item's polarity by the user (flip): it outranks
// learned evidence like a statement does.
func (s *Service) SetPolarity(ctx context.Context, id int64, polarity Polarity) (ItemView, foodevent.Undo, error) {
	if !polarity.Valid() {
		return ItemView{}, foodevent.Undo{}, fmt.Errorf("%w: polarity %q", domain.ErrInvalidInput, polarity)
	}
	var it Item
	undo, err := s.d.Events.Recorded(ctx, func(ctx context.Context) error {
		cur, err := s.d.Store.Item(ctx, id)
		if err != nil {
			return err
		}
		now := s.d.Now().UTC().Truncate(time.Second)
		it = cur
		it.Evidence = cur.Evidence.Faded(now.Sub(cur.UpdatedAt))
		it.UpdatedAt = now
		it.Source, it.Polarity = SourceUser, polarity
		return s.write(ctx, cur, &it, true)
	})
	if err != nil {
		return ItemView{}, foodevent.Undo{}, err
	}
	return viewOf(it, s.d.Now()), undo, nil
}

// Remove deletes an item with its evidence; the Undo puts it back.
func (s *Service) Remove(ctx context.Context, id int64) (foodevent.Undo, error) {
	return s.d.Events.Recorded(ctx, func(ctx context.Context) error {
		cur, err := s.d.Store.Item(ctx, id)
		if err != nil {
			return err
		}
		if err := s.d.Store.DeleteItem(ctx, id); err != nil {
			return err
		}
		before, err := snapshot(cur)
		if err != nil {
			return err
		}
		return s.d.Events.Record(ctx, foodevent.Event{Kind: foodevent.KindTasteItem, SubjectID: id, Action: foodevent.ActionDelete, Before: before})
	})
}

// write saves next over cur (existed: cur is a stored row) and records it.
func (s *Service) write(ctx context.Context, cur Item, next *Item, existed bool) error {
	if err := s.d.Store.SaveItem(ctx, next); err != nil {
		return err
	}
	e := foodevent.Event{Kind: foodevent.KindTasteItem, SubjectID: next.ID, Action: foodevent.ActionCreate}
	var err error
	if existed {
		e.Action = foodevent.ActionUpdate
		if e.Before, err = snapshot(cur); err != nil {
			return err
		}
	}
	if e.After, err = snapshot(*next); err != nil {
		return err
	}
	return s.d.Events.Record(ctx, e)
}

// ── Undo (foodevent.Restorer) ───────────────────────────────────────────

// itemJSON is a taste item's food event snapshot.
type itemJSON struct {
	ID          int64    `json:"id"`
	Level       Level    `json:"level"`
	Key         string   `json:"key"`
	Scope       Scope    `json:"scope"`
	WindowStart string   `json:"window_start,omitempty"`
	Source      Source   `json:"source"`
	Polarity    Polarity `json:"polarity,omitempty"`
	Evidence    Evidence `json:"evidence"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
}

func snapshot(it Item) (json.RawMessage, error) {
	j := itemJSON{
		ID: it.ID, Level: it.Level, Key: it.Key, Scope: it.Scope, Source: it.Source, Polarity: it.Polarity,
		Evidence: it.Evidence, CreatedAt: it.CreatedAt.Format(time.RFC3339), UpdatedAt: it.UpdatedAt.Format(time.RFC3339),
	}
	if !it.WindowStart.IsZero() {
		j.WindowStart = it.WindowStart.Format(time.DateOnly)
	}
	return json.Marshal(j)
}

func fromSnapshot(raw json.RawMessage) (Item, error) {
	var j itemJSON
	if err := json.Unmarshal(raw, &j); err != nil {
		return Item{}, fmt.Errorf("taste item snapshot: %w", err)
	}
	it := Item{ID: j.ID, Level: j.Level, Key: j.Key, Scope: j.Scope, Source: j.Source, Polarity: j.Polarity, Evidence: j.Evidence}
	var errs []error
	var err error
	it.CreatedAt, err = time.Parse(time.RFC3339, j.CreatedAt)
	errs = append(errs, err)
	it.UpdatedAt, err = time.Parse(time.RFC3339, j.UpdatedAt)
	errs = append(errs, err)
	if j.WindowStart != "" {
		it.WindowStart, err = time.Parse(time.DateOnly, j.WindowStart)
		errs = append(errs, err)
	}
	if err := errors.Join(errs...); err != nil {
		return Item{}, fmt.Errorf("taste item snapshot: %w", err)
	}
	return it, nil
}

// Restore puts item id back as before (nil: the change created it, so it
// goes). A row of the same ref created since gives way to the restored one.
func (s *Service) Restore(ctx context.Context, id int64, before json.RawMessage) error {
	if before == nil {
		return s.d.Store.DeleteItem(ctx, id)
	}
	it, err := fromSnapshot(before)
	if err != nil {
		return err
	}
	it.ID = id
	if other, ok, err := s.d.Store.ItemByRef(ctx, Ref{it.Level, it.Key}, it.Scope, it.WindowStart); err != nil {
		return err
	} else if ok && other.ID != id {
		if err := s.d.Store.DeleteItem(ctx, other.ID); err != nil {
			return err
		}
	}
	return s.d.Store.SaveItem(ctx, &it)
}

// ── Read API ────────────────────────────────────────────────────────────

// ItemView is an item as the API shows it: its polarity and score now.
type ItemView struct {
	ID          int64    `json:"id"`
	Level       Level    `json:"level"`
	Key         string   `json:"key"`
	Scope       Scope    `json:"scope"`
	WindowStart string   `json:"window_start,omitempty"`
	Source      Source   `json:"source"`
	Polarity    Polarity `json:"polarity"`
	Score       float64  `json:"score"`
}

// Learned is a "Recently learned" entry: an item the evidence just moved,
// with the Undo of that change (POST /api/food-events/revert).
type Learned struct {
	Item ItemView       `json:"item"`
	At   time.Time      `json:"at"`
	Undo foodevent.Undo `json:"undo"`
}

// View is the read model of GET /api/taste.
type View struct {
	Items  []ItemView `json:"items"`
	Recent []Learned  `json:"recent"`
}

// RecentDays is how far back "Recently learned" looks; RecentMax bounds it.
const (
	RecentDays = 14
	RecentMax  = 20
)

// View returns the items that lean one way and hold in the window starting
// at window (zero: always items only), strongest first, and the recently
// learned ones that are still as learned.
func (s *Service) View(ctx context.Context, window time.Time) (View, error) {
	now := s.d.Now()
	items, err := s.d.Store.Items(ctx)
	if err != nil {
		return View{}, err
	}
	out := View{Items: []ItemView{}, Recent: []Learned{}}
	byID := map[int64]Item{}
	for _, it := range items {
		byID[it.ID] = it
		if it.Active(window) && it.Effective(now) != "" {
			out.Items = append(out.Items, viewOf(it, now))
		}
	}
	slices.SortStableFunc(out.Items, func(a, b ItemView) int {
		switch {
		case math.Abs(a.Score) > math.Abs(b.Score):
			return -1
		case math.Abs(a.Score) < math.Abs(b.Score):
			return 1
		}
		return 0
	})
	events, err := s.d.History.Recent(ctx, foodevent.KindTasteItem, now.AddDate(0, 0, -RecentDays), RecentMax*4)
	if err != nil {
		return View{}, err
	}
	seen := map[int64]bool{}
	for _, e := range events { // newest first: only a subject's newest event counts
		if seen[e.SubjectID] {
			continue
		}
		seen[e.SubjectID] = true
		it, ok := byID[e.SubjectID]
		if !ok || e.Action == foodevent.ActionUndo || it.Source != SourceLearned || it.Effective(now) == "" {
			continue
		}
		out.Recent = append(out.Recent, Learned{Item: viewOf(it, now), At: e.At, Undo: foodevent.Undo{FirstEventID: e.ID, LastEventID: e.ID}})
		if len(out.Recent) == RecentMax {
			break
		}
	}
	return out, nil
}

// Item returns one item, domain.ErrNotFound when missing.
func (s *Service) Item(ctx context.Context, id int64) (Item, error) { return s.d.Store.Item(ctx, id) }

// Staples are the recipes that count as staples as of now (THEME-1).
func (s *Service) Staples(ctx context.Context, now time.Time) (map[int64]bool, error) {
	ids, err := s.d.Store.StapleFoods(ctx, now, Staples)
	if err != nil {
		return nil, err
	}
	out := make(map[int64]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out, nil
}

// Staple is a recipe on the Taste page.
type Staple struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// StapleList lists the staples by name.
func (s *Service) StapleList(ctx context.Context) ([]Staple, error) {
	set, err := s.Staples(ctx, s.d.Now())
	if err != nil {
		return nil, err
	}
	ids := slices.Sorted(maps.Keys(set))
	out := make([]Staple, 0, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	foods, err := s.d.Foods.LookupByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		if f := foods[id]; f != nil {
			out = append(out, Staple{ID: id, Name: f.Name})
		}
	}
	slices.SortFunc(out, func(a, b Staple) int { return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)) })
	return out, nil
}

// Profile returns the planner's taste profile at now for the window
// starting at window.
func (s *Service) Profile(ctx context.Context, now, window time.Time) (Profile, error) {
	items, err := s.d.Store.Items(ctx)
	if err != nil {
		return Profile{}, err
	}
	return NewProfile(items, now, window), nil
}

func viewOf(it Item, now time.Time) ItemView {
	v := ItemView{ID: it.ID, Level: it.Level, Key: it.Key, Scope: it.Scope, Source: it.Source, Polarity: it.Effective(now), Score: math.Round(it.Score(now)*100) / 100}
	if !it.WindowStart.IsZero() {
		v.WindowStart = it.WindowStart.Format(time.DateOnly)
	}
	return v
}
