package planning_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

// VAR-3 / AC-12 (H8 part): a recipe and its variants are one dish for H8.

const (
	chiliVariant int64 = 700 // a variant of chili (group 1)
	chiliHidden  int64 = 701 // an archived substitution variant of chili
)

func withChiliGroup(eaten func(*planning.State)) func() planning.State {
	return func() planning.State {
		s := baseState()
		for _, id := range []int64{chili, chiliVariant, chiliHidden} {
			f, ok := s.Foods[id]
			if !ok {
				f = recipe(id, "Chili variant", food.RoleMain, "omnivore", 600)
			}
			f.VariantGroup = 1
			s.Foods[id] = f
		}
		h := s.Foods[chiliHidden]
		h.Archived = true
		s.Foods[chiliHidden] = h
		manyMains(&s, 5)
		if eaten != nil {
			eaten(&s)
		}
		planning.ShareGroupHistory(s.Foods)
		return s
	}
}

func TestCheck_H8VariantGroupIsOneDish(t *testing.T) {
	last := day(sat).AddDate(0, 0, -7)
	eat := func(id int64) func(*planning.State) {
		return func(s *planning.State) {
			f := s.Foods[id]
			f.LastEaten = &last
			s.Foods[id] = f
		}
	}
	cook := func(ss ...planning.ProposedSession) planning.Proposal {
		p := planning.Proposal{Sessions: ss}
		for _, x := range ss {
			p.Plates = append(p.Plates, pplate(x.CookDate.Format(time.DateOnly), x.CookSlotID, onKey(x.FoodID, x.Key)))
		}
		return p
	}
	runRule(t, "H8", []ruleCase{
		{
			name: "variant of a recipe eaten last week", state: withChiliGroup(eat(chili)), target: windowTarget,
			prop: cook(session("s1", chiliVariant, sat, dinner, 1)), want: []string{planning.CodeVarietyRepeat},
		},
		{
			name: "recipe after its variant was eaten", state: withChiliGroup(eat(chiliVariant)), target: windowTarget,
			prop: cook(session("s1", chili, sat, dinner, 1)), want: []string{planning.CodeVarietyRepeat},
		},
		{
			name: "hidden variant's history counts for the group", state: withChiliGroup(eat(chiliHidden)), target: windowTarget,
			prop: cook(session("s1", chili, sat, dinner, 1)), want: []string{planning.CodeVarietyRepeat},
		},
		{
			name: "recipe and variant cooked in one window", state: withChiliGroup(nil), target: windowTarget,
			prop: cook(session("s1", chili, sat, dinner, 1), session("s2", chiliVariant, tue, dinner, 1)),
			want: []string{planning.CodeVarietyRepeat},
		},
		{
			name: "group not eaten: no repeat", state: withChiliGroup(nil), target: windowTarget,
			prop: cook(session("s1", chiliVariant, sat, dinner, 1)),
		},
		{
			name: "an unrelated main is no repeat", state: withChiliGroup(eat(chili)), target: windowTarget,
			prop: cook(session("s1", 100, sat, dinner, 1)),
		},
	})
}

func TestShareGroupHistory(t *testing.T) {
	old, newer := day(sat).AddDate(0, 0, -20), day(sat).AddDate(0, 0, -3)
	foods := map[int64]planning.FoodFacts{
		1: {ID: 1, VariantGroup: 5, LastEaten: &old},
		2: {ID: 2, VariantGroup: 5, LastEaten: &newer, Archived: true},
		3: {ID: 3, VariantGroup: 5},
		4: {ID: 4},
		5: {ID: 5, VariantGroup: 6},
	}
	planning.ShareGroupHistory(foods)
	for _, id := range []int64{1, 2, 3} {
		assert.Equal(t, &newer, foods[id].LastEaten, id)
	}
	assert.Nil(t, foods[4].LastEaten)
	assert.Nil(t, foods[5].LastEaten)
}

// The eligible count counts a variant group once: a library of one dish and
// its variants cannot avoid a repeat of the dish, so H8 relaxes.
func TestCheck_H8CountsAVariantGroupOnce(t *testing.T) {
	s := baseState()
	for id, f := range s.Foods {
		if f.Composed() && f.Role == food.RoleMain {
			f.VariantGroup = 1
			s.Foods[id] = f
		}
	}
	// Two cooks, one unrepeated dish (the group): relaxed. Five distinct mains would not be.
	last := day(sat).AddDate(0, 0, -7)
	f := s.Foods[curry]
	f.LastEaten = &last
	s.Foods[curry] = f
	s.Foods[oats] = func() planning.FoodFacts { o := s.Foods[oats]; o.LastEaten = &last; return o }()
	planning.ShareGroupHistory(s.Foods)
	got := only(check(s, windowTarget(s), planning.Proposal{
		Sessions: []planning.ProposedSession{session("s1", chili, sat, dinner, 1), session("s2", soup, sun, dinner, 1)},
		Plates:   []planning.ProposedPlate{pplate(sat, dinner, onKey(chili, "s1")), pplate(sun, dinner, onKey(soup, "s2"))},
	}), "H8")
	for _, is := range got {
		assert.NotEqual(t, planning.CodeVarietyRepeat, is.Code)
	}
}

// AC-12 (H8 part): the planner never places a recipe and its variant together
// where H8 forbids it.
func TestHeuristic_VariantGroupIsOneDish(t *testing.T) {
	s := batchCooker()
	group := map[int64]int64{101: 7, 104: 7, 105: 7, 106: 8, 107: 8} // 101 was eaten: its whole group counts
	for id, g := range group {
		f := s.Foods[id]
		f.VariantGroup = g
		s.Foods[id] = f
	}
	planning.ShareGroupHistory(s.Foods)
	tgt := windowTarget(s)
	p := choose(t, planning.ChooseInput{State: s, Target: tgt})

	cooked := map[int64]int{}
	for _, ps := range p.Sessions {
		if g := group[ps.FoodID]; g != 0 {
			cooked[g]++
		}
		assert.NotContains(t, []int64{104, 105}, ps.FoodID, "a variant of a recently eaten dish")
	}
	assert.LessOrEqual(t, cooked[8], 1, "one dish of the group in the window")
	assert.Empty(t, violations(planning.Check(planning.Merge(s, tgt, p), tgt)))
}
