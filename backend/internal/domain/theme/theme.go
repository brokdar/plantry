// Package theme holds the week themes (THEME-2..6): a focus for one planning
// window that shapes its cook sessions and new dishes and ends with it.
package theme

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/flavor"
)

// Kind is what a theme is about (THEME-2).
type Kind string

const (
	KindCuisine    Kind = "cuisine"
	KindIngredient Kind = "ingredient" // an ingredient or a season
	KindMood       Kind = "mood"       // a mood or a format
)

// Valid reports whether k is a known kind.
func (k Kind) Valid() bool {
	return k == KindCuisine || k == KindIngredient || k == KindMood
}

// Source is who set a theme.
type Source string

const (
	SourceUser       Source = "user"       // the plan page picker
	SourceChat       Source = "chat"       // the chat tool
	SourceSuggestion Source = "suggestion" // an accepted suggestion
	SourceAgain      Source = "again"      // "again?" from the window before
)

// MaxPerWindow is how many themes one window holds (THEME-2).
const MaxPerWindow = 2

// maxValueLen bounds a theme value.
const maxValueLen = 40

// Theme is one window's theme.
type Theme struct {
	ID          int64     `json:"id"`
	WindowStart time.Time `json:"-"`
	Kind        Kind      `json:"kind"`
	Value       string    `json:"value"`
	Source      Source    `json:"source"`
	// WholeWeek: the user asked for the whole week, so every cook session
	// leans to the theme, not 40-60 % (THEME-3).
	WholeWeek bool `json:"whole_week"`
}

// Norm validates t and returns it with its value lower case and trimmed.
func (t Theme) Norm() (Theme, error) {
	t.Value = strings.Join(strings.Fields(strings.ToLower(t.Value)), " ")
	if !t.Kind.Valid() {
		return t, fmt.Errorf("%w: theme kind %q", domain.ErrInvalidInput, t.Kind)
	}
	if t.Value == "" || len(t.Value) > maxValueLen {
		return t, fmt.Errorf("%w: theme value must be 1-%d characters", domain.ErrInvalidInput, maxValueLen)
	}
	return t, nil
}

// Matches reports whether a recipe with attributes a and tags fits t: a
// cuisine theme its cuisine; an ingredient or season theme a hero or a tag;
// a mood or format theme its format, kind, technique or a tag.
func (t Theme) Matches(a flavor.Attributes, tags []string) bool {
	eq := func(s string) bool { return s != "" && norm(s) == norm(t.Value) }
	tagged := slices.ContainsFunc(tags, eq)
	switch t.Kind {
	case KindCuisine:
		return eq(a.Cuisine)
	case KindIngredient:
		return tagged || slices.ContainsFunc(a.Heroes, eq)
	case KindMood:
		return tagged || eq(a.Format) || eq(a.Kind) || eq(a.Technique)
	}
	return false
}

// norm compares attribute keys ("middle_eastern") with typed values
// ("Middle Eastern").
func norm(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.ReplaceAll(s, "_", " "))), " ")
}

// Suggestion is the one theme the assistant offers a window (THEME-4).
type Suggestion struct {
	Kind  Kind   `json:"kind"`
	Value string `json:"value"`
	State string `json:"state"`
}

// Suggestion states.
const (
	StateOpen     = "open"
	StateAccepted = "accepted"
	StateDeclined = "declined"
)

// Decision is a decided suggestion, newest first in Store.RecentDecisions.
type Decision struct {
	State string
	At    time.Time
}
