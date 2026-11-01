package planning_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
)

type ruleCase struct {
	name   string
	state  func() planning.State
	target func(planning.State) planning.Target
	prop   planning.Proposal
	want   []string
}

func runRule(t *testing.T, rule string, cases []ruleCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := baseState()
			if tc.state != nil {
				s = tc.state()
			}
			got := codes(check(s, tc.target(s), tc.prop), rule)
			want := tc.want
			if want == nil {
				want = []string{}
			}
			assert.ElementsMatch(t, want, got)
		})
	}
}

func satDinner(planning.State) planning.Target { return targetCells(cell(sat, dinner)) }

// B5-AC1: H1
func TestCheck_H1(t *testing.T) {
	noneDinner := func() planning.State {
		s := baseState()
		setMode(&s, cell(sat, dinner), availability.ModeNone)
		s.Sessions = []planning.Session{persistedSession(7, chili, sat, dinner, 1, false)}
		s.Plates = []planning.Plate{persisted(70, sat, dinner, false, chili, 7)}
		return s
	}
	routineBreakfast := func() planning.State {
		s := baseState()
		setMode(&s, cell(sat, breakfast), availability.ModeRoutine)
		return s
	}
	occupiedDinner := func() planning.State {
		s := baseState()
		s.Sessions = []planning.Session{persistedSession(7, curry, sat, dinner, 1, false)}
		s.Plates = []planning.Plate{persisted(70, sat, dinner, false, curry, 7)}
		return s
	}
	runRule(t, "H1", []ruleCase{
		{
			name: "every target cell filled", target: func(planning.State) planning.Target {
				return targetCells(cell(sat, lunch), cell(sat, dinner))
			},
			prop: planning.Proposal{Plates: []planning.ProposedPlate{pplate(sat, lunch, fresh(curry)), pplate(sat, dinner, fresh(chili))}},
		},
		{
			name: "empty plan cell", target: func(planning.State) planning.Target {
				return targetCells(cell(sat, lunch), cell(sat, dinner))
			},
			prop: planning.Proposal{Plates: []planning.ProposedPlate{pplate(sat, lunch, fresh(curry))}},
			want: []string{planning.CodeMissingPlate},
		},
		{
			name: "plate in a none cell", state: noneDinner, target: satDinner,
			want: []string{planning.CodePlateInNoneCell},
		},
		{
			name: "none cell left empty", state: noneDinner, target: func(planning.State) planning.Target {
				return planning.Target{Cells: []plateCellKey{cell(sat, dinner)}, PlateIDs: []int64{70}, SessionIDs: []int64{7}}
			},
		},
		{
			name: "routine cell without its plate", state: routineBreakfast,
			target: func(planning.State) planning.Target { return targetCells(cell(sat, breakfast)) },
			want:   []string{planning.CodeMissingPlate},
		},
		{
			name: "two plates on one cell", state: occupiedDinner, target: satDinner,
			prop: planning.Proposal{Plates: []planning.ProposedPlate{pplate(sat, dinner, fresh(chili))}},
			want: []string{planning.CodeExtraPlate},
		},
	})
}

// B5-AC1: H2
func TestCheck_H2(t *testing.T) {
	satAll := func(planning.State) planning.Target {
		return targetCells(cell(sat, breakfast), cell(sat, lunch), cell(sat, dinner))
	}
	runRule(t, "H2", []ruleCase{
		{
			name: "main and side cooked at one occasion", target: satAll,
			prop: planning.Proposal{
				Sessions: []planning.ProposedSession{session("s1", chili, sat, dinner, 1), session("s2", rice, sat, dinner, 1)},
				Plates:   []planning.ProposedPlate{pplate(sat, dinner, onKey(chili, "s1"), onKey(rice, "s2"))},
			},
		},
		{
			name: "lunch and dinner both cook", target: satAll,
			prop: planning.Proposal{
				Sessions: []planning.ProposedSession{session("s1", chili, sat, dinner, 1), session("s2", curry, sat, lunch, 1)},
				Plates:   []planning.ProposedPlate{pplate(sat, dinner, onKey(chili, "s1")), pplate(sat, lunch, onKey(curry, "s2"))},
			},
			want: []string{planning.CodeTooManyCookOccasions},
		},
		{
			name: "fresh component without a session counts", target: satAll,
			prop: planning.Proposal{
				Sessions: []planning.ProposedSession{session("s1", chili, sat, dinner, 1)},
				Plates:   []planning.ProposedPlate{pplate(sat, dinner, onKey(chili, "s1")), pplate(sat, lunch, fresh(curry))},
			},
			want: []string{planning.CodeTooManyCookOccasions},
		},
		{
			name: "routine plates don't count", target: satAll,
			state: func() planning.State {
				s := baseState()
				setMode(&s, cell(sat, breakfast), availability.ModeRoutine)
				return s
			},
			prop: planning.Proposal{
				Sessions: []planning.ProposedSession{session("s1", chili, sat, dinner, 1), session("s2", oats, sat, breakfast, 1)},
				Plates:   []planning.ProposedPlate{pplate(sat, dinner, onKey(chili, "s1")), pplate(sat, breakfast, onKey(oats, "s2"))},
			},
		},
	})
}

