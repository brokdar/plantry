package planview

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/nutrition"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
	"github.com/jaltszeimer/plantry/backend/internal/domain/settings"
	"github.com/jaltszeimer/plantry/backend/internal/domain/slot"
	"github.com/jaltszeimer/plantry/backend/internal/domain/window"
)

// Plan is the GET /api/plan read model: one window with everything the board
// renders.
type Plan struct {
	Window      window.Window
	Today       time.Time
	IsCurrent   bool
	ShoppingDay int
	// Slots are the active slots in sort order.
	Slots []slot.TimeSlot
	// Cells are every date × active slot, date-major in slot order.
	Cells []Cell
	// Sessions cook in the window or are linked to a plate in it, in cook
	// cell order.
	Sessions []plate.SessionView
	// Days are the window's 7 days, per eater.
	Days []Day
	// Targets is nil when the profile has no kcal target.
	Targets *Targets
	Stats   Stats
	// Issues are the checker's H1, H2, H5 and H8 findings over the window
	// (D21); H3/H4 show as session warnings instead.
	Issues []planning.Issue
	// Draft is nil when the window holds no draft rows.
	Draft *Draft
	// RunningRunID is the generation running for the window, nil when none.
	RunningRunID *int64
}

// Draft summarises the window's draft rows (D26). RunID is the newest run
// that wrote one of them; Chooser and Notes are that run's.
type Draft struct {
	Plates   int
	Sessions int
	RunID    *int64
	Chooser  *string
	Notes    *string
}

// Cell is a cell's resolved availability and its plate, if any.
type Cell struct {
	availability.Cell
	Plate *plate.Plate
}

// Day is one day's nutrition for one eater. Complete is false when a food
// eaten that day lacks core macros; Missing names those foods.
type Day struct {
	Date     time.Time
	Macros   nutrition.Macros
	Complete bool
	Missing  []string
}

// Targets are the profile's daily targets in kcal and grams.
type Targets struct {
	Kcal     float64
	ProteinG float64
	FatG     float64
	CarbsG   float64
}

// Stats summarise the window.
type Stats struct {
	Plates int
	// CookDays is the number of distinct cook dates of sessions cooking in
	// the window.
	CookDays int
	// Warnings is the number of session warnings.
	Warnings int
}

// Plan returns the window starting at start, or the anchored window when
// start is nil. A start that is not a shopping day is ErrInvalidPlanStart.
func (s *Service) Plan(ctx context.Context, start *time.Time, today time.Time) (*Plan, error) {
	var p *Plan
	err := s.d.Tx.InTx(ctx, func(ctx context.Context) error {
		var err error
		p, err = s.plan(ctx, start, today)
		return err
	})
	return p, err
}

