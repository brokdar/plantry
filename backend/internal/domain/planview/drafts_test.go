package planview

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
)

// The AC-1 window runs Sat 2026-10-31 … Fri 2026-11-06.
var ac1Start = date("2026-10-31")

// fill puts a confirmed omelette plate on every empty cell of the AC-1
// window except skip, so H1 has nothing to report about them.
func (f *fixture) fill(skip ...cellKey) {
	taken := map[cellKey]bool{}
	for _, k := range skip {
		taken[k] = true
	}
	for _, p := range f.plates {
		taken[cellID(p.Date, p.SlotID)] = true
	}
	for d := ac1Start; !d.After(date("2026-11-06")); d = d.AddDate(0, 0, 1) {
		for _, sl := range []int64{breakfast, lunch, dinner} {
			if !taken[cellID(d, sl)] && f.overrides[cellID(d, sl)] != availability.ModeNone {
				f.leaf(d.Format("2006-01-02"), sl, appleID, "Apple", 150)
			}
		}
	}
}

func rulesAndCodes(issues []planning.Issue) [][2]string {
	out := [][2]string{}
	for _, is := range issues {
		out = append(out, [2]string{is.Rule, is.Code})
	}
	return out
}

// B2-AC1 (domain half)
func TestPlan_DraftSummary(t *testing.T) {
	t.Run("a window with one draft batch", func(t *testing.T) {
		f := newFixture()
		sid := f.draftSession(chiliID, "2026-11-01", dinner, 4, 7)
		f.draft(f.composed("2026-11-01", dinner, chiliID, sid), 7)
		f.draft(f.composed("2026-11-02", lunch, chiliID, sid), 7)
		f.runs[7] = RunNote{Chooser: ptr("heuristic"), Notes: ptr("A quiet week.")}
		f.ac1Confirmed()

		p, err := f.service().Plan(ctx, &ac1Start, date("2026-10-30"))
		require.NoError(t, err)
		require.NotNil(t, p.Draft)
		assert.Equal(t, Draft{Plates: 2, Sessions: 1, RunID: ptr(int64(7)), Chooser: ptr("heuristic"), Notes: ptr("A quiet week.")}, *p.Draft)

		states := map[plate.State]int{}
		for _, c := range p.Cells {
			if c.Plate != nil {
				states[c.Plate.State]++
			}
		}
		assert.Equal(t, map[plate.State]int{plate.StateDraft: 2, plate.StateConfirmed: 3}, states)
		for _, v := range p.Sessions {
			assert.Equal(t, v.ID == sid, v.State == plate.StateDraft)
		}
	})

	t.Run("the newest run names the draft", func(t *testing.T) {
		f := newFixture()
		sid := f.draftSession(chiliID, "2026-11-01", dinner, 2, 7)
		f.draft(f.composed("2026-11-01", dinner, chiliID, sid), 7)
		om := f.draftSession(omeletID, "2026-11-03", breakfast, 2, 9)
		f.draft(f.composed("2026-11-03", breakfast, omeletID, om), 9)
		f.runs[9] = RunNote{Chooser: ptr("llm")}

		p, err := f.service().Plan(ctx, &ac1Start, date("2026-10-30"))
		require.NoError(t, err)
		require.NotNil(t, p.Draft)
		assert.Equal(t, Draft{Plates: 2, Sessions: 2, RunID: ptr(int64(9)), Chooser: ptr("llm")}, *p.Draft)
	})

	t.Run("drafts without a run", func(t *testing.T) {
		f := newFixture()
		sid := f.session(chiliID, "2026-11-01", dinner, 2)
		cs := f.sessions[sid]
		cs.State = plate.StateDraft
		f.sessions[sid] = cs
		pid := f.composed("2026-11-01", dinner, chiliID, sid)
		f.plates[len(f.plates)-1].State = plate.StateDraft

		p, err := f.service().Plan(ctx, &ac1Start, date("2026-10-30"))
		require.NoError(t, err)
		require.NotNil(t, p.Draft, "plate %d", pid)
		assert.Equal(t, Draft{Plates: 1, Sessions: 1}, *p.Draft)
	})

	t.Run("a window without drafts", func(t *testing.T) {
		f := newFixture()
		f.ac1()
		p, err := f.service().Plan(ctx, &ac1Start, date("2026-10-30"))
		require.NoError(t, err)
		assert.Nil(t, p.Draft)
		assert.Zero(t, f.recipeListCalls, "H8's recipe list loads only for drafts")
	})
}