// B5-AC1: H3
func TestCheck_H3(t *testing.T) {
	batch := func(portions, frozen float64) planning.Proposal {
		s := session("s1", chili, sat, dinner, portions)
		s.FrozenPortions = frozen
		return planning.Proposal{
			Sessions: []planning.ProposedSession{s},
			Plates: []planning.ProposedPlate{
				pplate(sat, dinner, onKey(chili, "s1")), pplate(sun, lunch, onKey(chili, "s1")), pplate(sun, dinner, onKey(chili, "s1")),
			},
		}
	}
	freezing := func() planning.State {
		s := baseState()
		s.Settings.FreezingEnabled = true
		return s
	}
	target := func(planning.State) planning.Target {
		return targetCells(cell(sat, dinner), cell(sun, lunch), cell(sun, dinner))
	}
	frozenLate := func(frozen float64) planning.Proposal {
		s := session("s1", chili, sat, dinner, 2)
		s.FrozenPortions = frozen
		return planning.Proposal{
			Sessions: []planning.ProposedSession{s},
			Plates:   []planning.ProposedPlate{pplate(sat, dinner, onKey(chili, "s1")), pplate(wed, dinner, onKey(chili, "s1"))},
		}
	}
	frozenTarget := func(planning.State) planning.Target { return targetCells(cell(sat, dinner), cell(wed, dinner)) }
	runRule(t, "H3", []ruleCase{
		{name: "portions cover the links", target: target, prop: batch(3, 0)},
		{name: "deficit", target: target, prop: batch(2, 0), want: []string{"deficit"}},
		{name: "frozen portions cover frozen eating", state: freezing, target: frozenTarget, prop: frozenLate(1)},
		{name: "frozen deficit", state: freezing, target: frozenTarget, prop: frozenLate(0), want: []string{"frozen_deficit"}},
	})
}

// B5-AC1: H4
func TestCheck_H4(t *testing.T) {
	leftover := func(cookDate string, cookSlot int64, eatDate string, eatSlot int64) planning.Proposal {
		return planning.Proposal{
			Sessions: []planning.ProposedSession{session("s1", chili, cookDate, cookSlot, 2)},
			Plates:   []planning.ProposedPlate{pplate(cookDate, cookSlot, onKey(chili, "s1")), pplate(eatDate, eatSlot, onKey(chili, "s1"))},
		}
	}
	all := windowTarget
	runRule(t, "H4", []ruleCase{
		{name: "same-day lunch to dinner allowed", target: all, prop: leftover(sat, lunch, sat, dinner)},
		{name: "dinner to next lunch allowed", target: all, prop: leftover(sat, dinner, sun, lunch)},
		{name: "dinner to same-day lunch rejected", target: all, prop: leftover(sat, dinner, sat, lunch), want: []string{"leftover_before_cook"}},
		{name: "on use-by day allowed", target: all, prop: leftover(sat, dinner, tue, dinner)},
		{name: "past keep days rejected", target: all, prop: leftover(sat, dinner, wed, lunch), want: []string{"past_keep_days"}},
	})
}

