package planning_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/fake"
)

// Case 3: the model refuses a generation. It is one provider call, no repair
// is asked for, the run ends with the refusal key and the plan is unchanged.
func TestGenerationFailure_ModelRefuses(t *testing.T) {
	e := newLLMEnv(t)
	refusal := fake.Turn{StopReason: "refusal", Message: fake.ScriptedMessage{Content: []fake.ScriptedBlock{{Type: "text", Text: "I can't plan that."}}}}

	e.failedRun(t, e.gen(refusal), "error.ai.refused")

	assert.Len(t, e.client.Requests(), 1)
}
