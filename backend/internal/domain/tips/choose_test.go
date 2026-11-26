package tips_test

import (
	"slices"
	"testing"

	"github.com/jaltszeimer/plantry/backend/internal/chefsets"
	"github.com/jaltszeimer/plantry/backend/internal/domain/flavor"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/platefit"
	"github.com/jaltszeimer/plantry/backend/internal/domain/tips"
)

func ptr[T any](v T) *T { return &v }

// library turns a test set's foods into plate-fit components with stable ids.
func library(s chefsets.Set) map[string]platefit.Component {
	names := make([]string, 0, len(s.Foods))
	for n := range s.Foods {
		names = append(names, n)
	}
	slices.Sort(names)
	out := make(map[string]platefit.Component, len(names))
	for i, n := range names {
		f := s.Foods[n]
		a := f.Attrs
		t := flavor.Taste{Salt: a.Taste.Salt, Acid: a.Taste.Acid, Sweet: a.Taste.Sweet, Bitter: a.Taste.Bitter, Umami: a.Taste.Umami, Heat: a.Taste.Heat}
		c := platefit.Component{
			ID: int64(i + 1), Name: f.Name, Role: food.Role(f.Role), Leaf: f.Leaf, Fresh: ptr(a.Fresh),
			Attrs: flavor.Attributes{Texture: a.Texture, Colour: a.Colour, Taste: &t},
		}
		if !f.Leaf {
			c.Starch = ptr(a.HasStarch)
			c.Attrs = flavor.Attributes{
				Cuisine: a.Cuisine, Kind: a.Kind, Heroes: a.Heroes, Texture: a.Texture, Temperature: a.Temperature,
				Saucy: ptr(a.Saucy), Richness: ptr(a.Richness), Colour: a.Colour, Taste: &t,
				Technique: a.Technique, Format: a.Format,
			}
		}
		out[n] = c
	}
	return out
}

// rightKinds are the tip kinds that fix a planted flaw of a dimension, from
// culinary principle (independent of tips.Settles). A sharp element (acid,
// pickle) cuts heat and gives a contrast cuisine its contrast.
var rightKinds = map[string][]tips.Kind{
	"taste_balance": {tips.KindAcid, tips.KindPickle},
	"fresh":         {tips.KindHerbs, tips.KindPickle},
	"texture":       {tips.KindCrunch, tips.KindPickle, tips.KindSideSwap},
	"colour":        {tips.KindHerbs, tips.KindSideSwap},
	"temperature":   {tips.KindCool, tips.KindAcid, tips.KindPickle},
	"pairing":       {tips.KindCool, tips.KindAcid, tips.KindPickle, tips.KindSideSwap},
	"cuisine":       {tips.KindCuisineFix},
	"sauce_starch":  {tips.KindSideSwap, tips.KindSauce},
	"proportions":   {tips.KindSideSwap},
	"richness":      {tips.KindSideSwap, tips.KindSauce},
}

