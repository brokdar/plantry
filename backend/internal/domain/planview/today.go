package planview

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/setup"
	"github.com/jaltszeimer/plantry/backend/internal/domain/window"
)

// Verdict kinds (§3.2).
const (
	VerdictCook          = "cook"
	VerdictQuickCook     = "quick_cook"
	VerdictNothingToCook = "nothing_to_cook"
	VerdictNoPlates      = "no_plates"
	VerdictDraftOnly     = "draft_only"
	VerdictNoPlan        = "no_plan"
)

// Reminder kinds.
const (
	ReminderDefrost = "defrost"
	ReminderUseBy   = "use_by"
)

// QuickCookMinutes is the most prep + cook minutes a quick cook takes.
const QuickCookMinutes = 20

// rateLookbackDays is how many days before the date the rate question looks.
const rateLookbackDays = 3

// Today is the GET /api/today read model for one date.
type Today struct {
	Date        time.Time
	WindowStart time.Time
	Verdict     Verdict
	// Cook lists the sessions cooking on the date, in cell order.
	Cook []Cook
	// Meals has one entry per active slot of the date, in slot order.
	Meals      []Meal
	Reminders  []Reminder
	Rate       *Rate
	PlanBanner PlanBanner
	Setup      Setup
}

// Verdict is the day's headline. SessionID, FoodName, Portions and SlotID are
// set for cook and quick_cook only.
type Verdict struct {
	Kind      string
	SessionID int64
	FoodName  string
	Portions  float64
	SlotID    int64
}

// Cook is a session cooking on the date and its prep + cook minutes (nil when
// the recipe has neither).
type Cook struct {
	Session plate.SessionView
	Minutes *int
}

// Meal is one slot of the date: its availability, its plate and the session
// link of the plate's primary composed component.
type Meal struct {
	SlotID     int64
	Mode       availability.Mode
	PresetName *string
	Note       *string
	Plate      *plate.Plate
	Link       *plate.Link
}

// Reminder is a defrost (a plate tomorrow eats frozen portions) or use-by (a
// leftover eaten on its last good day) note.
type Reminder struct {
	Kind      string
	SessionID int64
	FoodName  string
	Date      time.Time
	SlotID    int64
}

// Rate is the plate the rate question asks about.
type Rate struct {
	PlateID  int64
	Date     time.Time
	SlotID   int64
	FoodName string
}

// PlanBanner says whether "Plan next week" is due and which window it plans.
type PlanBanner struct {
	Due       bool
	NextStart time.Time
}

// Setup is the recipe-library progress.
type Setup struct {
	RecipeCount  int
	RecipeTarget int
}

// Today builds the Today read model for date.
func (s *Service) Today(ctx context.Context, date time.Time) (*Today, error) {
	var t *Today
	err := s.d.Tx.InTx(ctx, func(ctx context.Context) error {
		var err error
		t, err = s.today(ctx, date)
		return err
	})
	return t, err
}

func (s *Service) today(ctx context.Context, date time.Time) (*Today, error) {
	e, err := s.loadEnv(ctx)
	if err != nil {
		return nil, err
	}
	date = dateOnly(date)
	w := window.Containing(date, e.shoppingDay)
	// The banner plans the window starting on the first shopping day on or
	// after the date (the shopping day itself included).
	next := window.Anchored(date, window.AnchorNextShoppingDay, e.shoppingDay)

	// One range covers the window, the rate lookback and the next window.
	from := w.Start
	if lb := date.AddDate(0, 0, -rateLookbackDays); lb.Before(from) {
		from = lb
	}
	to := next.End
	if w.End.After(to) {
		to = w.End
	}
	all, err := s.d.Plates.ListByDateRange(ctx, from, to)
	if err != nil {
		return nil, fmt.Errorf("list plates: %w", err)
	}
	tomorrow := date.AddDate(0, 0, 1)
	allSessions, foods, err := s.sessionViews(ctx, date, tomorrow, e)
	if err != nil {
		return nil, err
	}
	// Today reads confirmed rows only (D26); drafts only decide draft_only
	// and whether the next window is planned already.
	sessions := slices.DeleteFunc(allSessions, func(v plate.SessionView) bool { return v.State == plate.StateDraft })
	plates := make([]plate.Plate, 0, len(all))
	var drafts, inNext int
	for _, p := range all {
		if next.Contains(p.Date) {
			inNext++
		}
		if p.State == plate.StateDraft {
			if w.Contains(p.Date) {
				drafts++
			}
			continue
		}
		plates = append(plates, p)
	}
	avail, err := s.d.Availability.Resolve(ctx, date, date)
	if err != nil {
		return nil, fmt.Errorf("resolve availability: %w", err)
	}
	recipes, err := s.d.Recipes.CountComposedFoods(ctx)
	if err != nil {
		return nil, fmt.Errorf("count recipes: %w", err)
	}

	out := &Today{
		Date: date, WindowStart: w.Start,
		Cook: []Cook{}, Meals: []Meal{}, Reminders: []Reminder{},
		PlanBanner: PlanBanner{NextStart: next.Start},
		Setup:      Setup{RecipeCount: int(recipes), RecipeTarget: setup.RecipeTarget},
	}

	var inWindow, onDate int
	for _, p := range plates {
		if p.Date.Equal(date) {
			onDate++
		}
		if w.Contains(p.Date) {
			inWindow++
		}
	}
	out.PlanBanner.Due = !date.Before(next.Start.AddDate(0, 0, -1)) && inNext == 0

	for _, v := range sessions {
		if v.CookDate.Equal(date) {
			out.Cook = append(out.Cook, Cook{Session: v, Minutes: minutes(foods[v.FoodID])})
		}
	}
	out.Verdict = verdict(inWindow, onDate, drafts, out.Cook, foods)

	links := linksByComponent(sessions)
	byCell := platesByCell(plates)
	for _, c := range avail {
		m := Meal{SlotID: c.SlotID, Mode: c.Mode, PresetName: c.PresetName, Note: c.Note, Plate: byCell[cellID(c.Date, c.SlotID)]}
		if m.Plate != nil {
			if pc := primaryComposed(m.Plate); pc != nil {
				if l, ok := links[pc.ID]; ok {
					m.Link = &l
				}
			}
		}
		out.Meals = append(out.Meals, m)
	}

	out.Reminders = reminders(sessions, date, tomorrow, e.order)
	out.Rate = rate(plates, date, e.order)
	return out, nil
}

