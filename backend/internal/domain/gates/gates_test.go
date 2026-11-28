package gates

import (
	"encoding/json"
	"os"
	"slices"
	"testing"

	"github.com/jaltszeimer/plantry/backend/internal/domain/diet"
)

type fixtureIngredient struct {
	Name      string  `json:"name"`
	Grams     float64 `json:"grams"`
	ToTaste   bool    `json:"to_taste"`
	Unmatched bool    `json:"unmatched"`
	FoodID    int64   `json:"food_id"`
}

type fixtureCase struct {
	Name   string `json:"name"`
	Recipe struct {
		Name        string              `json:"name"`
		Portions    int                 `json:"portions"`
		Ingredients []fixtureIngredient `json:"ingredients"`
		Steps       []string            `json:"steps"`
		KeepDays    int                 `json:"keep_days"`
		Versions    int                 `json:"versions"`
	} `json:"recipe"`
	Household struct {
		Diet         string   `json:"diet"`
		NeverFoodIDs []int64  `json:"never_food_ids"`
		NeverNames   []string `json:"never_names"`
		Allergens    []string `json:"allergens"`
	} `json:"household"`
	Blocks []string `json:"blocks"`
	Flags  []string `json:"flags"`
}

func (f fixtureCase) inputs() (Recipe, Household) {
	r := Recipe{Name: f.Recipe.Name, Portions: f.Recipe.Portions, Steps: f.Recipe.Steps, KeepDays: f.Recipe.KeepDays, Versions: f.Recipe.Versions}
	for _, i := range f.Recipe.Ingredients {
		r.Ingredients = append(r.Ingredients, Ingredient{Name: i.Name, Grams: i.Grams, ToTaste: i.ToTaste, FoodID: i.FoodID, Matched: !i.Unmatched})
	}
	h := Household{Diet: diet.Class(f.Household.Diet), NeverFoodIDs: f.Household.NeverFoodIDs, NeverNames: f.Household.NeverNames, Allergens: f.Household.Allergens}
	return r, h
}

func codes(fs []Finding) []string {
	out := []string{}
	for _, f := range fs {
		if !slices.Contains(out, f.Code) {
			out = append(out, f.Code)
		}
	}
	slices.Sort(out)
	return out
}

func loadCases(t *testing.T) []fixtureCase {
	t.Helper()
	b, err := os.ReadFile("testdata/cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cs []fixtureCase
	if err := json.Unmarshal(b, &cs); err != nil {
		t.Fatal(err)
	}
	return cs
}

// TestPlantedRecipes: every planted case is blocked or flagged exactly as
// specified; the control cases come through clean (or with only their flag).
func TestPlantedRecipes(t *testing.T) {
	for _, c := range loadCases(t) {
		t.Run(c.Name, func(t *testing.T) {
			r, h := c.inputs()
			res := Check(r, h)
			want := func(l []string) []string {
				if l == nil {
					return []string{}
				}
				return l
			}
			if got := codes(res.Blocks); !slices.Equal(got, want(c.Blocks)) {
				t.Errorf("blocks = %v, want %v (%+v)", got, want(c.Blocks), res.Blocks)
			}
			if got := codes(res.Flags); !slices.Equal(got, want(c.Flags)) {
				t.Errorf("flags = %v, want %v (%+v)", got, want(c.Flags), res.Flags)
			}
		})
	}
}

func TestDeterministicAndPointers(t *testing.T) {
	cs := loadCases(t)
	for _, c := range cs {
		r, h := c.inputs()
		a, b := Check(r, h), Check(r, h)
		if !slices.EqualFunc(a.Blocks, b.Blocks, func(x, y Finding) bool { return x.Code == y.Code && x.Ingredient == y.Ingredient && x.Step == y.Step }) {
			t.Errorf("%s: not deterministic", c.Name)
		}
		for _, f := range append(a.Blocks, a.Flags...) {
			if f.Ingredient >= len(r.Ingredients) || f.Step >= len(r.Steps) || f.Reason == "" {
				t.Errorf("%s: bad finding %+v", c.Name, f)
			}
		}
	}
}

func TestFindingPointsAtLine(t *testing.T) {
	r := Recipe{Name: "Pasta", Portions: 2, Steps: []string{"Kochen."}, Ingredients: []Ingredient{
		{Name: "Spaghetti", Matched: true, Grams: 200}, {Name: "Pesto", Matched: true, Grams: 50},
	}}
	res := Check(r, Household{Allergens: []string{"nuts"}})
	if len(res.Blocks) != 1 || res.Blocks[0].Ingredient != 1 || res.Blocks[0].Params["allergen"] != "nuts" {
		t.Fatalf("got %+v", res.Blocks)
	}
}

func TestParamsTunable(t *testing.T) {
	r := Recipe{Name: "Suppe", Portions: 4, Steps: []string{"Kochen."}, Ingredients: []Ingredient{
		{Name: "Salz", Matched: true, Grams: 5}, {Name: "Wasser", Matched: true, Grams: 1000},
	}}
	if res := Check(r, Household{}); len(res.Flags) != 0 {
		t.Fatalf("default: %+v", res.Flags)
	}
	res := CheckWith(r, Household{}, Params{MaxSodiumMgPerPortion: 300, MinVersions: 3})
	if got := codes(res.Flags); !slices.Equal(got, []string{"high_sodium"}) {
		t.Fatalf("tuned: %v", got)
	}
}

func TestAllergens_RiceNoodlesAreGlutenFree(t *testing.T) {
	r := Recipe{Name: "Bowl", Portions: 2, Steps: []string{"Mix."}, Ingredients: []Ingredient{
		{Name: "Rice noodles", Grams: 200, Matched: true}, {Name: "Rice flour", Grams: 20, Matched: true},
	}}
	if got := codes(Check(r, Household{Allergens: []string{"gluten"}}).Blocks); len(got) != 0 {
		t.Errorf("rice products flagged: %v", got)
	}
	r.Ingredients[0].Name = "Egg noodles"
	if got := codes(Check(r, Household{Allergens: []string{"gluten"}}).Blocks); !slices.Equal(got, []string{"allergen"}) {
		t.Errorf("egg noodles: got %v", got)
	}
}

func TestCoreTempC_IsTheGroupMinimumOfTheStrictestProtein(t *testing.T) {
	for text, want := range map[string]int{"haehnchen in stuecken anbraten": 74, "hackfleisch braten": 71, "lachs und huehnchen": 74} {
		if got, ok := CoreTempC(text); !ok || got != want {
			t.Errorf("%q: got %d, %v; want %d", text, got, ok, want)
		}
	}
	if _, ok := CoreTempC("kokosmilch angiessen"); ok {
		t.Error("no protein named, yet a temperature")
	}
}