// checkTips is AC-4 over a split: at most 10 % of good plates get a tip, no
// classic plate gets one, and at least 70 % of planted flaws get a tip of the
// right kind.
func checkTips(t *testing.T, s chefsets.Set) {
	t.Helper()
	lib := library(s)
	var garnishes, sides []platefit.Component
	for _, c := range lib {
		if c.Leaf {
			garnishes = append(garnishes, c)
		} else {
			sides = append(sides, c)
		}
	}
	slices.SortFunc(garnishes, func(a, b platefit.Component) int { return int(a.ID - b.ID) })
	slices.SortFunc(sides, func(a, b platefit.Component) int { return int(a.ID - b.ID) })
	var good, goodTipped, flawed, right int
	for _, p := range s.Plates {
		pl := platefit.Plate{Main: lib[p.Components[0]]}
		for _, n := range p.Components[1:] {
			pl.Sides = append(pl.Sides, lib[n])
		}
		cands := tips.Candidates(pl, garnishes, sides)
		switch p.Kind {
		case chefsets.Classic:
			if len(cands) > 0 {
				t.Errorf("%s (classic): tip %s %s", p.ID, cands[0].Kind, cands[0].Food().Name)
			}
		case chefsets.Good:
			good++
			if len(cands) > 0 {
				goodTipped++
			}
		case chefsets.Flawed:
			flawed++
			if len(cands) == 0 {
				t.Logf("%s: no tip", p.ID)
				continue
			}
			ok := slices.ContainsFunc(p.Flaws, func(f chefsets.Flaw) bool { return slices.Contains(rightKinds[f.Dimension], cands[0].Kind) })
			if ok {
				right++
			} else {
				t.Logf("%s: tip %s (%s) for %+v", p.ID, cands[0].Kind, cands[0].Food().Name, p.Flaws)
			}
		}
	}
	t.Logf("good tipped %d/%d, flaws right %d/%d", goodTipped, good, right, flawed)
	if goodTipped*10 > good {
		t.Errorf("%d of %d good plates get a tip (max 10 %%)", goodTipped, good)
	}
	if right*10 < flawed*7 {
		t.Errorf("%d of %d flawed plates get the right tip kind (min 70 %%)", right, flawed)
	}
}

func TestCandidates_DevSet(t *testing.T) {
	s, err := chefsets.LoadDev()
	if err != nil {
		t.Fatal(err)
	}
	checkTips(t, s)
}

// TestCandidates_SealedSet runs AC-4 on the sealed split when
// PLANTRY_CHEF_SETS_SEALED is set (the orchestrator runs it).
func TestCandidates_SealedSet(t *testing.T) {
	s, ok, err := chefsets.LoadSealed()
	if !ok {
		t.Skipf("%s not set", chefsets.SealedDirEnv)
	}
	if err != nil {
		t.Fatal(err)
	}
	checkTips(t, s)
}

func TestCandidates_UnattributedNeverTips(t *testing.T) {
	rich := platefit.Component{
		ID: 1, Name: "Carbonara", Role: food.RoleMain, Fresh: ptr(false), Starch: ptr(true),
		Attrs: flavor.Attributes{Cuisine: "italian", Texture: "creamy", Temperature: "hot", Colour: "beige", Richness: ptr(3), Saucy: ptr(false), Taste: &flavor.Taste{Salt: 2, Umami: 2}},
	}
	lemon := platefit.Component{ID: 2, Name: "Zitrone", Leaf: true, Fresh: ptr(true), Attrs: flavor.Attributes{Texture: "soft", Colour: "beige", Taste: &flavor.Taste{Acid: 3}}}
	blank := platefit.Component{ID: 3, Name: "Unbekannt", Leaf: true}
	noTaste := lemon
	noTaste.ID, noTaste.Name, noTaste.Attrs.Taste = 4, "Ohne Geschmack", nil
	pl := platefit.Plate{Main: rich}
	if got := platefit.Read(pl).Raised(); !slices.Contains(got, platefit.TasteBalance) {
		t.Fatalf("plate raises %v, want taste_balance", got)
	}
	cands := tips.Candidates(pl, []platefit.Component{blank, noTaste}, nil)
	if len(cands) != 0 {
		t.Fatalf("unattributed garnish produced tip %+v", cands[0])
	}
	cands = tips.Candidates(pl, []platefit.Component{blank, noTaste, lemon}, nil)
	if len(cands) == 0 || cands[0].Food().ID != 2 || cands[0].Kind != tips.KindAcid {
		t.Fatalf("want a finishing-acid tip with the lemon, got %+v", cands)
	}
}

func TestCandidates_GoodPlateNoTip(t *testing.T) {
	main := platefit.Component{
		ID: 1, Name: "Curry", Role: food.RoleMain, Fresh: ptr(true), Starch: ptr(false),
		Attrs: flavor.Attributes{Cuisine: "neutral", Texture: "crunchy", Temperature: "hot", Colour: "green", Richness: ptr(1), Saucy: ptr(false), Taste: &flavor.Taste{Acid: 1}},
	}
	lemon := platefit.Component{ID: 2, Name: "Zitrone", Leaf: true, Fresh: ptr(true), Attrs: flavor.Attributes{Texture: "soft", Colour: "beige", Taste: &flavor.Taste{Acid: 3}}}
	if c := tips.Candidates(platefit.Plate{Main: main}, []platefit.Component{lemon}, nil); len(c) != 0 {
		t.Fatalf("good plate got tip %+v", c)
	}
}

