package config_test

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/crypto"
	"github.com/jaltszeimer/plantry/backend/internal/config"
)

func TestLoadOrCreateSecretKey_GeneratesKey(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data") // missing dir is created

	key, err := config.LoadOrCreateSecretKey(dir)
	require.NoError(t, err)

	assert.Len(t, key, 64)
	_, err = hex.DecodeString(key)
	require.NoError(t, err, "key must be hex")
	assert.GreaterOrEqual(t, len(key), crypto.MinSecretLen)
	_, err = crypto.New(key)
	require.NoError(t, err)

	info, err := os.Stat(filepath.Join(dir, "secret.key"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestLoadOrCreateSecretKey_StableAcrossCalls(t *testing.T) {
	dir := t.TempDir()

	first, err := config.LoadOrCreateSecretKey(dir)
	require.NoError(t, err)
	second, err := config.LoadOrCreateSecretKey(dir)
	require.NoError(t, err)

	assert.Equal(t, first, second)
}

func TestLoadOrCreateSecretKey_TrimsWhitespace(t *testing.T) {
	dir := t.TempDir()
	want := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "secret.key"), []byte(want+"\n"), 0o600))

	key, err := config.LoadOrCreateSecretKey(dir)
	require.NoError(t, err)
	assert.Equal(t, want, key)
}

func TestLoadOrCreateSecretKey_ShortFile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "secret.key"), []byte("tooshort\n"), 0o600))

	_, err := config.LoadOrCreateSecretKey(dir)
	require.ErrorContains(t, err, "must be at least 32 chars")
}

func TestLoadOrCreateSecretKey_EmptyFile(t *testing.T) {
	for name, content := range map[string]string{"empty": "", "whitespace only": " \n\t\n"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "secret.key"), []byte(content), 0o600))

			_, err := config.LoadOrCreateSecretKey(dir)
			require.ErrorContains(t, err, "must be at least 32 chars")
		})
	}
}

func TestLoadOrCreateSecretKey_UnreadableFile(t *testing.T) {
	dir := t.TempDir()
	// A directory at the key path cannot be read as a file (works even as root).
	require.NoError(t, os.Mkdir(filepath.Join(dir, "secret.key"), 0o700))

	_, err := config.LoadOrCreateSecretKey(dir)
	require.ErrorContains(t, err, "read secret key file")
}

func TestEnsureSecretKey_EnvWins(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PLANTRY_DB_PATH", filepath.Join(dir, "plantry.db"))
	t.Setenv("PLANTRY_SECRET_KEY", "env-secret-0123456789abcdef0123456789")

	cfg, err := config.Load()
	require.NoError(t, err)
	require.NoError(t, cfg.EnsureSecretKey())

	assert.Equal(t, "env-secret-0123456789abcdef0123456789", cfg.SecretKey)
	_, err = os.Stat(filepath.Join(dir, "secret.key"))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestEnsureSecretKey_UsesFileNextToDB(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PLANTRY_DB_PATH", filepath.Join(dir, "plantry.db"))
	t.Setenv("PLANTRY_SECRET_KEY", "")

	cfg, err := config.Load()
	require.NoError(t, err)
	require.NoError(t, cfg.EnsureSecretKey())

	data, err := os.ReadFile(filepath.Join(dir, "secret.key"))
	require.NoError(t, err)
	assert.Equal(t, string(data), cfg.SecretKey)
}
