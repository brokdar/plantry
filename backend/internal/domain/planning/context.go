package planning

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/preset"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
	"github.com/jaltszeimer/plantry/backend/internal/domain/settings"
	"github.com/jaltszeimer/plantry/backend/internal/domain/shopping"
	"github.com/jaltszeimer/plantry/backend/internal/domain/slot"
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
	// Corrections reads the user's recipe swaps (one bounded query).
	Corrections Corrections
	// Light reads the stored light requests of the window.
	Light LightStore
	// Version, when set, lets Build reuse the State it built last for the
	// same arguments while the data has not changed (see Versioner).
	Version Versioner
}

// Versioner reports a token that changes whenever the rows a State is built
// from may have changed: sqlite's per-connection change count and the
// file's data version. ok is false when it cannot tell; Build then builds
// afresh.
type Versioner interface {
	DataVersion(ctx context.Context) (token string, ok bool)
}

// Builder reads the State of a window.
type Builder struct {
	d    BuilderDeps
	mu   sync.Mutex
	memo []builtState // the last few States built, newest last
}

// builtState is a State Build returned, with the arguments and data version
// it was built for.
type builtState struct {
	key string
	s   State
}

// memoSize bounds the States kept: a chat tool call reads the state of one
// window about ten times, the planner the same window and its neighbours.
const memoSize = 4

// NewBuilder returns a context builder.
func NewBuilder(d BuilderDeps) *Builder { return &Builder{d: d} }

// Build reads the state of window w as of today. Its cells span the window
// widened to [from, to] (closure plates outside it), its plates and sessions
// that range plus the keep-days lookback on both sides, its foods every
// recipe plus every food a plate references, and its history the confirmed
// plates of the 4 weeks before the window. Reads: settings, profile, slots,
// availability, one plate range, one session range, the recipe list with
// its history, one header lookup and one nutrition batch.
//
// With a Versioner it returns a copy of the State it built for the same
// arguments when no row changed since: a chat tool call read the full state
// about ten times (12 000 builds for 1 230 tool calls in the agent tests, a
// third of their CPU).
func (b *Builder) Build(ctx context.Context, w window.Window, today, from, to time.Time) (State, error) {
	if b.d.Version == nil {
		return b.build(ctx, w, today, from, to)
	}
	v, ok := b.d.Version.DataVersion(ctx)
	if !ok {
		return b.build(ctx, w, today, from, to)
	}
	key := fmt.Sprintf("%s|%s|%s|%s|%s|%s", v, w.Start, w.End, today, from, to)
	b.mu.Lock()
	for _, m := range b.memo {
		if m.key == key {
			s := deepCopy(m.s)
			b.mu.Unlock()
			return s, nil
		}
	}
	b.mu.Unlock()
	s, err := b.build(ctx, w, today, from, to)
	if err != nil {
		return State{}, err
	}
	b.mu.Lock()
	b.memo = append(b.memo, builtState{key: key, s: deepCopy(s)})
	if len(b.memo) > memoSize {
		b.memo = b.memo[1:]
	}
	b.mu.Unlock()
	return s, nil
}

