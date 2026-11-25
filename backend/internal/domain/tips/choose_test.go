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
		Attrs: flavor.Attributes{Cuisine: "neutral", Texture: "crisp", Temperature: "hot", Colour: "brown", Richness: ptr(2), Saucy: ptr(false), Taste: &flavor.Taste{Salt: 2, Acid: 1}},
	}
	salad := platefit.Component{
		ID: 2, Name: "Kartoffelsalat", Role: food.RoleSideVeg, Fresh: ptr(false), Starch: ptr(true),
		Attrs: flavor.Attributes{Cuisine: "neutral", Texture: "soft", Temperature: "cold", Colour: "beige", Richness: ptr(1), Saucy: ptr(false), Taste: &flavor.Taste{Salt: 1, Acid: 2}},
	}
	beans := platefit.Component{
		ID: 3, Name: "Bohnen", Role: food.RoleSideVeg, Fresh: ptr(false), Starch: ptr(false),
		Attrs: flavor.Attributes{Cuisine: "neutral", Texture: "soft", Temperature: "hot", Colour: "beige", Richness: ptr(0), Saucy: ptr(false), Taste: &flavor.Taste{Salt: 1}},
	}
	rice := platefit.Component{
		ID: 4, Name: "Basmatireis", Role: food.RoleSideStarch, Fresh: ptr(false), Starch: ptr(true),
		Attrs: flavor.Attributes{Cuisine: "neutral", Texture: "soft", Temperature: "hot", Colour: "white", Richness: ptr(0), Saucy: ptr(false), Taste: &flavor.Taste{}},
	}
	carrots := platefit.Component{
		ID: 5, Name: "Möhren", Role: food.RoleSideVeg, Fresh: ptr(true), Starch: ptr(false),
		Attrs: flavor.Attributes{Cuisine: "neutral", Texture: "crunchy", Temperature: "cold", Colour: "orange", Richness: ptr(0), Saucy: ptr(false), Taste: &flavor.Taste{Acid: 1}},
	}
	unknown := salad
	unknown.Starch = nil
	for _, s := range []platefit.Component{salad, unknown} {
		pl := platefit.Plate{Main: main, Sides: []platefit.Component{s, beans}}
		if raised := platefit.Read(pl).Raised(); len(raised) == 0 {
			t.Fatal("plate raises nothing")
		}
		cands := tips.Candidates(pl, nil, []platefit.Component{rice, carrots})
		if len(cands) == 0 || cands[0].Food().ID != carrots.ID {
			t.Errorf("starch %v: want the carrots, got %+v", s.Starch, cands)
		}
		for _, c := range cands {
			if c.Food().ID == rice.ID {
				t.Errorf("starch %v: rice swapped in beside the potato salad", s.Starch)
			}
		}
	}
}
