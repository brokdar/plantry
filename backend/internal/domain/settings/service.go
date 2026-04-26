package settings

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/crypto"
)

// Service exposes read/write access to application settings with env-var
// fallback and transparent encryption of secret keys.
type Service struct {
	repo   Repository
	env    EnvSnapshot
	cipher crypto.Cipher
}

// NewService constructs a settings service. The cipher is used for encrypted
// keys only; pass crypto.NilCipher{} when no secret key is configured.
func NewService(repo Repository, env EnvSnapshot, cipher crypto.Cipher) *Service {
	if cipher == nil {
		cipher = crypto.NilCipher{}
	}
	return &Service{repo: repo, env: env, cipher: cipher}
}

// Get resolves the effective value for a key (db → env → default) and
// returns it along with the source. For encrypted keys, the Raw field is
// only populated if the value could be decrypted.
func (s *Service) Get(ctx context.Context, key string) (Value, error) {
	spec, ok := SpecFor(key)
	if !ok {
		return Value{}, fmt.Errorf("%w: %s", ErrUnknownKey, key)
	}
	return s.resolve(ctx, spec)
}

// List resolves every known key. Secret values are masked and the Raw field
// is cleared; callers must not leak Raw to HTTP responses for secrets
// regardless, but this keeps the surface safe.
func (s *Service) List(ctx context.Context) ([]Value, error) {
	out := make([]Value, 0, len(registry))
	for _, spec := range registry {
		v, err := s.resolve(ctx, spec)
		if err != nil {
			return nil, err
		}
		if v.IsSecret {
			v.MaskedPreview = Mask(v.Raw)
			v.Raw = ""
		}
		out = append(out, v)
	}
	return out, nil
}

// Set writes a DB override for the given key. Empty raw values are rejected
// unless the spec permits AllowEmpty; use Delete to clear an override.
func (s *Service) Set(ctx context.Context, key, raw string) error {
	spec, ok := SpecFor(key)
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnknownKey, key)
	}
	if spec.ReadOnly {
		return fmt.Errorf("%w: %s", ErrReadOnlyKey, key)
	}
	if err := validate(spec, raw); err != nil {
		return err
	}
	storeValue := raw
	if spec.Encrypted {
		enc, err := s.cipher.Encrypt([]byte(raw))
		if err != nil {
			return err
		}
		storeValue = enc
	}
	return s.repo.Upsert(ctx, Row{Key: key, Value: storeValue, Encrypted: spec.Encrypted})
}

// Delete removes the DB override for a key; the next Get reverts to env or
// default.
func (s *Service) Delete(ctx context.Context, key string) error {
	spec, ok := SpecFor(key)
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnknownKey, key)
	}
	if spec.ReadOnly {
		return fmt.Errorf("%w: %s", ErrReadOnlyKey, key)
	}
	return s.repo.Delete(ctx, key)
}

// EffectiveAI assembles the AI-specific configuration from the current
// effective settings. Missing values are returned as empty strings; callers
// decide how to handle (e.g. AIConfig.Provider == "" → AI disabled).
func (s *Service) EffectiveAI(ctx context.Context) (AIConfig, error) {
	provider, err := s.effectiveRaw(ctx, KeyAIProvider)
	if err != nil {
		return AIConfig{}, err
	}
	model, err := s.effectiveRaw(ctx, KeyAIModel)
	if err != nil {
		return AIConfig{}, err
	}
	apiKey, err := s.effectiveRaw(ctx, KeyAIAPIKey)
	if err != nil && !errors.Is(err, crypto.ErrSecretKeyMissing) {
		return AIConfig{}, err
	}
	fakeScript, err := s.effectiveRaw(ctx, KeyAIFakeScript)
	if err != nil {
		return AIConfig{}, err
	}
	rateStr, err := s.effectiveRaw(ctx, KeyAIRateLimit)
	if err != nil {
		return AIConfig{}, err
	}
	rate := 10
	if rateStr != "" {
		if n, convErr := strconv.Atoi(rateStr); convErr == nil && n >= 0 {
			rate = n
		}
	}
	return AIConfig{
		Provider:        provider,
		Model:           model,
		APIKey:          apiKey,
		FakeScript:      fakeScript,
		RateLimitPerMin: rate,
	}, nil
}

// EffectiveFDCKey returns the current FDC API key, falling back to env.
func (s *Service) EffectiveFDCKey(ctx context.Context) (string, error) {
	v, err := s.effectiveRaw(ctx, KeyFDCAPIKey)
	if err != nil && !errors.Is(err, crypto.ErrSecretKeyMissing) {
		return "", err
	}
	return v, nil
}

// CipherAvailable reports whether encrypted-key writes will succeed. The
// frontend uses this to show the "set PLANTRY_SECRET_KEY" banner.
func (s *Service) CipherAvailable() bool {
	return s.cipher.Available()
}

// resolve is the full value pipeline: db → env → default.
func (s *Service) resolve(ctx context.Context, spec Spec) (Value, error) {
	envRaw, envSet := s.env.Lookup(spec.EnvVar)

	row, ok, err := s.repo.Get(ctx, spec.Key)
	if err != nil {
		return Value{}, err
	}
	if ok {
		raw := row.Value
		if row.Encrypted {
			plain, decErr := s.cipher.Decrypt(row.Value)
			if decErr != nil {
				// Fall through to env/default. Surface the decryption error
				// in logs at the call site if needed; for API responses we
				// want the user to see env/default rather than a broken
				// entry.
				raw = ""
			} else {
				raw = string(plain)
			}
		}
		return Value{
			Key:        spec.Key,
			Raw:        raw,
			Source:     SourceDB,
			IsSecret:   spec.Encrypted,
			EnvAlsoSet: envSet,
			ReadOnly:   spec.ReadOnly,
		}, nil
	}

	if envSet {
		return Value{
			Key:      spec.Key,
			Raw:      envRaw,
			Source:   SourceEnv,
			IsSecret: spec.Encrypted,
			ReadOnly: spec.ReadOnly,
		}, nil
	}

	return Value{
		Key:      spec.Key,
		Raw:      spec.Default,
		Source:   SourceDefault,
		IsSecret: spec.Encrypted,
		ReadOnly: spec.ReadOnly,
	}, nil
}

func (s *Service) effectiveRaw(ctx context.Context, key string) (string, error) {
	v, err := s.Get(ctx, key)
	if err != nil {
		return "", err
	}
	return v.Raw, nil
}

func validate(spec Spec, raw string) error {
	if raw == "" && !spec.AllowEmpty {
		return fmt.Errorf("%w: %s requires a value", ErrInvalidKind, spec.Key)
	}
	switch spec.Kind {
	case KindInt:
		if raw == "" {
			return nil
		}
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			return fmt.Errorf("%w: %s must be a non-negative integer", ErrInvalidKind, spec.Key)
		}
		if spec.HasRange && (n < spec.Min || n > spec.Max) {
			return fmt.Errorf("%w: %s must be between %d and %d", ErrInvalidKind, spec.Key, spec.Min, spec.Max)
		}
	case KindBool:
		if raw == "" {
			return nil
		}
		if _, err := strconv.ParseBool(raw); err != nil {
			return fmt.Errorf("%w: %s must be true/false", ErrInvalidKind, spec.Key)
		}
	case KindEnum:
		for _, opt := range spec.Enum {
			if opt == raw {
				return nil
			}
		}
		return fmt.Errorf("%w: %s must be one of %v", ErrInvalidKind, spec.Key, spec.Enum)
	case KindString:
		// no further validation
	}
	return nil
}
