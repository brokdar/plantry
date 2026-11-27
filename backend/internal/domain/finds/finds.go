package finds

import (
	"context"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/gates"
	"github.com/jaltszeimer/plantry/backend/internal/domain/importer"
)

// State is a find's FIND-4 state.
type State string

// FIND-4 states.
const (
	StateSearching  State = "searching"
	StateReady      State = "ready"
	StateNeedsCheck State = "needs_check"
	StateFailed     State = "failed"
	StateInLibrary  State = "in_library"
	StateDiscarded  State = "discarded"
)

// ValidState reports whether s is a FIND-4 state.
func ValidState(s State) bool {
	switch s {
	case StateSearching, StateReady, StateNeedsCheck, StateFailed, StateInLibrary, StateDiscarded:
		return true
	}
	return false
}

// Trigger is why a find started (FIND-1).
type Trigger string

// Triggers. Chat and Discover are requests from the user (FIND-1c); the
// others are autonomous and arrive with later PRs.
const (
	TriggerChat     Trigger = "chat"
	TriggerDiscover Trigger = "discover"
	TriggerPlanning Trigger = "planning"
	// TriggerFill is "Fill my library" (COLD-3): requested work whose ready
	// finds enter the library on trial by themselves.
	TriggerFill Trigger = "fill"
)

// Requested reports whether the user asked for the find (GATE-4).
func (t Trigger) Requested() bool {
	return t == TriggerChat || t == TriggerDiscover || t == TriggerFill
}

// Origin is the label a Chef recipe carries (FIND-13).
type Origin string

// Origin labels.
const (
	OriginAdaptedLight  Origin = "adapted_light"
	OriginAdaptedMedium Origin = "adapted_medium"
	OriginInspired      Origin = "inspired"
)

// Source is one fetched version with its rating (FIND-9, FIND-10).
type Source struct {
	URL       string   `json:"url"`
	Site      string   `json:"site"`
	Author    string   `json:"author,omitempty"`
	Rating    *float64 `json:"rating,omitempty"`
	Votes     int      `json:"votes"`
	Published string   `json:"published,omitempty"`
	Score     float64  `json:"score"`
	// Base marks the version the find was adapted from.
	Base bool `json:"base"`
}

// Flag is a cross-check outlier (FIND-11) or a gate flag (GATE-3), shown
// with Apply/Keep on the review screen.
type Flag struct {
	Code    string  `json:"code"`
	Value   float64 `json:"value,omitempty"`
	Typical float64 `json:"typical,omitempty"`
	Reason  string  `json:"reason,omitempty"`
}

// Change is one adaptation and why (FIND-12, REV-1 "what changed and why").
type Change struct {
	Kind   string `json:"kind"` // swap | protein_swap
	From   string `json:"from,omitempty"`
	To     string `json:"to,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// Step is one pipeline step in Plantry's words: an i18n key and its
// parameters (FIND-14 "steps in Plantry's words", shown on the review).
type Step struct {
	Key    string            `json:"key"`
	Params map[string]string `json:"params,omitempty"`
}

// Find is a recipe the assistant looked for.
type Find struct {
	ID        int64           `json:"id"`
	Trigger   Trigger         `json:"trigger"`
	State     State           `json:"state"`
	Dish      string          `json:"dish"`
	Draft     *importer.Draft `json:"draft"`
	SourceURL string          `json:"source_url"`
	Sources   []Source        `json:"sources"`
	Flags     []Flag          `json:"flags"`
	Origin    *Origin         `json:"origin"`
	Changes   []Change        `json:"changes"`
	Gates     gates.Result    `json:"gates"`
	Steps     []Step          `json:"steps"`
	// Questions are the KIT-2/3 questions this find raised, asked once.
	Questions []string `json:"questions"`
	Reason    *string  `json:"reason"`
	FoodID    *int64   `json:"food_id"`
	RunID     *int64   `json:"run_id"`
	// PlacedOn is the start (YYYY-MM-DD) of the window the planner placed
	// the find in (C3); nil otherwise.
	PlacedOn *string `json:"placed_on"`
	// PassedOn is the start of the window a generation offered the ready
	// find to and did not use it in; such a find no longer counts toward
	// New dishes (FIND-3).
	PassedOn  *string    `json:"passed_on"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	DecidedAt *time.Time `json:"decided_at"`
}

// Outcome is what the pipeline stores when a find leaves searching.
type Outcome struct {
	State     State
	Draft     *importer.Draft
	SourceURL string
	Sources   []Source
	Flags     []Flag
	Origin    *Origin
	Changes   []Change
	Gates     gates.Result
	Steps     []Step
	Questions []string
	RunID     *int64
}

// Store persists finds (sqlite.FindRepo).
type Store interface {
	Create(ctx context.Context, dish string, trigger Trigger) (*Find, error)
	Get(ctx context.Context, id int64) (*Find, error)
	// Finish stores the outcome of a searching find; domain.ErrNotFound when
	// the find is not searching.
	Finish(ctx context.Context, id int64, o Outcome) (*Find, error)
	List(ctx context.Context, state *State, limit int) ([]Find, error)
	// Decide keeps (in_library, with food) or discards a ready or
	// needs_check find; domain.ErrDiscoveryDecided when it is decided already.
	Decide(ctx context.Context, id int64, state State, reason *string, foodID *int64) (*Find, error)
	// Stranded are the searching finds no job is queued or running for.
	Stranded(ctx context.Context) ([]int64, error)
	// SourceURLs are the URLs of every food and suggestion (duplicates).
	SourceURLs(ctx context.Context) ([]string, error)
	PenaliseSource(ctx context.Context, site string) error
	SourcePenalties(ctx context.Context) (map[string]int, error)
	MarkAsked(ctx context.Context, question string) error
	Asked(ctx context.Context) ([]string, error)
	// Place marks a ready find in_library as foodID, placed in the window
	// starting on windowStart; Unplace puts a placed find back to ready.
	Place(ctx context.Context, id, foodID int64, windowStart time.Time) (*Find, error)
	Unplace(ctx context.Context, id int64) (*Find, error)
	// Pass records that a generation offered the ready find to the window
	// starting on windowStart and did not use it.
	Pass(ctx context.Context, id int64, windowStart time.Time) error
	// Placed lists the finds placed in the window starting on windowStart.
	Placed(ctx context.Context, windowStart time.Time) ([]Find, error)
	// TouchSession keeps the planning session open until at least until;
	// SessionUntil is when it closes (zero: never opened).
	TouchSession(ctx context.Context, until time.Time) error
	SessionUntil(ctx context.Context) (time.Time, error)
	// BlockSearch skips domains in the find search until until;
	// SearchBlocked lists the ones skipped at now.
	BlockSearch(ctx context.Context, domains []string, until time.Time) error
	SearchBlocked(ctx context.Context, now time.Time) ([]string, error)
}
