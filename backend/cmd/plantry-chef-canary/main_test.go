package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRun_RefusesALiveRunWithoutTheFlag(t *testing.T) {
	env := map[string]string{"PLANTRY_AI_PROVIDER": "anthropic", "PLANTRY_AI_API_KEY": "k", "PLANTRY_AI_MODEL": "m"}
	err := run(context.Background(), []string{"-db", filepath.Join(t.TempDir(), "c.db")}, func(k string) string { return env[k] }, &bytes.Buffer{})
	require.ErrorContains(t, err, "PLANTRY_CHEF_CANARY_LIVE=1")
}

func TestRun_ScriptedSeedsLabelsAndReports(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "labels.json")
	require.NoError(t, os.WriteFile(script, []byte(`{"turns":[{"for_tool":"set_recipe_attributes","stop_reason":"tool_use",
		"message":{"content":[{"type":"tool_use","tool_use_id":"t1","tool_name":"set_recipe_attributes",
		"tool_input":{"recipes":[{"key":"r1","kind":"stew"}]}}]}}]}`), 0o600))
	out := filepath.Join(dir, "report.json")
	var stdout bytes.Buffer
	err := run(context.Background(), []string{"-db", filepath.Join(dir, "c.db"), "-fake-script", script, "-out", out}, func(string) string { return "" }, &stdout)
	require.NoError(t, err)
	// The count follows the DATA-1 dev split, which grows; only a non-empty comparison matters here.
	assert.Regexp(t, `recipes compared: [1-9][0-9]*`, stdout.String())
	assert.FileExists(t, out)
}
