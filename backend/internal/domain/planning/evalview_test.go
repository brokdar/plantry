package planning

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func compactJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return string(b)
}

func TestCompact_RoundsAndOmitsEmpty(t *testing.T) {
	target := 2000.4
	grams := 120.04
	ev := Evaluation{
		Days: []DayEval{{
			Date: "2026-11-02", Kcal: 1843.6, TargetKcal: &target, ProteinG: 91.26, ProteinPct: 19.84, FatPct: 31.04,
			MeatFoods: []string{}, Complete: true,
		}, {
			Date: "2026-11-03", Kcal: 900, Meat: true, MeatFoods: []string{"Chicken"}, NutritionMissing: true,
		}},
		Plates: []PlateEval{{
			Date: "2026-11-02", SlotID: 1, PlateID: -1, Eaters: 2, Kcal: 600.55, ProteinG: 30.04,
			Components: []ComponentEval{{FoodID: 7, Name: "Rice", Role: "side_starch", Grams: &grams, Kcal: 155.5, ProteinG: 3.01, NutritionComplete: true}},
		}},
		Issues: []Issue{},
	}
	got := compactJSON(t, ev.Compact(View{Detail: func(PlateEval) bool { return true }}))
	assert.JSONEq(t, `{
	  "days":[
	    {"date":"2026-11-02","kcal":1844,"target_kcal":2000,"protein_g":91.3,"protein_pct":19.8,"fat_pct":31},
	    {"date":"2026-11-03","kcal":900,"meat":true,"meat_foods":["Chicken"],"complete":false,"nutrition_missing":true}],
	  "meat_days":0,
	  "plates":[{"date":"2026-11-02","slot_id":1,"plate_id":-1,"eaters":2,"kcal":601,"protein_g":30,
	    "components":[{"food_id":7,"name":"Rice","role":"side_starch","grams":120,"kcal":156,"protein_g":3}]}],
	  "issues":[],
	  "totals":{"violations":0,"warnings":0,"soft":0}
	}`, got)
}

func TestCompact_ViewScopesDaysPlatesAndSoftIssues(t *testing.T) {
	ev := Evaluation{
		Days: []DayEval{{Date: "2026-11-02", Kcal: 1}, {Date: "2026-11-03", Kcal: 2}},
		Plates: []PlateEval{
			{Date: "2026-11-02", SlotID: 1, PlateID: -1, Components: []ComponentEval{{FoodID: 1, Name: "A"}}},
			{Date: "2026-11-02", SlotID: 2, PlateID: 5, Components: []ComponentEval{{FoodID: 2, Name: "B"}}},
			{Date: "2026-11-03", SlotID: 1, PlateID: 6, Components: []ComponentEval{{FoodID: 3, Name: "C"}}},
		},
		Issues: []Issue{
			{Rule: "S2", Code: "kcal_below_band", Severity: SeveritySoft, Date: "2026-11-03"},
			{Rule: "S2", Code: "kcal_below_band", Severity: SeveritySoft, Date: "2026-11-02"},
			{Rule: "H5", Code: "meat_days_exceeded", Severity: SeverityViolation, Date: "2026-11-03"},
		},
	}
	ev.Totals = totalsOf(ev.Issues)

	v := ev.ProposedView()
	require.Equal(t, map[string]bool{"2026-11-02": true}, v.Dates)
	out := ev.Compact(v)
	require.Len(t, out["days"], 1)
	plates := out["plates"].([]any)
	require.Len(t, plates, 2)
	assert.Contains(t, plates[0], "components", "the proposed plate keeps its components")
	assert.NotContains(t, plates[1], "components", "a persisted plate does not")
	assert.Len(t, out["issues"], 2, "the soft issue of the other day goes, the hard one stays")
	assert.Equal(t, json.Number("1"), out["totals"].(map[string]any)["soft"])

	// No proposed plate: everything stays, without components.
	rest := Evaluation{Days: ev.Days, Plates: ev.Plates[1:], Issues: ev.Issues}
	all := rest.Compact(rest.ProposedView())
	assert.Len(t, all["days"], 2)
	assert.NotContains(t, all["plates"].([]any)[0], "components")
}

// An option's op carries what the top-level fields repeat.
func TestCompact_TrimsOptionFieldsTheOpCarries(t *testing.T) {
	plate := int64(12)
	grams, amount := 50.0, 50.0
	ev := Evaluation{Issues: []Issue{{
		Rule: "S3", Code: "protein_below_target", Severity: SeveritySoft, Date: "2026-11-07",
		Options: []Option{{
			Action: "add", FoodID: 11, Name: "Parmesan", Date: "2026-11-07", SlotID: 3, PlateID: &plate, Grams: &grams,
			KcalDelta: 200.4, ProteinDeltaG: 17.54, DayKcal: 2158.2, DayProteinPct: 18.66, CookOccasions: 2,
			Op: OptionOp{Op: "add", PlateID: &plate, FoodID: 11, Amount: &amount, Unit: "g"},
		}},
	}}}
	got := compactJSON(t, ev.Compact(View{}))
	assert.Contains(t, got, `"options":[{"action":"add","cook_occasions":2,"day_kcal":2158,"day_protein_pct":18.7,"kcal_delta":200,"name":"Parmesan","op":{"amount":50,"food_id":11,"op":"add","plate_id":12,"unit":"g"},"protein_delta_g":17.5}]`)
}
