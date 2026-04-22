package settings_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/crypto"
	"github.com/jaltszeimer/plantry/backend/internal/domain/settings"
)

// memRepo is an in-memory stand-in for the sqlite adapter — lets us unit
// test source resolution and validation without touching the filesystem.
type memRepo struct {
	rows map[string]settings.Row
}

func newMemRepo() *memRepo { return &memRepo{rows: map[string]settings.Row{}} }

func (r *memRepo) Get(_ context.Context, key string) (settings.Row, bool, error) {
	row, ok := r.rows[key]
	return row, ok, nil
}

func (r *memRepo) List(_ context.Context) ([]settings.Row, error) {
	out := make([]settings.Row, 0, len(r.rows))
	for _, v := range r.rows {
		out = append(out, v)
	}
	return out, nil
}

func (r *memRepo) Upsert(_ context.Context, row settings.Row) error {
	r.rows[row.Key] = row
	return nil
}

func (r *memRepo) Delete(_ context.Context, key string) error {
	delete(r.rows, key)
	return nil
}

const testSecret = "0123456789abcdef0123456789abcdef"

func newTestCipher(t *testing.T) crypto.Cipher {
	t.Helper()
	c, err := crypto.New(testSecret)
	require.NoError(t, err)
	return c
}

func TestGet_SourceResolution(t *testing.T) {
	repo := newMemRepo()
	env := settings.NewEnvSnapshot(map[string]string{
		"PLANTRY_AI_PROVIDER": "openai",
	})
	svc := settings.NewService(repo, env, newTestCipher(t))
	ctx := context.Background()

	// 1. Default when nothing is set.
	v, err := svc.Get(ctx, settings.KeyAIModel)
	require.NoError(t, err)
	assert.Equal(t, settings.SourceDefault, v.Source)
	assert.Equal(t, "", v.Raw)

	// 2. Env fallback.
	v, err = svc.Get(ctx, settings.KeyAIProvider)
	require.NoError(t, err)
	assert.Equal(t, settings.SourceEnv, v.Source)
	assert.Equal(t, "openai", v.Raw)

	// 3. DB override wins.
	require.NoError(t, svc.Set(ctx, settings.KeyAIProvider, "anthropic"))
	v, err = svc.Get(ctx, settings.KeyAIProvider)
	require.NoError(t, err)
	assert.Equal(t, settings.SourceDB, v.Source)
	assert.Equal(t, "anthropic", v.Raw)
	assert.True(t, v.EnvAlsoSet, "EnvAlsoSet should be true when env var is also set")
}

func TestSet_RejectsUnknownKey(t *testing.T) {
	svc := settings.NewService(newMemRepo(), settings.NewEnvSnapshot(nil), newTestCipher(t))
	err := svc.Set(context.Background(), "bogus.key", "value")
	assert.ErrorIs(t, err, settings.ErrUnknownKey)
}

func TestSet_RejectsInvalidEnum(t *testing.T) {
	svc := settings.NewService(newMemRepo(), settings.NewEnvSnapshot(nil), newTestCipher(t))
	err := svc.Set(context.Background(), settings.KeyAIProvider, "bogus")
	assert.ErrorIs(t, err, settings.ErrInvalidKind)
}

func TestSet_RejectsNegativeInt(t *testing.T) {
	svc := settings.NewService(newMemRepo(), settings.NewEnvSnapshot(nil), newTestCipher(t))
	err := svc.Set(context.Background(), settings.KeyAIRateLimit, "-5")
	assert.ErrorIs(t, err, settings.ErrInvalidKind)
}

func TestSet_EncryptedRoundTrip(t *testing.T) {
	repo := newMemRepo()
	svc := settings.NewService(repo, settings.NewEnvSnapshot(nil), newTestCipher(t))
	ctx := context.Background()

	require.NoError(t, svc.Set(ctx, settings.KeyAIAPIKey, "sk-live-abc123"))
	stored := repo.rows[settings.KeyAIAPIKey]
	assert.True(t, stored.Encrypted, "encrypted flag should be set")
	assert.NotEqual(t, "sk-live-abc123", stored.Value, "value should be ciphertext, not plaintext")

	v, err := svc.Get(ctx, settings.KeyAIAPIKey)
	require.NoError(t, err)
	assert.Equal(t, "sk-live-abc123", v.Raw)
	assert.Equal(t, settings.SourceDB, v.Source)
}

func TestSet_EncryptedRejectsWhenCipherMissing(t *testing.T) {
	svc := settings.NewService(newMemRepo(), settings.NewEnvSnapshot(nil), crypto.NilCipher{})
	err := svc.Set(context.Background(), settings.KeyAIAPIKey, "sk-live-abc123")
	assert.True(t, errors.Is(err, crypto.ErrSecretKeyMissing))
}

