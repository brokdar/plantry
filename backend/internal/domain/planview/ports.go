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

// FoodReader loads food headers by id and their eating history.
type FoodReader interface {
	LookupByIDs(ctx context.Context, ids []int64) (map[int64]*food.Food, error)
	// History returns each food's history over confirmed plates as of
	// today; foods never eaten may be missing.
	History(ctx context.Context, ids []int64, today time.Time) (map[int64]food.History, error)
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

// Recipes counts composed foods (setup progress) and lists their ids (the
// recipes the variety rule H8 could pick instead).
type Recipes interface {
	CountComposedFoods(ctx context.Context) (int64, error)
	ComposedFoodIDs(ctx context.Context) ([]int64, error)
}

// Diets sets foods' effective diet classes (D28). *food.Service satisfies
// it.
type Diets interface {
	DeriveDiet(ctx context.Context, foods ...*food.Food) error
}

// RunNote is what the plan's draft summary shows of an AI run.
type RunNote struct {
	Chooser *string
	Notes   *string
}

// RunNotes reads an AI run's chooser and notes from the run log.
type RunNotes interface {
	RunNote(ctx context.Context, id int64) (RunNote, error)
}

// Progress reports the generation running for a window (D9). The planning
// runner implements it; it never touches the database.
type Progress interface {
	// Running returns the id of the run generating the window that starts
	// on start, if one runs.
	Running(start time.Time) (runID int64, ok bool)
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
	Recipes      Recipes
	Runs         RunNotes
	Diets        Diets
	// Progress is nil while no generation runner is wired: running_run_id
	// is then always null.
	Progress Progress
	// Tx runs each read model's reads in one transaction, so a concurrent
	// write cannot produce an inconsistent view.
	Tx domain.Transactor
}

// Service builds the read models.
type Service struct{ d Deps }

// NewService creates a read-model service.
func NewService(d Deps) *Service { return &Service{d: d} }
