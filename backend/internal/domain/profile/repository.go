package profile

import "context"

// Repository is the port adapters must implement for profile persistence.
type Repository interface {
	Get(ctx context.Context) (*Profile, error)
	Update(ctx context.Context, p *Profile) (*Profile, error)
}
