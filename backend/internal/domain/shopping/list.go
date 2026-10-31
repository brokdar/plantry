// Package shopping derives the shopping list of a date range from cook
// sessions and leaf plate components (spec §7.7, Phase 2 B6) and sorts it
// into store sections (D18).
package shopping

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/settings"
	"github.com/jaltszeimer/plantry/backend/internal/domain/slot"
	"github.com/jaltszeimer/plantry/backend/internal/domain/units"
	"github.com/jaltszeimer/plantry/backend/internal/domain/window"
)

// maxDepth guards the tree walk against pathological recursion.
const maxDepth = 16

// LeafUse is a leaf component on a plate: its per-eater quantity and the
// plate's cell and eaters.
type LeafUse struct {
	FoodID int64
	Date   time.Time
	SlotID int64
	Eaters float64
	Amount float64
	Unit   string
	Grams  float64
}

// Source loads what a range buys for.
type Source interface {
	// SessionsCooking returns the sessions with cook_date in [from, to].
	SessionsCooking(ctx context.Context, from, to time.Time) ([]plate.CookSession, error)
	// LeafUses returns the leaf components on plates dated in [from, to].
	LeafUses(ctx context.Context, from, to time.Time) ([]LeafUse, error)
}

// List is a shopping list: the non-empty sections in walk order and the
// number of items.
type List struct {
	Sections []Section
	Count    int
}

// Section is one store section's items, sorted by name.
type Section struct {
	Section food.ShopSection
	Items   []Item
}

// Item is one leaf to buy. Display is in the contributions' unit when they
// all share one, otherwise in grams; Buy is set for count units only.
type Item struct {
	FoodID     int64
	Name       string
	TotalGrams float64
	Display    units.Quantity
	Buy        *units.Quantity
	UsedIn     []Use
}

// Use is what an item is bought for: a session (its recipe and cook cell)
// or, with SessionID nil, a leaf component (the leaf and its plate's cell).
type Use struct {
	SessionID *int64
	FoodID    int64
	FoodName  string
	Date      time.Time
	SlotID    int64
}

// Slots lists time slots (their sort order gives the cell order, D12).
type Slots interface {
	List(ctx context.Context, activeOnly bool) ([]slot.TimeSlot, error)
}

// Deps are the service's dependencies.
type Deps struct {
	Source   Source
	Foods    food.TreeReader
	Settings plate.SettingsReader
	Slots    Slots
}

// Service builds shopping lists.
type Service struct{ d Deps }

// NewService creates a shopping list service.
func NewService(d Deps) *Service { return &Service{d: d} }

// Range is a shopping range and the plan window it belongs to.
type Range struct {
	From, To time.Time
	Window   window.Window
}

// Range resolves the range to shop for as of today. With r nil it is the
// default, ShoppingDefault of the anchored window; otherwise r's dates in
// the window containing r.From.
func (s *Service) Range(ctx context.Context, r *Range, today time.Time) (Range, error) {
	sd, err := plate.IntSetting(ctx, s.d.Settings, settings.KeyPlanShoppingDay)
	if err != nil {
		return Range{}, err
	}
	if r != nil {
		return Range{From: r.From, To: r.To, Window: window.Containing(r.From, sd)}, nil
	}
	anchor, err := s.d.Settings.Get(ctx, settings.KeyPlanAnchor)
	if err != nil {
		return Range{}, fmt.Errorf("read %s: %w", settings.KeyPlanAnchor, err)
	}
	w := window.Anchored(today, anchor.Raw, sd)
	from, to := window.ShoppingDefault(today, w)
	return Range{From: from, To: to, Window: w}, nil
}

// acc accumulates one leaf's contributions.
type acc struct {
	leaf     *food.Food
	started  bool
	amount   float64
	unit     string
	mixed    bool
	grams    float64
	uses     []Use
	sessions map[int64]bool
}

// add sums a contribution. Metric mass and volume add up in their base unit
// (g, ml), so 500 ml and 1 l share a unit; any other unit difference makes
// the item mixed (shown in grams).
func (a *acc) add(amount float64, unit string, grams float64) {
	amount, u := units.ToBase(amount, unit)
	if !a.started {
		a.started = true
		a.unit = u
	} else if u != a.unit {
		a.mixed = true
	}
	a.amount += amount
	a.grams += grams
}

