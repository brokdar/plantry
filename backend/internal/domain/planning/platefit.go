package planning

import (
	"math"
	"slices"
	"strings"

	"github.com/jaltszeimer/plantry/backend/internal/domain/diet"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/nutrition"
	"github.com/jaltszeimer/plantry/backend/internal/domain/platefit"
)

// Plate fit in the heuristic (PLATE-4, PLATE-5): when plan.plate_fit is on,
// a side (pickSide, addSide) that brings a clash to its plate is no
// candidate, and the candidates rank by the plate's raised gaps after
// nutrition and before the existing keys. No-cook add-ons (addNoCook) are
// filtered by their own rules (fillerFits), not by a reading. Minor gaps
// never filter or rank.

// freshWords are ingredients that make a raw, green or citrus element.
var freshWords = []string{
	"salat", "salad", "lettuce", "gurke", "cucumber", "rucola", "rocket", "arugula", "kresse", "cress",
	"petersilie", "parsley", "basilikum", "basil", "koriander", "cilantro", "coriander", "schnittlauch", "chive",
	"dill", "minze", "mint", "zitrone", "lemon", "limette", "lime", "fruehlingszwiebel", "spring onion", "radieschen", "radish",
}

// rawVegWords are vegetables served raw as a leaf or a rohkost recipe (a
// tomato inside a sauce does not make a recipe fresh, so they never count as
// recipe ingredients): the fresh element of a plate.
var rawVegWords = []string{
	"paprika", "bell pepper", "tomat", "karotte", "moehre", "carrot", "rohkost", "kohlrabi", "sellerie", "celery",
	"feldsalat", "sprout", "keim", "avocado", "fenchel", "fennel",
}

// starchLeafWords are ingredients that make a dish a starch (with
// pastaWords and starchWords) once a portion holds starchMinG of them.
var starchLeafWords = []string{"tortilla", "quinoa", "spaetzle", "mehl", "flour", "teig", "dough"}

// starchMinG is the grams per portion of starch ingredients that make a
// dish a starch: a roux's flour does not, a quiche's crust does.
const starchMinG = 40

// kcalBucket: candidates whose kcal differ from the need by about the same
// (within this) are equal for nutrition, so plate fit orders them.
const kcalBucket = 25

// sideKcalBucket is kcalBucket for a side cooked with its main, chosen
// before the day's top-ups: sides within it are equal for nutrition.
const sideKcalBucket = 100

// excessFat is the fat kcal of mult portions of m above the week's fat
// range top (fatHigh, in %) of their own kcal, in sideKcalBucket steps. A
// side or filler with more (cheese, cream, parmesan salad) lifts the week's
// fat share where a lean one would not (PLATE-4: nutrition before plate
// fit); a lean one gives 0.
func excessFat(m nutrition.Macros, mult, fatHigh float64) float64 {
	return math.Round(max(0, mult*(m.Fat*9-m.Kcal*fatHigh/100)) / sideKcalBucket)
}

func hasWord(name string, lists ...[]string) bool {
	n := diet.Fold(name)
	for _, words := range lists {
		if slices.ContainsFunc(words, func(w string) bool { return strings.Contains(n, w) }) {
			return true
		}
	}
	return false
}

// freshStarch derives plate fit's fresh and starch inputs from a food's
// name and ingredients (A1 has no such attributes). A leaf is judged by its
// name; a recipe without ingredients is unknown (nil).
func freshStarch(f FoodFacts, leaves []food.LeafGrams) (fresh, starch *bool) {
	if f.Kind != food.KindComposed {
		v := hasWord(f.Name, freshWords, rawVegWords)
		return &v, nil
	}
	if len(leaves) == 0 {
		return nil, nil
	}
	fr := hasWord(f.Name, freshWords, saladWords)
	st := f.Role == food.RoleSideStarch || hasWord(f.Name, pastaWords, starchWords, starchLeafWords)
	starchG := 0.0
	for _, l := range leaves {
		fr = fr || hasWord(l.Name, freshWords)
		if hasWord(l.Name, pastaWords, starchWords, starchLeafWords) {
			starchG += l.Grams
		}
	}
	st = st || starchG >= starchMinG
	return &fr, &st
}

