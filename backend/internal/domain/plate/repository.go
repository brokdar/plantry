package plate

import (
	"context"
	"time"
)

// Repository is the port adapters implement for plate persistence. Every
// method joins the unit of work carried by ctx (D21).
type Repository interface {
	// Create inserts the plate and its components (CookSessionID already set
	// on composed ones).
	Create(ctx context.Context, p *Plate) error
	// Get returns the plate with its components and feedback.
	Get(ctx context.Context, id int64) (*Plate, error)
	// GetIDByCell returns the id of the (date, slot) cell's plate, or an
	// error wrapping domain.ErrNotFound when the cell is empty.
	GetIDByCell(ctx context.Context, date time.Time, slotID int64) (int64, error)
	// Update writes date, slot, eaters and note and bumps updated_at.
	Update(ctx context.Context, p *Plate) error
	// Touch bumps updated_at after a component change.
	Touch(ctx context.Context, id int64) error
	Delete(ctx context.Context, id int64) error
	// ListByDateRange returns the plates in [from, to] with components and
	// feedback, ordered by date, slot_id, id.
	ListByDateRange(ctx context.Context, from, to time.Time) ([]Plate, error)
	// ListByIDs returns the plates of ids that exist, like ListByDateRange.
	ListByIDs(ctx context.Context, ids []int64) ([]Plate, error)
	// SetState writes the state of the plates of ids and bumps updated_at.
	SetState(ctx context.Context, ids []int64, st State) error

	CreateComponent(ctx context.Context, pc *PlateComponent) error
	GetComponent(ctx context.Context, id int64) (*PlateComponent, error)
	// UpdateComponent writes food, quantity and session.
	UpdateComponent(ctx context.Context, pc *PlateComponent) error
	DeleteComponent(ctx context.Context, id int64) error

	// RestoreFeedback writes a plate's feedback row as it was, rated_at
	// included.
	RestoreFeedback(ctx context.Context, plateID int64, fb Feedback) error
}

// SessionRepository is the port adapters implement for cook sessions.
type SessionRepository interface {
	CreateSession(ctx context.Context, s *CookSession) error
	// GetSession returns the session or an error wrapping domain.ErrNotFound.
	GetSession(ctx context.Context, id int64) (*CookSession, error)
	// UpdateSession writes cook cell, portions, frozen portions, cooked_at and
	// note and bumps updated_at.
	UpdateSession(ctx context.Context, s *CookSession) error
	DeleteSession(ctx context.Context, id int64) error
	// ListLinks returns the components linked to the session with their
	// plate's cell and eaters (PlateID, ComponentID, FoodID, Cell, PerEater,
	// Eaters). Order is not significant; OrderLinks sorts them.
	ListLinks(ctx context.Context, sessionID int64) ([]Link, error)
	// PlateIDsOfSessions returns the plates with a component eating from one
	// of the sessions.
	PlateIDsOfSessions(ctx context.Context, ids []int64) ([]int64, error)
	// ListSessionsByIDs returns the sessions of ids that exist.
	ListSessionsByIDs(ctx context.Context, ids []int64) ([]CookSession, error)
	// ListInRange returns the sessions cooking in [from, to] or linked to a
	// plate dated in it, with every link of each keyed by session id.
	ListInRange(ctx context.Context, from, to time.Time) ([]CookSession, map[int64][]Link, error)
	// SetSessionsState writes the state of the sessions of ids and bumps
	// updated_at.
	SetSessionsState(ctx context.Context, ids []int64, st State) error
}

// OutcomeEvent is what accept, discard, unaccept or a restore did to an AI
// run's rows (D25).
type OutcomeEvent string

// Outcome events.
const (
	OutcomeAccepted  OutcomeEvent = "accepted"
	OutcomeDiscarded OutcomeEvent = "discarded"
	// OutcomeReset is an undone accept.
	OutcomeReset OutcomeEvent = "reset"
	// OutcomeRestored is drafts brought back, such as an undone discard.
	OutcomeRestored OutcomeEvent = "restored"
)

// RunOutcomes records the outcome of the AI runs whose rows accept, discard,
// unaccept and restore change (D25): open → accepted on accept, → discarded
// on discard, partially_accepted once a run sees both; reset (unaccept)
// undoes an accept: accepted → open, partially_accepted → discarded;
// restored (drafts back, e.g. the Undo of a discard) undoes a discard:
// discarded → open, partially_accepted → accepted. Record joins the unit of
// work carried by ctx.
type RunOutcomes interface {
	Record(ctx context.Context, runID int64, ev OutcomeEvent) error
	// AgentRuns returns the ids, ascending, of the agent runs that wrote an
	// audit event to one of the plates or sessions of rows (D11).
	AgentRuns(ctx context.Context, rows RowSet) ([]int64, error)
}
