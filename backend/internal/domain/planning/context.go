package planning

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/preset"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
	"github.com/jaltszeimer/plantry/backend/internal/domain/settings"
	"github.com/jaltszeimer/plantry/backend/internal/domain/window"
)

// Context builder (D18): everything a generation reads, with a bounded
// number of queries however large the library or the plan.

// historyWeeks is how far back the history and the 8-week eating counts
// reach.
const (
	historyDays  = 4 * 7
	timesEatenIn = 8 * 7
)

// maxFoods caps the candidate listing.
const maxFoods = 10000

// BuilderDeps are the context builder's read ports.
type BuilderDeps struct {
	Foods     Foods
	FoodRows  FoodRows
	Nutrition Nutrition
	Cells     Cells
	Plates    PlateRows
	Sessions  SessionRows
	Presets   Presets
	Slots     Slots
	Settings  SettingsReader
	Profile   ProfileReader
}

// Builder reads the State of a window.
type Builder struct{ d BuilderDeps }

// NewBuilder returns a context builder.
func NewBuilder(d BuilderDeps) *Builder { return &Builder{d: d} }

// Build reads the state of window w as of today. Its cells span the window
// widened to [from, to] (closure plates outside it), its plates and sessions
// that range plus the keep-days lookback on both sides, its foods every
// recipe plus every food a plate references, and its history the confirmed
// plates of the 4 weeks before the window. Reads: settings, profile, slots,
// availability, one plate range, one session range, the recipe list with
// its history, one header lookup and one nutrition batch.
func (b *Builder) Build(ctx context.Context, w window.Window, today, from, to time.Time) (State, error) {
	s := State{Window: w, Today: today}
	var err error
	if s.Settings, err = b.settings(ctx); err != nil {
		return State{}, err
	}
	prof, err := b.d.Profile.Get(ctx)
	if err != nil {
		return State{}, fmt.Errorf("read profile: %w", err)
	}
	s.Profile = Profile{
		KcalTarget: prof.KcalTarget, ProteinPct: prof.ProteinPct, FatPct: prof.FatPct, CarbsPct: prof.CarbsPct,
		DietType: prof.DietType, MeatDaysPerWeekMax: prof.MeatDaysPerWeekMax, HouseholdPortions: prof.HouseholdPortions,
		Locale: prof.Locale, Restrictions: prof.DietaryRestrictions,
	}
	if prof.SystemPrompt != nil {
		s.Profile.Notes = *prof.SystemPrompt
	}
	slots, err := b.d.Slots.List(ctx, false)
	if err != nil {
		return State{}, fmt.Errorf("list slots: %w", err)
	}
	s.SlotOrder = make(plate.SlotOrder, len(slots))
	s.SlotNames = make(map[int64]string, len(slots))
	for _, sl := range slots {
		s.SlotOrder[sl.ID] = sl.SortOrder
		s.SlotNames[sl.ID] = sl.NameKey
	}

	recipes, err := b.d.Foods.List(ctx, food.ListQuery{Kind: food.KindComposed, Limit: maxFoods, Today: today})
	if err != nil {
		return State{}, fmt.Errorf("list recipes: %w", err)
	}
	foods := make(map[int64]*food.Food, len(recipes.Items))
	ptrs := make([]*food.Food, len(recipes.Items))
	lookback := s.Settings.DefaultKeepDays
	for i := range recipes.Items {
		f := &recipes.Items[i]
		foods[f.ID], ptrs[i] = f, f
		if f.KeepDays != nil {
			lookback = max(lookback, *f.KeepDays)
		}
	}
	if s.Settings.FreezingEnabled {
		lookback = max(lookback, s.Settings.FreezerKeepDays)
	}
	if err := b.d.Foods.Derive(ctx, today, ptrs...); err != nil {
		return State{}, fmt.Errorf("food history: %w", err)
	}

	from, to = minDate(from, w.Start), maxDate(to, w.End)
	cells, err := b.d.Cells.Resolve(ctx, from, to)
	if err != nil {
		return State{}, fmt.Errorf("resolve availability: %w", err)
	}
	s.Cells = make([]Cell, len(cells))
	for i, c := range cells {
		s.Cells[i] = Cell{Date: c.Date, SlotID: c.SlotID, Mode: c.Mode, PresetID: c.PresetID}
	}

	lo, hi := from.AddDate(0, 0, -lookback), to.AddDate(0, 0, lookback)
	histFrom := minDate(w.Start, today).AddDate(0, 0, -timesEatenIn)
	plates, err := b.d.Plates.ListByDateRange(ctx, minDate(lo, histFrom), hi)
	if err != nil {
		return State{}, fmt.Errorf("list plates: %w", err)
	}
	sessions, _, err := b.d.Sessions.ListInRange(ctx, lo, hi)
	if err != nil {
		return State{}, fmt.Errorf("list sessions: %w", err)
	}
	referenced := map[int64]bool{}
	timesEaten := map[int64]int{}
	eatenSince := today.AddDate(0, 0, -timesEatenIn)
	for _, p := range plates {
		confirmed := p.State == plate.StateConfirmed
		if confirmed && p.Date.Before(w.Start) && !p.Date.Before(w.Start.AddDate(0, 0, -historyDays)) {
			s.History = append(s.History, historyPlate(p))
		}
		if confirmed && p.Date.Before(today) && !p.Date.Before(eatenSince) {
			for _, id := range distinctFoods(p) {
				timesEaten[id]++
			}
		}
		if p.Date.Before(lo) {
			continue
		}
		s.Plates = append(s.Plates, statePlate(p))
		for _, c := range p.Components {
			referenced[c.FoodID] = true
		}
	}
	s.Sessions = make([]Session, len(sessions))
	for i, ss := range sessions {
		s.Sessions[i] = Session{CookSession: ss, Confirmed: ss.State == plate.StateConfirmed}
		referenced[ss.FoodID] = true
	}
	s.Foods = map[int64]FoodFacts{}
	if err := b.addFoods(ctx, &s, foods, referenced, timesEaten, prof); err != nil {
		return State{}, err
	}
	return s, nil
}

