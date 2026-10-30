package plate

import "time"

// PlanSnapshot is the only undo and copy format (D22): plates by cell with
// their components, plus the sessions they link by ref. Producers are the
// removing mutations (plate delete, clear range, session delete); the
// consumer is Service.Apply.
type PlanSnapshot struct {
	Plates   []SnapshotPlate   `json:"plates"`
	Sessions []SnapshotSession `json:"sessions"`
}

// IsEmpty reports whether the snapshot holds nothing.
func (s PlanSnapshot) IsEmpty() bool { return len(s.Plates) == 0 && len(s.Sessions) == 0 }

// SnapshotPlate is one plate by cell.
//
// PlateID is set when the plate itself survived the mutation and only some of
// its components were removed (a session delete keeps a plate that has other
// components). Apply then adds the components back to that plate if it still
// sits on the same cell, and otherwise recreates the plate.
type SnapshotPlate struct {
	PlateID    *int64              `json:"plate_id,omitempty"`
	Date       string              `json:"date"`
	SlotID     int64               `json:"slot_id"`
	Eaters     float64             `json:"eaters"`
	Note       *string             `json:"note"`
	Feedback   *SnapshotFeedback   `json:"feedback,omitempty"`
	Components []SnapshotComponent `json:"components"`
}

// SnapshotFeedback is a plate's feedback row.
type SnapshotFeedback struct {
	Status  string    `json:"status"`
	Note    *string   `json:"note"`
	RatedAt time.Time `json:"rated_at"`
}

// SnapshotComponent is one component. Composed: Portions (per eater) and,
// optionally, Session. Leaf: Amount, Unit and Grams. SortOrder is the
// component's original position on its plate.
type SnapshotComponent struct {
	FoodID    int64        `json:"food_id"`
	Portions  *float64     `json:"portions,omitempty"`
	Amount    *float64     `json:"amount,omitempty"`
	Unit      *string      `json:"unit,omitempty"`
	Grams     *float64     `json:"grams,omitempty"`
	SortOrder int          `json:"sort_order"`
	Session   *SessionLink `json:"session,omitempty"`
}

// SessionLink names the session a composed component eats from: Ref, a
// session in the same snapshot, or ID, a session that still exists. A
// composed component without a link gets a new fresh session.
type SessionLink struct {
	Ref string `json:"ref,omitempty"`
	ID  int64  `json:"id,omitempty"`
}

// SnapshotSession is a session the snapshot recreates.
type SnapshotSession struct {
	Ref            string     `json:"ref"`
	FoodID         int64      `json:"food_id"`
	CookDate       string     `json:"cook_date"`
	CookSlotID     int64      `json:"cook_slot_id"`
	Portions       float64    `json:"portions"`
	FrozenPortions float64    `json:"frozen_portions"`
	CookedAt       *time.Time `json:"cooked_at"`
	Note           *string    `json:"note"`
}

// ConflictMode is what Apply does with a snapshot plate whose cell is taken
// or marked No plate.
type ConflictMode string

const (
	// ConflictFail aborts the whole Apply with ErrCellOccupied or
	// ErrCellUnavailable; nothing is written. Restore uses it.
	ConflictFail ConflictMode = "fail"
	// ConflictSkip leaves the cell alone and reports it.
	ConflictSkip ConflictMode = "skip"
	// ConflictOverwrite removes the cell's plate (reported in
	// ApplyResult.Removed) and places the snapshot's. A No-plate cell is
	// skipped.
	ConflictOverwrite ConflictMode = "overwrite"
)

// ApplyOptions configures Apply.
type ApplyOptions struct {
	OnConflict ConflictMode
}

// ApplyResult reports what Apply did.
type ApplyResult struct {
	// PlateIDs are the plates written, in snapshot order: new plates and
	// surviving plates (SnapshotPlate.PlateID) that got components back.
	PlateIDs           []int64
	SkippedOccupied    []CellKey
	SkippedUnavailable []CellKey
	// Removed holds the plates ConflictOverwrite replaced.
	Removed PlanSnapshot
}