func (s *Service) plan(ctx context.Context, start *time.Time, today time.Time) (*Plan, error) {
	e, err := s.loadEnv(ctx)
	if err != nil {
		return nil, err
	}
	today = dateOnly(today)
	var w window.Window
	if start != nil {
		if !window.IsStart(*start, e.shoppingDay) {
			return nil, fmt.Errorf("%w: %s", domain.ErrInvalidPlanStart, start.Format(time.DateOnly))
		}
		w = window.Containing(*start, e.shoppingDay)
	} else {
		anchor, err := s.d.Settings.Get(ctx, settings.KeyPlanAnchor)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", settings.KeyPlanAnchor, err)
		}
		w = window.Anchored(today, anchor.Raw, e.shoppingDay)
	}

	plates, err := s.d.Plates.ListByDateRange(ctx, w.Start, w.End)
	if err != nil {
		return nil, fmt.Errorf("list plates: %w", err)
	}
	avail, err := s.d.Availability.Resolve(ctx, w.Start, w.End)
	if err != nil {
		return nil, fmt.Errorf("resolve availability: %w", err)
	}
	sessions, foods, err := s.sessionViews(ctx, w.Start, w.End, e)
	if err != nil {
		return nil, err
	}
	days, err := s.days(ctx, w.Days(), plates)
	if err != nil {
		return nil, err
	}
	prof, err := s.d.Profile.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("get profile: %w", err)
	}
	issues, err := s.issues(ctx, checkInput{w: w, today: today, env: e, cells: avail, plates: plates, sessions: sessions, foods: foods, profile: prof})
	if err != nil {
		return nil, err
	}
	draft, err := s.draft(ctx, plates, sessions)
	if err != nil {
		return nil, err
	}

	byCell := platesByCell(plates)
	cells := make([]Cell, len(avail))
	for i, c := range avail {
		cells[i] = Cell{Cell: c, Plate: byCell[cellID(c.Date, c.SlotID)]}
	}
	stats := Stats{Plates: len(plates)}
	cookDays := map[string]bool{}
	for _, v := range sessions {
		if w.Contains(v.CookDate) {
			cookDays[v.CookDate.Format(time.DateOnly)] = true
		}
		stats.Warnings += len(v.Warnings)
	}
	stats.CookDays = len(cookDays)

	p := &Plan{
		Window: w, Today: today, IsCurrent: w.Contains(today), ShoppingDay: e.shoppingDay,
		Slots: e.active, Cells: cells, Sessions: sessions, Days: days, Targets: targets(prof), Stats: stats,
		Issues: issues, Draft: draft,
	}
	if s.d.Progress != nil {
		if id, ok := s.d.Progress.Running(w.Start); ok {
			p.RunningRunID = &id
		}
	}
	return p, nil
}

// env is what both read models need from settings and slots.
type env struct {
	shoppingDay int
	batch       plate.BatchSettings
	order       plate.SlotOrder
	active      []slot.TimeSlot
}

func (s *Service) loadEnv(ctx context.Context) (env, error) {
	var e env
	var err error
	if e.shoppingDay, err = plate.IntSetting(ctx, s.d.Settings, settings.KeyPlanShoppingDay); err != nil {
		return e, err
	}
	if e.batch, err = plate.ReadBatchSettings(ctx, s.d.Settings); err != nil {
		return e, err
	}
	all, err := s.d.Slots.List(ctx, false)
	if err != nil {
		return e, fmt.Errorf("list slots: %w", err)
	}
	e.order = make(plate.SlotOrder, len(all))
	e.active = []slot.TimeSlot{}
	for _, sl := range all {
		e.order[sl.ID] = sl.SortOrder
		if sl.Active {
			e.active = append(e.active, sl)
		}
	}
	return e, nil
}

// sessionViews loads the sessions touching [from, to] as views in cook cell
// order, plus their foods by id.
func (s *Service) sessionViews(ctx context.Context, from, to time.Time, e env) ([]plate.SessionView, map[int64]*food.Food, error) {
	sessions, links, err := s.d.Sessions.ListInRange(ctx, from, to)
	if err != nil {
		return nil, nil, fmt.Errorf("list sessions: %w", err)
	}
	ids := make([]int64, 0, len(sessions))
	for _, cs := range sessions {
		ids = append(ids, cs.FoodID)
	}
	foods := map[int64]*food.Food{}
	if len(ids) > 0 {
		if foods, err = s.d.Foods.LookupByIDs(ctx, ids); err != nil {
			return nil, nil, fmt.Errorf("lookup session foods: %w", err)
		}
	}
	out := make([]plate.SessionView, 0, len(sessions))
	for _, cs := range sessions {
		f, ok := foods[cs.FoodID]
		if !ok {
			return nil, nil, fmt.Errorf("%w: food %d of session %d", domain.ErrNotFound, cs.FoodID, cs.ID)
		}
		out = append(out, plate.BuildView(cs, f.Name, e.batch.KeepDays(f.KeepDays), links[cs.ID], e.batch, e.order))
	}
	slices.SortStableFunc(out, func(a, b plate.SessionView) int {
		if c := plate.Compare(a.CookCell(), b.CookCell(), e.order); c != 0 {
			return c
		}
		return cmp.Compare(a.ID, b.ID)
	})
	return out, foods, nil
}

