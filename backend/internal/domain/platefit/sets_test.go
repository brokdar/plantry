package platefit_test

import (
	"reflect"
	"slices"
	"testing"

	"github.com/jaltszeimer/plantry/backend/internal/chefsets"
	"github.com/jaltszeimer/plantry/backend/internal/domain/flavor"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/platefit"
)

func ptr[T any](v T) *T { return &v }

// component turns a labelled food of a test set into a plate component. A
// leaf carries only taste, texture, colour and fresh.
func component(f chefsets.Food) platefit.Component {
	a := f.Attrs
	t := flavor.Taste{Salt: a.Taste.Salt, Acid: a.Taste.Acid, Sweet: a.Taste.Sweet, Bitter: a.Taste.Bitter, Umami: a.Taste.Umami, Heat: a.Taste.Heat}
	c := platefit.Component{
		Name: f.Name, Role: food.Role(f.Role), Leaf: f.Leaf, Fresh: ptr(a.Fresh),
		Attrs: flavor.Attributes{Texture: a.Texture, Colour: a.Colour, Taste: &t},
	}
	if f.Leaf {
		return c
	}
	c.Starch = ptr(a.HasStarch)
	c.Attrs = flavor.Attributes{
		Cuisine: a.Cuisine, Kind: a.Kind, Heroes: a.Heroes, Texture: a.Texture, Temperature: a.Temperature,
		Saucy: ptr(a.Saucy), Richness: ptr(a.Richness), Colour: a.Colour, Taste: &t,
		Technique: a.Technique, Format: a.Format,
	}
	return c
}

func plateOf(s chefsets.Set, p chefsets.Plate) platefit.Plate {
	out := platefit.Plate{Main: component(s.Foods[p.Components[0]])}
	for _, c := range p.Components[1:] {
		out.Sides = append(out.Sides, component(s.Foods[c]))
	}
	return out
}

// checkSet is AC-3 over a split: the same plate reads the same in 100 runs;
// classic and good plates raise nothing; every flawed plate raises something
// and shows each dimension it is tagged with as a clash or gap of the tagged
// type (a tagged gap may be minor where the cuisine relaxes it).
func checkSet(t *testing.T, s chefsets.Set) {
	t.Helper()
	for _, p := range s.Plates {
		plate := plateOf(s, p)
		r := platefit.Read(plate)
		for range 100 {
			if again := platefit.Read(plate); !reflect.DeepEqual(again, r) {
				t.Fatalf("%s: reading changed: %+v then %+v", p.ID, r, again)
			}
		}
		switch p.Kind {
		case chefsets.Classic, chefsets.Good:
			if raised := r.Raised(); len(raised) > 0 {
				t.Errorf("%s (%s): raises %v", p.ID, p.Kind, raised)
			}
			if p.Kind == chefsets.Good && len(r.Gaps) > 0 {
				t.Errorf("%s: a good plate has gaps %+v", p.ID, r.Gaps)
			}
		case chefsets.Flawed:
			if len(r.Raised()) == 0 {
				t.Errorf("%s: a flawed plate raises nothing, reading %+v", p.ID, r)
			}
			for _, fl := range p.Flaws {
				d := platefit.Dimension(fl.Dimension)
				found := slices.Contains(r.Clashes, d)
				if fl.Type == "gap" {
					found = slices.ContainsFunc(r.Gaps, func(g platefit.Gap) bool { return g.Dimension == d })
				}
				if !found {
					t.Errorf("%s: tagged %s %s (%s), reading %+v", p.ID, fl.Dimension, fl.Type, fl.Expect, r)
				}
			}
		}
	}
}

func TestDevSet(t *testing.T) {
	s, err := chefsets.LoadDev()
	if err != nil {
		t.Fatal(err)
	}
	checkSet(t, s)
}

// TestSealedSet runs AC-3 on the sealed split when PLANTRY_CHEF_SETS_SEALED is
// set (the orchestrator runs it).
func TestSealedSet(t *testing.T) {
	s, ok, err := chefsets.LoadSealed()
	if !ok {
		t.Skipf("%s not set", chefsets.SealedDirEnv)
	}
	if err != nil {
		t.Fatal(err)
	}
	checkSet(t, s)
}
