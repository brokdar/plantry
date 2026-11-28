package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/crypto"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/fake"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/llmresolver"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	settingsdom "github.com/jaltszeimer/plantry/backend/internal/domain/settings"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

const secretAPIKey = "sk-test-SECRET-0123456789"

// The startup line names the build, schema, Chef switch, provider, model and
// the local day; the API key never appears in it.
func TestLogStartup(t *testing.T) {
	ctx := context.Background()
	conn := testhelper.NewTestDB(t)
	cipher, err := crypto.New("0123456789abcdef0123456789abcdef")
	require.NoError(t, err)
	svc := settingsdom.NewService(sqlite.NewAppSettingsRepo(conn), settingsdom.NewEnvSnapshot(nil), cipher)
	require.NoError(t, seedFreshInstall(ctx, svc, true))
	require.NoError(t, svc.Set(ctx, settingsdom.KeyAIProvider, "anthropic"))
	require.NoError(t, svc.Set(ctx, settingsdom.KeyAIModel, "some-model"))
	require.NoError(t, svc.Set(ctx, settingsdom.KeyAIAPIKey, secretAPIKey))

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	berlin := time.FixedZone("CET", 3600)
	require.NoError(t, logStartup(ctx, logger, conn, svc, time.Date(2026, 11, 28, 23, 30, 0, 0, berlin)))

	var line map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &line))
	assert.Equal(t, "server.start", line["msg"])
	assert.Equal(t, true, line["chef_enabled"])
	assert.Equal(t, "anthropic", line["ai_provider"])
	assert.Equal(t, "some-model", line["ai_model"])
	assert.Equal(t, "2026-11-28", line["today"])
	assert.Equal(t, "CET", line["tz"])
	assert.Equal(t, version, line["version"])
	assert.Greater(t, line["schema_version"], float64(0))
	assert.NotContains(t, buf.String(), secretAPIKey)
	assert.NotContains(t, buf.String(), "0123456789abcdef0123456789abcdef")
}

// No log line, at debug level, carries the API key or the secret key: not the
// startup line, not the lines of an AI call through the resolver and recorder.
func TestLogs_NeverCarryKeys(t *testing.T) {
	ctx := context.Background()
	const secretKey = "0123456789abcdef0123456789abcdef"
	conn := testhelper.NewTestDB(t)
	cipher, err := crypto.New(secretKey)
	require.NoError(t, err)
	svc := settingsdom.NewService(sqlite.NewAppSettingsRepo(conn), settingsdom.NewEnvSnapshot(nil), cipher)
	require.NoError(t, svc.Set(ctx, settingsdom.KeyAIProvider, "fake"))
	require.NoError(t, svc.Set(ctx, settingsdom.KeyAIModel, "fake-model"))
	require.NoError(t, svc.Set(ctx, settingsdom.KeyAIAPIKey, secretAPIKey))
	script := filepath.Join(t.TempDir(), "script.json")
	require.NoError(t, os.WriteFile(script, []byte(`{"turns":[{"stop_reason":"end_turn","message":{"content":[{"type":"text","text":"hello"}]},"usage":{"input_tokens":10,"output_tokens":2}}]}`), 0o600))
	require.NoError(t, svc.Set(ctx, settingsdom.KeyAIFakeScript, script))

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	require.NoError(t, logStartup(ctx, logger, conn, svc, time.Now()))

	rec := airun.NewRecorder(sqlite.NewAIRunRepo(conn), logger, time.Now)
	resolver := llmresolver.New(svc, func(provider, apiKey, fakeScript string) (llm.Client, error) {
		c, err := fake.New(fakeScript)
		if err != nil {
			return nil, err
		}
		return rec.Meter(c, provider), nil
	})
	client, model, err := resolver.Current(ctx)
	require.NoError(t, err)
	runCtx, run := rec.Start(ctx, airun.Start{Kind: airun.KindChat})
	out, err := client.Complete(runCtx, llm.Request{Model: model})
	require.NoError(t, err)
	require.Equal(t, "hello", out)
	run.Finish(runCtx, airun.StatusOK, "")

	logs := buf.String()
	require.Contains(t, logs, "ai.run.end", "the AI call was logged")
	require.Contains(t, logs, "server.start")
	assert.NotContains(t, logs, secretAPIKey)
	assert.NotContains(t, logs, secretKey)
}
