// Package setup reports first-run setup progress: whether the user finished
// the setup flow and how many recipes exist towards the suggested target.
package setup

import (
	"context"
	"fmt"
	"strconv"

	"github.com/jaltszeimer/plantry/backend/internal/domain/settings"
)

// RecipeTarget is the number of recipes setup suggests adding.
const RecipeTarget = 10

// Settings reads and writes typed settings; satisfied by *settings.Service.
type Settings interface {
	Get(ctx context.Context, key string) (settings.Value, error)
	Set(ctx context.Context, key, raw string) error
}

// FoodCounter counts composed foods (recipes).
type FoodCounter interface {
	CountComposedFoods(ctx context.Context) (int64, error)
}

// Status is the setup progress snapshot.
type Status struct {
	Completed    bool
	RecipeCount  int
	RecipeTarget int
}

// Service computes setup status and marks setup complete.
type Service struct {
	settings Settings
	foods    FoodCounter
}

// NewService constructs a setup Service.
func NewService(s Settings, foods FoodCounter) *Service {
	return &Service{settings: s, foods: foods}
}

// Status returns the current setup progress.
func (s *Service) Status(ctx context.Context) (Status, error) {
	v, err := s.settings.Get(ctx, settings.KeySetupCompleted)
	if err != nil {
		return Status{}, fmt.Errorf("get %s: %w", settings.KeySetupCompleted, err)
	}
	completed, err := strconv.ParseBool(v.Raw)
	if err != nil {
		return Status{}, fmt.Errorf("parse %s: %w", settings.KeySetupCompleted, err)
	}
	n, err := s.foods.CountComposedFoods(ctx)
	if err != nil {
		return Status{}, fmt.Errorf("count composed foods: %w", err)
	}
	return Status{Completed: completed, RecipeCount: int(n), RecipeTarget: RecipeTarget}, nil
}

// Complete marks setup as completed and returns the new status. Idempotent.
func (s *Service) Complete(ctx context.Context) (Status, error) {
	if err := s.settings.Set(ctx, settings.KeySetupCompleted, "true"); err != nil {
		return Status{}, fmt.Errorf("set %s: %w", settings.KeySetupCompleted, err)
	}
	return s.Status(ctx)
}
