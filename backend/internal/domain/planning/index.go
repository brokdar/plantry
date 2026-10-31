package planning

import (
	"slices"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// cellID is a location-independent map key for a (date, slot) cell.
type cellID struct {
	day  int
	slot int64
}

func dayOf(t time.Time) int { return t.Year()*1000 + t.YearDay() }

func idOf(c plate.CellKey) cellID { return cellID{day: dayOf(c.Date), slot: c.SlotID} }

// sessionRef names a session: a persisted one by id, a proposed one by key.
type sessionRef struct {
	id  int64
	key string
}

func refOf(c Component) (sessionRef, bool) {
	switch {
	case c.SessionKey != "":
		return sessionRef{key: c.SessionKey}, true
	case c.SessionID != 0:
		return sessionRef{id: c.SessionID}, true
	}
	return sessionRef{}, false
}

func (s Session) ref() sessionRef {
	if s.Proposed() {
		return sessionRef{key: s.Key}
	}
	return sessionRef{id: s.ID}
}

// index is a State with the lookups the rules and the score share, built
// once per Check or Score.
type index struct {
	s       State
	t       Target
	cells   map[cellID]Cell
	target  map[cellID]bool
	days    []time.Time // dates with a target cell, ascending
	byCell  map[cellID][]int
	byPlate map[int64]int
	byRef   map[sessionRef]int
	dupKeys map[string]bool
	links   map[sessionRef][]plate.Link
	// occasions holds each day's cooking occasions (H2): the distinct cook
	// cells of sessions and of fresh components without a session, routine
	// cells excluded.
	occasions map[int]map[cellID]bool
}

func newIndex(s State, t Target) *index {
	x := &index{
		s:         s,
		t:         t,
		cells:     make(map[cellID]Cell, len(s.Cells)),
		target:    make(map[cellID]bool, len(t.Cells)),
		byCell:    make(map[cellID][]int, len(s.Plates)),
		byPlate:   make(map[int64]int, len(s.Plates)),
		byRef:     make(map[sessionRef]int, len(s.Sessions)),
		dupKeys:   map[string]bool{},
		links:     make(map[sessionRef][]plate.Link, len(s.Sessions)),
		occasions: map[int]map[cellID]bool{},
	}
	for _, c := range s.Cells {
		x.cells[idOf(c.Key())] = c
	}
	seenDay := map[int]bool{}
	for _, c := range t.Cells {
		x.target[idOf(c)] = true
		if d := dayOf(c.Date); !seenDay[d] {
			seenDay[d] = true
			x.days = append(x.days, c.Date)
		}
	}
	slices.SortFunc(x.days, time.Time.Compare)
	for i, ss := range s.Sessions {
		r := ss.ref()
		if _, dup := x.byRef[r]; dup && ss.Proposed() {
			x.dupKeys[ss.Key] = true
			continue
		}
		x.byRef[r] = i
		x.addOccasion(ss.CookCell())
	}
	for i, p := range s.Plates {
		cid := idOf(p.Cell())
		x.byCell[cid] = append(x.byCell[cid], i)
		x.byPlate[p.ID] = i
		for _, c := range p.Components {
			r, linked := refOf(c)
			if !linked {
				if c.Portions != nil {
					x.addOccasion(p.Cell())
				}
				continue
			}
			portions := 0.0
			if c.Portions != nil {
				portions = *c.Portions
			}
			x.links[r] = append(x.links[r], plate.Link{
				PlateID: p.ID, FoodID: c.FoodID, Cell: p.Cell(), PerEater: portions, Eaters: p.Eaters,
			})
		}
	}
	return x
}

func (x *index) addOccasion(c plate.CellKey) {
	cid := idOf(c)
	if cell, ok := x.cells[cid]; ok && cell.Mode == availability.ModeRoutine {
		return
	}
	if x.occasions[cid.day] == nil {
		x.occasions[cid.day] = map[cellID]bool{}
	}
	x.occasions[cid.day][cid] = true
}

func (x *index) inTarget(c plate.CellKey) bool { return x.target[idOf(c)] }

func (x *index) food(id int64) (FoodFacts, bool) {
	f, ok := x.s.Foods[id]
	return f, ok
}

// keepDays resolves a session food's keep days.
func (x *index) keepDays(foodID int64) int {
	f := x.s.Foods[foodID]
	return x.s.Settings.KeepDays(f.KeepDays)
}

// checked reports whether a session is the target's business: proposed, or
// eaten from a target cell.
func (x *index) checked(ss Session) bool {
	if ss.Proposed() || x.inTarget(ss.CookCell()) {
		return true
	}
	for _, l := range x.links[ss.ref()] {
		if x.inTarget(l.Cell) {
			return true
		}
	}
	return false
}

// usage is what a session's links consume.
func (x *index) usage(ss Session) plate.Usage {
	return plate.Consumption(ss.CookSession, x.links[ss.ref()], x.keepDays(ss.FoodID))
}

// varietyRole reports whether H8 applies to a recipe of role r: mains,
// standalones and recipes without a role. Sides, sauces, drinks and desserts
// repeat by design.
func varietyRole(r food.Role) bool {
	switch r {
	case food.RoleMain, food.RoleStandalone, "":
		return true
	}
	return false
}
