package agent

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/window"
)

var planningWindow window.Window

func TestRemovedMemo(t *testing.T) {
	t.Parallel()
	r := &removedMemo{}
	ctx := withTurn(context.Background(), chatTurn{conversationID: 7})
	other := withTurn(context.Background(), chatTurn{conversationID: 8})
	r.add(ctx, []int64{3, 4})
	assert.Equal(t, map[int64]bool{3: true, 4: true}, r.avoid(ctx))
	assert.Empty(t, r.avoid(other), "another conversation does not know")
	assert.Equal(t, map[int64]bool{3: true, 4: true, 9: true}, r.avoid(withExtraAvoid(ctx, []int64{9})), "nor the running call's own removals")
	var none *removedMemo
	assert.NotPanics(t, func() { none.add(ctx, []int64{1}); none.avoid(ctx) })
}

func TestRecommend_ExactlyOne(t *testing.T) {
	t.Parallel()
	dirty := []planning.Issue{{Code: planning.CodeAddOnExcess}}
	count := func(ws ...[]planning.Way) (n int) {
		for _, l := range ws {
			for _, w := range l {
				if w.Recommended {
					n++
					assert.NotEmpty(t, w.Because)
				}
			}
		}
		return
	}
	tests := []struct {
		name         string
		ways, offers []planning.Way
		wantWay      bool
	}{
		{"a clean way wins over an offer", []planning.Way{{Reaches: true}}, []planning.Way{{}}, true},
		{"an unclean way loses to a clean offer", []planning.Way{{NewIssues: dirty}}, []planning.Way{{}}, false},
		{"only an unclean way", []planning.Way{{NewIssues: dirty}}, nil, true},
		{"only an offer", nil, []planning.Way{{}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recommend(tt.ways, tt.offers, false)
			assert.Equal(t, 1, count(tt.ways, tt.offers))
			if tt.wantWay {
				assert.True(t, tt.ways[0].Recommended)
			}
		})
	}
}

func TestAdviceFor_NothingUnasked(t *testing.T) {
	t.Parallel()
	soft := planning.Issue{Rule: "S2", Code: "kcal_below_band", Severity: planning.SeveritySoft, Date: "2026-11-25", Options: []planning.Option{{Name: "Ciabatta", Recommended: true}}}
	floor := planning.Issue{Rule: "H9", Code: planning.CodeKcalBelowFloor, Severity: planning.SeverityViolation, Date: "2026-11-25"}
	ctx := context.Background()
	for name, tt := range map[string]struct {
		after planning.Evaluation
		want  string
	}{
		"nothing new":     {planning.Evaluation{}, ""},
		"a soft issue":    {planning.Evaluation{Issues: []planning.Issue{soft}}, "state the option marked recommended as your closing offer"},
		"a day under min": {planning.Evaluation{Issues: []planning.Issue{floor, soft}}, "under its minimum"},
	} {
		t.Run(name, func(t *testing.T) {
			next, _ := adviceFor(ctx, Services{}, planningWindow, planning.Evaluation{}, tt.after, false, nil)
			assert.Contains(t, next, tt.want)
			assert.NotContains(t, next, "apply it now")
			// A check of a requested change says to save it, never to ask.
			dry, _ := adviceFor(ctx, Services{}, planningWindow, planning.Evaluation{}, tt.after, true, nil)
			if tt.want != "" {
				assert.Contains(t, dry, "NOT saved")
				assert.Contains(t, dry, "never ask first")
			}
		})
	}
}

