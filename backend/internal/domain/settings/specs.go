package settings

import "github.com/jaltszeimer/plantry/backend/internal/domain/airun"

// registry defines every setting the app knows about. Unknown keys are
// rejected by the service. Order matters only for List.
var registry = []Spec{
	{
		Key:        KeyAIProvider,
		Kind:       KindEnum,
		EnvVar:     "PLANTRY_AI_PROVIDER",
		AllowEmpty: true,
		Enum:       []string{"", "openai", "anthropic", "fake"},
	},
	{
		Key:        KeyAIModel,
		Kind:       KindString,
		EnvVar:     "PLANTRY_AI_MODEL",
		AllowEmpty: true,
	},
	{
		Key:        KeyAIAPIKey,
		Kind:       KindString,
		Encrypted:  true,
		EnvVar:     "PLANTRY_AI_API_KEY",
		AllowEmpty: true,
	},
	{
		Key:     KeyAIRateLimit,
		Kind:    KindInt,
		EnvVar:  "PLANTRY_AI_RATE_LIMIT_PER_MIN",
		Default: "10",
	},
	{
		Key:        KeyAIFakeScript,
		Kind:       KindString,
		EnvVar:     "PLANTRY_AI_FAKE_SCRIPT",
		AllowEmpty: true,
	},
	{
		Key:        KeyFDCAPIKey,
		Kind:       KindString,
		Encrypted:  true,
		EnvVar:     "PLANTRY_FDC_API_KEY",
		AllowEmpty: true,
	},
	{
		Key:     KeyPlanAnchor,
		Kind:    KindEnum,
		Enum:    []string{"today", "next_shopping_day"},
		Default: "next_shopping_day",
	},
	{
		Key:      KeyPlanShoppingDay,
		Kind:     KindInt,
		Default:  "0",
		HasRange: true,
		Min:      0,
		Max:      6,
	},
	{
		Key:     KeySetupCompleted,
		Kind:    KindBool,
		Default: "false",
	},
	{
		Key:      KeyPlanMaxCookOccasionsPerDay,
		Kind:     KindInt,
		Default:  "1",
		HasRange: true,
		Min:      1,
		Max:      3,
	},
	{
		Key:     KeyPlanBatchCooking,
		Kind:    KindBool,
		Default: "true",
	},
	{
		Key:      KeyPlanDefaultKeepDays,
		Kind:     KindInt,
		Default:  "3",
		HasRange: true,
		Min:      1,
		Max:      7,
	},
	{
		Key:     KeyPlanFreezingEnabled,
		Kind:    KindBool,
		Default: "false",
	},
	{
		Key:      KeyPlanFreezerKeepDays,
		Kind:     KindInt,
		Default:  "90",
		HasRange: true,
		Min:      1,
		Max:      365,
	},
	{
		Key:      KeyPlanVarietyWeeks,
		Kind:     KindInt,
		Default:  "3",
		HasRange: true,
		Min:      0,
		Max:      8,
	},
	{
		Key:      KeyPlanExplorePerWeek,
		Kind:     KindInt,
		Default:  "0",
		HasRange: true,
		Min:      0,
		Max:      3,
	},
	{
		Key:     KeyDiscoveryDailyEnabled,
		Kind:    KindBool,
		Default: "true",
	},
	{
		Key:      KeyAIRunRetentionDays,
		Kind:     KindInt,
		Default:  "180",
		HasRange: true,
		Min:      7,
		Max:      3650,
	},
	{
		// JSON map model id → {in, out, cache_read?, cache_write?} in USD
		// per 1M tokens; overrides or extends the built-in price table.
		Key:     KeyAIPriceOverrides,
		Kind:    KindString,
		Default: "{}",
		Validate: func(raw string) error {
			_, err := airun.ParsePriceOverrides(raw)
			return err
		},
	},
}

// SpecFor returns the spec for a given key, or (Spec{}, false) if unknown.
func SpecFor(key string) (Spec, bool) {
	for _, s := range registry {
		if s.Key == key {
			return s, true
		}
	}
	return Spec{}, false
}
