package tips

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/platefit"
	"github.com/jaltszeimer/plantry/backend/internal/domain/window"
)

// State is a tip's state.
type State string

// States. A shown tip is live; applied is live until its Undo (then shown
// again); dismissed and withdrawn (the plate no longer needs it) are final.
const (
	StateShown     State = "shown"
	StateApplied   State = "applied"
	StateDismissed State = "dismissed"
	StateWithdrawn State = "withdrawn"
)

// Limits (TIP-6, tunable): one tip per plate, one across the plates of a
// batch, MaxPerWindow live tips in a window. Tips are the only unasked Chef
// lines yet, so MaxPerWindow keeps the window within the noise budget of
// PRO-4 (NoiseBudget).
const (
	MaxPerWindow = 3
	NoiseBudget  = 5
	// silenceAfter dismissals of one kind within silenceFor silence that
	// kind for silenceFor.
	silenceAfter = 3
	silenceFor   = 8 * 7 * 24 * time.Hour
)

// ErrConfirmed: the tip's plate is confirmed; it has to be unlocked first
// (TIP-7).
var ErrConfirmed = errors.New("the tip's plate is confirmed")

// ErrStale: the tip is no longer shown, or its plate changed under it.
var ErrStale = errors.New("the tip is no longer shown")

// Op is a tip's change as stored: add a leaf (Grams per eater) or swap the
// component ComponentID to FoodID.
type Op struct {
	Action      string  `json:"action"` // add or swap
	FoodID      int64   `json:"food_id"`
	ComponentID int64   `json:"component_id,omitempty"`
	Grams       float64 `json:"grams,omitempty"`
}

// Tip is one stored tip. FoodName and FromName (the swapped-out side) are
// filled when a window is synced.
type Tip struct {
	ID          int64
	PlateID     *int64
	RecipeID    *int64
	WindowStart time.Time
	Kind        Kind
	Dimension   platefit.Dimension
	Op          Op
	State       State
	CreatedAt   time.Time
	UpdatedAt   time.Time
	FoodName    string
	FromName    string
}

// Dismissal is a dismissed tip's kind and time.
type Dismissal struct {
	Kind Kind
	At   time.Time
}

// Counts are the tips created in a period by their state now.
type Counts struct {
	Shown     int `json:"shown"`
	Applied   int `json:"applied"`
	Dismissed int `json:"dismissed"`
}

// Store keeps the tips; calls join the unit of work in ctx.
type Store interface {
	Create(ctx context.Context, t *Tip) error
	Get(ctx context.Context, id int64) (Tip, error)
	Window(ctx context.Context, start time.Time) ([]Tip, error)
	SetState(ctx context.Context, id int64, st State, at time.Time) error
	DismissedSince(ctx context.Context, since time.Time) ([]Dismissal, error)
	// CountByState counts the tips created in [from, to) by state.
	CountByState(ctx context.Context, from, to time.Time) (map[State]int, error)
}

// Plates is the plate write path a tip's Apply uses (plate.Service).
type Plates interface {
	Get(ctx context.Context, id int64) (*plate.Plate, error)
	Recorded(ctx context.Context, fn func(ctx context.Context) error) (plate.Undo, error)
	AddComponent(ctx context.Context, plateID int64, pc plate.PlateComponent) (*plate.Plate, error)
	SwapComponent(ctx context.Context, componentID, newFoodID int64, quantity *plate.PlateComponent) (*plate.Plate, error)
}

// Deps are the service's dependencies.
type Deps struct {
	Store  Store
	Plates Plates
	// State builds the planner's state of a window (planning.Builder.Build).
	State       func(ctx context.Context, w window.Window, today time.Time) (planning.State, error)
	ChefOn      func(ctx context.Context) (bool, error)
	ShoppingDay func(ctx context.Context) (int, error)
	// Never are the household's never items (kitchen); Equipment the tools
	// present (profile kitchen).
	Never     func(ctx context.Context) (map[int64]bool, error)
	Equipment func(ctx context.Context) ([]string, error)
	Now       func() time.Time
}

// Service computes, stores, applies and dismisses tips.
type Service struct{ d Deps }

// NewService creates a Service.
func NewService(d Deps) *Service {
	if d.Now == nil {
		d.Now = time.Now
	}
	return &Service{d: d}
}

