package slot

import "context"

// Repository is the port adapters must implement for time slot persistence.
type Repository interface {
	Create(ctx context.Context, s *TimeSlot) error
	Get(ctx context.Context, id int64) (*TimeSlot, error)
	Update(ctx context.Context, s *TimeSlot) error
	Delete(ctx context.Context, id int64) error
	List(ctx context.Context, activeOnly bool) ([]TimeSlot, error)
	CountPlatesUsing(ctx context.Context, slotID int64) (int64, error)
}
