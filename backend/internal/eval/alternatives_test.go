package eval

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// generatedWeek is single_batch_cooker's heuristic week (one cooking
// occasion a day, batch cooking) and its state.
func generatedWeek(t *testing.T) (*env, Scenario, planning.State) {
	t.Helper()
	ctx := context.Background()
	lib, err := LoadLibrary()
	require.NoError(t, err)
	scs, err := LoadScenarios("single_batch_cooker")
	require.NoError(t, err)
	sc := scs[0]
	d, err := newDBs("")
	require.NoError(t, err)
	t.Cleanup(func() { _ = d.close() })
	conn, err := d.open("alternatives")
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	e := newEnv(conn, Config{Chooser: airun.ChooserHeuristic})
	require.NoError(t, e.seed(ctx, sc, lib))
	_, err = e.step(ctx, sc, sc.Steps[0], airun.ChooserHeuristic)
	require.NoError(t, err)
	return e, sc, e.state(t, sc)
}

func (e *env) state(t *testing.T, sc Scenario) planning.State {
	t.Helper()
	st, err := e.builder.Build(context.Background(), e.window, sc.Today.Time, e.window.Start, e.window.End)
	require.NoError(t, err)
	return st
}

// mainOf returns p's main component and its session.
func mainOf(st planning.State, p planning.Plate) (planning.Component, *planning.Session) {
	for _, c := range p.Components {
		if c.Position != plate.PositionMain {
			continue
		}
		for i := range st.Sessions {
			if st.Sessions[i].ID == c.SessionID {
				return c, &st.Sessions[i]
			}
		}
		return c, nil
	}
	return planning.Component{}, nil
}

func sameCell(ss *planning.Session, p planning.Plate) bool {
	return ss != nil && ss.CookDate.Equal(p.Date) && ss.CookSlotID == p.SlotID
}

func h2(st planning.State) []planning.Issue {
	var out []planning.Issue
	for _, is := range planning.Check(st, planning.WindowTarget(st)) {
		if is.Rule == "H2" {
			out = append(out, is)
		}
	}
	return out
}

// On a heuristic week with one cooking occasion a day, a leftover lunch's
// main, a cooking dinner's main and a side all get alternatives with a
// different dish. Before cook_at, every fresh candidate of a leftover main
// added a second occasion (H2) and the list was empty.
func TestAlternatives_GeneratedWeek(t *testing.T) {
	ctx := context.Background()
	e, sc, st := generatedWeek(t)
	lunch, dinner := e.slotIDs["lunch"], e.slotIDs["dinner"]
	var leftover, cooking, side *planning.Plate
	for i := range st.Plates {
		p := &st.Plates[i]
		c, ss := mainOf(st, *p)
		if c.FoodID == 0 || ss == nil {
			continue
		}
		switch {
		case leftover == nil && p.SlotID == lunch && !sameCell(ss, *p) && !ss.CookDate.Equal(p.Date):
			leftover = p
		case cooking == nil && p.SlotID == dinner && sameCell(ss, *p):
			cooking = p
		}
		if side == nil && !sameCell(ss, *p) && len(p.Components) == 2 {
			side = p
		}
	}
	require.NotNil(t, leftover, "a leftover lunch eating an earlier day's batch")
	require.NotNil(t, cooking, "a cooking dinner")
	require.NotNil(t, side, "a leftover plate with one side")

	ask := func(p *planning.Plate, what string) planning.AltResult {
		t.Helper()
		res, err := e.svc.Alternatives(ctx, e.window.Start, sc.Today.Time, planning.AltRequest{PlateID: p.ID, What: what, Limit: 10})
		require.NoError(t, err)
		return res
	}
	for _, c := range []struct {
		name string
		p    *planning.Plate
		what string
	}{{"leftover lunch main", leftover, planning.AltMain}, {"cooking dinner main", cooking, planning.AltMain}, {"side", side, planning.AltSide}} {
		res := ask(c.p, c.what)
		require.NotEmpty(t, res.Alternatives, "%s: %+v", c.name, res.EmptyReason)
		assert.Nil(t, res.EmptyReason, c.name)
		on := map[int64]bool{}
		for _, comp := range c.p.Components {
			on[comp.FoodID] = true
		}
		for _, a := range res.Alternatives {
			assert.False(t, on[a.FoodID], "%s: %s is already on the plate", c.name, a.Name)
		}
	}

	// The leftover main's best alternative cooks at an existing cooking
	// cell; applied through ReplaceComponent it adds no H2.
	res := ask(leftover, planning.AltMain)
	top := res.Alternatives[0]
	require.Equal(t, planning.SourceCookAt, top.Source, "%+v", top)
	require.NotNil(t, top.CookAt)
	require.Len(t, top.Ops, 1)
	assert.Equal(t, "replace", top.Ops[0].Op)
	assert.Equal(t, top.CookAt, top.Ops[0].CookAt)
	cookDate, err := time.Parse(time.DateOnly, top.CookAt.Date)
	require.NoError(t, err)
	require.Empty(t, h2(st))
	e.today = sc.Today.Time
	_, err = e.plates.ReplaceComponent(ctx, leftover.ID, plate.Selector{ComponentID: *top.Ops[0].PlateComponentID}, top.FoodID,
		plate.Source{CookAt: &plate.CellKey{Date: cookDate, SlotID: top.CookAt.SlotID}}, nil)
	require.NoError(t, err)
	after := e.state(t, sc)
	assert.Empty(t, h2(after), "a cook_at replacement adds no cooking occasion")
	for _, p := range after.Plates {
		if p.ID == leftover.ID {
			c, _ := mainOf(after, p)
			assert.Equal(t, top.FoodID, c.FoodID)
		}
	}

	// The cooking dinner's best alternative applied as its ops say: the old
	// batch re-anchors to its next meal (R1); still no H2.
	res = ask(cooking, planning.AltMain)
	top = res.Alternatives[0]
	t.Logf("cooking dinner top: %s %s ops %+v", top.Name, top.Source, top.Ops[0])
	src := plate.Source{}
	if top.SessionID != nil {
		src.SessionID = *top.SessionID
	}
	cid, _ := mainOf(st, *cooking)
	_, err = e.plates.ReplaceComponent(ctx, cooking.ID, plate.Selector{ComponentID: cid.ID}, top.FoodID, src, nil)
	require.NoError(t, err)
	assert.Empty(t, h2(e.state(t, sc)), "replacing a cooking main adds no H2")
}