// devCandidates returns the dev set's library split into garnishes and sides
// and every flawed or good plate's candidates.
func devCandidates(t *testing.T) (map[string]platefit.Component, []platefit.Component, []platefit.Component) {
	t.Helper()
	s, err := chefsets.LoadDev()
	if err != nil {
		t.Fatal(err)
	}
	lib := library(s)
	var garnishes, sides []platefit.Component
	for _, c := range lib {
		if c.Leaf {
			garnishes = append(garnishes, c)
		} else {
			sides = append(sides, c)
		}
	}
	return lib, garnishes, sides
}

// TestCandidates_KindSettlesItsDimension checks every candidate on every dev
// plate: its kind may settle the plate's top dimension, which it settles, and
// a cuisine fix answers a cuisine clash only.
func TestCandidates_KindSettlesItsDimension(t *testing.T) {
	s, err := chefsets.LoadDev()
	if err != nil {
		t.Fatal(err)
	}
	lib, garnishes, sides := devCandidates(t)
	for _, p := range s.Plates {
		pl := platefit.Plate{Main: lib[p.Components[0]]}
		for _, n := range p.Components[1:] {
			pl.Sides = append(pl.Sides, lib[n])
		}
		top, _ := tips.Top(platefit.Read(pl))
		for _, c := range tips.Candidates(pl, garnishes, sides) {
			if c.Dimension != top || !slices.Contains(c.Settles, top) {
				t.Errorf("%s: %s %s targets %s, settles %v; top is %s", p.ID, c.Kind, c.Food().Name, c.Dimension, c.Settles, top)
			}
			if !slices.Contains(tips.Settles[c.Kind], top) {
				t.Errorf("%s: %s %s may not settle %s", p.ID, c.Kind, c.Food().Name, top)
			}
			if c.Kind == tips.KindCuisineFix && top != platefit.CuisineFit {
				t.Errorf("%s: cuisine fix %s for %s", p.ID, c.Food().Name, top)
			}
		}
	}
}

