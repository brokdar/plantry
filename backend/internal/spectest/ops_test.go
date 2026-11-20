//go:build spectest

package spectest

import (
	"fmt"
	"slices"
	"sync"
	"testing"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

// opCase is one generated write: a household, an op type and a target.
type opCase struct {
	HH     string
	Kind   string
	Target string
	Op     map[string]any
	Exempt bool // the request itself asks for the consequences (away, delete)
}

// opRun is what happened when the op ran on a fresh copy of the household.
type opRun struct {
	opCase
	Saved                   bool
	Err                     bool
	Raw                     string
	NewViol                 []string // hard violations that were not open before
	UndoExact               bool
	Undone                  bool
	Before                  planning.State
	After                   planning.State
	LightBefore, LightAfter string
	Evening                 planning.Evaluation
}

var (
	opsOnce sync.Once
	opRuns  []opRun
)

// genOps enumerates ops over every op type and several target cells.
func genOps(t *testing.T, hh string) []opCase {
	e := newEnv(t, "chat-"+hh, chatWeek)
	s := e.st()
	lunch, dinner := mealPlates(s)
	var cases []opCase
	add := func(kind, target string, op map[string]any, exempt bool) {
		cases = append(cases, opCase{hh, kind, target, op, exempt})
	}
	cell := func(p planning.Plate) map[string]any {
		return map[string]any{"date": p.Date.Format("2006-01-02"), "slot_id": p.SlotID}
	}
	plates := append(append([]planning.Plate{}, lunch...), dinner...)
	slices.SortFunc(plates, func(a, b planning.Plate) int { return int(a.ID - b.ID) })
	// move / swap: a dinner onto the next day's dinner, a lunch onto another lunch.
	for i, p := range plates {
		if i%4 != 1 {
			continue
		}
		for _, q := range plates {
			if q.SlotID == p.SlotID && q.Date.After(p.Date) {
				add("move", fmt.Sprintf("plate %d", p.ID), map[string]any{"op": "move", "plate_id": p.ID, "to": cell(q), "occupant": "swap"}, false)
				break
			}
		}
	}
	// away: two whole days, two single meals.
	for i, p := range plates {
		switch i % 5 {
		case 0:
			add("away_day", p.Date.Format("01-02"), map[string]any{"op": "away", "date": p.Date.Format("2006-01-02")}, true)
		case 2:
			add("away_meal", fmt.Sprintf("plate %d", p.ID), map[string]any{"op": "away", "date": p.Date.Format("2006-01-02"), "slot_id": p.SlotID}, true)
		}
	}
	// guests, remove a side, add a leaf, replace the main, set_quantity.
	var leaf int64
	for id, f := range s.Foods {
		if f.Kind == food.KindLeaf && !f.Sweet && (leaf == 0 || id < leaf) && f.NutritionComplete && f.Macros.Kcal > 0 {
			leaf = id
		}
	}
	var otherMain int64
	for id, f := range s.Foods {
		if f.Kind == food.KindComposed && f.Role == food.RoleMain && f.NutritionComplete && (otherMain == 0 || id < otherMain) {
			otherMain = id
		}
	}
	for i, p := range plates {
		switch i % 4 {
		case 0:
			add("set_eaters", fmt.Sprintf("plate %d", p.ID), map[string]any{"op": "set_eaters", "plate_id": p.ID, "eaters": p.Eaters + 1}, false)
		case 1:
			for _, c := range p.Components {
				if f := s.Foods[c.FoodID]; f.Role != food.RoleMain && c.ID != 0 {
					add("remove", fmt.Sprintf("comp %d", c.ID), map[string]any{"op": "remove", "plate_component_id": c.ID}, false)
					break
				}
			}
			if leaf != 0 {
				add("add", fmt.Sprintf("plate %d", p.ID), map[string]any{"op": "add", "plate_id": p.ID, "food_id": leaf, "amount": 100.0, "unit": "g"}, false)
			}
		case 2:
			for _, c := range p.Components {
				if f := s.Foods[c.FoodID]; f.Role == food.RoleMain && otherMain != 0 && c.FoodID != otherMain {
					add("replace", fmt.Sprintf("comp %d", c.ID), map[string]any{"op": "replace", "plate_component_id": c.ID, "food_id": otherMain, "portions": 1.0}, false)
					break
				}
			}
		case 3:
			for _, c := range p.Components {
				if c.Portions != nil && s.Foods[c.FoodID].Role != food.RoleMain {
					add("set_quantity", fmt.Sprintf("comp %d", c.ID), map[string]any{"op": "set_quantity", "plate_component_id": c.ID, "portions": 1.5}, false)
					break
				}
			}
		}
	}
	// delete one plate; move one cook to the cell of its first meal's day before.
	if p, ok := pick(dinner, 3); ok {
		add("delete_plate", fmt.Sprintf("plate %d", p.ID), map[string]any{"op": "delete_plate", "plate_id": p.ID}, true)
	}
	n := 0
	for _, ss := range s.Sessions {
		if ss.Confirmed || ss.CookDate.Before(s.Window.Start.AddDate(0, 0, 1)) || n >= 2 {
			continue
		}
		add("move_cook", fmt.Sprintf("session %d", ss.ID), map[string]any{
			"op": "move_cook", "session_id": ss.ID,
			"cook_at": map[string]any{"date": ss.CookDate.AddDate(0, 0, -1).Format("2006-01-02"), "slot_id": ss.CookSlotID},
		}, false)
		n++
	}
	return cases
}

func allOps(t *testing.T) []opRun {
	opsOnce.Do(func() {
		var cases []opCase
		for _, h := range households {
			cases = append(cases, genOps(t, h)...)
		}
		opRuns = make([]opRun, len(cases))
		var wg sync.WaitGroup
		sem := make(chan struct{}, 12)
		for i, c := range cases {
			wg.Add(1)
			go func() {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				opRuns[i] = runOp(t, c)
			}()
		}
		wg.Wait()
	})
	return opRuns
}

func runOp(t *testing.T, c opCase) opRun {
	e := newEnv(t, "chat-"+c.HH, chatWeek)
	r := opRun{opCase: c, Before: e.st(), LightBefore: e.lightRows()}
	before := e.violations()
	snap := snapshot(r.Before)
	rs, chs := e.turn("do it", tc{"apply_changes", map[string]any{"ops": []any{c.Op}}})
	if len(rs) == 1 {
		r.Raw, r.Err = rs[0].Raw, rs[0].Err
		r.Saved = len(chs) > 0
	}
	r.After, r.LightAfter = e.st(), e.lightRows()
	for k := range e.violations() {
		if !before[k] {
			r.NewViol = append(r.NewViol, k)
		}
	}
	slices.Sort(r.NewViol)
	if r.Saved {
		urs, _ := e.turn("undo that", tc{"undo_last_change", `{}`})
		r.Undone = len(urs) == 1 && !urs[0].Err
		r.UndoExact = snapshot(e.st()) == snap
	}
	return r
}
