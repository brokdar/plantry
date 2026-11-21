//go:build spectest

package spectest

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
	"github.com/jaltszeimer/plantry/backend/internal/domain/slot"
)

// Spec v1.1 SB1/SB2: slot bands and expected shares. The households keep the
// default bands, so the stored bands equal the defaults (the oracle reads the stored ones).

type band struct{ Min, Max float64 }

func (b band) mid() float64 { return (b.Min + b.Max) / 2 }

// defaultBand is SB1: by slot name, a custom slot by kind.
func defaultBand(name string, kind slot.Kind) band {
	n := strings.ToLower(name)
	switch {
	case strings.Contains(n, "breakfast"):
		return band{20, 30}
	case strings.Contains(n, "lunch"):
		return band{30, 40}
	case strings.Contains(n, "dinner"):
		return band{25, 40}
	case strings.Contains(n, "snack"):
		return band{5, 15}
	}
	if kind == slot.KindLight {
		return band{5, 15}
	}
	return band{25, 40}
}

// bandOfSlot is the slot's stored band (State.SlotBand, SB1); the default only when none is stored.
func bandOfSlot(s planning.State, id int64) band {
	if b, ok := s.SlotBand[id]; ok {
		return band{b.MinPct, b.MaxPct}
	}
	return defaultBand(s.SlotNames[id], s.SlotKind[id])
}

// shares is SB2: band midpoint over the sum of all configured slots' midpoints.
func shares(s planning.State) map[int64]float64 {
	var sum float64
	for id := range s.SlotNames {
		sum += bandOfSlot(s, id).mid()
	}
	out := map[int64]float64{}
	for id := range s.SlotNames {
		out[id] = bandOfSlot(s, id).mid() / sum
	}
	return out
}

// unplanned is the sum of the expected shares of the day's slots that are not
// planned (SB3): away or default-none cells, and routine cells without a plate.
func unplanned(s planning.State, date string) float64 {
	sh := shares(s)
	plated := map[int64]bool{}
	for _, p := range s.Plates {
		if p.Date.Format("2006-01-02") == date {
			plated[p.SlotID] = true
		}
	}
	var sum float64
	for _, c := range s.Cells {
		if c.Date.Format("2006-01-02") != date {
			continue
		}
		if string(c.Mode) == "none" || (string(c.Mode) == "routine" && !plated[c.SlotID]) {
			sum += sh[c.SlotID]
		}
	}
	return sum
}

// slotExpected is SB5 for one day: the expected kcal of each planned non-routine
// slot, and the shift of its band in kcal. A routine meal counts with its real
// kcal; away, default-none and unplated routine slots stay at their share.
func slotExpected(s planning.State, ev planning.Evaluation, date string) (exp map[int64]float64, shift map[int64]float64) {
	return slotExpectedWith(s, ev, date, shares(s))
}

// slotExpectedWith is slotExpected for explicit expected shares (a preset).
func slotExpectedWith(s planning.State, ev planning.Evaluation, date string, sh map[int64]float64) (exp map[int64]float64, shift map[int64]float64) {
	T := fullTarget(s)
	plateKcal := map[int64]float64{}
	for _, p := range ev.Plates {
		if p.Date == date && p.PlateID > 0 {
			plateKcal[p.SlotID] = p.Kcal
		}
	}
	var unpl, routineKcal, shP float64
	var planned []int64
	for _, c := range s.Cells {
		if c.Date.Format("2006-01-02") != date {
			continue
		}
		_, plated := plateKcal[c.SlotID]
		switch string(c.Mode) {
		case "none":
			unpl += sh[c.SlotID]
		case "routine":
			if plated {
				routineKcal += plateKcal[c.SlotID]
			} else {
				unpl += sh[c.SlotID]
			}
		default:
			planned = append(planned, c.SlotID)
			shP += sh[c.SlotID]
		}
	}
	exp, shift = map[int64]float64{}, map[int64]float64{}
	if shP == 0 {
		return
	}
	E := T*(1-unpl) - routineKcal
	for _, id := range planned {
		exp[id] = E * sh[id] / shP
		shift[id] = exp[id] - T*sh[id]
	}
	return
}

func fullTarget(s planning.State) float64 {
	if s.Profile.KcalTarget == nil {
		return 0
	}
	return *s.Profile.KcalTarget
}

func fullFloor(s planning.State) float64 {
	t := fullTarget(s)
	return max(0.75*t, profile.MinDailyKcal(s.Profile.Sex))
}

