package profile

import (
	"reflect"
	"testing"
)

func TestApplyFeedback(t *testing.T) {
	tests := []struct {
		name   string
		prefs  map[string]any
		status string
		tags   []string
		want   map[string]any
	}{
		{
			name:   "loved appends tag to likes",
			prefs:  map[string]any{},
			status: "loved",
			tags:   []string{"spicy"},
			want:   map[string]any{"likes": []string{"spicy"}},
		},
		{
			name:   "disliked appends tag to dislikes",
			prefs:  map[string]any{},
			status: "disliked",
			tags:   []string{"mushroom"},
			want:   map[string]any{"dislikes": []string{"mushroom"}},
		},
		{
			name:   "loved with multiple tags appends all",
			prefs:  map[string]any{},
			status: "loved",
			tags:   []string{"spicy", "vegetarian", "quick"},
			want:   map[string]any{"likes": []string{"spicy", "vegetarian", "quick"}},
		},
		{
			name:   "cooked is no-op on prefs",
			prefs:  map[string]any{"foo": "bar"},
			status: "cooked",
			tags:   []string{"spicy"},
			want:   map[string]any{"foo": "bar"},
		},
		{
			name:   "skipped is no-op on prefs",
			prefs:  map[string]any{"foo": "bar"},
			status: "skipped",
			tags:   []string{"spicy"},
			want:   map[string]any{"foo": "bar"},
		},
		{
			name:   "empty tags is no-op",
			prefs:  map[string]any{"foo": "bar"},
			status: "loved",
			tags:   []string{},
			want:   map[string]any{"foo": "bar"},
		},
		{
			name:   "nil tags is no-op",
			prefs:  map[string]any{},
			status: "loved",
			tags:   nil,
			want:   map[string]any{},
		},
		{
			name:   "dedupes against existing likes",
			prefs:  map[string]any{"likes": []string{"spicy"}},
			status: "loved",
			tags:   []string{"spicy", "quick"},
			want:   map[string]any{"likes": []string{"spicy", "quick"}},
		},
		{
			name:   "normalizes []any from JSON round-trip",
			prefs:  map[string]any{"likes": []any{"spicy"}},
			status: "loved",
			tags:   []string{"quick"},
			want:   map[string]any{"likes": []string{"spicy", "quick"}},
		},
		{
			name:   "ignores non-string entries in []any",
			prefs:  map[string]any{"likes": []any{"spicy", 42, nil}},
			status: "loved",
			tags:   []string{"quick"},
			want:   map[string]any{"likes": []string{"spicy", "quick"}},
		},
		{
			name:   "preserves unrelated keys",
			prefs:  map[string]any{"foo": "bar", "baz": 1.0},
			status: "loved",
			tags:   []string{"spicy"},
			want:   map[string]any{"foo": "bar", "baz": 1.0, "likes": []string{"spicy"}},
		},
		{
			name:   "append-only: disliked on already-liked keeps both",
			prefs:  map[string]any{"likes": []string{"spicy"}},
			status: "disliked",
			tags:   []string{"spicy"},
			want:   map[string]any{"likes": []string{"spicy"}, "dislikes": []string{"spicy"}},
		},
		{
			name:   "nil prefs map tolerated",
			prefs:  nil,
			status: "loved",
			tags:   []string{"spicy"},
			want:   map[string]any{"likes": []string{"spicy"}},
		},
		{
			name:   "unknown status is no-op",
			prefs:  map[string]any{},
			status: "other",
			tags:   []string{"spicy"},
			want:   map[string]any{},
		},
		{
			name:   "empty string tag filtered out",
			prefs:  map[string]any{},
			status: "loved",
			tags:   []string{"", "spicy", ""},
			want:   map[string]any{"likes": []string{"spicy"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ApplyFeedback(tt.prefs, tt.status, tt.tags)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ApplyFeedback() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestApplyFeedback_DoesNotMutateInput(t *testing.T) {
	prefs := map[string]any{"likes": []string{"spicy"}}
	original := map[string]any{"likes": []string{"spicy"}}

	_ = ApplyFeedback(prefs, "loved", []string{"quick"})

	if !reflect.DeepEqual(prefs, original) {
		t.Errorf("ApplyFeedback mutated input prefs; got %#v, want %#v", prefs, original)
	}
}
