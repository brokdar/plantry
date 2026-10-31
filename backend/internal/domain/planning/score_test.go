package planning_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

func score(s planning.State, t planning.Target, p planning.Proposal) planning.Score {
	m := planning.Merge(s, t, p)
	return planning.ComputeScore(m, t, planning.Check(m, t))
}

// B5-AC2
func TestScore_Coverage(t *testing.T) {
	filled := func(skip int) planning.Proposal {
		var p planning.Proposal
		for i, c := range baseState().Cells {
			if i != skip {
				p.Plates = append(p.Plates, planning.ProposedPlate{Date: c.Date, SlotID: c.SlotID, Components: []planning.ProposedComponent{fresh(curry)}})
			}
		}
		return p
	}
	allNone := func() planning.State {
		s := baseState()
		for i := range s.Cells {
			s.Cells[i].Mode = availability.ModeNone
		}
		return s
	}
	tests := []struct {
		name  string
		state planning.State
		prop  planning.Proposal
		want  float64
	}{
		{"20 of 21", baseState(), filled(4), 20.0 / 21},
		{"all filled", baseState(), filled(-1), 1},
		{"nothing filled", baseState(), planning.Proposal{}, 0},
		{"no plan cells", allNone(), planning.Proposal{}, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := score(tc.state, windowTarget(tc.state), tc.prop).Coverage
			assert.InDelta(t, tc.want, got, 1e-9)
		})
	}
	assert.InDelta(t, 0.952, score(baseState(), windowTarget(baseState()), filled(0)).Coverage, 5e-4)
}

// B5-AC2
func TestScore_HardViolations(t *testing.T) {
	tests := []struct {
		name   string
		issues []planning.Issue
		want   map[string]int
	}{
		{"none", nil, map[string]int{}},
		{"counted by rule, warnings excluded", []planning.Issue{
			{Rule: "H2", Severity: planning.SeverityViolation},
			{Rule: "H2", Severity: planning.SeverityViolation},
			{Rule: "H4", Severity: planning.SeverityViolation},
			{Rule: "H5", Severity: planning.SeverityWarning},
		}, map[string]int{"H2": 2, "H4": 1}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := baseState()
			assert.Equal(t, tc.want, planning.ComputeScore(s, windowTarget(s), tc.issues).HardViolations)
		})
	}
}

// B5-AC2
func TestScore_MacroDeviation(t *testing.T) {
	withTargets := func(kcal, protein *float64) planning.State {
		s := baseState()
		s.Profile.KcalTarget = kcal
		s.Profile.ProteinPct = protein
		return s
	}
	twoDays := planning.Proposal{Plates: []planning.ProposedPlate{
		// Sat: 600 + 500 = 1100 kcal (45 % under 2000).
		pplate(sat, lunch, fresh(chili)), pplate(sat, dinner, fresh(curry)),
		// Sun: 4 × 600 = 2400 kcal (20 % over).
		{Date: day(sun), SlotID: dinner, Components: []planning.ProposedComponent{{FoodID: chili, Portions: 4}}},
	}}
	satOnly := planning.Proposal{Plates: []planning.ProposedPlate{pplate(sat, lunch, fresh(chili)), pplate(sat, dinner, fresh(curry))}}
	tests := []struct {
		name          string
		state         planning.State
		prop          planning.Proposal
		daily, weekly *float64
	}{
		{"no kcal target", withTargets(nil, nil), twoDays, nil, nil},
		{"no planned days", withTargets(f64(2000), nil), planning.Proposal{}, nil, nil},
		// daily = (45 + 20) / 2; weekly: average 1750 kcal = 12.5 %.
		{"kcal over two days", withTargets(f64(2000), nil), twoDays, f64(32.5), f64(12.5)},
		// protein target 100 g; Sat protein 30 + 25 = 55 g (45 %).
		{"kcal and protein", withTargets(f64(2000), f64(20)), satOnly, f64(45), f64(45)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := score(tc.state, windowTarget(tc.state), tc.prop).MacroDeviation
			if tc.daily == nil {
				assert.Nil(t, got.DailyPct)
				assert.Nil(t, got.WeeklyPct)
				return
			}
			require.NotNil(t, got.DailyPct)
			require.NotNil(t, got.WeeklyPct)
			assert.InDelta(t, *tc.daily, *got.DailyPct, 1e-9)
			assert.InDelta(t, *tc.weekly, *got.WeeklyPct, 1e-9)
		})
	}
}

// B5-AC2
func TestScore_CookOccasions(t *testing.T) {
	tests := []struct {
		name     string
		prop     planning.Proposal
		satN     int
		sunN     int
		prepared int
	}{
		{"fresh only", planning.Proposal{Plates: []planning.ProposedPlate{pplate(sat, dinner, fresh(chili))}}, 1, 0, 0},
		{"batch with leftovers on two days", planning.Proposal{
			Sessions: []planning.ProposedSession{session("s1", chili, sat, dinner, 3)},
			Plates: []planning.ProposedPlate{
				pplate(sat, dinner, onKey(chili, "s1")), pplate(sun, lunch, onKey(chili, "s1")), pplate(mon, lunch, onKey(chili, "s1")),
			},
		}, 1, 0, 2},
		{"two occasions on one day", planning.Proposal{Plates: []planning.ProposedPlate{
			pplate(sat, lunch, fresh(curry)), pplate(sat, dinner, fresh(chili)),
		}}, 2, 0, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := baseState()
			got := score(s, windowTarget(s), tc.prop).CookOccasions
			assert.Len(t, got.PerDay, 7)
			assert.Equal(t, tc.satN, got.PerDay[sat])
			assert.Equal(t, tc.sunN, got.PerDay[sun])
			assert.Equal(t, tc.prepared, got.DaysWithPrepared)
		})
	}
}