// TestCandidates_KindPerDimension pins the kind a dimension gets on dev
// plates and the foods it never gets (each once measured as a wrong tip).
func TestCandidates_KindPerDimension(t *testing.T) {
	lib, garnishes, sides := devCandidates(t)
	tests := []struct {
		name   string
		plate  []string
		top    platefit.Dimension
		kinds  []tips.Kind
		never  []string
		noTips bool
		first  string
	}{
		{"taste balance gets an acid", []string{"Spaghetti Carbonara", "Gedünsteter Brokkoli"}, platefit.TasteBalance, []tips.Kind{tips.KindAcid, tips.KindPickle}, nil, false, ""},
		{"texture gets crunch, never lemon or basil", []string{"Grünes Thai-Curry", "Basmatireis", "Limettenspalten"}, platefit.Texture, []tips.Kind{tips.KindCrunch, tips.KindPickle, tips.KindSideSwap}, []string{"Zitronenspalten", "Basilikumblätter", "Frische Minze"}, false, ""},
		{"heat gets a cool or sharp element", []string{"Grünes Thai-Curry", "Basmatireis", "Gedünsteter Brokkoli"}, platefit.Temperature, []tips.Kind{tips.KindCool, tips.KindAcid, tips.KindPickle}, nil, false, ""},
		{"fresh gets herbs", []string{"Gemüselasagne", "Ofengemüse"}, platefit.Fresh, []tips.Kind{tips.KindHerbs, tips.KindPickle}, nil, false, ""},
		{"colour gets herbs of the cuisine", []string{"Hähnchen-Curry", "Basmatireis", "Gurken-Raita"}, platefit.Colour, []tips.Kind{tips.KindHerbs, tips.KindSideSwap}, nil, false, "Frischer Koriander"},
		{"cuisine clash gets a cuisine fix", []string{"Hähnchen-Tikka-Masala", "Sushireis", "Kachumber"}, platefit.CuisineFit, []tips.Kind{tips.KindCuisineFix}, nil, false, ""},
		{"sauce without starch gets a plain starch, never a gratin", []string{"Coq au Vin", "Salade verte"}, platefit.SauceStarch, []tips.Kind{tips.KindSideSwap}, []string{"Gratin Dauphinois"}, false, ""},
		{"two starches get a side swap", []string{"Chili con Carne", "Basmatireis", "Salzkartoffeln", "Tomatensalsa"}, platefit.Proportions, []tips.Kind{tips.KindSideSwap}, nil, false, ""},
		{"richness gets a side swap", []string{"Coq au Vin", "Gratin Dauphinois", "Salade verte"}, platefit.Richness, []tips.Kind{tips.KindSideSwap, tips.KindSauce}, nil, false, ""},
		{"dal and salad: swapping the salad for rice opens new gaps", []string{"Linsen-Dal", "Kachumber"}, platefit.SauceStarch, nil, nil, true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pl := platefit.Plate{Main: lib[tt.plate[0]]}
			for _, n := range tt.plate[1:] {
				pl.Sides = append(pl.Sides, lib[n])
			}
			if top, _ := tips.Top(platefit.Read(pl)); top != tt.top {
				t.Fatalf("top %s, want %s", top, tt.top)
			}
			cands := tips.Candidates(pl, garnishes, sides)
			if tt.noTips {
				if len(cands) > 0 {
					t.Fatalf("want no tip, got %s %s", cands[0].Kind, cands[0].Food().Name)
				}
				return
			}
			if len(cands) == 0 {
				t.Fatal("no tip")
			}
			if tt.first != "" && cands[0].Food().Name != tt.first {
				t.Errorf("first tip %s, want %s", cands[0].Food().Name, tt.first)
			}
			if !slices.Contains(tt.kinds, cands[0].Kind) {
				t.Errorf("first tip %s %s, want one of %v", cands[0].Kind, cands[0].Food().Name, tt.kinds)
			}
			for _, c := range cands {
				if slices.Contains(tt.never, c.Food().Name) {
					t.Errorf("tip %s %s", c.Kind, c.Food().Name)
				}
			}
		})
	}
}

// TestCandidates_NoSecondStarch: a starch never joins a plate that holds one,
// whether the other starch is known (plate fit reads two starches) or unknown
// (it could be one).
func TestCandidates_NoSecondStarch(t *testing.T) {
	main := platefit.Component{
		ID: 1, Name: "Schnitzel", Role: food.RoleMain, Fresh: ptr(false), Starch: ptr(false),
		Attrs: flavor.Attributes{Cuisine: "indian", Texture: "crisp", Temperature: "hot", Colour: "brown", Richness: ptr(2), Saucy: ptr(false), Taste: &flavor.Taste{Salt: 2, Acid: 1}},
	}
	salad := platefit.Component{
		ID: 2, Name: "Kartoffelsalat", Role: food.RoleSideVeg, Fresh: ptr(false), Starch: ptr(true),
		Attrs: flavor.Attributes{Cuisine: "indian", Texture: "soft", Temperature: "cold", Colour: "beige", Richness: ptr(1), Saucy: ptr(false), Taste: &flavor.Taste{Salt: 1, Acid: 2}},
	}
	beans := platefit.Component{
		ID: 3, Name: "Bohnen", Role: food.RoleSideVeg, Fresh: ptr(false), Starch: ptr(false),
		Attrs: flavor.Attributes{Cuisine: "italian", Texture: "soft", Temperature: "hot", Colour: "beige", Richness: ptr(0), Saucy: ptr(false), Taste: &flavor.Taste{Salt: 1}},
	}
	rice := platefit.Component{
		ID: 4, Name: "Basmatireis", Role: food.RoleSideStarch, Fresh: ptr(false), Starch: ptr(true),
		Attrs: flavor.Attributes{Cuisine: "indian", Texture: "soft", Temperature: "hot", Colour: "white", Richness: ptr(0), Saucy: ptr(false), Taste: &flavor.Taste{}},
	}
	carrots := platefit.Component{
		ID: 5, Name: "Möhren", Role: food.RoleSideVeg, Fresh: ptr(true), Starch: ptr(false),
		Attrs: flavor.Attributes{Cuisine: "neutral", Texture: "crunchy", Temperature: "cold", Colour: "orange", Richness: ptr(0), Saucy: ptr(false), Taste: &flavor.Taste{Acid: 1}},
	}
	unknown := salad
	unknown.Starch = nil
	for _, s := range []platefit.Component{salad, unknown} {
		pl := platefit.Plate{Main: main, Sides: []platefit.Component{s, beans}}
		if !slices.Contains(platefit.Read(pl).Clashes, platefit.CuisineFit) {
			t.Fatal("plate has no cuisine clash")
		}
		for _, c := range tips.Candidates(pl, nil, []platefit.Component{rice, carrots}) {
			if c.Swap == 1 && c.With.ID == rice.ID {
				t.Errorf("starch %v: rice swapped in beside the potato salad: %+v", s.Starch, c)
			}
		}
	}
}