// addFoods sets the facts of foods plus the foods of ids not among them.
func (b *Builder) addFoods(ctx context.Context, s *State, foods map[int64]*food.Food, ids map[int64]bool, timesEaten map[int64]int, prof *profile.Profile) error {
	var missing []int64
	for id := range ids {
		if _, ok := foods[id]; !ok {
			missing = append(missing, id)
		}
	}
	if len(missing) > 0 {
		slices.Sort(missing)
		found, err := b.d.FoodRows.LookupByIDs(ctx, missing)
		if err != nil {
			return fmt.Errorf("lookup foods: %w", err)
		}
		ptrs := make([]*food.Food, 0, len(found))
		for id, f := range found {
			foods[id] = f
			ptrs = append(ptrs, f)
		}
		if err := b.d.Foods.DeriveDiet(ctx, ptrs...); err != nil {
			return fmt.Errorf("food diet: %w", err)
		}
	}
	all := make([]int64, 0, len(foods))
	for id := range foods {
		all = append(all, id)
	}
	slices.Sort(all)
	macros, err := b.d.Nutrition.PerPortionDetailedMany(ctx, all)
	if err != nil {
		return fmt.Errorf("nutrition: %w", err)
	}
	likes := stringSet(prof.Preferences[profile.PreferenceKeyLikes])
	dislikes := stringSet(prof.Preferences[profile.PreferenceKeyDislikes])
	for _, id := range all {
		s.Foods[id] = facts(foods[id], macros[id], timesEaten[id], likes, dislikes)
	}
	return nil
}

// facts maps a food to what the planner reads about it.
func facts(f *food.Food, d food.Detailed, timesEaten int, likes, dislikes map[string]bool) FoodFacts {
	ff := FoodFacts{
		ID: f.ID, Name: f.Name, Kind: f.Kind, Macros: d.Macros, NutritionComplete: d.Complete,
		KeepDays: f.KeepDays, Freezable: f.Freezable != nil && *f.Freezable, Favorite: f.Favorite,
		LastEaten: f.History.LastEaten, TimesEaten8Weeks: timesEaten, ReferencePortions: 1, DietClass: f.DietClass,
	}
	if f.Role != nil {
		ff.Role = *f.Role
	}
	if f.ReferencePortions != nil && *f.ReferencePortions > 0 {
		ff.ReferencePortions = *f.ReferencePortions
	}
	if f.PrepMinutes != nil {
		ff.Minutes += *f.PrepMinutes
	}
	if f.CookMinutes != nil {
		ff.Minutes += *f.CookMinutes
	}
	for _, t := range f.Tags {
		if likes[t] {
			ff.LikeScore++
		}
		if dislikes[t] {
			ff.LikeScore--
		}
	}
	return ff
}

func stringSet(v any) map[string]bool {
	out := map[string]bool{}
	switch s := v.(type) {
	case []string:
		for _, t := range s {
			out[t] = true
		}
	case []any:
		for _, e := range s {
			if t, ok := e.(string); ok {
				out[t] = true
			}
		}
	}
	return out
}

func statePlate(p plate.Plate) Plate {
	out := Plate{
		ID: p.ID, Date: p.Date, SlotID: p.SlotID, Eaters: p.Eaters, Confirmed: p.State == plate.StateConfirmed,
		Components: make([]Component, len(p.Components)),
	}
	for i, c := range p.Components {
		out.Components[i] = Component{FoodID: c.FoodID, Portions: c.Portions, Grams: c.Grams}
		if c.CookSessionID != nil {
			out.Components[i].SessionID = *c.CookSessionID
		}
	}
	return out
}

func historyPlate(p plate.Plate) HistoryPlate {
	return HistoryPlate{Date: p.Date, SlotID: p.SlotID, FoodIDs: distinctFoods(p)}
}