// List builds the list for [from, to]: every session cooking in the range,
// scaled by portions / reference_portions through its recipe tree, and every
// leaf component on a plate in the range, × eaters. Sessions cooking before
// the range were bought for already, so their leftovers add nothing. The
// food trees load level by level (food.LoadTrees), never per session.
func (s *Service) List(ctx context.Context, from, to time.Time) (*List, error) {
	sessions, err := s.d.Source.SessionsCooking(ctx, from, to)
	if err != nil {
		return nil, err
	}
	leaves, err := s.d.Source.LeafUses(ctx, from, to)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(sessions)+len(leaves))
	for _, cs := range sessions {
		ids = append(ids, cs.FoodID)
	}
	for _, lu := range leaves {
		ids = append(ids, lu.FoodID)
	}
	nodes, err := food.LoadTrees(ctx, s.d.Foods, ids)
	if err != nil {
		return nil, err
	}

	accs := map[int64]*acc{}
	get := func(leaf *food.Food) *acc {
		a, ok := accs[leaf.ID]
		if !ok {
			a = &acc{leaf: leaf, sessions: map[int64]bool{}}
			accs[leaf.ID] = a
		}
		return a
	}

	for _, cs := range sessions {
		recipe := nodes[cs.FoodID]
		ref := 1.0
		if recipe.ReferencePortions != nil && *recipe.ReferencePortions > 0 {
			ref = *recipe.ReferencePortions
		}
		use := Use{SessionID: &cs.ID, FoodID: recipe.ID, FoodName: recipe.Name, Date: cs.CookDate, SlotID: cs.CookSlotID}
		err := expand(nodes, recipe, cs.Portions/ref, 0, func(leaf *food.Food, amount float64, unit string, grams float64) {
			a := get(leaf)
			a.add(amount, unit, grams)
			if !a.sessions[cs.ID] {
				a.sessions[cs.ID] = true
				a.uses = append(a.uses, use)
			}
		})
		if err != nil {
			return nil, err
		}
	}
	for _, lu := range leaves {
		leaf := nodes[lu.FoodID]
		a := get(leaf)
		a.add(lu.Amount*lu.Eaters, lu.Unit, lu.Grams*lu.Eaters)
		a.uses = append(a.uses, Use{FoodID: leaf.ID, FoodName: leaf.Name, Date: lu.Date, SlotID: lu.SlotID})
	}

	slots, err := s.d.Slots.List(ctx, false)
	if err != nil {
		return nil, fmt.Errorf("list slots: %w", err)
	}
	order := make(plate.SlotOrder, len(slots))
	for _, sl := range slots {
		order[sl.ID] = sl.SortOrder
	}
	return group(accs, order), nil
}

// expand walks composed food f at factor and emits each leaf line. A leaf
// child keeps its own unit, scaled by factor. A composed child contributes
// the share of its batch the parent uses: factor × link grams / the child's
// total child grams.
func expand(nodes map[int64]*food.Food, f *food.Food, factor float64, depth int,
	emit func(leaf *food.Food, amount float64, unit string, grams float64),
) error {
	if depth > maxDepth {
		return fmt.Errorf("%w: food tree deeper than %d", domain.ErrInvalidInput, maxDepth)
	}
	for _, ch := range f.Children {
		child := nodes[ch.ChildID]
		if child.Kind == food.KindLeaf {
			emit(child, ch.Amount*factor, ch.Unit, ch.Grams*factor)
			continue
		}
		total := 0.0
		for _, gc := range child.Children {
			total += gc.Grams
		}
		if total <= 0 {
			continue
		}
		if err := expand(nodes, child, factor*ch.Grams/total, depth+1, emit); err != nil {
			return err
		}
	}
	return nil
}

// group turns the accumulators into sections in walk order, items sorted by
// name, uses in cell order (D12; sessions before leaf components in a cell).
func group(accs map[int64]*acc, order plate.SlotOrder) *List {
	bySection := map[food.ShopSection][]Item{}
	for _, a := range accs {
		it := Item{FoodID: a.leaf.ID, Name: a.leaf.Name, TotalGrams: a.grams, UsedIn: a.uses}
		if a.mixed {
			it.Display = units.Display(a.grams, "g")
		} else {
			it.Display = units.Display(a.amount, a.unit)
			if buy, ok := units.Buy(a.amount, a.unit); ok {
				it.Buy = &buy
			}
		}
		slices.SortStableFunc(it.UsedIn, func(a, b Use) int { return compareUses(a, b, order) })
		sec := SectionOf(a.leaf)
		bySection[sec] = append(bySection[sec], it)
	}
	out := &List{Sections: []Section{}}
	for _, sec := range food.ShopSections {
		items := bySection[sec]
		if len(items) == 0 {
			continue
		}
		slices.SortFunc(items, func(a, b Item) int {
			if c := strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)); c != 0 {
				return c
			}
			return cmp.Compare(a.FoodID, b.FoodID)
		})
		out.Sections = append(out.Sections, Section{Section: sec, Items: items})
		out.Count += len(items)
	}
	return out
}

func compareUses(a, b Use, order plate.SlotOrder) int {
	ca := plate.CellKey{Date: a.Date, SlotID: a.SlotID}
	if c := plate.Compare(ca, plate.CellKey{Date: b.Date, SlotID: b.SlotID}, order); c != 0 {
		return c
	}
	// Sessions first.
	if (a.SessionID == nil) != (b.SessionID == nil) {
		if a.SessionID != nil {
			return -1
		}
		return 1
	}
	return strings.Compare(a.FoodName, b.FoodName)
}
