// Package settings provides a runtime, DB-backed store for user-editable
// application options with env-var fallback. Values are resolved in the order
// database > environment > default. API keys and similarly sensitive values
// are encrypted at rest via adapters/crypto.
package settings

import "errors"

// Source describes where the effective value for a setting comes from.
type Source string

const (
	SourceDB      Source = "db"
	SourceEnv     Source = "env"
	SourceDefault Source = "default"
)

// Sentinel errors.
var (
	ErrUnknownKey  = errors.New("unknown settings key")
	ErrReadOnlyKey = errors.New("settings key is read-only")
	ErrInvalidKind = errors.New("invalid value for settings key")
)

// Kind describes the canonical type of a setting's value.
type Kind string

const (
	KindString Kind = "string"
	KindInt    Kind = "int"
	KindBool   Kind = "bool"
	KindEnum   Kind = "enum"
)

// Spec describes a single setting: its kind, whether it stores a secret that
// must be encrypted at rest, optional env-var fallback, and the default used
// when neither db nor env contributes.
type Spec struct {
	Key        string
	Kind       Kind
	Encrypted  bool
	EnvVar     string
	Default    string
	AllowEmpty bool
	Enum       []string // valid values when Kind == KindEnum
	ReadOnly   bool     // true for infra/system info surfaced in /api/settings/system
}

// Value is a resolved setting result.
type Value struct {
	Key           string
	Raw           string
	Source        Source
	IsSecret      bool
	MaskedPreview string
	EnvAlsoSet    bool
	ReadOnly      bool
}

// Mask renders a secret with only the first and last few chars visible. The
// full value is replaced with a fixed-width mask when too short to safely
// preview.
func Mask(raw string) string {
	if raw == "" {
		return ""
	}
	if len(raw) < 8 {
		return "********"
	}
	return raw[:3] + "****" + raw[len(raw)-4:]
}

// AIConfig is the effective AI configuration the resolver needs to build an
// LLM client.
type AIConfig struct {
	Provider        string
	Model           string
	APIKey          string
	FakeScript      string
	RateLimitPerMin int
}

// Editable keys — must stay in sync with the registry in specs.go.
const (
	KeyAIProvider   = "ai.provider"
	KeyAIModel      = "ai.model"
	KeyAIAPIKey     = "ai.api_key"
	KeyAIRateLimit  = "ai.rate_limit_per_min"
	KeyAIFakeScript = "ai.fake_script"
	KeyFDCAPIKey    = "fdc.api_key"
)
