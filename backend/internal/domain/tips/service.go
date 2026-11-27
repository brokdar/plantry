package tips

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/platefit"
	"github.com/jaltszeimer/plantry/backend/internal/domain/slot"
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
	// Taste learns from applied and dismissed tips (taste.Service.Tip);
	// nil learns nothing.
	Taste TipLearner
	Now   func() time.Time
}

// TipLearner learns from a tip's Apply or Dismiss on its dimension.
type TipLearner interface {
	Tip(ctx context.Context, dimension string, applied bool) error
}

// Service computes, stores, applies and dismisses tips.
type Service struct {
	d Deps
	// syncMu serializes Sync: it reads the stored tips and then creates, so
	// overlapping requests would tip one plate twice (one process, one user).
	syncMu sync.Mutex
}

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

// garnishGrams is the amount per eater a garnish of a kind is added in, as a
// cook portions it: a scatter of herbs, a spoon of crunch, a few slices of
// pickle, a wedge of citrus, a side spoon of a cool element.
var garnishGrams = map[Kind]float64{KindAcid: 10, KindHerbs: 4, KindCrunch: 12, KindPickle: 30, KindCool: 60}

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
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
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
	used := map[string]int{}    // kind/food tipped in this window, in any state
	var live, ended []Tip
	for _, t := range stored {
		used[foodKey(t.Kind, t.Op.FoodID)]++
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
	for _, id := range y.ranked() {
		if len(live) >= MaxPerWindow {
			break
		}
		pp := y.plates[id]
		if tipped[id] || (pp.batch != 0 && batches[pp.batch]) {
			continue
		}
		t, ok := y.pick(pp, func(c Change) bool {
			return !y.silenced[c.Kind] && !cooled[cooldownKey(pp.mainID, c.Kind)] && used[foodKey(c.Kind, c.Food().ID)] < maxRepeat
		})
		if !ok {
			continue
		}
		t.WindowStart, t.State, t.CreatedAt, t.UpdatedAt = y.w.Start, StateShown, now, now
		if err := s.d.Store.Create(ctx, &t); err != nil {
			return View{}, err
		}
		live = append(live, t)
		used[foodKey(t.Kind, t.Op.FoodID)]++
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

// ranked are the window's plates that may get a new tip (main meals from
// today on that raise something), by the lift a tip brings: a clash before a
// major gap, then a dinner before a lunch, then the nearest plate. Ranking
// before the limits keeps the window's few tips for the plates that need
// them most, not the first in date order.
func (y *run) ranked() []int64 {
	type cand struct {
		id           int64
		clash, lunch bool
		at           int
	}
	var cs []cand
	for i, id := range y.order {
		pp := y.plates[id]
		if pp.p.Date.Before(y.today) || !y.mainMeal(pp) {
			continue
		}
		r := platefit.Read(pp.fit)
		top, ok := Top(r)
		if !ok {
			continue
		}
		cs = append(cs, cand{id: id, clash: slices.Contains(r.Clashes, top), lunch: y.slotName(pp) != "dinner", at: i})
	}
	slices.SortStableFunc(cs, func(a, b cand) int {
		if a.clash != b.clash {
			if a.clash {
				return -1
			}
			return 1
		}
		if a.lunch != b.lunch {
			if b.lunch {
				return -1
			}
			return 1
		}
		return cmp.Compare(a.at, b.at)
	})
	out := make([]int64, len(cs))
	for i, c := range cs {
		out[i] = c.id
	}
	return out
}

// mainMeal reports whether a plate may carry a tip: a full meal (not a light
// slot, breakfast or snack) built around a main. Plate fit's dimensions are
// rules of a lunch or dinner plate; oats or a snack plate is not judged by them.
func (y *run) mainMeal(pp planPlate) bool {
	if y.st.SlotKind[pp.p.SlotID] == slot.KindLight {
		return false
	}
	if n := y.slotName(pp); n == "breakfast" || n == "snack" {
		return false
	}
	return y.st.Foods[pp.mainID].Role == food.RoleMain
}

func (y *run) slotName(pp planPlate) string {
	return strings.TrimPrefix(y.st.SlotNames[pp.p.SlotID], "slot.")
}

// maxRepeat is how often one kind suggests the same food within a window: a
// tip that says the same thing again is noise.
const maxRepeat = 2

func foodKey(k Kind, food int64) string { return fmt.Sprintf("%s/%d", k, food) }

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
	for _, id := range slices.Concat(y.st.NoCook, y.st.Garnish) {
		if f := y.st.Foods[id]; !f.Composed() && !y.never[id] && !y.st.RatherNot[id] {
			y.garnish = append(y.garnish, componentOf(f))
		}
	}
	ids := make([]int64, 0, len(y.st.Foods))
	for id := range y.st.Foods {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	picky := y.picky()
	for _, id := range ids {
		f := y.st.Foods[id]
		// A food the household would rather not get beside a meal
		// (State.RatherNot) is never suggested.
		// A picky household (refusing several side dishes) gets no side
		// swap, garnish only; a single refused side is only never suggested.
		if !picky && f.Composed() && !f.Archived && slices.Contains(sideRoles, f.Role) && f.NutritionComplete && !y.never[id] && !y.st.RatherNot[id] {
			y.sides = append(y.sides, componentOf(f))
		}
	}
	// Sides the household has eaten come first: a swap prefers one.
	eaten := map[int64]bool{}
	for _, h := range y.st.History {
		for _, id := range h.FoodIDs {
			eaten[id] = true
		}
	}
	slices.SortStableFunc(y.sides, func(a, b platefit.Component) int {
		return cmp.Compare(b2i(eaten[b.ID]), b2i(eaten[a.ID]))
	})
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

// pickyMin is how many side dishes a household must refuse (State.RatherNot)
// to be picky: it gets no side swap then.
const pickyMin = 2

// picky reports whether the household refuses at least pickyMin side dishes.
func (y *run) picky() bool {
	n := 0
	for id := range y.st.RatherNot {
		if y.st.Foods[id].Composed() {
			n++
		}
	}
	return n >= pickyMin
}

// refuses reports whether the household refuses food: it would rather not
// get some food again (State.RatherNot, from its disliked and not-eaten
// plates). It gets no bold garnish then (classicGarnish).
func (y *run) refuses() bool { return len(y.st.RatherNot) > 0 }

// componentOf is f as plate fit reads it.
func componentOf(f planning.FoodFacts) platefit.Component {
	return platefit.Component{ID: f.ID, Name: f.Name, Role: f.Role, Leaf: !f.Composed(), Attrs: f.Attrs, Fresh: f.Fresh, Starch: f.Starch}
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

// servedThatDay reports whether another plate of pp's day already serves the
// food: a tip never repeats a food the day has (the same salad at lunch and
// dinner).
func (y *run) servedThatDay(pp planPlate, foodID int64) bool {
	for id, o := range y.plates {
		if id != pp.p.ID && o.p.Date.Equal(pp.p.Date) && onState(o.p, foodID) {
			return true
		}
	}
	return false
}

// pick is the plate's best candidate change that passes the checks, as an
// unsaved tip.
func (y *run) pick(pp planPlate, allowed func(Change) bool) (Tip, bool) {
	for _, ch := range candidates(pp.fit, y.garnish, y.sides, y.refuses()) {
		if !allowed(ch) || y.servedThatDay(pp, ch.Food().ID) {
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
		return Op{Action: "add", FoodID: ch.Add.ID, Grams: garnishGrams[ch.Kind]}
	}
	return Op{Action: "swap", FoodID: ch.With.ID, ComponentID: pp.compIDs[ch.Swap]}
}

// holds reports whether a stored shown tip still applies: the plate is a
// main meal, it still raises what the change settles, the change is still a
// candidate and it still passes the checks.
func (y *run) holds(pp planPlate, t Tip) bool {
	if !y.mainMeal(pp) {
		return false
	}
	for _, ch := range candidates(pp.fit, y.garnish, y.sides, y.refuses()) {
		if ch.Kind == t.Kind && y.opOf(pp, ch) == t.Op {
			return !y.servedThatDay(pp, ch.Food().ID) && y.passes(pp, ch, t.Op)
		}
	}
	return false
}

// passes is TIP-3: the change already passed the checks. Diet, the day's
// nutrition targets and every plan rule through the planner's
// evaluation (no new issue anywhere in the window, week rules included); never items and P3 by the candidate lists (catalogue foods with
// complete nutrition, never items left out); a garnish is a no-cook leaf (no
// equipment, well under 5 active minutes); a swapped-in side needs only
// tools at hand and stays within the slot's time allowance.
func (y *run) passes(pp planPlate, ch Change, op Op) bool {
	next := slices.Clone(pp.p.Components)
	var left int64 // the batch the swapped side ate from
	if op.Action == "add" {
		g := op.Grams
		next = append(next, planning.Component{FoodID: op.FoodID, Grams: &g, Position: plate.PositionExtra})
	} else {
		i := slices.IndexFunc(next, func(c planning.Component) bool { return c.ID == op.ComponentID })
		if i < 0 || !y.fitsKitchen(pp, next[i], y.st.Foods[op.FoodID]) {
			return false
		}
		left = next[i].SessionID
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
	if left != 0 {
		st.Sessions = y.leftBatch(st, pp.p, left)
	}
	// The whole window is the target, as for before: a change that raises
	// an issue anywhere (the week's soft rules included) is not offered; an
	// issue the plan already had does not block.
	tgt := planning.WindowTarget(st)
	tgt.Cells = append(tgt.Cells, pp.p.Cell())
	after := planning.EvaluateLean(st, tgt)
	was := map[string]bool{}
	for _, is := range y.before.Issues {
		was[issueKey(is)] = true
	}
	for _, is := range after.Issues {
		if !was[issueKey(is)] && is.Code != planning.CodeMissingPlate {
			return false
		}
	}
	return true
}

// leftBatch is st's sessions after the swapped plate p left the batch sess,
// as the plate service settles it (D11, plate.reanchor): a batch no plate
// eats from any more is gone; an uncooked batch whose cook meal was p moves
// its cook to its earliest remaining meal from today on. Without this the
// check missed a new cook occasion on the later day (H2, CP1e family week 2:
// Monday's rice swapped for couscous moved the rice cook to Tuesday lunch).
func (y *run) leftBatch(st planning.State, p planning.Plate, sess int64) []planning.Session {
	var first *planning.Plate
	eaten := false
	for i, q := range st.Plates {
		if !slices.ContainsFunc(q.Components, func(c planning.Component) bool { return c.SessionID == sess }) {
			continue
		}
		eaten = true
		if q.Date.Before(y.today) {
			continue
		}
		if first == nil || q.Date.Before(first.Date) || (q.Date.Equal(first.Date) && st.SlotOrder[q.SlotID] < st.SlotOrder[first.SlotID]) {
			first = &st.Plates[i]
		}
	}
	out := slices.Clone(st.Sessions)
	i := slices.IndexFunc(out, func(s planning.Session) bool { return s.ID == sess })
	switch {
	case i < 0:
	case !eaten:
		out = slices.Delete(out, i, i+1)
	case first != nil && out[i].CookedAt == nil && out[i].CookCell().Equal(p.Cell()):
		out[i].CookDate, out[i].CookSlotID = first.Date, first.SlotID
	}
	return out
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
// change whose Undo reverts it through POST /api/plan/revert. The plate
// service's rules for a hand edit apply, a confirmed plate's included (TIP-7).
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
	// The tip's plate change is a tip signal, not a hand edit.
	return s.d.Plates.Recorded(plate.WithoutTaste(ctx), func(ctx context.Context) error {
		// Re-read inside the transaction: a second Apply (another tab) that
		// waited for this one must find the tip applied and refuse.
		if cur, err := s.d.Store.Get(ctx, id); err != nil {
			return err
		} else if cur.State != StateShown {
			return ErrStale
		}
		switch t.Op.Action {
		case "add":
			g, unit := t.Op.Grams, "g"
			if _, err := s.d.Plates.AddComponent(ctx, p.ID, plate.PlateComponent{FoodID: t.Op.FoodID, Amount: &g, Unit: &unit, Position: plate.PositionExtra}); err != nil {
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
		if err := s.d.Store.SetState(ctx, t.ID, StateApplied, s.d.Now()); err != nil {
			return err
		}
		return s.learn(ctx, t, true)
	})
}

// learn passes an applied or dismissed tip to the taste learner.
func (s *Service) learn(ctx context.Context, t Tip, applied bool) error {
	if s.d.Taste == nil {
		return nil
	}
	return s.d.Taste.Tip(ctx, string(t.Dimension), applied)
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
	// One unit of work for the state and what the taste learns from it.
	_, err = s.d.Plates.Recorded(ctx, func(ctx context.Context) error {
		if err := s.d.Store.SetState(ctx, id, StateDismissed, s.d.Now()); err != nil {
			return err
		}
		return s.learn(ctx, t, false)
	})
	return err
}

// On reports whether the Chef is on (chef.enabled).
func (s *Service) On(ctx context.Context) (bool, error) { return s.d.ChefOn(ctx) }

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

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}