// plateComponent is f as plate fit reads it.
func plateComponent(f FoodFacts) platefit.Component {
	return platefit.Component{
		ID: f.ID, Name: f.Name, Role: f.Role, Leaf: !f.Composed(),
		Attrs: f.Attrs, Fresh: f.Fresh, Starch: f.Starch,
	}
}

// plateFitOn reports whether plate fit judges a plate with main mainID.
func (s State) plateFitOn(mainID int64) bool {
	_, ok := s.Foods[mainID]
	return s.Settings.PlateFit && ok
}

// sideFit reads the plate of main with the sides others plus cand: clash
// when cand adds a clash the plate did not have or is a second protein
// (secondProtein), raised the count of the plate's raised gaps with cand
// read by its known values (lower is better).
func (s State) sideFit(mainID int64, others []int64, cand FoodFacts) (clash bool, raised int) {
	clash, raised = s.sideRead(mainID, others, cand)
	return clash || s.secondProtein(mainID, others, cand), raised
}

// secondProtein reports whether side cand is a protein of its own (a protein
// side, meat, fish, tofu or an egg dish; a bean or lentil side is a
// vegetable here) on a plate whose main or
// other sides already carry one (carriesProtein): marinated tofu beside a
// ragù or chicken legs is not a meal anyone cooks (the filler rule, for
// cooked sides).
func (s State) secondProtein(mainID int64, others []int64, cand FoodFacts) bool {
	name := diet.Fold(cand.Name)
	if !(cand.Role == food.RoleSideProtein || cand.Groups.MeatG+cand.Groups.FishG >= 30 ||
		containsAny(name, proteinWords) || hasWholeWord(name, eggWords)) {
		return false
	}
	for _, id := range append([]int64{mainID}, others...) {
		if g, ok := s.Foods[id]; ok && id != cand.ID && carriesProtein(g) {
			return true
		}
	}
	return false
}

// sideRead is sideFit by the plate fit reading alone.
func (s State) sideRead(mainID int64, others []int64, cand FoodFacts) (clash bool, raised int) {
	pl := platefit.Plate{Main: plateComponent(s.Foods[mainID])}
	for _, id := range others {
		if f, ok := s.Foods[id]; ok && id != mainID && id != cand.ID {
			pl.Sides = append(pl.Sides, plateComponent(f))
		}
	}
	before := platefit.Read(pl)
	sides := pl.Sides
	pl.Sides = append(slices.Clone(sides), plateComponent(cand))
	for _, d := range platefit.Read(pl).Clashes {
		if !slices.Contains(before.Clashes, d) {
			clash = true
		}
	}
	// A gap closes only by the candidate's known values (platefit.Proven):
	// a blank candidate never outranks one that really closes it.
	pl.Sides = append(slices.Clone(sides), platefit.Proven(plateComponent(cand)))
	for _, g := range platefit.Read(pl).Gaps {
		if !g.Minor {
			raised++
		}
	}
	return clash, raised
}

// plateFitOf is sideFit for cand added to the merged plate p; with lift (the
// H9 floor lift, when nothing else lifts the day) a second protein is no
// clash.
func (s State) plateFitOf(p Plate, cand FoodFacts, lift bool) (on, clash bool, raised int) {
	mainID := s.mainOf(p)
	if !s.plateFitOn(mainID) {
		return false, false, 0
	}
	others := plateFoods(p)
	if lift {
		clash, raised = s.sideRead(mainID, others, cand)
	} else {
		clash, raised = s.sideFit(mainID, others, cand)
	}
	return true, clash, raised
}

// plateFoods are the food IDs of plate p's components.
func plateFoods(p Plate) []int64 {
	out := make([]int64, 0, len(p.Components))
	for _, c := range p.Components {
		out = append(out, c.FoodID)
	}
	return out
}
