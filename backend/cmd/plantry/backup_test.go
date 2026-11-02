package main

import (
	"bytes"
	"database/sql"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func rows(t *testing.T, conn *sql.DB, table string) int {
	t.Helper()
	var n int
	require.NoError(t, conn.QueryRow("SELECT COUNT(*) FROM "+table).Scan(&n))
	return n
}

// Z4-AC3 (AC-7.3): a backup taken while a write loop runs is a consistent
// snapshot: it opens, passes integrity_check, migrates (a no-op) and both
// tables written in one transaction have the same row count.
func TestBackupDuringWrites(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "data", "plantry.db")
	require.NoError(t, os.MkdirAll(filepath.Dir(src), 0o755))
	conn, err := openDB(src)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	require.NoError(t, migrate(conn))
	_, err = conn.Exec(`CREATE TABLE bk_a (n INTEGER); CREATE TABLE bk_b (n INTEGER)`)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(src), "secret.key"), []byte("0123456789abcdef0123456789abcdef"), 0o600))

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			tx, err := conn.Begin()
			if err != nil {
				return
			}
			_, _ = tx.Exec("INSERT INTO bk_a VALUES (?)", i)
			_, _ = tx.Exec("INSERT INTO bk_b VALUES (?)", i)
			_ = tx.Commit()
		}
	}()

	outDir := filepath.Join(dir, "out")
	require.NoError(t, os.MkdirAll(outDir, 0o755))
	dst := filepath.Join(outDir, "backup.db")
	var out bytes.Buffer
	t.Setenv("PLANTRY_DB_PATH", src)
	require.NoError(t, runBackup([]string{"-out", dst}, &out))
	close(stop)
	wg.Wait()
	assert.Contains(t, out.String(), dst)

	cp, err := openDB(dst)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cp.Close() })
	var ok string
	require.NoError(t, cp.QueryRow("PRAGMA integrity_check").Scan(&ok))
	assert.Equal(t, "ok", ok)
	require.NoError(t, migrate(cp))
	assert.Equal(t, rows(t, cp, "bk_a"), rows(t, cp, "bk_b"))
	assert.LessOrEqual(t, rows(t, cp, "bk_a"), rows(t, conn, "bk_a"))
	assert.Equal(t, rows(t, conn, "foods"), rows(t, cp, "foods"))

	key, err := os.ReadFile(filepath.Join(outDir, "secret.key"))
	require.NoError(t, err)
	assert.Equal(t, "0123456789abcdef0123456789abcdef", string(key))
}

func TestBackupRefusesExistingAndMissingOut(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "plantry.db")
	conn, err := openDB(src)
	require.NoError(t, err)
	require.NoError(t, conn.Close())
	t.Setenv("PLANTRY_DB_PATH", src)

	require.Error(t, runBackup(nil, &bytes.Buffer{}))
	existing := filepath.Join(dir, "x.db")
	require.NoError(t, os.WriteFile(existing, []byte("x"), 0o600))
	require.Error(t, runBackup([]string{"-out", existing}, &bytes.Buffer{}))
	// An env-supplied key has no file: the backup still succeeds.
	require.NoError(t, runBackup([]string{"-out", filepath.Join(t.TempDir(), "b.db")}, &bytes.Buffer{}))
}

func TestBackupDBFileModeAndCleanupOnKeyFailure(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "plantry.db")
	conn, err := openDB(src)
	require.NoError(t, err)
	require.NoError(t, conn.Close())

	outDir := filepath.Join(t.TempDir(), "out")
	require.NoError(t, os.MkdirAll(outDir, 0o755))
	dst := filepath.Join(outDir, "b.db")
	require.NoError(t, backupDB(src, dst))
	st, err := os.Stat(dst)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), st.Mode().Perm())

	// A secret.key that cannot be read removes the written backup DB.
	require.NoError(t, os.Mkdir(filepath.Join(dir, secretKeyFile), 0o755))
	dst2 := filepath.Join(outDir, "c.db")
	require.Error(t, backupDB(src, dst2))
	_, err = os.Stat(dst2)
	assert.True(t, os.IsNotExist(err))
}
