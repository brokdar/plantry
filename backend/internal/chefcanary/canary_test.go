package chefcanary_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/fake"
	"github.com/jaltszeimer/plantry/backend/internal/chefcanary"
	"github.com/jaltszeimer/plantry/backend/internal/chefsets"
	"github.com/jaltszeimer/plantry/backend/internal/domain/flavor"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

func TestRun_LabelsTheSetAndScoresIt(t *testing.T) {
	ctx := context.Background()
	set, err := chefsets.LoadDev()
	require.NoError(t, err)
	db := testhelper.NewTestDB(t)
	require.NoError(t, chefcanary.Seed(ctx, db, set))
	require.NoError(t, chefcanary.Seed(ctx, db, set), "a database with recipes is not seeded again")

	// Every call answers kind "stew" for each key of a full batch.
	entries := make([]string, flavor.LabelBatch)
	for i := range entries {
		entries[i] = fmt.Sprintf(`{"key":"r%d","kind":"stew"}`, i+1)
	}
	input := `{"recipes":[` + strings.Join(entries, ",") + `]}`
	client := fake.NewFromScript(&fake.Script{Turns: []fake.Turn{{
		ForTool: flavor.LabelTool, StopReason: "tool_use",
		Message: fake.ScriptedMessage{Content: []fake.ScriptedBlock{{Type: "tool_use", ToolUseID: "t1", ToolName: flavor.LabelTool, ToolInput: json.RawMessage(input)}}},
		Usage:   fake.ScriptedUsage{InputTokens: 100, OutputTokens: 10},
	}}})

	rep, err := chefcanary.Run(ctx, chefcanary.Config{DB: db, Set: set, Client: client, Provider: "fake", Model: "fake-model"})
	require.NoError(t, err)

	recipes, stews := 0, 0
	for _, f := range set.Foods {
		if !f.Leaf {
			recipes++
			if f.Attrs.Kind == "stew" {
				stews++
			}
		}
	}
	assert.Equal(t, recipes, rep.Recipes)
	assert.Equal(t, (recipes+flavor.LabelBatch-1)/flavor.LabelBatch, rep.Runs)
	assert.Zero(t, rep.Failed)
	assert.Equal(t, int64(100*rep.Runs), rep.InputTokens)
	assert.True(t, rep.Unpriced)
	byField := map[string]chefcanary.FieldScore{}
	for _, f := range rep.Fields {
		byField[f.Field] = f
	}
	assert.Equal(t, chefcanary.FieldScore{Field: "kind", Agree: stews, Total: recipes}, byField["kind"])
	assert.Zero(t, byField["cuisine"].Agree, "a field the model did not set never agrees")
	assert.Contains(t, rep.Summary(), "kind")
}