// SB2 (helper check): with the default bands and slots breakfast, lunch, dinner
// the expected shares are 27.0 %, 37.8 % and 35.1 %.
func TestSB2_ExpectedShareFunction(t *testing.T) {
	e := newEnv(t, "chat-dev", chatWeek)
	sh := shares(e.st())
	assert.InDelta(t, 0.270, sh[1], 0.001)
	assert.InDelta(t, 0.378, sh[2], 0.001)
	assert.InDelta(t, 0.351, sh[3], 0.001)
}

// SB1 (v1.1 signed): any number of slots is allowed; an invalid set (sum of mins
// > 100 or sum of maxes < 100) is refused with the numbers, nothing lowered or
// normalised silently. The real tests are in bands_cc22_test.go.
func TestSB1_BandValidation(t *testing.T) {
	t.Skip("runs with -tags spectest,cc22 once the band API exists (CC22); contract in bands_cc22_test.go: refusal names the numbers, nothing is lowered or normalised, any slot count is accepted when valid")
}

// SB2/RL4: the filling minimum is 125 % of the slot's expected kcal.
func TestRL4_FillingMinimumIsFiveFourthsOfSlotShare(t *testing.T) {
	for _, wc := range allWays(t) {
		if wc.Goal != "heartier" || wc.Out.MinKcal == 0 {
			continue
		}
		t.Run(wc.name(), func(t *testing.T) {
			var slotID int64
			var date string
			for _, p := range wc.Base.Plates {
				if p.ID == wc.PlateID {
					slotID, date = p.SlotID, p.Date.Format("2006-01-02")
				}
			}
			exp, _ := slotExpected(wc.Base, planning.Evaluate(wc.Base, planning.WindowTarget(wc.Base)), date)
			assert.InDelta(t, 1.25*exp[slotID], wc.Out.MinKcal, 1.0, "min_kcal must be 125 %% of the slot's expected kcal (SB5, shifted by a routine meal)")
		})
	}
}

// SB3: a part-away day shows the assumed energy of the away slots and the
// planned part's target, and its protein target prorates the same way.
func TestSB3_AwayBoardFieldsAndProteinProrate(t *testing.T) {
	forOps(t, []string{"away_meal"}, func(t *testing.T, r opRun) {
		var res struct {
			After struct {
				Days []map[string]any `json:"days"`
			} `json:"after"`
		}
		_ = json.Unmarshal([]byte(r.Raw), &res)
		date := r.Op["date"].(string)
		var day map[string]any
		for _, d := range res.After.Days {
			if d["date"] == date {
				day = d
			}
		}
		if day == nil {
			t.Skip("the day is not in the result")
		}
		away := unplanned(r.After, date)
		T := fullTarget(r.After)
		assert.InDelta(t, T*away, num(day["away_kcal"]), 1.0, "away_kcal = day target x sum of the away slots' shares")
		assert.InDelta(t, T*(1-away), num(day["planned_target_kcal"]), 1.0, "planned_target_kcal = day target - away_kcal")
		pol := planning.PolicyOf(r.After.Profile)
		if want := pol.ProteinTargetG(1) * (1 - away); want > 0 {
			assert.InDelta(t, want, num(day["protein_target_g"]), 1.0, "the protein target prorates like the energy target")
		}
	})
}