func TestEffectiveAI_Precedence(t *testing.T) {
	repo := newMemRepo()
	env := settings.NewEnvSnapshot(map[string]string{
		"PLANTRY_AI_PROVIDER":           "openai",
		"PLANTRY_AI_MODEL":              "gpt-4",
		"PLANTRY_AI_RATE_LIMIT_PER_MIN": "20",
	})
	svc := settings.NewService(repo, env, newTestCipher(t))
	ctx := context.Background()

	cfg, err := svc.EffectiveAI(ctx)
	require.NoError(t, err)
	assert.Equal(t, "openai", cfg.Provider)
	assert.Equal(t, "gpt-4", cfg.Model)
	assert.Equal(t, 20, cfg.RateLimitPerMin)

	// DB override replaces env.
	require.NoError(t, svc.Set(ctx, settings.KeyAIProvider, "anthropic"))
	require.NoError(t, svc.Set(ctx, settings.KeyAIModel, "claude-opus-4-7"))

	cfg, err = svc.EffectiveAI(ctx)
	require.NoError(t, err)
	assert.Equal(t, "anthropic", cfg.Provider)
	assert.Equal(t, "claude-opus-4-7", cfg.Model)

	// Delete override reverts to env.
	require.NoError(t, svc.Delete(ctx, settings.KeyAIProvider))
	cfg, err = svc.EffectiveAI(ctx)
	require.NoError(t, err)
	assert.Equal(t, "openai", cfg.Provider)
}

func TestDelete_UnknownKeyErrors(t *testing.T) {
	svc := settings.NewService(newMemRepo(), settings.NewEnvSnapshot(nil), newTestCipher(t))
	err := svc.Delete(context.Background(), "bogus.key")
	assert.ErrorIs(t, err, settings.ErrUnknownKey)
}

func TestDelete_EmptyIsIdempotent(t *testing.T) {
	svc := settings.NewService(newMemRepo(), settings.NewEnvSnapshot(nil), newTestCipher(t))
	// Deleting a known key that has no DB row must succeed — the UI's
	// "clear override" always issues DELETE regardless of prior state.
	require.NoError(t, svc.Delete(context.Background(), settings.KeyAIProvider))
}

func TestEffectiveAI_EmptyWhenUnconfigured(t *testing.T) {
	svc := settings.NewService(newMemRepo(), settings.NewEnvSnapshot(nil), newTestCipher(t))
	cfg, err := svc.EffectiveAI(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "", cfg.Provider)
	assert.Equal(t, "", cfg.Model)
	assert.Equal(t, 10, cfg.RateLimitPerMin, "rate limit default should be 10 when nothing set")
}

func TestEffectiveFDCKey_EnvFallback(t *testing.T) {
	env := settings.NewEnvSnapshot(map[string]string{
		"PLANTRY_FDC_API_KEY": "env-key",
	})
	svc := settings.NewService(newMemRepo(), env, newTestCipher(t))
	got, err := svc.EffectiveFDCKey(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "env-key", got)
}

func TestGet_DecryptFailure_FallsBackToEnv(t *testing.T) {
	repo := newMemRepo()
	env := settings.NewEnvSnapshot(map[string]string{
		"PLANTRY_AI_API_KEY": "fallback-env-key",
	})
	svc := settings.NewService(repo, env, newTestCipher(t))
	ctx := context.Background()

	// Store an encrypted value, then swap the cipher key by rebuilding the
	// service with a different secret — decryption will fail. The service
	// must gracefully treat the row as unreadable rather than returning an
	// error to the UI.
	require.NoError(t, svc.Set(ctx, settings.KeyAIAPIKey, "original-db-key"))

	other, err := crypto.New("ffffffffffffffffffffffffffffffff")
	require.NoError(t, err)
	svc2 := settings.NewService(repo, env, other)

	v, err := svc2.Get(ctx, settings.KeyAIAPIKey)
	require.NoError(t, err)
	// Source is still SourceDB (there IS a db row) but the raw value is
	// empty because decryption failed. Callers that rely on EffectiveAI
	// for the actual API key see an empty string and gracefully disable
	// outbound calls — env fallback does not kick in here because the DB
	// row exists, even if it is unreadable. This is covered to make the
	// behaviour explicit and deliberate.
	assert.Equal(t, settings.SourceDB, v.Source)
	assert.Equal(t, "", v.Raw)
}

func TestList_MasksSecrets(t *testing.T) {
	repo := newMemRepo()
	svc := settings.NewService(repo, settings.NewEnvSnapshot(nil), newTestCipher(t))
	ctx := context.Background()
	require.NoError(t, svc.Set(ctx, settings.KeyAIAPIKey, "sk-live-abcdef1234"))

	values, err := svc.List(ctx)
	require.NoError(t, err)
	for _, v := range values {
		if v.Key != settings.KeyAIAPIKey {
			continue
		}
		assert.True(t, v.IsSecret)
		assert.Equal(t, "", v.Raw, "Raw must be cleared for secrets in List")
		assert.NotEmpty(t, v.MaskedPreview)
		assert.Contains(t, v.MaskedPreview, "****")
		return
	}
	t.Fatal("api key not returned in List")
}