func TestCheck_H4IssueNamesThePlateAndSession(t *testing.T) {
	s := baseState()
	s.Sessions = []planning.Session{persistedSession(7, chili, sat, dinner, 2, false)}
	s.Plates = []planning.Plate{persisted(70, sat, dinner, false, chili, 7), persisted(71, sat, lunch, false, chili, 7)}

	got := only(planning.Check(s, windowTarget(s)), "H4")

	require.Len(t, got, 1)
	assert.Equal(t, planning.Severity("violation"), got[0].Severity)
	require.NotNil(t, got[0].PlateID)
	assert.Equal(t, int64(71), *got[0].PlateID)
	require.NotNil(t, got[0].SessionID)
	assert.Equal(t, int64(7), *got[0].SessionID)
	assert.Equal(t, sat, got[0].Date)
	assert.Equal(t, lunch, *got[0].SlotID)
}

// B5-AC1: H5
func TestCheck_H5(t *testing.T) {
	vegetarian := func() planning.State {
		s := baseState()
		s.Profile.DietType = profile.DietVegetarian
		return s
	}
	meatCap := func(n int) func() planning.State {
		return func() planning.State {
			s := baseState()
			s.Profile.MeatDaysPerWeekMax = intp(n)
			return s
		}
	}
	twoMeatDays := planning.Proposal{Plates: []planning.ProposedPlate{pplate(sat, dinner, fresh(chili)), pplate(sun, dinner, fresh(chili))}}
	runRule(t, "H5", []ruleCase{
		{
			name: "vegetarian recipe for a vegetarian", state: vegetarian, target: satDinner,
			prop: planning.Proposal{Plates: []planning.ProposedPlate{pplate(sat, dinner, fresh(soup), fresh(rice))}},
		},
		{
			name: "omnivore recipe for a vegetarian", state: vegetarian, target: satDinner,
			prop: planning.Proposal{Plates: []planning.ProposedPlate{pplate(sat, dinner, fresh(chili))}},
			want: []string{planning.CodeDietIncompatible},
		},
		{
			name: "unknown class warns", target: satDinner,
			prop: planning.Proposal{Plates: []planning.ProposedPlate{pplate(sat, dinner, fresh(stew))}},
			want: []string{planning.CodeDietUnknown},
		},
		{name: "meat days within the cap", state: meatCap(2), target: windowTarget, prop: twoMeatDays},
		{
			name: "meat days over the cap", state: meatCap(1), target: windowTarget, prop: twoMeatDays,
			want: []string{planning.CodeMeatDaysExceeded},
		},
	})
}

func TestCheck_H5UnknownIsAWarning(t *testing.T) {
	got := only(check(baseState(), satDinner(baseState()), planning.Proposal{Plates: []planning.ProposedPlate{pplate(sat, dinner, fresh(stew))}}), "H5")
	require.Len(t, got, 1)
	assert.Equal(t, planning.SeverityWarning, got[0].Severity)
}

// B5-AC1: H6
func TestCheck_H6(t *testing.T) {
	withConfirmed := func() planning.State {
		s := baseState()
		s.Sessions = []planning.Session{persistedSession(7, chili, sat, dinner, 1, true)}
		s.Plates = []planning.Plate{persisted(70, sat, dinner, true, chili, 7)}
		return s
	}
	runRule(t, "H6", []ruleCase{
		{name: "target holds only drafts", state: withConfirmed, target: func(planning.State) planning.Target {
			return targetCells(cell(sun, dinner))
		}, prop: planning.Proposal{
			Sessions: []planning.ProposedSession{session("s1", curry, sun, dinner, 1)},
			Plates:   []planning.ProposedPlate{pplate(sun, dinner, onKey(curry, "s1"))},
		}},
		{name: "target names a confirmed plate and session", state: withConfirmed, target: func(planning.State) planning.Target {
			return planning.Target{Cells: []plateCellKey{cell(sat, dinner)}, PlateIDs: []int64{70}, SessionIDs: []int64{7}}
		}, want: []string{planning.CodeConfirmedInTarget, planning.CodeConfirmedInTarget}},
		{name: "proposed session cooks outside the target", target: func(planning.State) planning.Target {
			return targetCells(cell(sun, lunch))
		}, prop: planning.Proposal{
			Sessions: []planning.ProposedSession{session("s1", chili, sat, dinner, 1)},
			Plates:   []planning.ProposedPlate{pplate(sun, lunch, onKey(chili, "s1"))},
		}, want: []string{planning.CodeSessionOutsideTarget}},
	})
}