// A named dish that raises the week's meat by a real step over its limit is
// saved and its week effect told; a rise of a few grams says nothing.
func TestAdviceFor_WeekMeat(t *testing.T) {
	t.Parallel()
	meatIssue := planning.Issue{Rule: "S7", Code: planning.CodeWeekMeatAboveDGE, Severity: planning.SeveritySoft, Params: map[string]any{"meat_g": 471.0, "max_g": 300.0}}
	ev := func(meatG float64) planning.Evaluation {
		return planning.Evaluation{Week: &planning.WeekEval{MeatG: meatG}, Issues: []planning.Issue{meatIssue}}
	}
	ctx := context.Background()
	next, _ := adviceFor(ctx, Services{}, planningWindow, ev(321), ev(471), false, nil)
	assert.Contains(t, next, "the week's meat goes from 321 to 471 g, over the 300 g limit")
	assert.Contains(t, next, "swap a meat meal for a vegetarian one")
	dry, _ := adviceFor(ctx, Services{}, planningWindow, ev(321), ev(471), true, nil)
	assert.Contains(t, dry, "never ask first")
	small, _ := adviceFor(ctx, Services{}, planningWindow, ev(321), ev(325), false, nil)
	assert.Empty(t, small, "4 g more is not a step")
	under, _ := adviceFor(ctx, Services{}, planningWindow, ev(100), planning.Evaluation{Week: &planning.WeekEval{MeatG: 200}}, false, nil)
	assert.Empty(t, under, "still under the limit")
}

// A recommended offer never creates a warning: an alternative that lists no
// new issue but whose own check does make one is skipped.
func TestFirstClean_SkipsWhatItsCheckDirties(t *testing.T) {
	t.Parallel()
	op := []planning.OptionOp{{Op: "swap"}}
	dirty := planning.Alternative{Name: "Tuna pasta", Ops: op}
	clean := planning.Alternative{Name: "Lentil curry", Ops: op}
	listed := planning.Alternative{Name: "Pizza", Ops: op, NewIssues: []planning.Issue{{Code: "x"}}}
	calls := 0
	checks := func(ops []planning.OptionOp) bool { calls++; return calls > 1 }
	got, ok := firstClean([]planning.Alternative{listed, dirty, clean}, checks)
	assert.True(t, ok)
	assert.Equal(t, "Lentil curry", got.Name)
	_, ok = firstClean([]planning.Alternative{listed, dirty}, func([]planning.OptionOp) bool { return false })
	assert.False(t, ok)
}

// An offer never takes back what the user just asked for: the options that
// change a component the call made are dropped, and the next one is the
// recommended one.
func TestWithoutUndoOptions(t *testing.T) {
	t.Parallel()
	id := func(n int64) *int64 { return &n }
	issue := planning.Issue{Code: planning.CodeKcalAboveBand, Options: []planning.Option{
		{Name: "remove the bread", Recommended: true, Op: planning.OptionOp{Op: "remove", PlateComponentID: id(7)}},
		{Name: "smaller salmon", Op: planning.OptionOp{Op: "set_quantity", PlateComponentID: id(3)}},
	}}
	got := withoutUndoOptions([]planning.Issue{issue}, []int64{7})
	require.Len(t, got[0].Options, 1)
	assert.Equal(t, "smaller salmon", got[0].Options[0].Name)
	assert.True(t, got[0].Options[0].Recommended)
	assert.Len(t, issue.Options, 2, "the input is not changed")
	assert.Equal(t, []planning.Issue{issue}, withoutUndoOptions([]planning.Issue{issue}, nil))
}