func (b *Builder) build(ctx context.Context, w window.Window, today, from, to time.Time) (State, error) {
	s := State{Window: w, Today: today}
	var err error
	if s.Settings, err = ReadSettings(ctx, b.d.Settings); err != nil {
		return State{}, err
	}
	prof, err := b.d.Profile.Get(ctx)
	if err != nil {
		return State{}, fmt.Errorf("read profile: %w", err)
	}
	s.Profile = StateProfile(prof)
	slots, err := b.d.Slots.List(ctx, false)
	if err != nil {
		return State{}, fmt.Errorf("list slots: %w", err)
	}
	s.SlotOrder = make(plate.SlotOrder, len(slots))
	s.SlotNames = make(map[int64]string, len(slots))
	s.SlotKind = make(map[int64]slot.Kind, len(slots))
	var bands []slot.Band
	s.SlotBand = make(map[int64]slot.Band, len(slots))
	for _, sl := range slots {
		s.SlotKind[sl.ID] = sl.Kind
		s.SlotOrder[sl.ID] = sl.SortOrder
		s.SlotNames[sl.ID] = sl.NameKey
		if sl.Active {
			b := sl.Effective()
			bands = append(bands, b)
			s.SlotBand[sl.ID] = b
		}
	}
	s.SlotShare = slot.Shares(bands)
	if s.Light, err = b.d.Light.List(ctx, w.Start, w.End); err != nil {
		return State{}, fmt.Errorf("read light requests: %w", err)
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
		s.Cells[i] = StateCell(c)
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
		s.Plates = append(s.Plates, StatePlate(p))
		for _, c := range p.Components {
			referenced[c.FoodID] = true
		}
	}
	if s.Corrections, err = b.d.Corrections.UserReplacements(ctx, w.Start.AddDate(0, 0, -historyDays), w.Start); err != nil {
		return State{}, fmt.Errorf("read corrections: %w", err)
	}
	s.Sessions = make([]Session, len(sessions))
	for i, ss := range sessions {
		s.Sessions[i] = StateSession(ss)
		referenced[ss.FoodID] = true
	}
	// The no-cook candidates: raw-eaten leaves and no-cook recipes, so an
	// evaluation can offer them to close a gap.
	noCook := map[int64]bool{}
	for id, f := range foods {
		if NoCook(f) {
			noCook[id] = true
		}
	}
	leaves, err := b.d.Foods.List(ctx, food.ListQuery{Kind: food.KindLeaf, Limit: maxFoods, Today: today})
	if err != nil {
		return State{}, fmt.Errorf("list leaves: %w", err)
	}
	for i := range leaves.Items {
		if f := &leaves.Items[i]; NoCook(f) {
			foods[f.ID], noCook[f.ID] = f, true
			ptrs = append(ptrs, f)
		}
	}
	if err := b.d.Foods.DeriveDiet(ctx, ptrs[len(recipes.Items):]...); err != nil {
		return State{}, fmt.Errorf("no-cook diet: %w", err)
	}
	s.Foods = map[int64]FoodFacts{}
	if err := b.addFoods(ctx, &s, foods, referenced, timesEaten, prof); err != nil {
		return State{}, err
	}
	for id := range noCook {
		if f := s.Foods[id]; f.NutritionComplete && f.Macros.Kcal > 0 {
			s.NoCook = append(s.NoCook, id)
		}
	}
	slices.Sort(s.NoCook)
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

// ExploreTag is the tag discovery puts on a recipe the user has not tried.
const ExploreTag = "explore"

// WithNutrition sets the facts' macros, completeness, fibre flag and DGE
// groups from the food's resolved nutrition (the board and the builder share
// it).
func WithNutrition(ff FoodFacts, d food.Detailed) FoodFacts {
	ff.Macros, ff.NutritionComplete, ff.FiberMissing = d.Macros, d.Complete, d.FiberMissing
	ff.Groups = GroupsOf(d.Leaves)
	ff.Leaves = nil
	if len(d.Leaves) > 0 {
		ff.Leaves = map[int64]float64{}
		for _, l := range d.Leaves {
			ff.Leaves[l.ID] += l.Grams
		}
	}
	return ff
}

// facts maps a food to what the planner reads about it.
func facts(f *food.Food, d food.Detailed, timesEaten int, likes, dislikes map[string]bool) FoodFacts {
	ff := WithNutrition(BaseFacts(f), d)
	ff.TimesEaten8Weeks = timesEaten
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
		if t == ExploreTag {
			ff.ExploreTag = true
		}
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

// The row mappers below turn loaded rows into the planner's types. The
// context builder and the plan read model (planview) both use them, so the
// board's issues and the generator's check see the same state.

// BaseFacts are a food's facts from its header row and derived fields: id,
// name, kind, role, diet class, keep days, freezable, favorite and
// f.History.LastEaten. Macros, the 8-week count and the like score need
// further reads (the context builder's facts).
func BaseFacts(f *food.Food) FoodFacts {
	ff := FoodFacts{
		ID: f.ID, Name: food.Data(f.Name), Kind: f.Kind, KeepDays: f.KeepDays, Freezable: f.Freezable != nil && *f.Freezable,
		Favorite: f.Favorite, LastEaten: f.History.LastEaten, ReferencePortions: 1, DietClass: f.DietClass,
		NoCook: NoCook(f), Sweet: IsSweet(f), Tags: food.Labels(f.Tags),
	}
	if f.ServingG != nil {
		ff.ServingG = *f.ServingG
	}
	if f.Kind == food.KindLeaf {
		if r, _ := shopping.EffectiveRole(f); r != nil {
			ff.LeafRole = *r
		}
	}
	// Only composed foods carry a planner role; a leaf's stored role is a
	// user override for plate position and must not enter candidate lists.
	if f.Role != nil && f.Kind == food.KindComposed {
		ff.Role = *f.Role
	}
	return ff
}

// StateProfile maps the user profile.
func StateProfile(p *profile.Profile) Profile {
	out := Profile{
		KcalTarget: p.KcalTarget, ProteinPct: p.ProteinPct, FatPct: p.FatPct, CarbsPct: p.CarbsPct,
		DietType: p.DietType, MeatDaysPerWeekMax: p.MeatDaysPerWeekMax, HouseholdPortions: p.HouseholdPortions,
		Locale: p.Locale, Restrictions: p.DietaryRestrictions,
		Goal: p.Goal, Sex: p.Sex, WeightKg: p.WeightKg, BirthYear: p.BirthYear, ProteinGPerKg: p.ProteinGPerKg,
	}
	if p.SystemPrompt != nil {
		out.Notes = *p.SystemPrompt
	}
	return out
}

// StateCell maps a resolved availability cell.
func StateCell(c availability.Cell) Cell {
	return Cell{Date: c.Date, SlotID: c.SlotID, Mode: c.Mode, PresetID: c.PresetID}
}

// StateSession maps a persisted cook session.
func StateSession(cs plate.CookSession) Session {
	return Session{CookSession: cs, Confirmed: cs.State == plate.StateConfirmed}
}

// StatePlate maps a persisted plate.
func StatePlate(p plate.Plate) Plate {
	out := Plate{
		ID: p.ID, Date: p.Date, SlotID: p.SlotID, Eaters: p.Eaters, Confirmed: p.State == plate.StateConfirmed,
		Components: make([]Component, len(p.Components)),
	}
	for i, c := range p.Components {
		out.Components[i] = Component{ID: c.ID, FoodID: c.FoodID, Portions: c.Portions, Grams: c.Grams, Position: c.Position}
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

// ReadSettings reads the plan settings the rules and the heuristic use.
func ReadSettings(ctx context.Context, r SettingsReader) (Settings, error) {
	batch, err := plate.ReadBatchSettings(ctx, r)
	if err != nil {
		return Settings{}, err
	}
	maxCook, err := plate.IntSetting(ctx, r, settings.KeyPlanMaxCookOccasionsPerDay)
	if err != nil {
		return Settings{}, err
	}
	variety, err := plate.IntSetting(ctx, r, settings.KeyPlanVarietyWeeks)
	if err != nil {
		return Settings{}, err
	}
	v, err := r.Get(ctx, settings.KeyPlanBatchCooking)
	if err != nil {
		return Settings{}, fmt.Errorf("read %s: %w", settings.KeyPlanBatchCooking, err)
	}
	batchCooking, err := strconv.ParseBool(v.Raw)
	if err != nil {
		return Settings{}, fmt.Errorf("parse %s: %w", settings.KeyPlanBatchCooking, err)
	}
	maxDish, err := plate.IntSetting(ctx, r, settings.KeyPlanMaxDishMealsPerWeek)
	if err != nil {
		return Settings{}, err
	}
	return Settings{BatchSettings: batch, MaxCookOccasionsPerDay: maxCook, VarietyWeeks: variety, BatchCooking: batchCooking, MaxDishMealsPerWeek: maxDish}, nil
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