func TestMerge_KeepsConfirmedRowsTheTargetNames(t *testing.T) {
	s := baseState()
	s.Sessions = []planning.Session{persistedSession(7, chili, sat, dinner, 1, true), persistedSession(8, curry, sun, dinner, 1, false)}
	s.Plates = []planning.Plate{persisted(70, sat, dinner, true, chili, 7), persisted(80, sun, dinner, false, curry, 8)}
	tgt := planning.Target{Cells: []plateCellKey{cell(sat, dinner), cell(sun, dinner)}, PlateIDs: []int64{70, 80}, SessionIDs: []int64{7, 8}}

	m := planning.Merge(s, tgt, planning.Proposal{Plates: []planning.ProposedPlate{pplate(sun, dinner, fresh(soup))}})

	require.Len(t, m.Plates, 2)
	assert.Equal(t, int64(70), m.Plates[0].ID, "confirmed plate kept")
	assert.Equal(t, int64(-1), m.Plates[1].ID, "proposed plates get negative ids")
	assert.True(t, m.Plates[1].Proposed())
	assert.InDelta(t, 1.0, m.Plates[1].Eaters, 1e-9, "eaters default to household portions")
	require.Len(t, m.Sessions, 1)
	assert.Equal(t, int64(7), m.Sessions[0].ID)
	assert.Len(t, s.Plates, 2, "input state unchanged")
}

func TestMerge_DroppedSessionTakesItsComponentsAndPlateIDMerges(t *testing.T) {
	s := baseState()
	s.Sessions = []planning.Session{persistedSession(7, chili, sat, dinner, 1, false), persistedSession(8, rice, sat, dinner, 1, false)}
	s.Plates = []planning.Plate{{
		ID: 70, Date: day(sat), SlotID: dinner, Eaters: 1,
		Components: []planning.Component{{FoodID: chili, Portions: f64(1), SessionID: 7}, {FoodID: rice, Portions: f64(1), SessionID: 8}},
	}}
	tgt := planning.Target{Cells: []plateCellKey{cell(sat, dinner)}, SessionIDs: []int64{8}}
	p := planning.Proposal{
		Sessions: []planning.ProposedSession{session("s1", curry, sat, dinner, 1)},
		Plates:   []planning.ProposedPlate{{PlateID: 70, Date: day(sat), SlotID: dinner, Components: []planning.ProposedComponent{onKey(curry, "s1")}}},
	}

	m := planning.Merge(s, tgt, p)

	require.Len(t, m.Plates, 1, "the proposal merges into plate 70")
	assert.Equal(t, int64(70), m.Plates[0].ID)
	require.Len(t, m.Plates[0].Components, 2)
	assert.Equal(t, chili, m.Plates[0].Components[0].FoodID, "main kept")
	assert.Equal(t, "s1", m.Plates[0].Components[1].SessionKey, "side replaced")
	assert.Len(t, s.Plates[0].Components, 2, "input state unchanged")
	assert.Empty(t, planning.Check(m, tgt))
}

func TestCheck_RoutinePlateMayHoldLeaves(t *testing.T) {
	s := baseState()
	setMode(&s, cell(mon, breakfast), availability.ModeRoutine)
	s.Plates = []planning.Plate{{ID: -1000, Date: day(mon), SlotID: breakfast, Eaters: 1, Components: []planning.Component{{FoodID: egg, Grams: f64(60)}}}}
	assert.Empty(t, planning.Check(s, targetCells(cell(mon, breakfast))))
}

