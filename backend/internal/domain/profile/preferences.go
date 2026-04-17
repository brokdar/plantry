package profile

// PreferenceKeyLikes is the key under Profile.Preferences that stores component
// tags the user has loved (string slice).
const PreferenceKeyLikes = "likes"

// PreferenceKeyDislikes is the key under Profile.Preferences that stores
// component tags the user has disliked (string slice).
const PreferenceKeyDislikes = "dislikes"

// ApplyFeedback returns a new preferences map that incorporates a plate
// feedback event. The update is append-only: a tag that ends up in "likes"
// because a plate was loved is never retracted when a later plate bearing the
// same tag is disliked. Both entries accumulate. This keeps the heuristic
// simple and makes each feedback event independently meaningful; fine-grained
// preference tuning belongs to the agent's record_preference tool.
//
// status semantics:
//   - "loved"    → append tags to "likes"
//   - "disliked" → append tags to "dislikes"
//   - "cooked"   → no-op (cook tracking lives on the component row)
//   - "skipped"  → no-op
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

	var key string
	switch status {
	case "loved":
		key = PreferenceKeyLikes
	case "disliked":
		key = PreferenceKeyDislikes
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
