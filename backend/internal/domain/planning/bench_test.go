package planning_test

import (
	"testing"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

// benchState is a synthetic window: 300 recipes (eaten at staggered dates
// over the last 8 weeks), 4 weeks of history and a 21-plate proposal with
// one dinner batch per day feeding the next day's lunch.
func benchState() (planning.State, planning.Target, planning.Proposal) {
	s := baseState()
	classes := []string{"vegan", "vegetarian", "pescetarian", "omnivore"}
	for i := range 300 {
		id := int64(1000 + i)
		f := recipe(id, "Recipe", food.RoleMain, classes[i%4], 400+float64(i%7)*50)
		if i%3 != 0 {
			eaten := day(sat).AddDate(0, 0, -(i%56)-1)
			f.LastEaten = &eaten
		}
		s.Foods[id] = f
	}
	for d := range 28 {
		date := day(sat).AddDate(0, 0, -d-1)
		for _, slot := range []int64{breakfast, lunch, dinner} {
			s.History = append(s.History, planning.HistoryPlate{Date: date, SlotID: slot, FoodIDs: []int64{1000 + int64(d*3)%300}})
		}
	}
	var p planning.Proposal
	for i, d := range s.Window.Days() {
		key := "s" + string(rune('a'+i))
		main := int64(1000 + i*3)
		p.Sessions = append(p.Sessions, planning.ProposedSession{Key: key, FoodID: main, CookDate: d, CookSlotID: dinner, Portions: 2})
		p.Plates = append(p.Plates,
			planning.ProposedPlate{Date: d, SlotID: breakfast, Components: []planning.ProposedComponent{fresh(oats)}},
			planning.ProposedPlate{Date: d, SlotID: dinner, Components: []planning.ProposedComponent{onKey(main, key)}},
		)
		next := d.AddDate(0, 0, 1)
		if !next.After(s.Window.End) {
			p.Plates = append(p.Plates, planning.ProposedPlate{Date: next, SlotID: lunch, Components: []planning.ProposedComponent{onKey(main, key)}})
		}
	}
	p.Plates = append(p.Plates, planning.ProposedPlate{Date: s.Window.Start, SlotID: lunch, Components: []planning.ProposedComponent{fresh(curry)}})
	return s, windowTarget(s), p
}

// B5-AC4: the check part of BuildAndCheck stays under 10 ms/op.
func BenchmarkBuildAndCheck(b *testing.B) {
	s, t, p := benchState()
	b.ResetTimer()
	for range b.N {
		m := planning.Merge(s, t, p)
		planning.Check(m, t)
	}
}

func TestBuildAndCheck_Under10ms(t *testing.T) {
	s, tgt, p := benchState()
	if n := len(p.Plates); n != 21 {
		t.Fatalf("bench proposal has %d plates, want 21", n)
	}
	res := testing.Benchmark(func(b *testing.B) {
		for range b.N {
			planning.Check(planning.Merge(s, tgt, p), tgt)
		}
	})
	if per := time.Duration(res.NsPerOp()); per >= 10*time.Millisecond {
		t.Fatalf("check part takes %s/op, want < 10ms", per)
	}
}
