// Package testsvc wires the real plan services over a test database, the way
// cmd/plantry/main.go does, so tests outside the sqlite adapter can drive the
// plate write path without repeating its dependency list.
package testsvc

import (
	"database/sql"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/crypto"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/feedback"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planview"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
	"github.com/jaltszeimer/plantry/backend/internal/domain/settings"
)

// Plan returns the plate and availability services over db, wired to each
// other and to one unit of work.
func Plan(db *sql.DB) (*plate.Service, *availability.Service) {
	return PlanWithEvents(db, sqlite.NewPlateEventRepo(db))
}

// PlanWithEvents is Plan with the plate service writing its audit trail to
// events.
func PlanWithEvents(db *sql.DB, events plate.EventLog) (*plate.Service, *availability.Service) {
	tx := sqlite.NewTxRunner(db)
	avail := availability.NewService(sqlite.NewAvailabilityRepo(db), tx, sqlite.NewPresetRepo(db))
	plates := plate.NewService(plate.Deps{
		Plates:   sqlite.NewPlateRepo(db),
		Sessions: sqlite.NewCookSessionRepo(db),
		Slots:    sqlite.NewSlotRepo(db),
		Foods:    sqlite.NewFoodRepo(db),
		Cells:    avail,
		Profile:  profile.NewService(sqlite.NewProfileRepo(db)),
		Settings: Settings(db),
		Events:   events,
		Outcomes: sqlite.NewAIRunRepo(db),
		Tx:       tx,
	})
	avail.WithPlates(plates)
	return plates, avail
}

// Plates returns the plate service of Plan.
func Plates(db *sql.DB) *plate.Service {
	p, _ := Plan(db)
	return p
}

// PlanView returns the read-model service over db, wired like main.go.
func PlanView(db *sql.DB) *planview.Service {
	_, avail := Plan(db)
	foods := sqlite.NewFoodRepo(db)
	return planview.NewService(planview.Deps{
		Plates:       sqlite.NewPlateRepo(db),
		Sessions:     sqlite.NewCookSessionRepo(db),
		Foods:        foods,
		Nutrition:    food.NewNutritionResolver(foods),
		Availability: avail,
		Slots:        sqlite.NewSlotRepo(db),
		Settings:     Settings(db),
		Profile:      profile.NewService(sqlite.NewProfileRepo(db)),
		Recipes:      foods,
		Runs:         sqlite.NewAIRunRepo(db),
		Diets:        food.NewService(foods),
		Tx:           sqlite.NewTxRunner(db),
	})
}

// Feedback returns the plate-feedback service over db, wired like main.go.
func Feedback(db *sql.DB) *feedback.Service {
	return feedback.NewService(sqlite.NewTxRunner(db), sqlite.NewFeedbackRepo(db),
		sqlite.NewPlateRepo(db), sqlite.NewFoodRepo(db), sqlite.NewProfileRepo(db))
}

// Settings returns a settings service over db without env overrides.
func Settings(db *sql.DB) *settings.Service {
	return settings.NewService(sqlite.NewAppSettingsRepo(db), settings.NewEnvSnapshot(nil), crypto.NilCipher{})
}

// PlanningDeps returns the generation service's dependencies over db, wired
// like main.go with the heuristic chooser. The caller adds RunLog, Runs and,
// for synchronous runs, Go.
func PlanningDeps(db *sql.DB) planning.Deps {
	plates, avail := Plan(db)
	foods := sqlite.NewFoodRepo(db)
	set := Settings(db)
	return planning.Deps{
		Builder: planning.NewBuilder(planning.BuilderDeps{
			Foods: food.NewService(foods), FoodRows: foods, Nutrition: food.NewNutritionResolver(foods),
			Cells: avail, Plates: sqlite.NewPlateRepo(db), Sessions: sqlite.NewCookSessionRepo(db),
			Presets: sqlite.NewPresetRepo(db), Slots: sqlite.NewSlotRepo(db), Settings: set,
			Profile: profile.NewService(sqlite.NewProfileRepo(db)), Corrections: sqlite.NewPlateEventRepo(db),
		}),
		Plates: plates, PlateRows: sqlite.NewPlateRepo(db), Sessions: sqlite.NewCookSessionRepo(db), Cells: avail, Overrides: avail, FoodRows: foods, Settings: set,
		Tx: sqlite.NewTxRunner(db), Progress: planning.NewProgress(),
		Choosers: map[airun.Chooser]planning.Chooser{airun.ChooserHeuristic: planning.Heuristic{}},
	}
}
