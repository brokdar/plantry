package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/crypto"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	settingsdom "github.com/jaltszeimer/plantry/backend/internal/domain/settings"
)

func freshInstallSvc(t *testing.T, path string) (*settingsdom.Service, bool) {
	t.Helper()
	conn, err := openDB(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	fresh, err := isFreshDB(conn)
	require.NoError(t, err)
	require.NoError(t, migrate(conn))
	cipher, err := crypto.New("0123456789abcdef0123456789abcdef")
	require.NoError(t, err)
	return settingsdom.NewService(sqlite.NewAppSettingsRepo(conn), settingsdom.NewEnvSnapshot(nil), cipher), fresh
}

// ON-3: a database created on this start gets the Chef on once; an upgrade
// keeps what it had (off without a key, or the stored choice).
func TestSeedFreshInstall(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "plantry.db")

	svc, fresh := freshInstallSvc(t, path)
	require.True(t, fresh)
	require.NoError(t, seedFreshInstall(ctx, svc, fresh))
	on, err := svc.ChefEnabled(ctx)
	require.NoError(t, err)
	require.True(t, on)

	// Restart: the schema exists, nothing is seeded again.
	svc, fresh = freshInstallSvc(t, path)
	require.False(t, fresh)
	require.NoError(t, svc.Set(ctx, settingsdom.KeyChefEnabled, "false"))
	require.NoError(t, seedFreshInstall(ctx, svc, fresh))
	on, err = svc.ChefEnabled(ctx)
	require.NoError(t, err)
	require.False(t, on, "an explicit off survives a restart")
}

func TestSeedFreshInstall_UpgradeKeepsDefaultOff(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "plantry.db")
	_, _ = freshInstallSvc(t, path) // schema exists, no key stored

	svc, fresh := freshInstallSvc(t, path)
	require.False(t, fresh)
	require.NoError(t, seedFreshInstall(ctx, svc, fresh))
	on, err := svc.ChefEnabled(ctx)
	require.NoError(t, err)
	require.False(t, on)

	require.NoError(t, svc.Set(ctx, settingsdom.KeyChefEnabled, "true"))
	require.NoError(t, seedFreshInstall(ctx, svc, fresh))
	on, err = svc.ChefEnabled(ctx)
	require.NoError(t, err)
	require.True(t, on, "an explicit on is unchanged")
}

// A start that fails on the secret key must leave the database fresh, so the
// next good start still seeds the Chef.
func TestRun_BadSecretKeyLeavesDBFresh(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plantry.db")
	t.Setenv("PLANTRY_DB_PATH", path)
	t.Setenv("PLANTRY_SECRET_KEY", "too-short")

	require.Error(t, run())

	conn, err := openDB(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	fresh, err := isFreshDB(conn)
	require.NoError(t, err)
	require.True(t, fresh)
}