// days sums per-eater macros for each date: per-portion macros × portions for
// composed components, per-100 g macros × grams / 100 for leaves. Each food is
// resolved once.
func (s *Service) days(ctx context.Context, dates []time.Time, plates []plate.Plate) ([]Day, error) {
	resolved := map[int64]food.Detailed{}
	byDate := map[string]*Day{}
	missing := map[string]map[string]bool{}
	out := make([]Day, len(dates))
	for i, d := range dates {
		out[i] = Day{Date: d, Complete: true, Missing: []string{}}
		key := d.Format(time.DateOnly)
		byDate[key] = &out[i]
		missing[key] = map[string]bool{}
	}
	for _, p := range plates {
		day, ok := byDate[p.DateString()]
		if !ok {
			continue
		}
		for _, pc := range p.Components {
			det, ok := resolved[pc.FoodID]
			if !ok {
				var err error
				if det, err = s.d.Nutrition.PerPortionDetailed(ctx, pc.FoodID); err != nil {
					return nil, fmt.Errorf("resolve food %d: %w", pc.FoodID, err)
				}
				resolved[pc.FoodID] = det
			}
			day.Macros = addScaled(day.Macros, det.Macros, pc.Multiplier())
			for _, m := range det.Missing {
				missing[p.DateString()][m.Name] = true
			}
		}
	}
	for i := range out {
		names := missing[out[i].Date.Format(time.DateOnly)]
		for n := range names {
			out[i].Missing = append(out[i].Missing, n)
		}
		slices.SortFunc(out[i].Missing, func(a, b string) int {
			if c := strings.Compare(strings.ToLower(a), strings.ToLower(b)); c != 0 {
				return c
			}
			return strings.Compare(a, b)
		})
		out[i].Complete = len(names) == 0
	}
	return out, nil
}

func addScaled(a, m nutrition.Macros, k float64) nutrition.Macros {
	return nutrition.Macros{
		Kcal:    a.Kcal + m.Kcal*k,
		Protein: a.Protein + m.Protein*k,
		Fat:     a.Fat + m.Fat*k,
		Carbs:   a.Carbs + m.Carbs*k,
		Fiber:   a.Fiber + m.Fiber*k,
		Sodium:  a.Sodium + m.Sodium*k,
	}
}

// Energy per gram of each macro, for turning target percentages into grams.
const (
	kcalPerGramProtein = 4
	kcalPerGramFat     = 9
	kcalPerGramCarbs   = 4
)

// targets returns the profile's targets, nil without a kcal target. A missing
// percentage gives 0 g.
func targets(p *profile.Profile) *Targets {
	if p.KcalTarget == nil {
		return nil
	}
	kcal := *p.KcalTarget
	grams := func(pct *float64, perGram float64) float64 {
		if pct == nil {
			return 0
		}
		return kcal * *pct / 100 / perGram
	}
	return &Targets{
		Kcal:     kcal,
		ProteinG: grams(p.ProteinPct, kcalPerGramProtein),
		FatG:     grams(p.FatPct, kcalPerGramFat),
		CarbsG:   grams(p.CarbsPct, kcalPerGramCarbs),
	}
}

type cellKey struct {
	date   string
	slotID int64
}

func cellID(d time.Time, slotID int64) cellKey {
	return cellKey{d.Format(time.DateOnly), slotID}
}

func platesByCell(plates []plate.Plate) map[cellKey]*plate.Plate {
	out := make(map[cellKey]*plate.Plate, len(plates))
	for i := range plates {
		out[cellID(plates[i].Date, plates[i].SlotID)] = &plates[i]
	}
	return out
}

func dateOnly(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}
