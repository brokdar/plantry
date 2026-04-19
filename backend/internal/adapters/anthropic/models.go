package anthropic

// Model describes a selectable Anthropic model for the settings UI. The
// provider does not expose a public /v1/models endpoint, so this list is
// hand-maintained. Operators may additionally type a custom model id in the
// UI if they need one not listed here.
type Model struct {
	ID          string
	DisplayName string
}

// KnownModels returns the curated model list. Keep Opus first (most capable),
// Sonnet middle (balanced), Haiku last (fastest/cheapest) — the frontend
// renders them in order.
func KnownModels() []Model {
	return []Model{
		{ID: "claude-opus-4-7", DisplayName: "Claude Opus 4.7"},
		{ID: "claude-opus-4-6", DisplayName: "Claude Opus 4.6"},
		{ID: "claude-sonnet-4-6", DisplayName: "Claude Sonnet 4.6"},
		{ID: "claude-haiku-4-5", DisplayName: "Claude Haiku 4.5"},
	}
}
