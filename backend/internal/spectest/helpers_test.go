//go:build spectest

package spectest

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

// snapshot is the plan as the user sees it, with row ids left out (undo may
// recreate rows): plates by cell with their dishes and quantities, cook
// sessions by dish and cooking cell, light marks. Equal snapshots mean equal plans.
func snapshot(s planning.State) string {
	sess := map[int64]string{}
	var out []string
	for _, x := range s.Sessions {
		k := fmt.Sprintf("S food=%d cook=%s/%d portions=%.2f frozen=%.2f", x.FoodID, x.CookDate.Format("2006-01-02"), x.CookSlotID, x.Portions, x.FrozenPortions)
		sess[x.ID] = k
		out = append(out, k)
	}
	for _, p := range s.Plates {
		l := fmt.Sprintf("P %s/%d eaters=%.2f conf=%v", p.Date.Format("2006-01-02"), p.SlotID, p.Eaters, p.Confirmed)
		var cs []string
		for _, c := range p.Components {
			q := 0.0
			if c.Portions != nil {
				q = *c.Portions
			} else if c.Grams != nil {
				q = *c.Grams
			}
			cs = append(cs, fmt.Sprintf("%d:%.2f:%s:{%s}", c.FoodID, q, c.Position, sess[c.SessionID]))
		}
		sort.Strings(cs)
		out = append(out, l+" "+strings.Join(cs, ","))
	}
	for _, c := range s.Cells {
		pid := int64(0)
		if c.PresetID != nil {
			pid = *c.PresetID
		}
		out = append(out, fmt.Sprintf("C %s/%d %s preset=%d", c.Date.Format("2006-01-02"), c.SlotID, c.Mode, pid))
	}
	for _, l := range s.Light {
		out = append(out, fmt.Sprintf("L %v", l))
	}
	sort.Strings(out)
	return strings.Join(out, "\n")
}

func (e *env) lightRows() string {
	rows, err := e.db.Query(`SELECT date, scope, COALESCE(slot,''), COALESCE(level,'') FROM plan_light ORDER BY 1,2,3,4`)
	require.NoError(e.t, err)
	defer rows.Close()
	var out []string
	for rows.Next() {
		var a, b, c, d string
		require.NoError(e.t, rows.Scan(&a, &b, &c, &d))
		out = append(out, a+"|"+b+"|"+c+"|"+d)
	}
	return strings.Join(out, ";")
}

func slotWord(s planning.State, id int64) string {
	n := strings.ToLower(s.SlotNames[id])
	switch {
	case strings.Contains(n, "lunch"):
		return "lunch"
	case strings.Contains(n, "dinner"):
		return "dinner"
	}
	return n
}

// mealPlates are the draft plates of the window that hold a composed main,
// split by lunch and dinner.
func mealPlates(s planning.State) (lunch, dinner []planning.Plate) {
	for _, p := range s.Plates {
		if p.Confirmed || !s.Window.Contains(p.Date) {
			continue
		}
		hasMain := false
		for _, c := range p.Components {
			if f := s.Foods[c.FoodID]; f.Kind == food.KindComposed && f.Role == food.RoleMain {
				hasMain = true
			}
		}
		if !hasMain {
			continue
		}
		switch slotWord(s, p.SlotID) {
		case "lunch":
			lunch = append(lunch, p)
		case "dinner":
			dinner = append(dinner, p)
		}
	}
	return
}

func pick[T any](xs []T, i int) (T, bool) {
	var z T
	if len(xs) == 0 {
		return z, false
	}
	return xs[i%len(xs)], true
}

// waysOut is a find_ways tool result.
type waysOut struct {
	planning.WayResult
	Next        string  `json:"next"`
	Closing     string  `json:"closing"`
	RangeLow    float64 `json:"range_low"`
	RangeHigh   float64 `json:"range_high"`
	MinimumKcal float64 `json:"minimum_kcal"`
}

// wayCase is one find_ways call on a household's plate, with the plan it ran on.
type wayCase struct {
	HH      string
	PlateID int64
	Goal    string
	ToLimit bool
	Out     waysOut
	Raw     string
	Base    planning.State
}

var (
	wayOnce  sync.Once
	wayCases []wayCase
)

// allWays runs find_ways (the real tool, in a real chat turn) for every
// household in parallel and caches the results for the rule tests.
func allWays(t *testing.T) []wayCase {
	wayOnce.Do(func() {
		type job struct {
			hh      string
			plate   int64
			goal    string
			toLimit bool
		}
		var jobs []job
		for _, h := range households {
			e := newEnv(t, "chat-"+h, chatWeek)
			s := e.st()
			lunch, dinner := mealPlates(s)
			if p, ok := pick(dinner, 1); ok {
				jobs = append(jobs, job{h, p.ID, "lighter", false}, job{h, p.ID, "heartier", false}, job{h, p.ID, "lighter", true})
			}
			if p, ok := pick(dinner, 4); ok {
				jobs = append(jobs, job{h, p.ID, "lighter", false}, job{h, p.ID, "lighter", true})
			}
			if p, ok := pick(lunch, 2); ok {
				jobs = append(jobs, job{h, p.ID, "lighter", false}, job{h, p.ID, "heartier", false})
			}
		}
		out := make([]wayCase, len(jobs))
		var wg sync.WaitGroup
		for i, j := range jobs {
			wg.Add(1)
			go func() {
				defer wg.Done()
				e := newEnv(t, "chat-"+j.hh, chatWeek)
				in := map[string]any{"goal": j.goal, "plate_id": j.plate}
				if j.toLimit {
					in["to_limit"] = true
				}
				rs, _ := e.turn("make it "+j.goal, tc{"find_ways", in})
				wc := wayCase{HH: j.hh, PlateID: j.plate, Goal: j.goal, ToLimit: j.toLimit, Base: e.st()}
				if len(rs) == 1 {
					wc.Raw = rs[0].Raw
					_ = json.Unmarshal([]byte(rs[0].Raw), &wc.Out)
				}
				out[i] = wc
			}()
		}
		wg.Wait()
		wayCases = out
	})
	return wayCases
}

func (w wayCase) name() string {
	n := w.HH + "/p" + itoa(w.PlateID) + "/" + w.Goal
	if w.ToLimit {
		n += "/to_limit"
	}
	return n
}

// opMaps decodes a way's ops into generic maps.
func opMaps(ops []planning.OptionOp) []map[string]any {
	b, _ := json.Marshal(ops)
	var out []map[string]any
	_ = json.Unmarshal(b, &out)
	return out
}

func num(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int64:
		return float64(x)
	case int:
		return float64(x)
	}
	return 0
}
