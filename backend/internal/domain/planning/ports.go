package planning

import (
	"context"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/preset"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
	"github.com/jaltszeimer/plantry/backend/internal/domain/settings"
	"github.com/jaltszeimer/plantry/backend/internal/domain/slot"
)

// Chooser proposes the plates and sessions of a target (spec §6.1 step 3):
// the LLM or the heuristic.
type Chooser interface {
	Choose(ctx context.Context, in ChooseInput) (Proposal, error)
}

// ChooseInput is what a chooser plans from. State is the plan without the
// target's draft rows and with the routine cells applied (D8): the chooser
// fills the target's empty plan cells.
type ChooseInput struct {
	State  State
	Target Target
	// EatersByDate (YYYY-MM-DD → eaters) overrides the household portions
	// of a day's plates (D16).
	EatersByDate map[string]float64
	// IncludeNewRecipe asks for one never-eaten recipe (D15).
	IncludeNewRecipe bool
	// Avoid are the recipes of the rows the target replaces: a regenerate
	// should not bring them back.
	Avoid []int64
	// Side is set for the side scope: swap one side, keep everything else.
	Side        *SideSwap
	Instruction string
	// Locale is the profile's locale for the chooser's reasons and notes.
	Locale string
	// Stage reports a chooser's progress (the LLM's repair turns); nil
	// ignores it.
	Stage func(Stage)
}

// SideSwap is the side a side scope replaces: its session's occasion and
// size, and the plates eating it.
type SideSwap struct {
	FoodID         int64
	CookDate       time.Time
	CookSlotID     int64
	Portions       float64
	FrozenPortions float64
	Plates         []SidePlate
}

// SidePlate is a plate that eats the replaced side, PerEater portions each.
type SidePlate struct {
	PlateID  int64
	Date     time.Time
	SlotID   int64
	Eaters   float64
	PerEater float64
}

// Foods reads the food library (food.Service).
type Foods interface {
	List(ctx context.Context, q food.ListQuery) (*food.ListResult, error)
	Derive(ctx context.Context, today time.Time, foods ...*food.Food) error
	DeriveDiet(ctx context.Context, foods ...*food.Food) error
}

// FoodRows reads food rows (the food repository).
type FoodRows interface {
	LookupByIDs(ctx context.Context, ids []int64) (map[int64]*food.Food, error)
	CountComposedFoods(ctx context.Context) (int64, error)
}

// Nutrition resolves per-portion (composed) and per-100 g (leaf) macros.
type Nutrition interface {
	PerPortionDetailedMany(ctx context.Context, ids []int64) (map[int64]food.Detailed, error)
}

// Cells resolves cell availability.
type Cells interface {
	Resolve(ctx context.Context, from, to time.Time) ([]availability.Cell, error)
}

// PlateRows reads persisted plates (the plate repository).
type PlateRows interface {
	ListByDateRange(ctx context.Context, from, to time.Time) ([]plate.Plate, error)
}

// SessionRows reads persisted cook sessions (the session repository).
type SessionRows interface {
	ListInRange(ctx context.Context, from, to time.Time) ([]plate.CookSession, map[int64][]plate.Link, error)
}

// PlateService is the plate write path (plate.Service): the scope reads
// the rows a scope names, and the generator persists through RemoveDrafts
// and Apply (D10).
type PlateService interface {
	Get(ctx context.Context, id int64) (*plate.Plate, error)
	GetComponent(ctx context.Context, componentID int64) (*plate.PlateComponent, error)
	Session(ctx context.Context, id int64) (*plate.SessionView, error)
	DraftClosure(ctx context.Context, from, to time.Time) (plate.RowSet, []plate.Plate, error)
	PlateClosure(ctx context.Context, plateID int64) (plate.RowSet, []plate.Plate, error)
	RemoveDrafts(ctx context.Context, rows plate.RowSet) (plate.PlanSnapshot, error)
	Apply(ctx context.Context, snap plate.PlanSnapshot, opts plate.ApplyOptions) (*plate.ApplyResult, error)
}

// Presets reads the presets bound to routine cells.
type Presets interface {
	Get(ctx context.Context, id int64) (*preset.Preset, error)
}

// Slots lists the time slots (cell order, D12).
type Slots interface {
	List(ctx context.Context, activeOnly bool) ([]slot.TimeSlot, error)
}

// SettingsReader reads app settings.
type SettingsReader interface {
	Get(ctx context.Context, key string) (settings.Value, error)
}

// ProfileReader reads the user profile.
type ProfileReader interface {
	Get(ctx context.Context) (*profile.Profile, error)
}

// RunStart is the ai_runs row a generation starts.
type RunStart struct {
	Kind          airun.Kind
	PromptVersion string
	WindowStart   time.Time
	WindowEnd     time.Time
	Instruction   string
	Chooser       airun.Chooser
	ScopeJSON     string
}

// RunEnd is how a generation run finishes.
type RunEnd struct {
	Status         airun.Status
	ErrorKey       string
	RepairAttempts int
	ViolationsJSON string
	ScoreJSON      string
	Notes          string
}

// RunLog writes the ai_runs rows of generations (every run is one, D9).
type RunLog interface {
	// Start creates the row and returns a ctx carrying the run, for the
	// metered LLM client.
	Start(ctx context.Context, s RunStart) (context.Context, Run, error)
}

// Run is a started generation run.
type Run interface {
	ID() int64
	Finish(ctx context.Context, end RunEnd)
}

// RunRecord is a generation run as the run log stored it. Empty strings
// stand for NULL.
type RunRecord struct {
	ID             int64
	Kind           airun.Kind
	Status         airun.Status
	Chooser        airun.Chooser
	Notes          string
	ErrorKey       string
	ScopeJSON      string
	ViolationsJSON string
	ScoreJSON      string
	WindowStart    time.Time
	WindowEnd      time.Time
}

// RunCounts are what a run wrote that still exists: its plates and
// sessions and the distinct cook dates of the sessions.
type RunCounts struct {
	Plates   int
	Sessions int
	CookDays int
}

// Runs reads generation runs back.
type Runs interface {
	// PlanRun returns a generate or regenerate run, or domain.ErrNotFound.
	PlanRun(ctx context.Context, id int64) (RunRecord, error)
	PlanRunCounts(ctx context.Context, id int64) (RunCounts, error)
}
