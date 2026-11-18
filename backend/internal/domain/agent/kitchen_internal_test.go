package agent

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/window"
)

var planningWindow window.Window

func TestRemovedMemo(t *testing.T) {
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
			recommend(tt.ways, tt.offers)
			assert.Equal(t, 1, count(tt.ways, tt.offers))
			if tt.wantWay {
				assert.True(t, tt.ways[0].Recommended)
			}
		})
	}
}

func TestAdviceFor_NothingUnasked(t *testing.T) {
	soft := planning.Issue{Rule: "S2", Code: "kcal_below_band", Severity: planning.SeveritySoft, Date: "2026-11-25", Options: []planning.Option{{Name: "Ciabatta", Recommended: true}}}
	floor := planning.Issue{Rule: "H9", Code: planning.CodeKcalBelowFloor, Severity: planning.SeverityViolation, Date: "2026-11-25"}
	ctx := context.Background()
	for name, tt := range map[string]struct {
		after planning.Evaluation
		want  string
	}{
		"nothing new":     {planning.Evaluation{}, ""},
		"a soft issue":    {planning.Evaluation{Issues: []planning.Issue{soft}}, "offer the option marked recommended as a question"},
		"a day under min": {planning.Evaluation{Issues: []planning.Issue{floor, soft}}, "under its minimum"},
	} {
		t.Run(name, func(t *testing.T) {
			next, _ := adviceFor(ctx, Services{}, planningWindow, planning.Evaluation{}, tt.after, true)
			assert.Contains(t, next, tt.want)
			assert.NotContains(t, next, "apply it now")
		})
	}
}
