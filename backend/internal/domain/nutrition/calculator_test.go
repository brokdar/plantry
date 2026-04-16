package nutrition_test

import (
	"encoding/json"
	"math"
	"os"
	"testing"

	"github.com/jaltszeimer/plantry/backend/internal/domain/nutrition"
)

type testCase struct {
	Name               string                      `json:"name"`
	Ingredients        []nutrition.IngredientInput `json:"ingredients"`
	ReferencePortions  float64                     `json:"reference_portions"`
	ExpectedTotal      nutrition.Macros            `json:"expected_total"`
	ExpectedPerPortion nutrition.Macros            `json:"expected_per_portion"`
}

func loadCases(t *testing.T) []testCase {
	t.Helper()
	data, err := os.ReadFile("testdata/nutrition-cases.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var cases []testCase
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	return cases
}

const tolerance = 0.01

func assertMacrosEqual(t *testing.T, label string, want, got nutrition.Macros) {
	t.Helper()
	fields := []struct {
		name string
		w, g float64
	}{
		{"kcal", want.Kcal, got.Kcal},
		{"protein", want.Protein, got.Protein},
		{"fat", want.Fat, got.Fat},
		{"carbs", want.Carbs, got.Carbs},
		{"fiber", want.Fiber, got.Fiber},
		{"sodium", want.Sodium, got.Sodium},
	}
	for _, f := range fields {
		if math.Abs(f.w-f.g) > tolerance {
			t.Errorf("%s %s: want %.4f, got %.4f", label, f.name, f.w, f.g)
		}
	}
}

func TestFromIngredients(t *testing.T) {
	for _, tc := range loadCases(t) {
		t.Run(tc.Name, func(t *testing.T) {
			got := nutrition.FromIngredients(tc.Ingredients)
			assertMacrosEqual(t, "total", tc.ExpectedTotal, got)
		})
	}
}

func TestPlateTotal(t *testing.T) {
	cases := []struct {
		name  string
		input nutrition.PlateInput
		want  nutrition.Macros
	}{
		{
			name:  "empty plate",
			input: nutrition.PlateInput{},
			want:  nutrition.Macros{},
		},
		{
			name: "single component one portion",
			input: nutrition.PlateInput{Components: []nutrition.PlateComponentInput{
				{Macros: nutrition.Macros{Kcal: 200, Protein: 20, Fat: 5, Carbs: 30, Fiber: 2, Sodium: 0.5}, Portions: 1},
			}},
			want: nutrition.Macros{Kcal: 200, Protein: 20, Fat: 5, Carbs: 30, Fiber: 2, Sodium: 0.5},
		},
		{
			name: "single component two portions",
			input: nutrition.PlateInput{Components: []nutrition.PlateComponentInput{
				{Macros: nutrition.Macros{Kcal: 200, Protein: 20}, Portions: 2},
			}},
			want: nutrition.Macros{Kcal: 400, Protein: 40},
		},
		{
			name: "two components summed",
			input: nutrition.PlateInput{Components: []nutrition.PlateComponentInput{
				{Macros: nutrition.Macros{Kcal: 100, Carbs: 10}, Portions: 1},
				{Macros: nutrition.Macros{Kcal: 50, Carbs: 5}, Portions: 2},
			}},
			want: nutrition.Macros{Kcal: 200, Carbs: 20},
		},
		{
			name: "fractional portions",
			input: nutrition.PlateInput{Components: []nutrition.PlateComponentInput{
				{Macros: nutrition.Macros{Kcal: 300, Protein: 30}, Portions: 0.5},
			}},
			want: nutrition.Macros{Kcal: 150, Protein: 15},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := nutrition.PlateTotal(tc.input)
			assertMacrosEqual(t, "plate_total", tc.want, got)
		})
	}
}

func TestWeekTotals(t *testing.T) {
	comp := func(kcal, protein float64, portions float64) nutrition.PlateComponentInput {
		return nutrition.PlateComponentInput{
			Macros:   nutrition.Macros{Kcal: kcal, Protein: protein},
			Portions: portions,
		}
	}
	plate := func(comps ...nutrition.PlateComponentInput) nutrition.PlateInput {
		return nutrition.PlateInput{Components: comps}
	}

	cases := []struct {
		name     string
		input    []nutrition.DayPlate
		wantDays map[int]nutrition.Macros
		wantWeek nutrition.Macros
	}{
		{
			name:     "empty",
			input:    nil,
			wantDays: map[int]nutrition.Macros{},
			wantWeek: nutrition.Macros{},
		},
		{
			name:     "single plate single day",
			input:    []nutrition.DayPlate{{Day: 0, Plate: plate(comp(400, 30, 1))}},
			wantDays: map[int]nutrition.Macros{0: {Kcal: 400, Protein: 30}},
			wantWeek: nutrition.Macros{Kcal: 400, Protein: 30},
		},
		{
			name: "two plates same day accumulate",
			input: []nutrition.DayPlate{
				{Day: 0, Plate: plate(comp(200, 10, 1))},
				{Day: 0, Plate: plate(comp(300, 20, 1))},
			},
			wantDays: map[int]nutrition.Macros{0: {Kcal: 500, Protein: 30}},
			wantWeek: nutrition.Macros{Kcal: 500, Protein: 30},
		},
		{
			name: "two plates different days",
			input: []nutrition.DayPlate{
				{Day: 0, Plate: plate(comp(400, 30, 1))},
				{Day: 2, Plate: plate(comp(600, 50, 1))},
			},
			wantDays: map[int]nutrition.Macros{
				0: {Kcal: 400, Protein: 30},
				2: {Kcal: 600, Protein: 50},
			},
			wantWeek: nutrition.Macros{Kcal: 1000, Protein: 80},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := nutrition.WeekTotals(tc.input)
			if len(got.Days) != len(tc.wantDays) {
				t.Fatalf("days len: want %d, got %d", len(tc.wantDays), len(got.Days))
			}
			for day, wantM := range tc.wantDays {
				assertMacrosEqual(t, "day", wantM, got.Days[day])
			}
			assertMacrosEqual(t, "week", tc.wantWeek, got.Week)
		})
	}
}

func TestPerPortion(t *testing.T) {
	for _, tc := range loadCases(t) {
		t.Run(tc.Name, func(t *testing.T) {
			got := nutrition.PerPortion(nutrition.ComponentInput{
				Ingredients:       tc.Ingredients,
				ReferencePortions: tc.ReferencePortions,
			})
			assertMacrosEqual(t, "per_portion", tc.ExpectedPerPortion, got)
		})
	}
}