// B5-AC2
func TestScore_Variety(t *testing.T) {
	last := day(sat).AddDate(0, 0, -7)
	s := baseState()
	f := s.Foods[chili]
	f.LastEaten = &last
	s.Foods[chili] = f
	manyMains(&s, 5)

	tests := []struct {
		name string
		prop planning.Proposal
		want planning.Variety
	}{
		{"new mains, side ignored", planning.Proposal{Plates: []planning.ProposedPlate{
			pplate(sat, dinner, fresh(curry), fresh(rice)), pplate(sun, dinner, fresh(soup)),
		}}, planning.Variety{Distinct: 2, NewRecipes: 2}},
		{"a recent main is a repeat", planning.Proposal{
			Sessions: []planning.ProposedSession{session("s1", chili, sat, dinner, 1), session("s2", curry, sun, dinner, 1)},
			Plates:   []planning.ProposedPlate{pplate(sat, dinner, onKey(chili, "s1")), pplate(sun, dinner, onKey(curry, "s2"))},
		}, planning.Variety{Distinct: 2, Repeats: 1, NewRecipes: 1}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, score(s, windowTarget(s), tc.prop).Variety)
		})
	}
}

// B5-AC2
func TestScore_PreferenceFit(t *testing.T) {
	s := baseState()
	fav := s.Foods[chili]
	fav.Favorite = true
	s.Foods[chili] = fav
	liked := s.Foods[curry]
	liked.LikeScore = 2
	s.Foods[curry] = liked
	disliked := s.Foods[rice]
	disliked.LikeScore = -1
	s.Foods[rice] = disliked

	tests := []struct {
		name string
		prop planning.Proposal
		want float64
	}{
		{"neutral", planning.Proposal{Plates: []planning.ProposedPlate{pplate(sat, dinner, fresh(soup))}}, 0},
		{"favorite + liked − disliked", planning.Proposal{Plates: []planning.ProposedPlate{
			pplate(sat, dinner, fresh(chili), fresh(rice)), pplate(sun, dinner, fresh(curry)),
		}}, 2},
		{"leftovers count once per session", planning.Proposal{
			Sessions: []planning.ProposedSession{session("s1", chili, sat, dinner, 2)},
			Plates:   []planning.ProposedPlate{pplate(sat, dinner, onKey(chili, "s1")), pplate(sun, lunch, onKey(chili, "s1"))},
		}, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.InDelta(t, tc.want, score(s, windowTarget(s), tc.prop).PreferenceFit, 1e-9)
		})
	}
}

// B5-AC2
func TestScore_WastePortions(t *testing.T) {
	batch := func(portions, frozen float64) planning.Proposal {
		ss := session("s1", chili, sat, dinner, portions)
		ss.FrozenPortions = frozen
		return planning.Proposal{
			Sessions: []planning.ProposedSession{ss},
			Plates: []planning.ProposedPlate{
				pplate(sat, dinner, onKey(chili, "s1")), pplate(sun, lunch, onKey(chili, "s1")), pplate(sun, dinner, onKey(chili, "s1")),
			},
		}
	}
	tests := []struct {
		name string
		prop planning.Proposal
		want float64
	}{
		{"exact", batch(3, 0), 0},
		{"unfrozen surplus", batch(5, 0), 2},
		{"frozen surplus is not waste", batch(6, 2), 1},
		{"all surplus frozen", batch(5, 2), 0},
		{"fresh component without a session", planning.Proposal{Plates: []planning.ProposedPlate{pplate(sat, dinner, fresh(curry))}}, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := baseState()
			assert.InDelta(t, tc.want, score(s, windowTarget(s), tc.prop).WastePortions, 1e-9)
		})
	}
}

// B5-AC2
func TestScore_UnknownDataShare(t *testing.T) {
	s := baseState()
	incomplete := s.Foods[soup]
	incomplete.NutritionComplete = false
	s.Foods[soup] = incomplete

	tests := []struct {
		name string
		prop planning.Proposal
		want float64
	}{
		{"no plates", planning.Proposal{}, 0},
		{"all known", planning.Proposal{Plates: []planning.ProposedPlate{pplate(sat, dinner, fresh(chili))}}, 0},
		{"unknown class and incomplete nutrition", planning.Proposal{Plates: []planning.ProposedPlate{
			pplate(sat, dinner, fresh(chili)), pplate(sun, dinner, fresh(stew)), pplate(mon, dinner, fresh(soup)), pplate(tue, dinner, fresh(chili)),
		}}, 2.0 / 3},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.InDelta(t, tc.want, score(s, windowTarget(s), tc.prop).UnknownDataShare, 1e-9)
		})
	}
}

func TestScore_JSONHasEveryComponent(t *testing.T) {
	s := baseState()
	b, err := json.Marshal(score(s, windowTarget(s), planning.Proposal{}))
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(b, &m))
	for _, k := range []string{"coverage", "hard_violations", "macro_deviation", "cook_occasions", "variety", "preference_fit", "waste_portions", "unknown_data_share"} {
		assert.Contains(t, m, k)
	}
}
