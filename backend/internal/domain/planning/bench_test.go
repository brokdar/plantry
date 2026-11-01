package planning_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/window"

	"github.com/jaltszeimer/plantry/backend/internal/domain/diet"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper/testsvc"
)

// benchState is a synthetic window: 300 recipes (eaten at staggered dates
// over the last 8 weeks), 4 weeks of history and a 21-plate proposal with
// one dinner batch per day feeding the next day's lunch.
func benchState() (planning.State, planning.Target, planning.Proposal) {
	s := baseState()
	classes := diet.Order
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
func BenchmarkCheck(b *testing.B) {
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

// benchDB is the AC-11 database: the batch cooker fixture grown to 300
// recipes of 5 ingredients each, 4 weeks of confirmed history (3 plates a
// day) and a generated 21-plate window.
func benchDB(tb testing.TB) *svcEnv {
	tb.Helper()
	e := newSvcEnv(tb)
	var leaves []int64
	for i := range 60 {
		leaves = append(leaves, e.insert(`INSERT INTO foods (name, kind, source, kcal_100g, protein_100g, fat_100g, carbs_100g) VALUES (?, 'leaf', 'manual', 100, 5, 5, 10)`, fmt.Sprintf("Leaf %d", i)))
	}
	for i := range 285 {
		id := e.insert(`INSERT INTO foods (name, kind, role, reference_portions) VALUES (?, 'composed', 'main', 4)`, fmt.Sprintf("Bench %03d", i))
		for j := range 5 {
			e.exec(`INSERT INTO food_components (parent_id, child_id, amount, unit, grams, sort_order) VALUES (?, ?, 100, 'g', 100, ?)`, id, leaves[(i+j*7)%len(leaves)], j)
		}
		e.mains = append(e.mains, id)
	}
	taken := map[string]bool{"2026-10-25": true, "2026-10-28": true, "2026-11-01": true}
	for d := range 28 {
		date := day(sat).AddDate(0, 0, -d-1).Format(time.DateOnly)
		for k, slot := range []int64{e.breakfast, e.lunch, e.dinner} {
			if slot == e.dinner && taken[date] {
				continue
			}
			e.userPlate(date, slot, e.mains[10+(d*3+k)%280])
		}
	}
	end := e.runs.end(e.generate(e.req()))
	require.NotEqual(tb, airun.StatusError, end.Status)
	return e
}

// buildAndCheck reads the window's state the way a generation does and
// checks it.
func buildAndCheck(tb testing.TB, e *svcEnv, w window.Window, t planning.Target) {
	err := e.deps.Tx.InTx(e.ctx, func(ctx context.Context) error {
		st, err := e.deps.Builder.Build(ctx, w, day(svcToday), w.Start, w.End)
		if err != nil {
			return err
		}
		planning.Check(st, t)
		return nil
	})
	if err != nil {
		tb.Fatal(err)
	}
}

func benchWindow(e *svcEnv) (window.Window, planning.Target) {
	w := window.Window{Start: day(sat), End: day(fri)}
	var t planning.Target
	for _, d := range w.Days() {
		for _, slot := range []int64{e.breakfast, e.lunch, e.dinner} {
			t.Cells = append(t.Cells, plate.CellKey{Date: d, SlotID: slot})
		}
	}
	return w, t
}

// B6-AC8 (AC-11): the context build over SQLite plus the check.
func BenchmarkBuildAndCheck(b *testing.B) {
	e := benchDB(b)
	w, t := benchWindow(e)
	b.ResetTimer()
	for range b.N {
		buildAndCheck(b, e, w, t)
	}
}

func TestBuildAndCheck_Under50ms(t *testing.T) {
	if os.Getenv("PLANTRY_PERF_ASSERT") != "1" {
		t.Skip("timing assertion: set PLANTRY_PERF_ASSERT=1 (without -race); BenchmarkBuildAndCheck runs it too")
	}
	e := benchDB(t)
	w, tgt := benchWindow(e)
	var n int
	require.NoError(t, e.db.QueryRow(`SELECT COUNT(*) FROM foods WHERE kind = 'composed'`).Scan(&n))
	require.Equal(t, 300, n)
	require.Len(t, e.window(), 21)
	res := testing.Benchmark(func(b *testing.B) {
		for range b.N {
			buildAndCheck(b, e, w, tgt)
		}
	})
	if per := time.Duration(res.NsPerOp()); per >= 50*time.Millisecond {
		t.Fatalf("build and check take %s/op, want < 50ms", per)
	}
}

// BenchmarkPlanView is GET /api/plan's read model over the AC-11 database
// while its window holds drafts. A vegetarian profile also derives the
// uneaten recipes' diet classes (H8).
func BenchmarkPlanView(b *testing.B) {
	e := benchDB(b)
	view := testsvc.PlanView(e.db)
	start := day(sat)
	for _, dietType := range []string{"omnivore", "vegetarian"} {
		b.Run(dietType, func(b *testing.B) {
			e.exec(`UPDATE user_profile SET diet_type = ? WHERE id = 1`, dietType)
			b.ResetTimer()
			for range b.N {
				if _, err := view.Plan(e.ctx, &start, day(svcToday)); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
