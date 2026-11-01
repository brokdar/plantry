// Package eval is the offline eval suite of plan generation (spec §8.4):
// JSON scenarios over a shared recipe library, run through the real
// generation service and checker on a fresh SQLite database each, and a
// JSONL report with a summary and a diff against an earlier report.
package eval

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"
	"time"
)

//go:embed library.json scenarios/*.json
var builtin embed.FS

// Library is the shared seed library: leaf ingredients with nutrition per
// 100 g and the recipes made of them.
type Library struct {
	Ingredients []Ingredient `json:"ingredients"`
	Recipes     []Recipe     `json:"recipes"`
}

// Ingredient is a leaf food; its diet class comes from its name.
type Ingredient struct {
	Name    string  `json:"name"`
	Kcal    float64 `json:"kcal"`
	Protein float64 `json:"protein"`
	Fat     float64 `json:"fat"`
	Carbs   float64 `json:"carbs"`
}

// Recipe is a composed food. Ingredients are [name, grams] pairs.
type Recipe struct {
	Name        string             `json:"name"`
	Role        string             `json:"role"`
	Portions    float64            `json:"portions"`
	KeepDays    *int               `json:"keep_days"`
	Freezable   *bool              `json:"freezable"`
	Favorite    bool               `json:"favorite"`
	Tags        []string           `json:"tags"`
	Ingredients []RecipeIngredient `json:"ingredients"`
}

// RecipeIngredient is a recipe's [name, grams] pair.
type RecipeIngredient struct {
	Name  string
	Grams float64
}

// UnmarshalJSON reads the [name, grams] pair.
func (r *RecipeIngredient) UnmarshalJSON(b []byte) error {
	var pair []json.RawMessage
	if err := json.Unmarshal(b, &pair); err != nil || len(pair) != 2 {
		return fmt.Errorf("ingredient %s: want [name, grams]", b)
	}
	if err := json.Unmarshal(pair[0], &r.Name); err != nil {
		return fmt.Errorf("ingredient %s: %w", b, err)
	}
	return json.Unmarshal(pair[1], &r.Grams)
}

// Date is a YYYY-MM-DD date.
type Date struct{ time.Time }

// UnmarshalJSON parses YYYY-MM-DD.
func (d *Date) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		return err
	}
	d.Time = t
	return nil
}

// Scenario is one eval fixture (spec §8.4): who plans what, with which
// history, and the steps that end in the run the eval measures.
type Scenario struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	// Today is the date of every step without its own.
	Today Date `json:"today"`
	// Start is the window's start (a shopping day).
	Start Date `json:"start"`
	// Settings are app settings by key (values as the settings API takes
	// them).
	Settings map[string]string `json:"settings"`
	Profile  ProfileSpec       `json:"profile"`
	// Slots are the active slots in day order, by short name.
	Slots []string `json:"slots"`
	// Recipes limits the library to these recipe names; empty uses all.
	Recipes      []string         `json:"recipes"`
	Presets      []PresetSpec     `json:"presets"`
	Availability AvailabilitySpec `json:"availability"`
	// Plates are the user's confirmed plates: history before the window,
	// and fixed rows inside it.
	Plates []PlateSpec `json:"plates"`
	// Steps run in order; the last one that starts a run is measured.
	Steps  []Step `json:"steps"`
	Expect Expect `json:"expect"`
}

// ProfileSpec is the user profile of a scenario.
type ProfileSpec struct {
	DietType           string   `json:"diet_type"`
	HouseholdPortions  float64  `json:"household_portions"`
	KcalTarget         *float64 `json:"kcal_target"`
	ProteinPct         *float64 `json:"protein_pct"`
	FatPct             *float64 `json:"fat_pct"`
	CarbsPct           *float64 `json:"carbs_pct"`
	MeatDaysPerWeekMax *int     `json:"meat_days_per_week_max"`
	Notes              string   `json:"notes"`
}

// PresetSpec is a one-plate preset of one recipe on one slot (a routine).
type PresetSpec struct {
	Name   string `json:"name"`
	Slot   string `json:"slot"`
	Recipe string `json:"recipe"`
}

// AvailabilitySpec are a scenario's weekday defaults and date overrides.
type AvailabilitySpec struct {
	Defaults  []DefaultSpec  `json:"defaults"`
	Overrides []OverrideSpec `json:"overrides"`
}

// DefaultSpec sets the mode of a slot on weekdays (0 = Monday).
type DefaultSpec struct {
	Weekdays []int  `json:"weekdays"`
	Slot     string `json:"slot"`
	Mode     string `json:"mode"`
	Preset   string `json:"preset"`
}