// SF1(c) v1.1: an away whose planned meals end under the prorated minimum is
// saved, the reply tells the day under its minimum and makes one offer.
func TestSF1c_AwayDeficitSavedReportedWithOffer(t *testing.T) {
	forOps(t, []string{"away_meal", "away_day"}, func(t *testing.T, r opRun) {
		newH9 := false
		for _, k := range r.NewViol {
			if strings.HasPrefix(k, "H9/") {
				newH9 = true
			}
		}
		if !newH9 {
			t.Skip("no new day under its minimum")
		}
		assert.True(t, r.Saved, "the away is a fact and must be saved: %.200s", r.Raw)
		assert.Regexp(t, `(?i)minimum|mindest`, r.Raw, "the reply must report the day under its minimum")
		// a fix needs a planned meal of that day that is not marked light and is not a routine
		// slot (RL6: a light mark is not broken by a top-up). Where none exists, v1.1 says
		// nothing about the offer (OQ-v1.1-6, proposals.md): the case is not decided.
		date := r.Op["date"].(string)
		fixable := false
		marked := map[string]bool{}
		for _, l := range r.After.Light {
			if l.Date == date {
				marked[l.Slot] = true
				if l.Scope == planning.LightScopeDay {
					marked["lunch"], marked["dinner"] = true, true
				}
			}
		}
		for _, c := range r.After.Cells {
			if c.Date.Format("2006-01-02") == date && string(c.Mode) == "plan" && !marked[slotWord(r.After, c.SlotID)] {
				for _, p := range r.After.Plates {
					if p.Date.Equal(c.Date) && p.SlotID == c.SlotID {
						fixable = true
					}
				}
			}
		}
		if !fixable {
			// v1.2 RL9 (P-1): the one offer removes the light mark of the marked meal that would
			// close or reduce the gap, applied only on a yes; no marked meal: plainly, no offer
			lever := len(marked) > 0
			if lever {
				assert.Regexp(t, `(?i)"scope":"none"|light mark|no longer light|leicht`, r.Raw, "the offer must remove a light mark")
				assert.Equal(t, r.LightBefore, r.LightAfter, "the mark is removed only on a yes")
			} else {
				assert.NotRegexp(t, `"offers?":\[\{`, r.Raw, "no lever exists: no offer")
			}
			return
		}
		assert.Regexp(t, `(?i)"offers?"|closing|I can also|day_fix`, r.Raw, "one offer to fix it")
	})
}

// NU11 (S10): a lunch or dinner outside its slot band is reported, with no
// options, never as a violation; marked meals are exempt.
func TestNU11_SlotBandIssueMatchesOracle(t *testing.T) {
	for _, h := range households {
		t.Run(h, func(t *testing.T) {
			e := newEnv(t, "chat-"+h, chatWeek)
			s := e.st()
			ev := planning.Evaluate(s, planning.WindowTarget(s))
			T := fullTarget(s)
			complete := map[string]bool{}
			for _, d := range ev.Days {
				complete[d.Date] = d.Complete && !d.Away
			}
			marked := map[string]bool{}
			for _, l := range s.Light {
				marked[l.Date+"/"+l.Slot] = true
				if l.Scope == planning.LightScopeDay {
					marked[l.Date+"/lunch"], marked[l.Date+"/dinner"] = true, true
				}
			}
			want := map[string]bool{}
			for _, p := range ev.Plates {
				w := slotWord(s, p.SlotID)
				if (w != "lunch" && w != "dinner") || !complete[p.Date] || marked[p.Date+"/"+w] || p.PlateID < 0 {
					continue
				}
				b := bandOfSlot(s, p.SlotID)
				_, shift := slotExpected(s, ev, p.Date)
				if p.Kcal < b.Min/100*T+shift[p.SlotID] || p.Kcal > b.Max/100*T+shift[p.SlotID] {
					want[p.Date+"/"+w] = true
				}
			}
			got := map[string]bool{}
			for _, is := range ev.Issues {
				if is.Code == "meal_outside_slot_band" {
					assert.Equal(t, planning.SeveritySoft, is.Severity, "S10 is soft")
					assert.Empty(t, is.Options, "S10 carries no options")
					if is.SlotID != nil {
						got[is.Date+"/"+slotWord(s, *is.SlotID)] = true
					}
				}
			}
			assert.Equal(t, want, got, "S10 issues must equal the lunch/dinner plates outside their band")
		})
	}
}

// preset is a named set of bands by slot name (spec v1.1 SB1).
type preset struct {
	name  string
	bands map[string]band
}

var presets = []preset{
	{"Lunch is the biggest meal", map[string]band{"breakfast": {20, 30}, "lunch": {35, 45}, "dinner": {20, 30}, "snack": {5, 15}}},
	{"Dinner is the biggest meal", map[string]band{"breakfast": {20, 30}, "lunch": {20, 30}, "dinner": {35, 45}, "snack": {5, 15}}},
}

func presetShares(p preset, slots []string) map[string]float64 {
	var sum float64
	for _, n := range slots {
		sum += p.bands[n].mid()
	}
	out := map[string]float64{}
	for _, n := range slots {
		out[n] = p.bands[n].mid() / sum
	}
	return out
}

