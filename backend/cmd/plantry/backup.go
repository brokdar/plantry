package main

import (
	"bytes"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/jaltszeimer/plantry/backend/internal/config"
)

const secretKeyFile = "secret.key"

// backupHelp is the backup command's help text.
const backupHelp = `Usage: plantry backup -out <file>

Writes a consistent copy of the configured database (safe while the server
runs) to <file>, which must not exist, and copies the secret.key that
encrypts stored API keys beside it. An existing secret.key in the target
directory is kept when identical and refused when different (older backups
there need it). Without a secret.key file (the key comes from
PLANTRY_SECRET_KEY) the backup succeeds with a warning: keep that key
separately. Restore: stop, replace plantry.db and secret.key, start.
`

// runBackup implements `plantry backup -out <file>` (see backupHelp).
func runBackup(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("backup", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		_, _ = io.WriteString(stderr, backupHelp)
		fs.PrintDefaults()
	}
	out := fs.String("out", "", "path of the backup database file (must not exist)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *out == "" {
		return errors.New("backup: -out <file> is required")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if err := backupDB(cfg.DBPath, *out, stderr); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "backup written to %s\n", *out)
	return nil
}

// backupDB copies the database at src to dst and the secret key file that
// sits next to src (if any) to dst's directory. A missing key file is a
// warning on warn, not an error.
func backupDB(src, dst string, warn io.Writer) error {
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("backup: source database: %w", err)
	}
	if _, err := os.Stat(dst); err == nil {
		return fmt.Errorf("backup: %s already exists", dst)
	}
	srcKey, dstKey := filepath.Join(filepath.Dir(src), secretKeyFile), filepath.Join(filepath.Dir(dst), secretKeyFile)
	key, err := readSecretKey(srcKey, dstKey)
	if err != nil {
		return err
	}
	conn, err := openDB(src)
	if err != nil {
		return fmt.Errorf("backup: open %s: %w", src, err)
	}
	defer func() { _ = conn.Close() }()
	// Pre-create the file 0600: it holds the encrypted API keys. VACUUM INTO
	// accepts an empty existing file.
	f, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("backup: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("backup: %w", err)
	}
	if err := vacuumInto(conn, dst); err != nil {
		_ = os.Remove(dst)
		return fmt.Errorf("backup: %w", err)
	}
	if key == nil {
		if filepath.Clean(srcKey) != filepath.Clean(dstKey) {
			_, _ = fmt.Fprintf(warn, "warning: no %s beside %s; stored API keys need PLANTRY_SECRET_KEY to restore, keep it separately\n", secretKeyFile, src)
		}
		return nil
	}
	if err := writeFileAtomic(dstKey, key); err != nil {
		// Don't leave a half backup behind: a rerun would be refused.
		_ = os.Remove(dst)
		return fmt.Errorf("backup: %w", err)
	}
	return nil
}

func vacuumInto(conn *sql.DB, dst string) error {
	_, err := conn.Exec("VACUUM INTO '" + strings.ReplaceAll(dst, "'", "''") + "'")
	return err
}

// readSecretKey returns the key file at src to copy to dst, or nil when
// there is none (an env-supplied key) or dst already holds it. A different
// key at dst is an error: overwriting it would orphan the backups there.
func readSecretKey(src, dst string) ([]byte, error) {
	if filepath.Clean(src) == filepath.Clean(dst) {
		return nil, nil
	}
	key, err := os.ReadFile(src)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("backup: %w", err)
	}
	existing, err := os.ReadFile(dst)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return key, nil
	case err != nil:
		return nil, fmt.Errorf("backup: %w", err)
	case bytes.Equal(existing, key):
		return nil, nil
	default:
		return nil, fmt.Errorf("backup: %s already exists with a different key; use another directory", dst)
	}
}

// writeFileAtomic writes data to path (0600) through a temp file and a
// rename, so a failure never leaves a partial file at path.
func writeFileAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }() // no-op after the rename
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