// verdict applies the §3.2 rules in order over confirmed rows: no confirmed
// plate on the date while the window has drafts, no plate in the window, no
// plate on the date, a session still to cook (the main with the most
// portions), otherwise nothing to cook. Sessions already marked cooked never
// head it; zero recorded minutes count as unknown, never quick.
func verdict(inWindow, onDate, drafts int, cook []Cook, foods map[int64]*food.Food) Verdict {
	switch {
	case onDate == 0 && drafts > 0:
		return Verdict{Kind: VerdictDraftOnly}
	case inWindow == 0:
		return Verdict{Kind: VerdictNoPlan}
	case onDate == 0:
		return Verdict{Kind: VerdictNoPlates}
	}
	var best *Cook
	for i := range cook {
		c := &cook[i]
		if c.Session.CookedAt != nil {
			continue
		}
		if best == nil || better(*c, *best, foods) {
			best = c
		}
	}
	if best == nil {
		return Verdict{Kind: VerdictNothingToCook}
	}
	kind := VerdictCook
	if best.Minutes != nil && *best.Minutes > 0 && *best.Minutes <= QuickCookMinutes {
		kind = VerdictQuickCook
	}
	v := best.Session
	return Verdict{Kind: kind, SessionID: v.ID, FoodName: v.FoodName, Portions: v.Portions, SlotID: v.CookSlotID}
}

// better reports whether a beats b for the verdict: role main first, then more
// portions. Ties keep the earlier cell.
func better(a, b Cook, foods map[int64]*food.Food) bool {
	am, bm := isMain(foods[a.Session.FoodID]), isMain(foods[b.Session.FoodID])
	if am != bm {
		return am
	}
	return a.Session.Portions > b.Session.Portions
}

func isMain(f *food.Food) bool { return f != nil && f.Role != nil && *f.Role == food.RoleMain }

// minutes is prep + cook minutes, nil when the food has neither.
func minutes(f *food.Food) *int {
	if f == nil || (f.PrepMinutes == nil && f.CookMinutes == nil) {
		return nil
	}
	n := 0
	if f.PrepMinutes != nil {
		n += *f.PrepMinutes
	}
	if f.CookMinutes != nil {
		n += *f.CookMinutes
	}
	return &n
}

func linksByComponent(sessions []plate.SessionView) map[int64]plate.Link {
	out := map[int64]plate.Link{}
	for _, v := range sessions {
		for _, l := range v.Links {
			out[l.ComponentID] = l
		}
	}
	return out
}

// primaryComposed is the plate's first composed component with role main, else
// its first composed component, else nil. Components are in sort order.
func primaryComposed(p *plate.Plate) *plate.PlateComponent {
	var first *plate.PlateComponent
	for i := range p.Components {
		pc := &p.Components[i]
		if !pc.IsComposed() {
			continue
		}
		if pc.FoodRole != nil && *pc.FoodRole == string(food.RoleMain) {
			return pc
		}
		if first == nil {
			first = pc
		}
	}
	return first
}

// reminders lists use-by notes for leftovers eaten on date on their use-by
// day and defrost notes for frozen links tomorrow, in cell order.
func reminders(sessions []plate.SessionView, date, tomorrow time.Time, order plate.SlotOrder) []Reminder {
	out := []Reminder{}
	for _, v := range sessions {
		for _, l := range v.Links {
			r := Reminder{SessionID: v.ID, FoodName: v.FoodName, Date: l.Cell.Date, SlotID: l.Cell.SlotID}
			switch {
			case l.Kind == plate.KindLeftover && l.Cell.Date.Equal(date) && v.UseBy.Equal(date):
				r.Kind = ReminderUseBy
			case l.Kind == plate.KindFrozen && l.Cell.Date.Equal(tomorrow):
				r.Kind = ReminderDefrost
			default:
				continue
			}
			out = append(out, r)
		}
	}
	slices.SortStableFunc(out, func(a, b Reminder) int {
		ca := plate.CellKey{Date: a.Date, SlotID: a.SlotID}
		cb := plate.CellKey{Date: b.Date, SlotID: b.SlotID}
		if c := plate.Compare(ca, cb, order); c != 0 {
			return c
		}
		return cmp.Compare(a.SessionID, b.SessionID)
	})
	return out
}

// rate picks the latest plate in [date−3, date−1] that holds a composed
// component and has no feedback.
func rate(plates []plate.Plate, date time.Time, order plate.SlotOrder) *Rate {
	from := date.AddDate(0, 0, -rateLookbackDays)
	var best *plate.Plate
	var bestFood string
	for i := range plates {
		p := &plates[i]
		if p.Date.Before(from) || !p.Date.Before(date) || p.Feedback != nil {
			continue
		}
		pc := primaryComposed(p)
		if pc == nil {
			continue
		}
		if best == nil || plate.Before(best.Cell(), p.Cell(), order) {
			best, bestFood = p, pc.FoodName
		}
	}
	if best == nil {
		return nil
	}
	return &Rate{PlateID: best.ID, Date: best.Date, SlotID: best.SlotID, FoodName: bestFood}
}
