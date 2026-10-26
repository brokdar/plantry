package config

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/crypto"
)

const secretKeyFile = "secret.key"

// EnsureSecretKey fills c.SecretKey from the key file next to the database
// when PLANTRY_SECRET_KEY is unset. The env var always wins.
func (c *Config) EnsureSecretKey() error {
	if c.SecretKey != "" {
		return nil
	}
	key, err := LoadOrCreateSecretKey(filepath.Dir(c.DBPath))
	if err != nil {
		return err
	}
	c.SecretKey = key
	return nil
}

// LoadOrCreateSecretKey returns the key stored in <dir>/secret.key,
// generating a random 32-byte (64 hex chars) key with mode 0600 on first use.
func LoadOrCreateSecretKey(dir string) (string, error) {
	path := filepath.Join(dir, secretKeyFile)

	data, err := os.ReadFile(path)
	if err == nil {
		key := strings.TrimSpace(string(data))
		if len(key) < crypto.MinSecretLen {
			return "", fmt.Errorf("secret key file %s: must be at least %d chars", path, crypto.MinSecretLen)
		}
		return key, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("read secret key file: %w", err)
	}

	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate secret key: %w", err)
	}
	key := hex.EncodeToString(buf)

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create data dir: %w", err)
	}
	if err := writeFileAtomic(path, []byte(key)); err != nil {
		return "", fmt.Errorf("write secret key file: %w", err)
	}

	slog.Info("generated secret key", "path", path)
	return key, nil
}

// writeFileAtomic writes data to a temp file (mode 0600) in the same
// directory and renames it into place.
func writeFileAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }() // no-op after a successful rename

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