// PlateFit is a plate's reading as the plate detail shows it: the raised
// dimensions (empty for a plate that fits).
type PlateFit struct {
	PlateID int64                `json:"plate_id"`
	Raised  []platefit.Dimension `json:"raised"`
}

// View is a window's live tips and every plate's fit. On is false while the
// Chef is off: nothing is computed then.
type View struct {
	On    bool
	Start time.Time
	Tips  []Tip
	// Ended are the window's dismissed and withdrawn tips (the trial export).
	Ended []Tip
	Fit   []PlateFit
}

// garnishGrams is the amount per eater a garnish of a kind is added in.
var garnishGrams = map[Kind]float64{KindAcid: 10, KindHerbs: 5, KindCrunch: 15, KindPickle: 30}

const defaultGarnishGrams = 10

// run is one sync of a window: its state, checks and candidates.
type run struct {
	w        window.Window
	today    time.Time
	st       planning.State
	before   planning.Evaluation
	never    map[int64]bool
	tools    []string
	garnish  []platefit.Component
	sides    []platefit.Component
	plates   map[int64]planPlate
	order    []int64
	silenced map[Kind]bool
}

// planPlate is a window plate as plate fit reads it: Sides[i] is the
// component CompIDs[i].
type planPlate struct {
	p       planning.Plate
	fit     platefit.Plate
	compIDs []int64
	mainID  int64
	batch   int64 // the main's session, 0 without
}

// Sync brings the stored tips of the window that holds date up to date and
// returns the live ones (TIP-1, 3, 6): a shown tip whose change no longer
// holds is withdrawn, an applied tip whose change is gone again (Undo) is
// shown again, and plates without a tip get one within the limits.
func (s *Service) Sync(ctx context.Context, date time.Time) (View, error) {
	on, err := s.d.ChefOn(ctx)
	if err != nil || !on {
		return View{}, err
	}
	y, err := s.load(ctx, date)
	if err != nil {
		return View{}, err
	}
	stored, err := s.d.Store.Window(ctx, y.w.Start)
	if err != nil {
		return View{}, err
	}
	now := s.d.Now()
	cooled := map[string]bool{} // recipe/kind dismissed in this window
	var live, ended []Tip
	for _, t := range stored {
		pp, ok := y.plateOf(t.PlateID)
		switch t.State {
		case StateDismissed:
			if t.RecipeID != nil {
				cooled[cooldownKey(*t.RecipeID, t.Kind)] = true
			}
			ended = append(ended, t)
			continue
		case StateWithdrawn:
			ended = append(ended, t)
			continue
		case StateApplied:
			if !ok || onState(pp.p, t.Op.FoodID) {
				live = append(live, t)
				continue
			}
			t.State = StateShown // its change was undone
			if err := s.d.Store.SetState(ctx, t.ID, StateShown, now); err != nil {
				return View{}, err
			}
		}
		if ok && y.holds(pp, t) {
			live = append(live, t)
			continue
		}
		if err := s.d.Store.SetState(ctx, t.ID, StateWithdrawn, now); err != nil {
			return View{}, err
		}
		t.State, t.UpdatedAt = StateWithdrawn, now
		ended = append(ended, t)
	}
	tipped, batches := map[int64]bool{}, map[int64]bool{}
	for _, t := range live {
		if pp, ok := y.plateOf(t.PlateID); ok {
			tipped[pp.p.ID] = true
			if pp.batch != 0 {
				batches[pp.batch] = true
			}
		}
	}
	for _, id := range y.order {
		if len(live) >= MaxPerWindow {
			break
		}
		pp := y.plates[id]
		if tipped[id] || (pp.batch != 0 && batches[pp.batch]) || pp.p.Date.Before(y.today) {
			continue
		}
		t, ok := y.pick(pp, func(k Kind) bool { return !y.silenced[k] && !cooled[cooldownKey(pp.mainID, k)] })
		if !ok {
			continue
		}
		t.WindowStart, t.State, t.CreatedAt, t.UpdatedAt = y.w.Start, StateShown, now, now
		if err := s.d.Store.Create(ctx, &t); err != nil {
			return View{}, err
		}
		live = append(live, t)
		tipped[id] = true
		if pp.batch != 0 {
			batches[pp.batch] = true
		}
	}
	for i := range live {
		y.name(&live[i])
	}
	for i := range ended {
		y.name(&ended[i])
	}
	v := View{On: true, Start: y.w.Start, Tips: live, Ended: ended, Fit: make([]PlateFit, 0, len(y.order))}
	for _, id := range y.order {
		raised := platefit.Read(y.plates[id].fit).Raised()
		if raised == nil {
			raised = []platefit.Dimension{}
		}
		v.Fit = append(v.Fit, PlateFit{PlateID: id, Raised: raised})
	}
	return v, nil
}

