package profile

import "slices"

// PreferenceKeyLikes is the key under Profile.Preferences that stores component
// tags the user has loved (string slice).
const PreferenceKeyLikes = "likes"

// PreferenceKeyDislikes is the key under Profile.Preferences that stores
// component tags the user has disliked (string slice).
const PreferenceKeyDislikes = "dislikes"

// ApplyFeedback returns a new preferences map that incorporates a plate
// feedback event (spec §6.6): the tags join one list and leave the opposite
// one, so a tag is never liked and disliked at once. Callers drop generic
// tags first (feedback.Service).
//
// status semantics:
//   - "loved"    → append tags to "likes", remove them from "dislikes"
//   - "disliked" → append tags to "dislikes", remove them from "likes"
//   - anything else → no-op
//
// Empty or nil tag slices, empty-string tags, and unknown statuses all return
// an unmodified copy of prefs. The returned map is a new allocation; the input
// is never mutated (defensive against map aliasing between goroutines).
func ApplyFeedback(prefs map[string]any, status string, tags []string) map[string]any {
	out := make(map[string]any, len(prefs)+1)
	for k, v := range prefs {
		out[k] = v
	}

	var key, opposite string
	switch status {
	case "loved":
		key, opposite = PreferenceKeyLikes, PreferenceKeyDislikes
	case "disliked":
		key, opposite = PreferenceKeyDislikes, PreferenceKeyLikes
	default:
		return out
	}

	clean := make([]string, 0, len(tags))
	for _, t := range tags {
		if t != "" {
			clean = append(clean, t)
		}
	}
	if len(clean) == 0 {
		return out
	}

	existing := normalizeStringSlice(out[key])
	seen := make(map[string]struct{}, len(existing)+len(clean))
	merged := make([]string, 0, len(existing)+len(clean))
	for _, t := range existing {
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		merged = append(merged, t)
	}
	for _, t := range clean {
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		merged = append(merged, t)
	}
	out[key] = merged
	if v, ok := out[opposite]; ok {
		kept := make([]string, 0)
		for _, t := range normalizeStringSlice(v) {
			if !slices.Contains(clean, t) {
				kept = append(kept, t)
			}
		}
		out[opposite] = kept
	}
	return out
}

// normalizeStringSlice converts the stored value at a preference key to
// []string regardless of whether it was written as a typed []string (fresh in
// memory) or decoded as []any (after JSON round-trip through the DB).
// Non-string entries are silently dropped.
func normalizeStringSlice(v any) []string {
	switch s := v.(type) {
	case []string:
		return s
	case []any:
		out := make([]string, 0, len(s))
		for _, e := range s {
			if str, ok := e.(string); ok {
				out = append(out, str)
			}
		}
		return out
	default:
		return nil
	}
}
