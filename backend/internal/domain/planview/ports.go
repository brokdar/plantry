// Package planview builds the plan read models (Phase 2 D23): GET /api/plan
// and GET /api/today. It never writes and takes the dates it answers for as
// arguments.
package planview

import (
	"context"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
	"github.com/jaltszeimer/plantry/backend/internal/domain/settings"
	"github.com/jaltszeimer/plantry/backend/internal/domain/slot"
)

// PlateReader loads plates for a date range with components and feedback in
// a bounded number of queries.
type PlateReader interface {
	ListByDateRange(ctx context.Context, from, to time.Time) ([]plate.Plate, error)
}

// SessionReader loads the sessions a range touches.
type SessionReader interface {
	// ListInRange returns every session cooking in [from, to] or linked to a
	// plate dated in [from, to], and all links of each, keyed by session id.
	ListInRange(ctx context.Context, from, to time.Time) ([]plate.CookSession, map[int64][]plate.Link, error)
}

// FoodReader loads food headers by id.
type FoodReader interface {
	LookupByIDs(ctx context.Context, ids []int64) (map[int64]*food.Food, error)
}

// Nutrition resolves a food's macros: per portion for a composed food, per
// 100 g for a leaf. *food.NutritionResolver satisfies it.
type Nutrition interface {
	PerPortionDetailed(ctx context.Context, foodID int64) (food.Detailed, error)
}

// Availability resolves cell modes for every date × active slot.
type Availability interface {
	Resolve(ctx context.Context, from, to time.Time) ([]availability.Cell, error)
}

// Slots lists time slots in sort order.
type Slots interface {
	List(ctx context.Context, activeOnly bool) ([]slot.TimeSlot, error)
}

// Settings reads app settings.
type Settings interface {
	Get(ctx context.Context, key string) (settings.Value, error)
}

// Profile reads the user profile (nutrition targets).
type Profile interface {
	Get(ctx context.Context) (*profile.Profile, error)
}

// RecipeCounter counts composed foods (setup progress).
type RecipeCounter interface {
	CountComposedFoods(ctx context.Context) (int64, error)
}

// Deps are the service's dependencies.
type Deps struct {
	Plates       PlateReader
	Sessions     SessionReader
	Foods        FoodReader
	Nutrition    Nutrition
	Availability Availability
	Slots        Slots
	Settings     Settings
	Profile      Profile
	Recipes      RecipeCounter
	// Tx runs each read model's reads in one transaction, so a concurrent
	// write cannot produce an inconsistent view.
	Tx domain.Transactor
}

// Service builds the read models.
type Service struct{ d Deps }

// NewService creates a read-model service.
func NewService(d Deps) *Service { return &Service{d: d} }