func distinctFoods(p plate.Plate) []int64 {
	var out []int64
	for _, c := range p.Components {
		if !slices.Contains(out, c.FoodID) {
			out = append(out, c.FoodID)
		}
	}
	return out
}

// settings reads the plan settings the rules and the heuristic use.
func (b *Builder) settings(ctx context.Context) (Settings, error) {
	batch, err := plate.ReadBatchSettings(ctx, b.d.Settings)
	if err != nil {
		return Settings{}, err
	}
	maxCook, err := plate.IntSetting(ctx, b.d.Settings, settings.KeyPlanMaxCookOccasionsPerDay)
	if err != nil {
		return Settings{}, err
	}
	variety, err := plate.IntSetting(ctx, b.d.Settings, settings.KeyPlanVarietyWeeks)
	if err != nil {
		return Settings{}, err
	}
	v, err := b.d.Settings.Get(ctx, settings.KeyPlanBatchCooking)
	if err != nil {
		return Settings{}, fmt.Errorf("read %s: %w", settings.KeyPlanBatchCooking, err)
	}
	batchCooking, err := strconv.ParseBool(v.Raw)
	if err != nil {
		return Settings{}, fmt.Errorf("parse %s: %w", settings.KeyPlanBatchCooking, err)
	}
	return Settings{BatchSettings: batch, MaxCookOccasionsPerDay: maxCook, VarietyWeeks: variety, BatchCooking: batchCooking}, nil
}

// routineBase is the id of the first routine plate in a state (D8): they
// are negative like proposed plates but far below a proposal's.
const routineBase = -1_000_000

// ApplyRoutines fills every empty routine cell of the target with its
// preset's plate for that slot (the first one when the preset has none for
// it), at the date's eaters (D8, D16). It adds the plates to s and returns
// the snapshot plates that write them; a preset that is gone leaves its
// cell empty (H1 reports it). One preset read per distinct preset.
func (b *Builder) ApplyRoutines(ctx context.Context, s *State, t Target, eaters func(time.Time) float64) ([]plate.SnapshotPlate, error) {
	x := newIndex(*s, t)
	presets := map[int64]*preset.Preset{}
	var out []plate.SnapshotPlate
	var newFoods []int64
	for _, c := range s.Cells {
		if c.Mode != availability.ModeRoutine || c.PresetID == nil || !x.inTarget(c.Key()) || len(x.byCell[idOf(c.Key())]) > 0 {
			continue
		}
		p, ok := presets[*c.PresetID]
		if !ok {
			got, err := b.d.Presets.Get(ctx, *c.PresetID)
			if err != nil && !errors.Is(err, domain.ErrNotFound) {
				return nil, fmt.Errorf("read preset %d: %w", *c.PresetID, err)
			}
			p, presets[*c.PresetID] = got, got
		}
		pp := presetPlate(p, c.SlotID)
		if pp == nil {
			continue
		}
		n := eaters(c.Date)
		sp := plate.SnapshotPlate{Date: c.Date.Format(time.DateOnly), SlotID: c.SlotID, Eaters: n, Components: make([]plate.SnapshotComponent, len(pp.Components))}
		st := Plate{ID: routineBase - int64(len(out)), Date: c.Date, SlotID: c.SlotID, Eaters: n, Components: make([]Component, len(pp.Components))}
		for i, pc := range pp.Components {
			sp.Components[i] = plate.SnapshotComponent{FoodID: pc.FoodID, Portions: pc.Portions, Amount: pc.Amount, Unit: pc.Unit, Grams: pc.Grams, SortOrder: i}
			st.Components[i] = Component{FoodID: pc.FoodID, Portions: pc.Portions, Grams: pc.Grams}
			if _, known := s.Foods[pc.FoodID]; !known && !slices.Contains(newFoods, pc.FoodID) {
				newFoods = append(newFoods, pc.FoodID)
			}
		}
		out = append(out, sp)
		s.Plates = append(s.Plates, st)
	}
	if len(newFoods) > 0 {
		prof, err := b.d.Profile.Get(ctx)
		if err != nil {
			return nil, fmt.Errorf("read profile: %w", err)
		}
		ids := make(map[int64]bool, len(newFoods))
		for _, id := range newFoods {
			ids[id] = true
		}
		if err := b.addFoods(ctx, s, map[int64]*food.Food{}, ids, nil, prof); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// presetPlate is the preset's plate for slotID, else its first plate; nil
// for a missing or empty preset.
func presetPlate(p *preset.Preset, slotID int64) *preset.Plate {
	if p == nil || len(p.Plates) == 0 {
		return nil
	}
	first := &p.Plates[0]
	for i := range p.Plates {
		pp := &p.Plates[i]
		if pp.SlotID == slotID {
			return pp
		}
		if pp.SortOrder < first.SortOrder {
			first = pp
		}
	}
	return first
}

func minDate(a, b time.Time) time.Time {
	if b.Before(a) {
		return b
	}
	return a
}

func maxDate(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}
