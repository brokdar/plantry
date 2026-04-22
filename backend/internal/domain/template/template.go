// Package template holds the aggregate, repository port, and service for
// saved plate compositions (templates).
package template

import "time"

// Template is a named, reusable plate composition (e.g. "Curry night").
type Template struct {
	ID         int64
	Name       string
	Components []TemplateComponent
	CreatedAt  time.Time
}

// TemplateComponent is one component referenced by a template, with the same
// portions + sort_order shape as plate_components.
type TemplateComponent struct {
	ID          int64
	TemplateID  int64
	ComponentID int64
	Portions    float64
	SortOrder   int
}
