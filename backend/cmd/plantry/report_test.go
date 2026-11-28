package main

import (
	"bytes"
	"context"
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
	md, err := buildReport(context.Background(), path, 30, now)
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
