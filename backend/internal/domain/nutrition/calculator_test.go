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
