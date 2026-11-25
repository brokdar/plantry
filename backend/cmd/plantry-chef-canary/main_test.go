package main

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/crypto"
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

// settingsDB writes a server-style database with an encrypted fake key.
func settingsDB(t *testing.T, secret string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "plantry.db")
	conn, err := sql.Open("sqlite", "file:"+path)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	_, err = conn.Exec(`CREATE TABLE app_settings (key TEXT PRIMARY KEY, value TEXT NOT NULL, encrypted INTEGER NOT NULL DEFAULT 0)`)
	require.NoError(t, err)
	c, err := crypto.New(secret)
	require.NoError(t, err)
	enc, err := c.Encrypt([]byte("sk-fake"))
	require.NoError(t, err)
	for _, r := range [][3]any{{"ai.provider", "anthropic", 0}, {"ai.model", "m-stored", 0}, {"ai.api_key", enc, 1}} {
		_, err = conn.Exec(`INSERT INTO app_settings VALUES (?, ?, ?)`, r[0], r[1], r[2])
		require.NoError(t, err)
	}
	return path
}

const testSecret = "0123456789abcdef0123456789abcdef"

func TestReadStoredAI_DecryptsWithKeyFileOrEnv(t *testing.T) {
	path := settingsDB(t, testSecret)
	keyFile := filepath.Join(filepath.Dir(path), "secret.key")
	require.NoError(t, os.WriteFile(keyFile, []byte(testSecret+"\n"), 0o600))

	got, err := readStoredAI(path, "", "")
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"ai.provider": "anthropic", "ai.model": "m-stored", "ai.api_key": "sk-fake"}, got)

	require.NoError(t, os.Remove(keyFile))
	got, err = readStoredAI(path, "", testSecret)
	require.NoError(t, err)
	assert.Equal(t, "sk-fake", got["ai.api_key"])

	_, err = readStoredAI(path, "", "ffffffffffffffffffffffffffffffff")
	require.ErrorContains(t, err, "wrong secret key")
	assert.NotContains(t, err.Error(), "sk-fake")
	_, err = readStoredAI(path, "", "")
	require.ErrorContains(t, err, "PLANTRY_SECRET_KEY")
}

func TestNewClient_EnvWinsOverSettingsDB(t *testing.T) {
	path := settingsDB(t, testSecret)
	env := map[string]string{"PLANTRY_CHEF_CANARY_LIVE": "1", "PLANTRY_SECRET_KEY": testSecret, "PLANTRY_AI_MODEL": "m-env"}
	_, provider, model, err := newClient(func(k string) string { return env[k] }, "", "", path, "")
	require.NoError(t, err)
	assert.Equal(t, "anthropic", provider)
	assert.Equal(t, "m-env", model)

	_, _, model, err = newClient(func(k string) string { return env[k] }, "m-flag", "", path, "")
	require.NoError(t, err)
	assert.Equal(t, "m-flag", model)
}

func TestNewClient_SettingsDBStillNeedsTheLiveFlag(t *testing.T) {
	path := settingsDB(t, testSecret)
	_, _, _, err := newClient(func(string) string { return "" }, "", "", path, "")
	require.ErrorContains(t, err, "PLANTRY_CHEF_CANARY_LIVE=1")
}
