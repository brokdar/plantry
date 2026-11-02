package anthropic

import "strings"

// Model describes an Anthropic model: its selectable display name and the
// request rules it needs. The provider exposes no public /v1/models endpoint,
// so this table is hand-maintained; operators may type a custom id in the UI.
type Model struct {
	// Prefix matches the model id (and dated variants such as
	// "claude-opus-5-5-20261001"); the longest matching prefix wins.
	Prefix      string
	DisplayName string
	// Sampling is false when the model rejects `temperature`.
	Sampling bool
	// ForcedTool is false when the model rejects a forced tool_choice
	// (always-thinking models); a forced tool becomes auto plus a system line.
	ForcedTool bool
}

// models is ordered newest family first: the settings UI renders it in this
// order, and the first entry's rules apply to ids no prefix matches.
var models = []Model{
	{Prefix: "claude-fable-5-1", DisplayName: "Claude Fable 5.1"},
	{Prefix: "claude-opus-5-5", DisplayName: "Claude Opus 5.5"},
	{Prefix: "claude-opus-5", DisplayName: "Claude Opus 5"},
	{Prefix: "claude-opus-4-8", DisplayName: "Claude Opus 4.8", ForcedTool: true},
	{Prefix: "claude-opus-4-7", DisplayName: "Claude Opus 4.7", ForcedTool: true},
	{Prefix: "claude-opus-4-6", DisplayName: "Claude Opus 4.6", Sampling: true, ForcedTool: true},
	{Prefix: "claude-sonnet-5-5", DisplayName: "Claude Sonnet 5.5"},
	{Prefix: "claude-sonnet-5", DisplayName: "Claude Sonnet 5"},
	{Prefix: "claude-sonnet-4-6", DisplayName: "Claude Sonnet 4.6", Sampling: true, ForcedTool: true},
	{Prefix: "claude-haiku-5-5", DisplayName: "Claude Haiku 5.5"},
	{Prefix: "claude-haiku-4-5", DisplayName: "Claude Haiku 4.5", Sampling: true, ForcedTool: true},
}

// Models returns the table in display order.
func Models() []Model { return append([]Model(nil), models...) }

// rulesFor returns the rules of the longest prefix matching id, else those of
// the newest family (the table's first entry).
func rulesFor(id string) Model {
	best := -1
	for i, m := range models {
		if strings.HasPrefix(id, m.Prefix) && (best < 0 || len(m.Prefix) > len(models[best].Prefix)) {
			best = i
		}
	}
	if best < 0 {
		return models[0]
	}
	return models[best]
}