// sauceStarchPlate is a sauced indian main with a plain bean side and a raita.
func sauceStarchPlate() (platefit.Plate, platefit.Component, platefit.Component) {
	side := func(id int64, name string, role food.Role, starch *bool) platefit.Component {
		return platefit.Component{
			ID: id, Name: name, Role: role, Fresh: ptr(false), Starch: starch,
			Attrs: flavor.Attributes{Cuisine: "indian", Texture: "soft", Temperature: "hot", Colour: "beige", Richness: ptr(0), Saucy: ptr(false), Taste: &flavor.Taste{Salt: 1}},
		}
	}
	main := side(1, "Chicken Korma", food.RoleMain, ptr(false))
	main.Attrs.Saucy, main.Attrs.Richness, main.Attrs.Taste = ptr(true), ptr(2), &flavor.Taste{Salt: 2, Acid: 1}
	raita := side(6, "Raita", food.RoleSideVeg, ptr(false))
	raita.Attrs.Temperature, raita.Fresh, raita.Attrs.Colour, raita.Attrs.Texture = "cold", ptr(true), "green", "crunchy"
	sides := []platefit.Component{side(2, "Bohnen", food.RoleSideVeg, ptr(false)), raita}
	return platefit.Plate{Main: main, Sides: sides}, side(4, "Reis", food.RoleSideStarch, ptr(true)), side(5, "Polenta", food.RoleSideStarch, ptr(true))
}

// TestCandidates_SauceStarchSwap: a sauce without a starch swaps a side for the
// starch of the main's palette, not for any starch.
func TestCandidates_SauceStarchSwap(t *testing.T) {
	pl, rice, polenta := sauceStarchPlate()
	if top, _ := tips.Top(platefit.Read(pl)); top != platefit.SauceStarch {
		t.Fatalf("top %s, want sauce_starch", top)
	}
	cands := tips.Candidates(pl, nil, []platefit.Component{polenta, rice})
	if len(cands) == 0 || cands[0].Kind != tips.KindSideSwap || cands[0].With.ID != rice.ID {
		t.Fatalf("want the palette rice swapped in, got %+v", cands)
	}
	for _, c := range cands {
		if c.With.ID == polenta.ID {
			t.Errorf("a starch outside the palette swapped in: %+v", c)
		}
	}
	if c := tips.Candidates(pl, nil, []platefit.Component{polenta}); len(c) != 0 {
		t.Errorf("non-palette starch alone got a tip: %+v", c)
	}
}

