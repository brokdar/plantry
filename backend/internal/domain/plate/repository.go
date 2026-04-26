package plate

import (
	"context"
	"time"
)

// Repository is the port adapters must implement for plate persistence.
type Repository interface {
	Create(ctx context.Context, p *Plate) error
	Get(ctx context.Context, id int64) (*Plate, error)
	Update(ctx context.Context, p *Plate) error
	Delete(ctx context.Context, id int64) error
	ListByWeek(ctx context.Context, weekID int64) ([]Plate, error)
	ListByDateRange(ctx context.Context, from, to time.Time) ([]Plate, error)

	CreateComponent(ctx context.Context, pc *PlateComponent) error
	GetComponent(ctx context.Context, id int64) (*PlateComponent, error)
	UpdateComponent(ctx context.Context, pc *PlateComponent) error
	DeleteComponent(ctx context.Context, id int64) error
	ListComponentsByPlate(ctx context.Context, plateID int64) ([]PlateComponent, error)

	CountUsingFood(ctx context.Context, foodID int64) (int64, error)
	CountUsingTimeSlot(ctx context.Context, slotID int64) (int64, error)

	SetSkipped(ctx context.Context, plateID int64, skipped bool, note *string) (*Plate, error)
	DeleteByWeek(ctx context.Context, weekID int64) (int64, error)
}
