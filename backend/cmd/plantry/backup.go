package main

import (
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

// runBackup implements `plantry backup -out <file>`: a consistent copy of the
// configured database (VACUUM INTO, safe under WAL while the server runs)
// plus the secret.key that encrypts stored API keys, copied beside it.
// Restore is: stop, replace plantry.db and secret.key, start.
func runBackup(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("backup", flag.ContinueOnError)
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
	if err := backupDB(cfg.DBPath, *out); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "backup written to %s\n", *out)
	return nil
}

// backupDB copies the database at src to dst and the secret key file that
// sits next to src (if any) to dst's directory.
func backupDB(src, dst string) error {
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("backup: source database: %w", err)
	}
	if _, err := os.Stat(dst); err == nil {
		return fmt.Errorf("backup: %s already exists", dst)
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
	if err := copySecretKey(filepath.Join(filepath.Dir(src), secretKeyFile), filepath.Join(filepath.Dir(dst), secretKeyFile)); err != nil {
		// Don't leave a half backup behind: a rerun would be refused.
		_ = os.Remove(dst)
		return err
	}
	return nil
}

func vacuumInto(conn *sql.DB, dst string) error {
	_, err := conn.Exec("VACUUM INTO '" + strings.ReplaceAll(dst, "'", "''") + "'")
	return err
}

// copySecretKey copies the key file when there is one (an env-supplied key
// has no file) and it is not already in place.
func copySecretKey(src, dst string) error {
	if filepath.Clean(src) == filepath.Clean(dst) {
		return nil
	}
	in, err := os.Open(src)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("backup: %w", err)
	}
	defer func() { _ = in.Close() }()
	f, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("backup: %w", err)
	}
	if _, err := io.Copy(f, in); err != nil {
		_ = f.Close()
		return fmt.Errorf("backup: %w", err)
	}
	return f.Close()
}