func cooldownKey(recipe int64, k Kind) string { return fmt.Sprintf("%d/%s", recipe, k) }

func (s *Service) load(ctx context.Context, date time.Time) (*run, error) {
	sd, err := s.d.ShoppingDay(ctx)
	if err != nil {
		return nil, err
	}
	now := s.d.Now()
	y := &run{
		w:     window.Containing(date, sd),
		today: time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC),
	}
	if y.st, err = s.d.State(ctx, y.w, y.today); err != nil {
		return nil, fmt.Errorf("tips state: %w", err)
	}
	if y.never, err = s.d.Never(ctx); err != nil {
		return nil, err
	}
	if y.tools, err = s.d.Equipment(ctx); err != nil {
		return nil, err
	}
	dismissed, err := s.d.Store.DismissedSince(ctx, now.Add(-silenceFor))
	if err != nil {
		return nil, err
	}
	perKind := map[Kind]int{}
	y.silenced = map[Kind]bool{}
	for _, d := range dismissed {
		if perKind[d.Kind]++; perKind[d.Kind] >= silenceAfter {
			y.silenced[d.Kind] = true
		}
	}
	y.before = planning.EvaluateLean(y.st, planning.WindowTarget(y.st))
	for _, id := range y.st.NoCook {
		if f := y.st.Foods[id]; !f.Composed() && !y.never[id] {
			y.garnish = append(y.garnish, componentOf(f))
		}
	}
	ids := make([]int64, 0, len(y.st.Foods))
	for id := range y.st.Foods {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, id := range ids {
		f := y.st.Foods[id]
		if f.Composed() && slices.Contains(sideRoles, f.Role) && f.NutritionComplete && !y.never[id] {
			y.sides = append(y.sides, componentOf(f))
		}
	}
	y.plates = map[int64]planPlate{}
	ps := slices.Clone(y.st.Plates)
	slices.SortStableFunc(ps, func(a, b planning.Plate) int {
		if c := a.Date.Compare(b.Date); c != 0 {
			return c
		}
		return cmp.Compare(y.st.SlotOrder[a.SlotID], y.st.SlotOrder[b.SlotID])
	})
	for _, p := range ps {
		if p.ID <= 0 || !y.w.Contains(p.Date) {
			continue
		}
		if pp, ok := y.plateFit(p); ok {
			y.plates[p.ID] = pp
			y.order = append(y.order, p.ID)
		}
	}
	return y, nil
}

// componentOf is f as plate fit reads it. Fresh and starch are not derived
// yet: plate fit treats them as unknown.
func componentOf(f planning.FoodFacts) platefit.Component {
	return platefit.Component{ID: f.ID, Name: f.Name, Role: f.Role, Leaf: !f.Composed(), Attrs: f.Attrs}
}

// plateFit reads p's main (its main position, else its first recipe) and
// sides; false for a plate without a known main.
func (y *run) plateFit(p planning.Plate) (planPlate, bool) {
	mi := slices.IndexFunc(p.Components, func(c planning.Component) bool { return c.Position == plate.PositionMain })
	if mi < 0 {
		mi = slices.IndexFunc(p.Components, func(c planning.Component) bool { return y.st.Foods[c.FoodID].Composed() })
	}
	if mi < 0 {
		return planPlate{}, false
	}
	main, ok := y.st.Foods[p.Components[mi].FoodID]
	if !ok {
		return planPlate{}, false
	}
	pp := planPlate{p: p, fit: platefit.Plate{Main: componentOf(main)}, mainID: main.ID, batch: p.Components[mi].SessionID}
	for i, c := range p.Components {
		f, ok := y.st.Foods[c.FoodID]
		if i == mi || !ok {
			continue
		}
		pp.fit.Sides = append(pp.fit.Sides, componentOf(f))
		pp.compIDs = append(pp.compIDs, c.ID)
	}
	return pp, true
}

func (y *run) plateOf(id *int64) (planPlate, bool) {
	if id == nil {
		return planPlate{}, false
	}
	pp, ok := y.plates[*id]
	return pp, ok
}

