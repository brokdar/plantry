package agent

import (
	"context"
	"encoding/json"
	"regexp"
	"testing"

	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/stretchr/testify/assert"
)

// E2 (13 of 145 find_ways calls): the goal object of apply_changes is
// accepted next to the string.
func TestFlattenGoal(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		`{"goal":{"kind":"lighter","plate_id":5}}`:                          `{"goal":"lighter","plate_id":5}`,
		`{"goal":"{\"kind\":\"heartier\",\"plate_id\":7}"}`:                 `{"goal":"heartier","plate_id":7}`,
		`{"goal":{"kind":"lower_day","date":"2026-12-01"},"to_limit":true}`: `{"date":"2026-12-01","goal":"lower_day","to_limit":true}`,
		`{"goal":{"kind":"lighter","plate_id":5},"plate_id":9}`:             `{"goal":"lighter","plate_id":9}`,
		// Twins: the string form, an object without a kind, or text that is no object stay as sent.
		`{"goal":"lighter","plate_id":5}`: `{"goal":"lighter","plate_id":5}`,
		`{"goal":{"plate_id":5}}`:         `{"goal":{"plate_id":5}}`,
		`{"goal":"{lighter"}`:             `{"goal":"{lighter"}`,
		`{"goal":"lighter now"}`:          `{"goal":"lighter now"}`,
		`{"goal":{"kind":7}}`:             `{"goal":{"kind":7}}`,
	} {
		assert.JSONEq(t, want, string(flattenGoal(json.RawMessage(in))), in)
	}
}

// E1: the text promises an offer only when one exists.
func TestAdviceFor_R12b_OfferPromisedOnlyWhenOne(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	open := planning.Issue{Rule: "H1", Code: "other_rule", Severity: planning.SeverityViolation, Date: "2026-11-25"}
	next, _ := adviceFor(ctx, Services{}, planningWindow, planning.Evaluation{}, planning.Evaluation{Issues: []planning.Issue{open}}, false, nil)
	assert.Contains(t, next, "no checked fix exists")
	assert.NotContains(t, next, "its offer")
	assert.NotContains(t, next, "offers[0]")

	// Twin: an issue with options of its own keeps the promise.
	soft := planning.Issue{Rule: "S2", Code: "kcal_below_band", Severity: planning.SeveritySoft, Date: "2026-11-25", Options: []planning.Option{{Name: "Ciabatta"}}}
	next, _ = adviceFor(ctx, Services{}, planningWindow, planning.Evaluation{}, planning.Evaluation{Issues: []planning.Issue{soft}}, false, nil)
	assert.Contains(t, next, "the program tells it and its offer")
}

// E3 and E4: the to_limit cues name "sättigend" (audit light_limit-13/-15) and
// the regenerate wish says a time limit is enforced (g2-planner-15).
func TestToolTexts_R12b(t *testing.T) {
	t.Parallel()
	assert.Contains(t, findWaysSchema, "sättigend")
	assert.Contains(t, string(toolRegenerate(Services{}).Schema), "enforced")
}

// E5: an empty find_ways forbids other writes; a way with new warnings is
// applied without asking (audit protein-17, h2-lighter2-03).
func TestNextStepText_R12b(t *testing.T) {
	t.Parallel()
	res := planning.WayResult{Goal: planning.GoalLighter}
	none := nextStepText(nil, nil, res, false)
	assert.Contains(t, none, "change nothing this turn")
	assert.Contains(t, none, "apply no other change")

	reach := planning.Way{Reaches: true}
	assert.NotContains(t, nextStepText([]planning.Way{reach}, nil, res, false), "do not ask first", "a clean way")

	warned := planning.Way{Reaches: true, NewIssues: []planning.Issue{{Code: "kcal_below_band"}}}
	assert.Contains(t, nextStepText([]planning.Way{warned}, nil, res, false), "do not ask first (its new warnings do not block")
	assert.Contains(t, nextStepText([]planning.Way{warned}, []planning.Way{reach}, res, false), "do not ask first (its new warnings do not block")
}

// E6: plainStatic left doubled words in the tool texts the model reads.
func TestPlainStatic_R12bNoGarbledWords(t *testing.T) {
	t.Parallel()
	garbled := regexp.MustCompile(`(?i)\b(cooking cooking|cook cooking|an side|side or side|warning\|warning)\b|cookings:\[`)
	for _, tl := range defaultTools(Services{}) {
		for _, text := range []string{tl.Description, string(tl.Schema)} {
			assert.Empty(t, garbled.FindString(plainStatic(text)), tl.Name)
		}
	}
	assert.Equal(t, "batches:[{id}]", plainStatic("sessions:[{id}]"))
	assert.Equal(t, "a cooking and the lunch cooking", plainStatic("a session and the lunch batch"), "the other rewrites stay")
	assert.Equal(t, "the cooking of a dish; two cookings", plainStatic("the cook session of a dish; two cook batches"))
}