// TestCandidates_NoFittingGarnishNoTip: a plate missing a fresh element gets
// herbs when the library has them and no tip at all when its only fresh leaf
// is cucumber (a cool element, not a herb).
func TestCandidates_NoFittingGarnishNoTip(t *testing.T) {
	lib, _, _ := devCandidates(t)
	pl := platefit.Plate{Main: lib["Gemüselasagne"], Sides: []platefit.Component{lib["Ofengemüse"]}}
	if top, _ := tips.Top(platefit.Read(pl)); top != platefit.Fresh {
		t.Fatalf("top %s, want fresh", top)
	}
	cucumber := platefit.Component{ID: 500, Name: "Gurke", Leaf: true, Fresh: ptr(true), Attrs: flavor.Attributes{Texture: "crunchy", Colour: "green", Taste: &flavor.Taste{}}}
	if c := tips.Candidates(pl, []platefit.Component{cucumber}, nil); len(c) != 0 {
		t.Fatalf("cucumber offered: %s %s", c[0].Kind, c[0].Food().Name)
	}
	c := tips.Candidates(pl, []platefit.Component{cucumber, lib["Frische Petersilie"]}, nil)
	if len(c) == 0 || c[0].Kind != tips.KindHerbs || c[0].Food().Name != "Frische Petersilie" {
		t.Fatalf("want parsley as fresh herbs, got %+v", c)
	}
}

func tipPlate() (platefit.Plate, platefit.Component, platefit.Component) {
	main := platefit.Component{
		ID: 1, Name: "Omelett", Role: food.RoleMain, Fresh: ptr(false), Starch: ptr(false),
		Attrs: flavor.Attributes{Cuisine: "neutral", Texture: "crisp", Temperature: "hot", Colour: "beige", Richness: ptr(1), Saucy: ptr(false), Taste: &flavor.Taste{Salt: 2, Acid: 1}},
	}
	chives := platefit.Component{
		ID: 2, Name: "Schnittlauch", Role: food.RoleSideVeg, Leaf: true, Fresh: ptr(true),
		Attrs: flavor.Attributes{Texture: "soft", Colour: "green", Taste: &flavor.Taste{}},
	}
	return platefit.Plate{Main: main}, chives, platefit.Component{}
}

// TestCandidates_GarnishRules: a garnish goes only on a plate of at most three
// components and never when the plate already has that element.
func TestCandidates_GarnishRules(t *testing.T) {
	p, chives, _ := tipPlate()
	if len(tips.Candidates(p, []platefit.Component{chives}, nil)) == 0 {
		t.Fatal("a bare plate gets no garnish")
	}
	busy := p
	for i := range 3 {
		s := chives
		s.ID, s.Name, s.Leaf = int64(10+i), "Beilage", false
		busy.Sides = append(busy.Sides, s)
	}
	for _, c := range tips.Candidates(busy, []platefit.Component{chives}, nil) {
		if c.Add != nil {
			t.Errorf("garnish on a plate of %d components: %+v", len(busy.Sides)+1, c)
		}
	}
	has := p
	has.Sides = []platefit.Component{chives}
	other := chives
	other.ID, other.Name = 3, "Petersilie"
	for _, c := range tips.Candidates(has, []platefit.Component{other}, nil) {
		if c.Add != nil && c.Kind == tips.KindHerbs {
			t.Errorf("herbs added to a plate with herbs: %+v", c)
		}
	}
	clash := p
	clash.Main.Attrs.Cuisine = "indian"
	clash.Sides = []platefit.Component{{
		ID: 20, Name: "Spaghetti", Role: food.RoleSideStarch, Fresh: ptr(false), Starch: ptr(true),
		Attrs: flavor.Attributes{Cuisine: "italian", Texture: "soft", Temperature: "hot", Colour: "beige", Richness: ptr(0), Saucy: ptr(false), Taste: &flavor.Taste{Salt: 1}},
	}}
	if !slices.Contains(platefit.Read(clash).Clashes, platefit.CuisineFit) {
		t.Fatal("plate has no cuisine clash")
	}
	for _, c := range tips.Candidates(clash, []platefit.Component{chives}, nil) {
		if c.Add != nil {
			t.Errorf("garnish on a clash plate: %+v", c)
		}
	}
}