// Every branch of adviceFor (a check or a saved change) that names an option
// holds that option: next never promises an offer, or a fix marked recommended,
// that the result does not carry (auditor 3: a day under its minimum promised
// "the fix marked recommended" and had none).
func TestAdviceFor_NextNeverNamesAnAbsentOption(t *testing.T) {
	t.Parallel()
	rec := []planning.Option{{Name: "Ciabatta", Recommended: true}}
	floor := func(opts []planning.Option) planning.Issue {
		return planning.Issue{Rule: "H9", Code: planning.CodeKcalBelowFloor, Severity: planning.SeverityViolation, Date: "2026-11-25", Options: opts}
	}
	soft := func(opts []planning.Option) planning.Issue {
		return planning.Issue{Rule: "S2", Code: planning.CodeKcalBelowBand, Severity: planning.SeveritySoft, Date: "2026-11-25", Options: opts}
	}
	other := planning.Issue{Rule: "H2", Code: planning.CodeTooManyCookOccasions, Severity: planning.SeverityViolation, Date: "2026-11-25"}
	meat := planning.Issue{Rule: "S7", Code: planning.CodeWeekMeatAboveDGE, Severity: planning.SeveritySoft, Params: map[string]any{"meat_g": 471.0, "max_g": 300.0}}
	for name, after := range map[string]planning.Evaluation{
		"a floor breach without options":      {Issues: []planning.Issue{floor(nil)}},
		"a floor breach with options":         {Issues: []planning.Issue{floor(rec)}},
		"a problem without options":           {Issues: []planning.Issue{other}},
		"a problem with a soft issue options": {Issues: []planning.Issue{other, soft(rec)}},
		"a soft issue with options":           {Issues: []planning.Issue{soft(rec)}},
		"a soft issue without options":        {Issues: []planning.Issue{soft(nil)}},
		"the week's meat":                     {Week: &planning.WeekEval{MeatG: 471}, Issues: []planning.Issue{meat}},
	} {
		for _, dry := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s dry=%v", name, dry), func(t *testing.T) {
				before := planning.Evaluation{Week: &planning.WeekEval{MeatG: 321}}
				next, offers := adviceFor(context.Background(), Services{}, planningWindow, before, after, dry, nil)
				for _, phrase := range []string{"offers[0]", "offer[0]"} {
					if strings.Contains(next, phrase) {
						assert.NotEmpty(t, offers, "next names %s: %s", phrase, next)
					}
				}
				hasRec := false
				for _, is := range after.Issues {
					for _, o := range is.Options {
						hasRec = hasRec || o.Recommended
					}
				}
				for _, phrase := range []string{"fix marked recommended", "option marked recommended"} {
					if strings.Contains(next, phrase) {
						assert.True(t, hasRec, "next names the %s but no issue holds one: %s", phrase, next)
					}
				}
			})
		}
	}
}

// A day under its minimum with no checked fix says so: next promises none.
func TestAdviceFor_FloorBreachWithoutAFixSaysSo(t *testing.T) {
	t.Parallel()
	floor := planning.Issue{Rule: "H9", Code: planning.CodeKcalBelowFloor, Severity: planning.SeverityViolation, Date: "2026-11-25"}
	next, offers := adviceFor(context.Background(), Services{}, planningWindow, planning.Evaluation{}, planning.Evaluation{Issues: []planning.Issue{floor}}, false, nil)
	assert.Empty(t, offers)
	assert.Contains(t, next, "no checked fix reaches it")
	assert.Contains(t, next, "offer no fix, name none")
}

// G3: a reply never offers to undo what the call just did: food added to a
// meal the call made lighter, food taken from one it made heartier.
func TestWithoutRegressOptions(t *testing.T) {
	t.Parallel()
	plateAt := func(slot int64, kcal float64) planning.PlateEval {
		return planning.PlateEval{Date: "2026-11-24", SlotID: slot, Kcal: kcal}
	}
	before := planning.Evaluation{Plates: []planning.PlateEval{plateAt(2, 600), plateAt(3, 800)}}
	now := planning.Evaluation{Plates: []planning.PlateEval{plateAt(2, 600), plateAt(3, 500)}}
	issue := planning.Issue{Options: []planning.Option{
		{Name: "bread on dinner", Date: "2026-11-24", SlotID: 3, KcalDelta: 120, Recommended: true},
		{Name: "bread on lunch", Date: "2026-11-24", SlotID: 2, KcalDelta: 120},
	}}
	got := withoutRegressOptions(before, now, []planning.Issue{issue})
	if assert.Len(t, got[0].Options, 1) {
		assert.Equal(t, "bread on lunch", got[0].Options[0].Name)
		assert.True(t, got[0].Options[0].Recommended, "the option left is the recommendation")
	}
	assert.Equal(t, []planning.Issue{issue}, withoutRegressOptions(before, before, []planning.Issue{issue}), "no change, no filter")
}