func onState(p planning.Plate, foodID int64) bool {
	return slices.ContainsFunc(p.Components, func(c planning.Component) bool { return c.FoodID == foodID })
}

// pick is the plate's best candidate change that passes the checks, as an
// unsaved tip.
func (y *run) pick(pp planPlate, allowed func(Kind) bool) (Tip, bool) {
	for _, ch := range Candidates(pp.fit, y.garnish, y.sides) {
		if !allowed(ch.Kind) {
			continue
		}
		op := y.opOf(pp, ch)
		if !y.passes(pp, ch, op) {
			continue
		}
		id, main := pp.p.ID, pp.mainID
		return Tip{PlateID: &id, RecipeID: &main, Kind: ch.Kind, Dimension: ch.Dimension, Op: op}, true
	}
	return Tip{}, false
}

func (y *run) opOf(pp planPlate, ch Change) Op {
	if ch.Add != nil {
		g, ok := garnishGrams[ch.Kind]
		if !ok {
			g = defaultGarnishGrams
		}
		return Op{Action: "add", FoodID: ch.Add.ID, Grams: g}
	}
	return Op{Action: "swap", FoodID: ch.With.ID, ComponentID: pp.compIDs[ch.Swap]}
}

// holds reports whether a stored shown tip still applies: the plate still
// raises what the change settles, the change is still a candidate and it
// still passes the checks.
func (y *run) holds(pp planPlate, t Tip) bool {
	for _, ch := range Candidates(pp.fit, y.garnish, y.sides) {
		if ch.Kind == t.Kind && y.opOf(pp, ch) == t.Op {
			return y.passes(pp, ch, t.Op)
		}
	}
	return false
}

// passes is TIP-3: the change already passed the checks. Diet, the day's
// nutrition targets and every hard plan rule through the planner's
// evaluation (no new violation in the window, no new issue on the plate's
// day); never items and P3 by the candidate lists (catalogue foods with
// complete nutrition, never items left out); a garnish is a no-cook leaf (no
// equipment, well under 5 active minutes); a swapped-in side needs only
// tools at hand and stays within the slot's time allowance.
func (y *run) passes(pp planPlate, ch Change, op Op) bool {
	next := slices.Clone(pp.p.Components)
	if op.Action == "add" {
		g := op.Grams
		next = append(next, planning.Component{FoodID: op.FoodID, Grams: &g, Position: plate.PositionExtra})
	} else {
		i := slices.IndexFunc(next, func(c planning.Component) bool { return c.ID == op.ComponentID })
		if i < 0 || !y.fitsKitchen(pp, next[i], y.st.Foods[op.FoodID]) {
			return false
		}
		portions := 1.0
		if next[i].Portions != nil {
			portions = *next[i].Portions
		}
		next[i] = planning.Component{FoodID: op.FoodID, Portions: &portions, Position: next[i].Position}
	}
	st := y.st
	st.Plates = slices.Clone(st.Plates)
	for i := range st.Plates {
		if st.Plates[i].ID == pp.p.ID {
			st.Plates[i].Components = next
		}
	}
	// The plate's own cell is the target: before (the whole window) holds
	// every issue the plate had.
	after := planning.EvaluateLean(st, planning.Target{Cells: []plate.CellKey{pp.p.Cell()}})
	was := map[string]bool{}
	for _, is := range y.before.Issues {
		was[issueKey(is)] = true
	}
	day := pp.p.Date.Format(time.DateOnly)
	for _, is := range after.Issues {
		if was[issueKey(is)] || is.Code == planning.CodeMissingPlate {
			continue
		}
		if is.Severity == planning.SeverityViolation || is.Date == day {
			return false
		}
	}
	return true
}

func issueKey(is planning.Issue) string {
	var slot, plateID, sess int64
	if is.SlotID != nil {
		slot = *is.SlotID
	}
	if is.PlateID != nil {
		plateID = *is.PlateID
	}
	if is.SessionID != nil {
		sess = *is.SessionID
	}
	return fmt.Sprintf("%s|%s|%s|%d|%d|%d", is.Rule, is.Code, is.Date, slot, plateID, sess)
}

// toolsFor are the tools a technique needs.
var toolsFor = map[string]string{
	"bake": "oven", "roast": "oven", "grill": "oven",
	"fry": "stovetop", "saute": "stovetop", "boil": "stovetop", "simmer": "stovetop", "steam": "stovetop", "braise": "stovetop",
}

