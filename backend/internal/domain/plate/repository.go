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
}
