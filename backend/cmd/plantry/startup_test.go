package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/crypto"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
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