// ac1Confirmed adds AC-1's batch on Tue/Wed as confirmed rows.
func (f *fixture) ac1Confirmed() {
	sid := f.session(chiliID, "2026-11-03", dinner, 6)
	f.composed("2026-11-03", dinner, chiliID, sid)
	f.composed("2026-11-04", lunch, chiliID, sid)
	f.composed("2026-11-04", dinner, chiliID, sid)
}

func TestPlan_RunningRunID(t *testing.T) {
	f := newFixture()
	p, err := f.service().Plan(ctx, &ac1Start, date("2026-10-30"))
	require.NoError(t, err)
	assert.Nil(t, p.RunningRunID, "no runner wired")

	f.progress = fakeProgress{"2026-10-31": 12}
	p, err = f.service().Plan(ctx, &ac1Start, date("2026-10-30"))
	require.NoError(t, err)
	assert.Equal(t, ptr(int64(12)), p.RunningRunID)

	next := date("2026-11-07")
	p, err = f.service().Plan(ctx, &next, date("2026-10-30"))
	require.NoError(t, err)
	assert.Nil(t, p.RunningRunID, "another window's run")
}

// B2-AC2
func TestPlan_Issues(t *testing.T) {
	t.Run("two cooking occasions on Sunday", func(t *testing.T) {
		f := newFixture()
		lunchID := f.session(omeletID, "2026-11-01", lunch, 2)
		f.composed("2026-11-01", lunch, omeletID, lunchID)
		f.ac1()
		f.fill()
		p, err := f.service().Plan(ctx, &ac1Start, date("2026-10-30"))
		require.NoError(t, err)
		require.Equal(t, [][2]string{{"H2", planning.CodeTooManyCookOccasions}}, rulesAndCodes(p.Issues))
		assert.Equal(t, "2026-11-01", p.Issues[0].Date)
		assert.Equal(t, planning.SeverityViolation, p.Issues[0].Severity)
	})

	t.Run("a plate in a none cell", func(t *testing.T) {
		f := newFixture()
		pid := f.leaf("2026-11-02", lunch, appleID, "Apple", 150)
		f.overrides[cellID(date("2026-11-02"), lunch)] = availability.ModeNone
		f.fill()
		p, err := f.service().Plan(ctx, &ac1Start, date("2026-10-30"))
		require.NoError(t, err)
		require.Equal(t, [][2]string{{"H1", planning.CodePlateInNoneCell}}, rulesAndCodes(p.Issues))
		assert.Equal(t, &pid, p.Issues[0].PlateID)
	})

	t.Run("an empty plan cell misses its plate", func(t *testing.T) {
		f := newFixture()
		f.fill(cellID(date("2026-11-05"), dinner))
		p, err := f.service().Plan(ctx, &ac1Start, date("2026-10-30"))
		require.NoError(t, err)
		require.Equal(t, [][2]string{{"H1", planning.CodeMissingPlate}}, rulesAndCodes(p.Issues))
		assert.Equal(t, "2026-11-05", p.Issues[0].Date)
	})

	t.Run("chili on a vegetarian profile", func(t *testing.T) {
		f := newFixture()
		f.profile.p.DietType = profile.DietVegetarian
		f.diets = fakeDiets{chiliID: "omnivore", appleID: "vegan"}
		f.ac1()
		f.fill()
		p, err := f.service().Plan(ctx, &ac1Start, date("2026-10-30"))
		require.NoError(t, err)
		codes := rulesAndCodes(p.Issues)
		require.Len(t, codes, 3, "one per chili plate")
		for _, c := range codes {
			assert.Equal(t, [2]string{"H5", planning.CodeDietIncompatible}, c)
		}
	})

	t.Run("without a diet source H5 stays silent", func(t *testing.T) {
		f := newFixture()
		f.profile.p.DietType = profile.DietVegetarian
		f.ac1()
		f.fill()
		p, err := f.service().Plan(ctx, &ac1Start, date("2026-10-30"))
		require.NoError(t, err)
		assert.Empty(t, p.Issues)
	})

	t.Run("a draft repeats a recently eaten main", func(t *testing.T) {
		f := newFixture()
		eaten := f.session(chiliID, "2026-10-25", dinner, 2)
		f.composed("2026-10-25", dinner, chiliID, eaten)
		sid := f.draftSession(chiliID, "2026-11-01", dinner, 2, 7)
		f.draft(f.composed("2026-11-01", dinner, chiliID, sid), 7)
		f.runs[7] = RunNote{}
		f.fill()
		p, err := f.service().Plan(ctx, &ac1Start, date("2026-10-30"))
		require.NoError(t, err)
		require.Equal(t, [][2]string{{"H8", planning.CodeVarietyRepeat}}, rulesAndCodes(p.Issues),
			"the omelette was an unrepeated main to pick, so this is a violation")
		assert.Equal(t, &sid, p.Issues[0].SessionID)
		assert.Equal(t, "2026-10-25", p.Issues[0].Params["last_eaten"])
	})

	t.Run("a past draft does not count as eaten", func(t *testing.T) {
		f := newFixture()
		old := f.draftSession(chiliID, "2026-10-25", dinner, 2, 5)
		f.draft(f.composed("2026-10-25", dinner, chiliID, old), 5)
		sid := f.draftSession(chiliID, "2026-11-01", dinner, 2, 7)
		f.draft(f.composed("2026-11-01", dinner, chiliID, sid), 7)
		f.runs[7] = RunNote{}
		f.fill()
		p, err := f.service().Plan(ctx, &ac1Start, date("2026-10-30"))
		require.NoError(t, err)
		assert.Empty(t, p.Issues)
	})
}

