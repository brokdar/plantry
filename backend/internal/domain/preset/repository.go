package preset

import (
	"context"

	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// ListFilter is the search/filter shape consumed by Repository.List.
//
// Empty Search ⇒ no name filter. Empty SlotIDs ⇒ no slot filter (else: OR
// semantics; preset matches if any of its plates' slot_id is in the list).
// Empty Tags ⇒ no tag filter (else: AND semantics; preset must carry every
// tag in the list). Sort governs ordering: SortName (default) or SortRecent
// (last_used_at DESC NULLS LAST, then name).
type ListFilter struct {
	Search  string
	SlotIDs []int64
	Tags    []string
	Sort    SortOrder
	Limit   int // 0 ⇒ no limit
	Offset  int
}

// SortOrder controls the ordering of List results.
type SortOrder string

const (
	SortName   SortOrder = "name"
	SortRecent SortOrder = "recent"
)

// ListResult is the paginated shape returned by Repository.List.
type ListResult struct {
	Items []Preset
	Total int
}

// Repository is the persistence port for presets.
type Repository interface {
	Create(ctx context.Context, p *Preset) error
	Get(ctx context.Context, id int64) (*Preset, error)
	List(ctx context.Context, filter ListFilter) (*ListResult, error)

	// UpdateName changes the name and bumps updated_at. Returns the fresh row.
	UpdateName(ctx context.Context, id int64, name string) (*Preset, error)

	// ReplacePlates replaces the entire plates+components tree for preset id.
	// Plates inside plates argument are assigned new IDs.
	ReplacePlates(ctx context.Context, presetID int64, plates []Plate) error

	// ReplaceTags replaces the entire tag list for preset id.
	ReplaceTags(ctx context.Context, presetID int64, tags []string) error

	// AddTag and RemoveTag are partial-edit operations used by the agent.
	AddTag(ctx context.Context, presetID int64, tag string) error
	RemoveTag(ctx context.Context, presetID int64, tag string) error

	// TouchLastUsed sets last_used_at = now. Called after a successful Apply.
	TouchLastUsed(ctx context.Context, presetID int64) error

	Delete(ctx context.Context, id int64) error

	// KnownTags returns the top-N tags by frequency across all presets.
	KnownTags(ctx context.Context, limit int) ([]TagUsage, error)

	CountUsingFood(ctx context.Context, foodID int64) (int64, error)
}

// TagUsage is one row of the known-tags catalogue.
type TagUsage struct {
	Tag   string
	Count int64
}

// TxRunner runs fn inside a single transaction with preset + plate repos bound to it.
// Mirrors template.TxRunner so apply pipelines can write plates atomically with
// the preset's last_used_at update.
type TxRunner interface {
	RunInPresetTx(ctx context.Context, fn func(Repository, plate.Repository) error) error
}

// FoodLookup is the food-side surface presets need for kind-aware quantity
// validation and grams resolution on apply.
type FoodLookup interface {
	// Exists reports whether a food exists.
	Exists(ctx context.Context, foodID int64) (bool, error)
	// KindOf returns "leaf" or "composed".
	KindOf(ctx context.Context, foodID int64) (string, error)
}

// PlateSource reads plates by id, used when creating presets from existing
// planner plates.
type PlateSource interface {
	Get(ctx context.Context, plateID int64) (*plate.Plate, error)
}
