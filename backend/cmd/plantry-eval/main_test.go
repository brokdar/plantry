package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/eval"
)

// UPDATE_GOLDEN=1 rewrites testdata/*.golden.
var update = os.Getenv("UPDATE_GOLDEN") == "1"

func fixedSHA() string { return "0123456789ab" }

// fixedNow stops the clock, so latencies are 0 and the output is stable.
func fixedNow() time.Time { return time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC) }

func noEnv(string) string { return "" }

func runCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	err := run(context.Background(), args, &out, noEnv, fixedSHA, fixedNow)
	return out.String(), err
}

// B9-AC3: two scripted LLM runs with different -prompt-version labels write
// two JSONL reports, and -compare prints the per-component diff.
func TestCompareGolden(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.jsonl"), filepath.Join(dir, "b.jsonl")

	_, err := runCLI(t, "-chooser", "llm", "-runs", "1", "-prompt-version", "p1-a", "-out", a)
	require.NoError(t, err)
	out, err := runCLI(t, "-chooser", "llm", "-runs", "1", "-prompt-version", "p1-b", "-out", b, "-compare", a)
	require.NoError(t, err)

	for path, label := range map[string]string{a: "p1-a", b: "p1-b"} {
		f, err := os.Open(path)
		require.NoError(t, err)
		recs, err := eval.ReadJSONL(f)
		require.NoError(t, f.Close())
		require.NoError(t, err)
		require.Len(t, recs, 7)
		for _, r := range recs {
			assert.Equal(t, label, r.PromptVersion)
			assert.Equal(t, "0123456789ab", r.GitSHA)
		}
	}

	golden := filepath.Join("testdata", "compare.golden")
	if update {
		require.NoError(t, os.WriteFile(golden, []byte(replacePath(out, b)), 0o600))
	}
	want, err := os.ReadFile(golden)
	require.NoError(t, err)
	assert.Equal(t, string(want), replacePath(out, b))
}

// replacePath hides the temp report path of the "Wrote" line.
func replacePath(s, path string) string {
	return string(bytes.ReplaceAll([]byte(s), []byte(path), []byte("b.jsonl")))
}

func TestHeuristicRun(t *testing.T) {
	out := filepath.Join(t.TempDir(), "eval.jsonl")
	stdout, err := runCLI(t, "-chooser", "heuristic", "-runs", "1", "-scenarios", "single_batch_cooker,away_thu_fri", "-out", out)
	require.NoError(t, err)
	assert.Contains(t, stdout, "Report 0123456789ab heuristic/h1/")
	assert.Contains(t, stdout, "single_batch_cooker")
	assert.Contains(t, stdout, "away_thu_fri")
	assert.NotContains(t, stdout, "FAIL")
	b, err := os.ReadFile(out)
	require.NoError(t, err)
	assert.Equal(t, 2, bytes.Count(b, []byte("\n")))
}

func TestRunFlags(t *testing.T) {
	_, err := runCLI(t, "-runs", "0")
	assert.ErrorContains(t, err, "-runs must be at least 1")

	_, err = runCLI(t, "-scenarios", "nope")
	assert.ErrorContains(t, err, `unknown scenario "nope"`)

	_, err = runCLI(t, "-chooser", "magic", "-scenarios", "family_daily", "-out", filepath.Join(t.TempDir(), "x.jsonl"))
	assert.ErrorContains(t, err, `unknown chooser "magic"`)

	_, err = runCLI(t, "-compare", filepath.Join(t.TempDir(), "missing.jsonl"))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

// Live mode needs PLANTRY_EVAL_LIVE=1 and a provider; it is never reached
// here without them.
func TestLiveModeConfig(t *testing.T) {
	env := map[string]string{"PLANTRY_EVAL_LIVE": "1"}
	getenv := func(k string) string { return env[k] }
	var out bytes.Buffer
	err := run(context.Background(), []string{"-chooser", "llm", "-scenarios", "family_daily"}, &out, getenv, fixedSHA, fixedNow)
	assert.ErrorContains(t, err, "live mode needs PLANTRY_AI_API_KEY")

	env["PLANTRY_AI_API_KEY"], env["PLANTRY_AI_MODEL"], env["PLANTRY_AI_PROVIDER"] = "k", "m", "fake"
	err = run(context.Background(), []string{"-chooser", "llm", "-scenarios", "family_daily"}, &out, getenv, fixedSHA, fixedNow)
	assert.ErrorContains(t, err, `not "fake"`)

	l, model, err := live(func(k string) string {
		return map[string]string{"PLANTRY_AI_PROVIDER": "anthropic", "PLANTRY_AI_API_KEY": "k"}[k]
	}, "claude-x")
	require.NoError(t, err)
	assert.Equal(t, "anthropic", l.Provider)
	assert.Equal(t, "claude-x", model)
}