// B2-AC3
func TestToday_Drafts(t *testing.T) {
	t.Run("a draft-only window", func(t *testing.T) {
		f := newFixture()
		sid := f.draftSession(chiliID, "2026-11-01", dinner, 4, 7)
		f.draft(f.composed("2026-11-01", dinner, chiliID, sid), 7)
		f.draft(f.composed("2026-11-02", lunch, chiliID, sid), 7)
		td, err := f.service().Today(ctx, date("2026-11-01"))
		require.NoError(t, err)
		assert.Equal(t, Verdict{Kind: VerdictDraftOnly}, td.Verdict)
		assert.Empty(t, td.Cook, "draft sessions are not cooked")
		for _, m := range td.Meals {
			assert.Nil(t, m.Plate, "draft plates are not eaten")
		}
	})

	t.Run("a date without confirmed plates in a window with drafts", func(t *testing.T) {
		f := newFixture()
		f.ac1()
		sid := f.draftSession(omeletID, "2026-11-04", breakfast, 2, 7)
		f.draft(f.composed("2026-11-04", breakfast, omeletID, sid), 7)
		td, err := f.service().Today(ctx, date("2026-11-04"))
		require.NoError(t, err)
		assert.Equal(t, Verdict{Kind: VerdictDraftOnly}, td.Verdict)
	})

	t.Run("a confirmed cook session today still cooks", func(t *testing.T) {
		f := newFixture()
		sid := f.ac1()
		om := f.draftSession(omeletID, "2026-11-01", breakfast, 2, 7)
		f.draft(f.composed("2026-11-01", breakfast, omeletID, om), 7)
		td, err := f.service().Today(ctx, date("2026-11-01"))
		require.NoError(t, err)
		assert.Equal(t, Verdict{Kind: VerdictCook, SessionID: sid, FoodName: "Chili", Portions: 6, SlotID: dinner}, td.Verdict)
		require.Len(t, td.Cook, 1, "the draft omelette is not on the cook list")
	})

	t.Run("a draft plate is never the one to rate", func(t *testing.T) {
		f := newFixture()
		f.ac1()
		sid := f.draftSession(omeletID, "2026-11-03", breakfast, 2, 7)
		f.draft(f.composed("2026-11-03", breakfast, omeletID, sid), 7)
		td, err := f.service().Today(ctx, date("2026-11-04"))
		require.NoError(t, err)
		require.NotNil(t, td.Rate)
		assert.Equal(t, "Chili", td.Rate.FoodName)
	})
}
