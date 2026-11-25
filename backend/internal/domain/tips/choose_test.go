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

// rightKinds are the tip kinds that fix a planted flaw of a dimension. Acid
// stands in for a cool element (plate fit reads a sharp element as cutting
// heat) and a citrus garnish is a fresh element.
var rightKinds = map[string][]tips.Kind{
	"taste_balance": {tips.KindAcid, tips.KindPickle},
	"fresh":         {tips.KindHerbs, tips.KindAcid, tips.KindCool, tips.KindSideSwap},
	"texture":       {tips.KindCrunch, tips.KindSideSwap},
	"colour":        {tips.KindHerbs, tips.KindSideSwap},
	"temperature":   {tips.KindCool, tips.KindAcid, tips.KindPickle},
	"pairing":       {tips.KindCool, tips.KindAcid, tips.KindPickle},
	"cuisine":       {tips.KindCuisineFix},
	"sauce_starch":  {tips.KindSideSwap},
	"proportions":   {tips.KindSideSwap},
	"richness":      {tips.KindSideSwap},
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
