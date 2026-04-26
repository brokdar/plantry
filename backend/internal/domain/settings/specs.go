package settings

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
		Key:     KeyPlanWeekStartsOn,
		Kind:    KindEnum,
		Enum:    []string{"monday", "sunday", "saturday"},
		Default: "monday",
	},
	{
		Key:     KeyPlanAnchor,
		Kind:    KindEnum,
		Enum:    []string{"today", "next_shopping_day", "fixed_weekday"},
		Default: "today",
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
		Key:      KeyPlanWindowDays,
		Kind:     KindInt,
		Default:  "7",
		HasRange: true,
		Min:      5,
		Max:      14,
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

// Keys returns all known keys in registry order.
func Keys() []string {
	out := make([]string, len(registry))
	for i, s := range registry {
		out[i] = s.Key
	}
	return out
}