// fitsKitchen: the new side's technique needs a tool at hand, and its
// minutes are known and no more than the old side's or the plate stays
// within the day's time allowance.
func (y *run) fitsKitchen(pp planPlate, old planning.Component, f planning.FoodFacts) bool {
	if tool, ok := toolsFor[f.Attrs.Technique]; ok && !slices.Contains(y.tools, tool) {
		return false
	}
	if f.Minutes <= 0 && !f.NoCook {
		return false
	}
	oldMin := y.st.Foods[old.FoodID].Minutes
	if f.Minutes <= oldMin {
		return true
	}
	allowance := y.st.Profile.MinutesWeekday
	if wd := pp.p.Date.Weekday(); wd == time.Saturday || wd == time.Sunday {
		allowance = y.st.Profile.MinutesWeekend
	}
	total := 0
	for _, c := range pp.p.Components {
		total += y.st.Foods[c.FoodID].Minutes
	}
	return allowance > 0 && total-oldMin+f.Minutes <= allowance
}

func (y *run) name(t *Tip) {
	t.FoodName = y.st.Foods[t.Op.FoodID].Name
	if t.Op.Action != "swap" {
		return
	}
	if pp, ok := y.plateOf(t.PlateID); ok {
		for _, c := range pp.p.Components {
			if c.ID == t.Op.ComponentID {
				t.FromName = y.st.Foods[c.FoodID].Name
			}
		}
	}
}

// Apply makes a shown tip's change on its plate (TIP-4): a normal plate
// change whose Undo reverts it through POST /api/plan/revert. A confirmed
// plate is refused with ErrConfirmed (TIP-7: unlock first).
func (s *Service) Apply(ctx context.Context, id int64) (plate.Undo, error) {
	t, err := s.d.Store.Get(ctx, id)
	if err != nil {
		return plate.Undo{}, err
	}
	if t.State != StateShown || t.PlateID == nil {
		return plate.Undo{}, ErrStale
	}
	p, err := s.d.Plates.Get(ctx, *t.PlateID)
	if err != nil {
		return plate.Undo{}, err
	}
	if p.State == plate.StateConfirmed {
		return plate.Undo{}, ErrConfirmed
	}
	return s.d.Plates.Recorded(ctx, func(ctx context.Context) error {
		switch t.Op.Action {
		case "add":
			g, unit := t.Op.Grams, "g"
			if _, err := s.d.Plates.AddComponent(ctx, p.ID, plate.PlateComponent{FoodID: t.Op.FoodID, Amount: &g, Unit: &unit}); err != nil {
				return err
			}
		case "swap":
			if !slices.ContainsFunc(p.Components, func(c plate.PlateComponent) bool { return c.ID == t.Op.ComponentID }) {
				return ErrStale
			}
			if _, err := s.d.Plates.SwapComponent(ctx, t.Op.ComponentID, t.Op.FoodID, nil); err != nil {
				return err
			}
		default:
			return fmt.Errorf("%w: tip action %q", domain.ErrInvalidInput, t.Op.Action)
		}
		return s.d.Store.SetState(ctx, t.ID, StateApplied, s.d.Now())
	})
}

// Dismiss dismisses a shown tip: it cools down for its recipe in the window,
// and the third dismissal of a kind within eight weeks silences the kind.
func (s *Service) Dismiss(ctx context.Context, id int64) error {
	t, err := s.d.Store.Get(ctx, id)
	if err != nil {
		return err
	}
	if t.State != StateShown {
		return ErrStale
	}
	return s.d.Store.SetState(ctx, id, StateDismissed, s.d.Now())
}

// Counts returns the tips created in [from, to): shown counts every one.
func (s *Service) Counts(ctx context.Context, from, to time.Time) (Counts, error) {
	m, err := s.d.Store.CountByState(ctx, from, to)
	if err != nil {
		return Counts{}, err
	}
	var c Counts
	for st, n := range m {
		c.Shown += n
		switch st {
		case StateApplied:
			c.Applied = n
		case StateDismissed:
			c.Dismissed = n
		}
	}
	return c, nil
}

// MarshalOp encodes an op for the store.
func MarshalOp(op Op) (string, error) {
	b, err := json.Marshal(op)
	return string(b), err
}

// UnmarshalOp decodes a stored op.
func UnmarshalOp(s string) (Op, error) {
	var op Op
	err := json.Unmarshal([]byte(s), &op)
	return op, err
}