// TestCandidates_ClashSwapBelongsToCuisine: a cuisine clash is fixed only by a
// side of the main's cuisine (by its cuisine or the palette), never by a
// neutral generic salad; with no such side in the library there is no tip.
func TestCandidates_ClashSwapBelongsToCuisine(t *testing.T) {
	side := func(id int64, name, cuisine string, role food.Role, starch bool) platefit.Component {
		return platefit.Component{
			ID: id, Name: name, Role: role, Fresh: ptr(false), Starch: ptr(starch),
			Attrs: flavor.Attributes{Cuisine: cuisine, Texture: "soft", Temperature: "hot", Colour: "beige", Richness: ptr(0), Saucy: ptr(false), Taste: &flavor.Taste{Salt: 1}},
		}
	}
	main := side(1, "Chicken Korma", "indian", food.RoleMain, false)
	main.Attrs.Richness = ptr(2)
	wrong := side(2, "Spaghetti", "italian", food.RoleSideStarch, true)
	pl := platefit.Plate{Main: main, Sides: []platefit.Component{wrong}}
	if !slices.Contains(platefit.Read(pl).Clashes, platefit.CuisineFit) {
		t.Fatal("plate has no cuisine clash")
	}
	carrot := side(3, "Möhrensalat", "neutral", food.RoleSideVeg, false)
	carrot.Attrs.Temperature = "cold"
	naan := side(4, "Naan", "indian", food.RoleSideStarch, true)
	for _, c := range tips.Candidates(pl, nil, []platefit.Component{carrot}) {
		t.Errorf("library without an Indian side got a tip: %+v", c)
	}
	cands := tips.Candidates(pl, nil, []platefit.Component{carrot, naan})
	if len(cands) == 0 || cands[0].Food().ID != naan.ID {
		t.Errorf("want naan swapped in, got %+v", cands)
	}
	// A dish of unknown starch beside could be a second starch: no naan.
	unk := side(7, "Gemüsepfanne", "indian", food.RoleSideVeg, false)
	unk.Starch = nil
	two := platefit.Plate{Main: main, Sides: []platefit.Component{wrong, unk}}
	if !slices.Contains(platefit.Read(two).Clashes, platefit.CuisineFit) {
		t.Fatal("plate has no cuisine clash")
	}
	for _, c := range tips.Candidates(two, nil, []platefit.Component{naan}) {
		t.Errorf("starch swapped in beside a dish of unknown starch: %+v", c)
	}
	for _, c := range cands {
		if c.Food().ID == carrot.ID {
			t.Errorf("neutral salad swapped in for an Indian main")
		}
	}
}

// TestCandidates_NoSecondSaladSwap: a swap never brings a second side of a
// kind the plate already has.
func TestCandidates_NoSecondSaladSwap(t *testing.T) {
	salad := func(id int64, name, cuisine string) platefit.Component {
		return platefit.Component{
			ID: id, Name: name, Role: food.RoleSideVeg, Fresh: ptr(true), Starch: ptr(false),
			Attrs: flavor.Attributes{Cuisine: cuisine, Kind: "salad", Texture: "crisp", Temperature: "cold", Colour: "green", Richness: ptr(0), Saucy: ptr(false), Taste: &flavor.Taste{Acid: 1}},
		}
	}
	main := platefit.Component{
		ID: 1, Name: "Tuna pasta", Role: food.RoleMain, Fresh: ptr(false), Starch: ptr(true),
		Attrs: flavor.Attributes{Cuisine: "italian", Kind: "pasta", Texture: "soft", Temperature: "hot", Colour: "red", Richness: ptr(1), Saucy: ptr(true), Taste: &flavor.Taste{Salt: 2, Acid: 2}},
	}
	pl := platefit.Plate{Main: main, Sides: []platefit.Component{salad(2, "Green salad", "mediterranean"), salad(3, "Yogurt salad", "middle_eastern")}}
	for _, c := range tips.Candidates(pl, nil, []platefit.Component{salad(4, "Carrot salad", "mediterranean")}) {
		t.Errorf("second salad swapped in: %+v", c)
	}
}
