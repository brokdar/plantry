package agent

import (
	"slices"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

// reportCheck is the plan the report was computed from, for the days the change
// touched, before and after it: plates with their components, the batches and
// the stored light marks. The code reads it back to check the facts against the
// stored plan; the model never sees it (withoutFacts).
type reportCheck struct {
	BeforeDays []daySnap `json:"before_days"`
	AfterDays  []daySnap `json:"after_days"`
}

type daySnap struct {
	Date     string                 `json:"date"`
	PlateIDs []int64                `json:"plate_ids"`
	Plates   []plateSnap            `json:"plates"`
	Sessions []sessionSnap          `json:"sessions,omitempty"`
	Light    []planning.LightIntent `json:"light,omitempty"`
}

type plateSnap struct {
	PlateID    int64      `json:"plate_id"`
	SlotID     int64      `json:"slot_id"`
	Meal       string     `json:"meal"`
	Eaters     float64    `json:"eaters"`
	Confirmed  bool       `json:"confirmed,omitempty"`
	Components []compSnap `json:"components"`
}

type compSnap struct {
	FoodID    int64    `json:"food_id"`
	Name      string   `json:"name"`
	Portions  *float64 `json:"portions,omitempty"`
	Grams     *float64 `json:"grams,omitempty"`
	SessionID int64    `json:"session_id,omitempty"`
}

type sessionSnap struct {
	SessionID      int64   `json:"session_id"`
	FoodID         int64   `json:"food_id"`
	Name           string  `json:"name"`
	CookDate       string  `json:"cook_date"`
	CookSlotID     int64   `json:"cook_slot_id"`
	Portions       float64 `json:"portions"`
	FrozenPortions float64 `json:"frozen_portions,omitempty"`
}

// snapDays reads the plates, batches and light marks of dates from a state:
// batches are the ones cooked on those dates or eaten on them.
func snapDays(st planning.State, dates []string) []daySnap {
	out := make([]daySnap, 0, len(dates))
	for _, date := range dates {
		d := daySnap{Date: date, PlateIDs: []int64{}, Plates: []plateSnap{}}
		used := map[int64]bool{}
		plates := slices.Clone(st.Plates)
		slices.SortFunc(plates, func(a, b planning.Plate) int {
			if c := a.Date.Compare(b.Date); c != 0 {
				return c
			}
			return int(st.SlotOrder[a.SlotID]) - int(st.SlotOrder[b.SlotID])
		})
		for _, p := range plates {
			if p.Date.Format(time.DateOnly) != date {
				continue
			}
			ps := plateSnap{PlateID: p.ID, SlotID: p.SlotID, Meal: slotWord(st, p.SlotID), Eaters: p.Eaters, Confirmed: p.Confirmed, Components: []compSnap{}}
			for _, c := range p.Components {
				ps.Components = append(ps.Components, compSnap{FoodID: c.FoodID, Name: foodName(st, c.FoodID), Portions: c.Portions, Grams: c.Grams, SessionID: c.SessionID})
				if c.SessionID != 0 {
					used[c.SessionID] = true
				}
			}
			d.PlateIDs = append(d.PlateIDs, p.ID)
			d.Plates = append(d.Plates, ps)
		}
		for _, s := range st.Sessions {
			if used[s.ID] || s.CookDate.Format(time.DateOnly) == date {
				d.Sessions = append(d.Sessions, sessionSnap{SessionID: s.ID, FoodID: s.FoodID, Name: foodName(st, s.FoodID), CookDate: s.CookDate.Format(time.DateOnly), CookSlotID: s.CookSlotID, Portions: s.Portions, FrozenPortions: s.FrozenPortions})
			}
		}
		for _, l := range st.Light {
			if l.Date == date {
				d.Light = append(d.Light, l)
			}
		}
		out = append(out, d)
	}
	return out
}

// checkOf is the snapshot of the touched days before and after the change.
func checkOf(in reportInput, dates []string) *reportCheck {
	return &reportCheck{BeforeDays: snapDays(in.beforeSt, dates), AfterDays: snapDays(in.afterSt, dates)}
}
