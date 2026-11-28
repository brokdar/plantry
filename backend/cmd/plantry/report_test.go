package main

import (
	"bytes"
	"context"
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/db"
	settingsdom "github.com/jaltszeimer/plantry/backend/internal/domain/settings"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

var updateGolden = flag.Bool("update", false, "rewrite testdata/report.golden.md")

func seededDB(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "plantry.db")
	conn, err := openDB(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	require.NoError(t, migrate(conn))
	testhelper.SeedReport(t, conn)
	return path
}

func reportAt(t *testing.T, path string) string {
	t.Helper()
	now, err := time.Parse("2006-01-02 15:04:05", testhelper.ReportNow)
	require.NoError(t, err)
	md, err := buildReport(context.Background(), path, 30, now, settingsdom.NewEnvSnapshot(nil))
	require.NoError(t, err)
	return md
}

// The golden report of the fixture month. It is built while the writing
// connection of the fixture is still open (the server's case) and holds
// neither the stored API key nor the text typed into a run.
func TestReportGolden(t *testing.T) {
	md := reportAt(t, seededDB(t))
	assert.NotContains(t, md, "SECRETKEYVALUE")
	assert.NotContains(t, md, "TYPEDTEXT")

	golden := filepath.Join("testdata", "report.golden.md")
	if *updateGolden {
		require.NoError(t, os.WriteFile(golden, []byte(md), 0o644))
	}
	want, err := os.ReadFile(golden)
	require.NoError(t, err)
	assert.Equal(t, string(want), md)
}

func TestReportEmptyDatabaseSaysNoData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plantry.db")
	conn, err := openDB(path)
	require.NoError(t, err)
	require.NoError(t, migrate(conn))
	require.NoError(t, conn.Close())

	md := reportAt(t, path)
	assert.Contains(t, md, "No data: no AI runs in the window.")
	assert.Contains(t, md, "Nothing stands out.")
}

func TestReportCommandWritesFileAndRefusesExisting(t *testing.T) {
	path := seededDB(t)
	t.Setenv("PLANTRY_DB_PATH", path)
	out := filepath.Join(t.TempDir(), "report.md")

	var stdout, stderr bytes.Buffer
	require.NoError(t, runReport([]string{"-out", out, "-days", "7"}, &stdout, &stderr))
	assert.Contains(t, stdout.String(), out)
	got, err := os.ReadFile(out)
	require.NoError(t, err)
	assert.Contains(t, string(got), "window: last 7 days")

	assert.Error(t, runReport([]string{"-out", out}, &stdout, &stderr), "an existing file is not overwritten")
	assert.Error(t, runReport(nil, &stdout, &stderr), "-out is required")
	assert.Error(t, runReport([]string{"-out", filepath.Join(t.TempDir(), "x.md"), "-days", "0"}, &stdout, &stderr))
}

// A backup from an older build is read as it is: the sections whose tables
// the schema lacks say so and the rest renders. The report never migrates.
func TestReportOldSchemaSkipsMissingTables(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plantry.db")
	conn, err := openDB(path)
	require.NoError(t, err)
	goose.SetBaseFS(db.Migrations)
	require.NoError(t, goose.SetDialect("sqlite3"))
	require.NoError(t, goose.UpTo(conn, "migrations", 75))
	require.NoError(t, conn.Close())

	md := reportAt(t, path)
	assert.Contains(t, md, "- Schema version: 75")
	assert.Contains(t, md, "- Help answers: not available (schema 75; start the server once to update)")
	assert.Contains(t, md, "- Import checks: not available (schema 75; start the server once to update)")
	assert.Contains(t, md, "No data: no AI runs in the window.")
	assert.Contains(t, md, "- Chef jobs by kind/status: no data")
}

func TestReportSettingsShowEffectiveValueAndSource(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plantry.db")
	conn, err := openDB(path)
	require.NoError(t, err)
	require.NoError(t, migrate(conn))
	_, err = conn.Exec(`INSERT INTO app_settings (key, value, encrypted) VALUES ('ai.model', 'claude-haiku-5-5', 0)`)
	require.NoError(t, err)
	require.NoError(t, conn.Close())

	now, err := time.Parse("2006-01-02 15:04:05", testhelper.ReportNow)
	require.NoError(t, err)
	md, err := buildReport(context.Background(), path, 30, now, settingsdom.NewEnvSnapshot(map[string]string{
		"PLANTRY_AI_PROVIDER": "anthropic", "PLANTRY_AI_MODEL": "env-model", "PLANTRY_AI_API_KEY": "SECRETENV",
	}))
	require.NoError(t, err)
	assert.Contains(t, md, "- ai.model: claude-haiku-5-5 (stored)")
	assert.Contains(t, md, "- ai.provider: anthropic (environment)")
	assert.Contains(t, md, "- chef.monthly_cap: 5 (default)")
	assert.Contains(t, md, "- chef.enabled: ")
	assert.NotContains(t, md, "not set: default or environment")
	assert.NotContains(t, md, "SECRETENV")
}