// B5-AC1: H7
func TestCheck_H7(t *testing.T) {
	withConfirmed := func() planning.State {
		s := baseState()
		s.Sessions = []planning.Session{persistedSession(7, chili, sat, dinner, 3, true)}
		s.Plates = []planning.Plate{persisted(70, sat, dinner, true, chili, 7)}
		return s
	}
	sunDinner := func(planning.State) planning.Target { return targetCells(cell(sun, dinner)) }
	one := func(comps ...planning.ProposedComponent) planning.Proposal {
		return planning.Proposal{Plates: []planning.ProposedPlate{pplate(sun, dinner, comps...)}}
	}
	runRule(t, "H7", []ruleCase{
		{name: "valid proposal", state: withConfirmed, target: sunDinner, prop: one(onID(chili, 7))},
		{name: "unknown food", target: sunDinner, prop: one(fresh(999)), want: []string{planning.CodeUnknownFood}},
		{name: "session of a leaf", target: sunDinner, prop: planning.Proposal{
			Sessions: []planning.ProposedSession{session("s1", egg, sun, dinner, 1)},
			Plates:   []planning.ProposedPlate{pplate(sun, dinner, onKey(egg, "s1"))},
		}, want: []string{planning.CodeNotComposed, planning.CodeNotComposed}},
		{name: "plate without components", target: sunDinner, prop: one(), want: []string{planning.CodePlateWithoutComponent}},
		{name: "plate outside the target", target: sunDinner, prop: planning.Proposal{
			Plates: []planning.ProposedPlate{pplate(sun, dinner, fresh(curry)), pplate(mon, dinner, fresh(soup))},
		}, want: []string{planning.CodePlateOutsideTarget}},
		{
			name: "plate on a confirmed cell", state: withConfirmed, target: func(planning.State) planning.Target {
				return targetCells(cell(sat, dinner))
			}, prop: planning.Proposal{Plates: []planning.ProposedPlate{pplate(sat, dinner, fresh(curry))}},
			want: []string{planning.CodePlateOnConfirmedCell},
		},
		{name: "unknown session key", target: sunDinner, prop: one(onKey(chili, "nope")), want: []string{planning.CodeUnknownSession}},
		{name: "unknown session id", target: sunDinner, prop: one(onID(chili, 404)), want: []string{planning.CodeUnknownSession}},
		{
			name: "link to a session of another food", state: withConfirmed, target: sunDinner, prop: one(onID(curry, 7)),
			want: []string{planning.CodeSessionFoodMismatch},
		},
		{
			name: "zero portions", target: sunDinner, prop: one(planning.ProposedComponent{FoodID: curry}),
			want: []string{planning.CodeInvalidPortions},
		},
		{name: "duplicate session key", target: sunDinner, prop: planning.Proposal{
			Sessions: []planning.ProposedSession{session("s1", curry, sun, dinner, 1), session("s1", curry, sun, dinner, 1)},
			Plates:   []planning.ProposedPlate{pplate(sun, dinner, onKey(curry, "s1"))},
		}, want: []string{planning.CodeDuplicateSessionKey}},
		{name: "no eaters", state: func() planning.State {
			s := baseState()
			s.Profile.HouseholdPortions = 0
			return s
		}, target: sunDinner, prop: one(fresh(curry)), want: []string{planning.CodeInvalidPortions}},
	})
}

func TestCheck_H7DuplicateKeysInFirstSeenOrder(t *testing.T) {
	keys := []string{"s9", "s1", "s5", "s3"}
	var p planning.Proposal
	for _, k := range keys {
		p.Sessions = append(p.Sessions, session(k, curry, sun, dinner, 1), session(k, curry, sun, dinner, 1), session(k, curry, sun, dinner, 1))
	}
	got := []any{}
	for _, is := range only(check(baseState(), targetCells(cell(sun, dinner)), p), "H7") {
		if is.Code == planning.CodeDuplicateSessionKey {
			got = append(got, is.Params["session_key"])
		}
	}
	assert.Equal(t, []any{"s9", "s1", "s5", "s3"}, got)
}

// manyMains adds n vegan mains never eaten.
func manyMains(s *planning.State, n int) {
	for i := range n {
		id := int64(100 + i)
		s.Foods[id] = recipe(id, "Main", food.RoleMain, "vegan", 500)
	}
}