// OverrideSpec sets the mode of one cell.
type OverrideSpec struct {
	Date Date   `json:"date"`
	Slot string `json:"slot"`
	Mode string `json:"mode"`
}

// CellSpec names a cell.
type CellSpec struct {
	Date Date   `json:"date"`
	Slot string `json:"slot"`
}

// PlateSpec is a confirmed plate of one recipe, fresh at its cell.
type PlateSpec struct {
	CellSpec
	Recipe string `json:"recipe"`
	// BatchPortions sets the portions its cook session produces (default:
	// what the plate eats), leaving a surplus for leftovers.
	BatchPortions float64 `json:"batch_portions"`
}

// Step actions.
const (
	ActionGenerate   = "generate"
	ActionRegenerate = "regenerate_day"
	ActionAccept     = "accept"
	ActionReplan     = "replan"
)

// Step is one action of a scenario. Today overrides the scenario's.
type Step struct {
	Action string `json:"action"`
	Today  *Date  `json:"today"`
	// Date is regenerate_day's day.
	Date *Date `json:"date"`
	// Instruction is generate's and regenerate_day's.
	Instruction string `json:"instruction"`
	// From, To, None, UnlockConfirmed, Spare and Regenerate are replan's.
	From            *Date      `json:"from"`
	To              *Date      `json:"to"`
	None            []CellSpec `json:"none"`
	UnlockConfirmed bool       `json:"unlock_confirmed"`
	Spare           string     `json:"spare"`
	Regenerate      bool       `json:"regenerate"`
}

func (s Step) startsRun() bool {
	switch s.Action {
	case ActionGenerate, ActionRegenerate:
		return true
	case ActionReplan:
		return s.Regenerate
	}
	return false
}

// Expect are a scenario's own expectations on top of "no hard violations".
type Expect struct {
	// KeptConfirmed: every confirmed plate before the measured step is
	// unchanged after it (except those in the step's No-plate cells).
	KeptConfirmed bool `json:"kept_confirmed"`
	// NoPlate cells hold no plate at the end.
	NoPlate []CellSpec `json:"no_plate"`
	// UnchangedBefore: the plates dated before it and the sessions cooking
	// before it are unchanged by the measured step.
	UnchangedBefore *Date `json:"unchanged_before"`
	// MinPlates is the fewest plates the measured run must write.
	MinPlates int `json:"min_plates"`
	// MinRepeats is the fewest H8 repeats the measured run's score must
	// count (variety relaxed for lack of recipes).
	MinRepeats int `json:"min_repeats"`
	// FrozenSpare: the measured re-plan froze spare portions of an
	// uncooked batch cooking on or after its From.
	FrozenSpare bool `json:"frozen_spare"`
}

// LoadLibrary reads the built-in library.
func LoadLibrary() (Library, error) {
	var lib Library
	b, err := builtin.ReadFile("library.json")
	if err != nil {
		return lib, err
	}
	if err := json.Unmarshal(b, &lib); err != nil {
		return lib, fmt.Errorf("library.json: %w", err)
	}
	return lib, nil
}

// LoadScenarios reads the built-in scenarios sorted by name; names, when
// given, selects them (an unknown name is an error).
func LoadScenarios(names ...string) ([]Scenario, error) {
	sub, err := fs.Sub(builtin, "scenarios")
	if err != nil {
		return nil, err
	}
	return loadScenarios(sub, names)
}

func loadScenarios(fsys fs.FS, names []string) ([]Scenario, error) {
	files, err := fs.Glob(fsys, "*.json")
	if err != nil {
		return nil, err
	}
	var out []Scenario
	for _, f := range files {
		b, err := fs.ReadFile(fsys, f)
		if err != nil {
			return nil, err
		}
		var sc Scenario
		dec := json.NewDecoder(strings.NewReader(string(b)))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&sc); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		if want := strings.TrimSuffix(path.Base(f), ".json"); sc.Name != want {
			return nil, fmt.Errorf("%s: name %q must match the file name", f, sc.Name)
		}
		if len(sc.Steps) == 0 || !slices.ContainsFunc(sc.Steps, Step.startsRun) {
			return nil, fmt.Errorf("%s: no step starts a run", f)
		}
		out = append(out, sc)
	}
	if len(names) == 0 {
		return out, nil
	}
	var picked []Scenario
	for _, n := range names {
		i := slices.IndexFunc(out, func(s Scenario) bool { return s.Name == n })
		if i < 0 {
			return nil, fmt.Errorf("unknown scenario %q", n)
		}
		picked = append(picked, out[i])
	}
	return picked, nil
}
