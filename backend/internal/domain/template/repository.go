package template

import (
	"context"

	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// Repository is the persistence port for templates.
type Repository interface {
	Create(ctx context.Context, t *Template) error
	Get(ctx context.Context, id int64) (*Template, error)
	UpdateName(ctx context.Context, id int64, name string) (*Template, error)
	Delete(ctx context.Context, id int64) error
	List(ctx context.Context) ([]Template, error)
	ReplaceComponents(ctx context.Context, templateID int64, comps []TemplateComponent) error
	ListComponentsByTemplate(ctx context.Context, templateID int64) ([]TemplateComponent, error)
	CountUsingFood(ctx context.Context, foodID int64) (int64, error)
}

// TxRunner runs fn inside a single transaction with template + plate repos bound to it.
type TxRunner interface {
	RunInTemplateTx(ctx context.Context, fn func(Repository, plate.Repository) error) error
}

// FoodChecker reports whether a food exists.
type FoodChecker interface {
	Exists(ctx context.Context, foodID int64) (bool, error)
}

// PlateComponentSource reads the components of a plate (used when cloning a
// plate's composition into a new template).
type PlateComponentSource interface {
	ListComponentsByPlate(ctx context.Context, plateID int64) ([]plate.PlateComponent, error)
}