// B5-AC1: H8
func TestCheck_H8(t *testing.T) {
	eatenLastWeek := func(mains int) func() planning.State {
		return func() planning.State {
			s := baseState()
			last := day(sat).AddDate(0, 0, -7)
			for _, id := range []int64{chili, curry, soup, oats, stew} {
				f := s.Foods[id]
				f.LastEaten = &last
				s.Foods[id] = f
			}
			manyMains(&s, mains)
			return s
		}
	}
	cooks := func(ss ...planning.ProposedSession) planning.Proposal {
		p := planning.Proposal{Sessions: ss}
		for _, x := range ss {
			p.Plates = append(p.Plates, pplate(x.CookDate.Format(time.DateOnly), x.CookSlotID, onKey(x.FoodID, x.Key)))
		}
		return p
	}
	runRule(t, "H8", []ruleCase{
		{
			name: "not eaten recently", state: eatenLastWeek(5), target: windowTarget,
			prop: cooks(session("s1", 100, sat, dinner, 1)),
		},
		{
			name: "eaten last week", state: eatenLastWeek(5), target: windowTarget,
			prop: cooks(session("s1", chili, sat, dinner, 1)), want: []string{planning.CodeVarietyRepeat},
		},
		{name: "eaten before the variety weeks", state: func() planning.State {
			s := baseState()
			long := day(sat).AddDate(0, 0, -22)
			f := s.Foods[chili]
			f.LastEaten = &long
			s.Foods[chili] = f
			return s
		}, target: windowTarget, prop: cooks(session("s1", chili, sat, dinner, 1))},
		{
			name: "same main cooked twice in the window", state: eatenLastWeek(5), target: windowTarget,
			prop: cooks(session("s1", 100, sat, dinner, 1), session("s2", 100, tue, dinner, 1)),
			want: []string{planning.CodeVarietyRepeat},
		},
		{
			name: "fresh main eaten last week", state: eatenLastWeek(5), target: windowTarget,
			prop: planning.Proposal{Plates: []planning.ProposedPlate{pplate(sat, dinner, fresh(chili))}},
			want: []string{planning.CodeVarietyRepeat},
		},
		{
			name: "same fresh main twice in window", state: eatenLastWeek(5), target: windowTarget,
			prop: planning.Proposal{Plates: []planning.ProposedPlate{pplate(sat, dinner, fresh(100)), pplate(tue, dinner, fresh(100))}},
			want: []string{planning.CodeVarietyRepeat},
		},
		{
			name: "fresh main after a session of it", state: eatenLastWeek(5), target: windowTarget,
			prop: planning.Proposal{
				Sessions: []planning.ProposedSession{session("s1", 100, sat, dinner, 1)},
				Plates:   []planning.ProposedPlate{pplate(sat, dinner, onKey(100, "s1")), pplate(tue, dinner, fresh(100))},
			},
			want: []string{planning.CodeVarietyRepeat},
		},
		{
			name: "routine breakfasts repeat freely", state: func() planning.State {
				s := eatenLastWeek(5)()
				setMode(&s, cell(sat, breakfast), availability.ModeRoutine)
				setMode(&s, cell(sun, breakfast), availability.ModeRoutine)
				return s
			}, target: windowTarget,
			prop: planning.Proposal{Plates: []planning.ProposedPlate{pplate(sat, breakfast, fresh(oats)), pplate(sun, breakfast, fresh(oats))}},
		},
		{
			name: "sides repeat freely", state: eatenLastWeek(5), target: windowTarget,
			prop: cooks(session("s1", rice, sat, dinner, 1), session("s2", rice, sun, dinner, 1)),
		},
		{
			name: "relaxed when the library is too small", state: eatenLastWeek(1), target: windowTarget,
			prop: cooks(session("s1", 100, sat, dinner, 1), session("s2", chili, sun, dinner, 1)),
			want: []string{planning.CodeVarietyRelaxed},
		},
	})
}

func TestCheck_H8RelaxedIsAWarning(t *testing.T) {
	s := baseState()
	last := day(sat).AddDate(0, 0, -7)
	for id, f := range s.Foods {
		f.LastEaten = &last
		s.Foods[id] = f
	}
	got := only(check(s, windowTarget(s), planning.Proposal{
		Sessions: []planning.ProposedSession{session("s1", chili, sat, dinner, 1)},
		Plates:   []planning.ProposedPlate{pplate(sat, dinner, onKey(chili, "s1"))},
	}), "H8")
	require.Len(t, got, 1)
	assert.Equal(t, planning.SeverityWarning, got[0].Severity)
	assert.Equal(t, "s1", got[0].Params["session_key"])
}