// SB1/SB2/SB3: per preset, on the dev household's day (target 2200 kcal, male
// floor 1500, so the 75 % term 1650 decides): the expected shares with and
// without a snack slot, the light cap of the biggest meal, and the planned
// part's floor when the other main meal is away. The numbers are the spec's;
// the product part needs the band-setting API (CC22).
func TestSB1_PresetsShareCapAndAwayProration(t *testing.T) {
	const target, floor75 = 2200.0, 1650.0
	want := map[string]struct {
		noSnack  map[string]float64
		withSnap map[string]float64
		capSlot  string
	}{
		"Lunch is the biggest meal":  {map[string]float64{"breakfast": 25.0 / 90, "lunch": 40.0 / 90, "dinner": 25.0 / 90}, map[string]float64{"breakfast": 0.25, "lunch": 0.40, "dinner": 0.25}, "lunch"},
		"Dinner is the biggest meal": {map[string]float64{"breakfast": 25.0 / 90, "lunch": 25.0 / 90, "dinner": 40.0 / 90}, map[string]float64{"breakfast": 0.25, "lunch": 0.25, "dinner": 0.40}, "dinner"},
	}
	for _, p := range presets {
		t.Run(p.name, func(t *testing.T) {
			w := want[p.name]
			// without a snack slot the midpoints 25/40/25 sum to 90 and normalise
			ns := presetShares(p, []string{"breakfast", "lunch", "dinner"})
			var sum float64
			for k, v := range w.noSnack {
				assert.InDelta(t, v, ns[k], 1e-9, k)
				sum += ns[k]
			}
			assert.InDelta(t, 1.0, sum, 1e-9)
			// with one snack slot the midpoints sum to exactly 100
			ws := presetShares(p, []string{"breakfast", "lunch", "dinner", "snack"})
			for k, v := range w.withSnap {
				assert.InDelta(t, v, ws[k], 1e-9, k)
			}
			assert.InDelta(t, 0.10, ws["snack"], 1e-9)
			// validation: sum of mins <= 100 <= sum of maxes, with and without a snack
			for _, slots := range [][]string{{"breakfast", "lunch", "dinner"}, {"breakfast", "lunch", "dinner", "snack"}} {
				var mn, mx float64
				for _, n := range slots {
					mn += p.bands[n].Min
					mx += p.bands[n].Max
				}
				assert.LessOrEqual(t, mn, 100.0, slots)
				assert.GreaterOrEqual(t, mx, 100.0, slots)
			}
			// light cap of the biggest meal (no snack): 75 % of its expected kcal
			wantCap := 0.75 * target * ns[w.capSlot]
			assert.InDelta(t, wantCap, 0.75*target*w.noSnack[w.capSlot], 1e-9)
			// away proration on one day: the other main meal is away
			other := "dinner"
			if w.capSlot == "dinner" {
				other = "lunch"
			}
			planned := 1 - ns[other]
			assert.InDelta(t, floor75*planned, floor75*(1-w.noSnack[other]), 1e-9)
			assert.InDelta(t, target*ns[other], target*w.noSnack[other], 1e-9, "away_kcal")
		})
	}
	t.Run("product", func(t *testing.T) {
		t.Skip("runs with -tags spectest,cc22 once the band API exists (CC22): TestSB1cc22_PresetDrivesLightCapAndAwayProration")
	})
}

// RP9 (P-4): an issue listed as resolved carries the figures it had before the write.
func TestRP9_ResolvedIssuesCarryBeforeFigures(t *testing.T) {
	forOps(t, nil, func(t *testing.T, r opRun) {
		var res struct {
			After struct {
				Issues struct {
					Resolved []struct {
						Date   string         `json:"date"`
						Label  string         `json:"label"`
						Params map[string]any `json:"params"`
					} `json:"resolved"`
				} `json:"issues"`
			} `json:"after"`
		}
		_ = json.Unmarshal([]byte(r.Raw), &res)
		before := planning.Evaluate(r.Before, planning.WindowTarget(r.Before))
		n := 0
		for _, is := range res.After.Issues.Resolved {
			tk, ok := is.Params["target_kcal"]
			if !ok || is.Date == "" {
				continue
			}
			for _, d := range before.Days {
				if d.Date == is.Date && d.TargetKcal != nil {
					n++
					assert.InDelta(t, *d.TargetKcal, num(tk), 1.0, "resolved %q on %s must show the before-write target", is.Label, is.Date)
					if lo, ok := is.Params["range_low_kcal"]; ok {
						assert.InDelta(t, d.BandLow, num(lo), 1.0, "and the before-write range")
					}
				}
			}
		}
		if n == 0 {
			t.Skip("no resolved day issue")
		}
	})
}