// A side swap keeps its plate's main: H8 judges only what the run chose, so
// the kept main is not flagged, while the board (persisted data) still is.
func TestCheck_H8JudgesOnlyWhatARunChose(t *testing.T) {
	const side = int64(300)
	s := baseState()
	last := day(sat).AddDate(0, 0, -7)
	f := s.Foods[chili]
	f.LastEaten = &last
	s.Foods[chili] = f
	manyMains(&s, 5)
	s.Foods[side] = recipe(side, "Salad", food.RoleSideVeg, "vegan", 100)
	s.Sessions = []planning.Session{
		persistedSession(1, chili, sun, dinner, 1, false),
		persistedSession(2, rice, sun, dinner, 1, false),
	}
	s.Plates = []planning.Plate{persisted(1, sun, dinner, false, chili, 1)}
	s.Plates[0].Components = append(s.Plates[0].Components, planning.Component{FoodID: rice, Portions: f64(1), SessionID: 2})

	swap := targetCells(cell(sun, dinner))
	swap.SessionIDs = []int64{2}
	got := only(check(s, swap, planning.Proposal{
		Sessions: []planning.ProposedSession{session("s1", side, sun, dinner, 1)},
		Plates:   []planning.ProposedPlate{{PlateID: 1, Date: day(sun), SlotID: dinner, Components: []planning.ProposedComponent{onKey(side, "s1")}}},
	}), "H8")
	assert.Empty(t, got, "the kept main is not the run's choice")

	board := only(planning.Check(s, windowTarget(s)), "H8")
	require.Len(t, board, 1)
	assert.Equal(t, planning.CodeVarietyRepeat, board[0].Code)
}

// A single-plate run relaxes H8 the way the window does: the window's
// unconfirmed cooks, not the target's, are weighed against the library, and
// the board agrees on the saved result.
func TestCheck_H8RelaxesOnTheWindow(t *testing.T) {
	s := baseState()
	last := day(sat).AddDate(0, 0, -7)
	for id, f := range s.Foods {
		f.LastEaten = &last
		s.Foods[id] = f
	}
	manyMains(&s, 1) // one eligible recipe
	freshPlate := func(id int64, date string, foodID int64) planning.Plate {
		return planning.Plate{ID: id, Date: day(date), SlotID: dinner, Eaters: 1, Components: []planning.Component{{FoodID: foodID, Portions: f64(1)}}}
	}
	s.Plates = []planning.Plate{freshPlate(1, sat, 100)}

	got := only(check(s, targetCells(cell(sun, dinner)), planning.Proposal{
		Plates: []planning.ProposedPlate{pplate(sun, dinner, fresh(chili))},
	}), "H8")
	require.Len(t, got, 1)
	assert.Equal(t, planning.CodeVarietyRelaxed, got[0].Code)
	assert.Equal(t, planning.SeverityWarning, got[0].Severity)

	s.Plates = append(s.Plates, freshPlate(2, sun, chili))
	board := only(planning.Check(s, windowTarget(s)), "H8")
	require.Len(t, board, 1)
	assert.Equal(t, planning.CodeVarietyRelaxed, board[0].Code, "the board agrees")
}

func TestIssue_JSONShape(t *testing.T) {
	s := baseState()
	got := only(check(s, targetCells(cell(sat, lunch), cell(sat, dinner)), planning.Proposal{
		Sessions: []planning.ProposedSession{session("s1", chili, sat, dinner, 1), session("s2", curry, sat, lunch, 1)},
		Plates:   []planning.ProposedPlate{pplate(sat, dinner, onKey(chili, "s1")), pplate(sat, lunch, onKey(curry, "s2"))},
	}), "H2")
	require.Len(t, got, 1)
	b, err := json.Marshal(got[0])
	require.NoError(t, err)
	assert.JSONEq(t, `{"rule":"H2","code":"too_many_cook_occasions","severity":"violation","date":"2026-11-07","params":{"count":2,"max":1}}`, string(b))
}

func TestCheck_CleanWindowHasNoIssues(t *testing.T) {
	s := baseState()
	setMode(&s, cell(sat, breakfast), availability.ModeNone)
	tgt := targetCells(cell(sat, breakfast), cell(sat, lunch), cell(sat, dinner), cell(sun, lunch))
	got := check(s, tgt, planning.Proposal{
		Sessions: []planning.ProposedSession{session("s1", chili, sat, dinner, 2), session("s2", curry, sat, dinner, 1)},
		Plates: []planning.ProposedPlate{
			pplate(sat, dinner, onKey(chili, "s1")), pplate(sun, lunch, onKey(chili, "s1")), pplate(sat, lunch, onKey(curry, "s2")),
		},
	})
	// Curry cooks at dinner but is eaten at lunch: only H4 fires.
	assert.Equal(t, []string{"leftover_before_cook"}, codes(got, "H4"))
	assert.Len(t, got, 1)
}
