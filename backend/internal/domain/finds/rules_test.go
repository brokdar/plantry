package finds_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/finds"
	"github.com/jaltszeimer/plantry/backend/internal/domain/importer"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
)

func ptr[T any](v T) *T { return &v }

func draft(grams float64, minutes int) *importer.Draft {
	return &importer.Draft{
		Name: "Soup", ReferencePortions: 2, TotalMinutes: ptr(minutes), Instructions: []string{"Cook."},
		Ingredients: []importer.DraftIngredient{{Name: "lentils", Amount: grams, Unit: "g"}, {Name: "salt", Amount: 5, Unit: "g"}},
	}
}

func TestRank_BayesTierVotesPenalty(t *testing.T) {
	t.Parallel()
	cands := []finds.Candidate{
		{Source: finds.Source{URL: "a", Site: "chefkoch.de", Rating: ptr(4.9), Votes: 5000}, Draft: draft(300, 30)},
		{Source: finds.Source{URL: "b", Site: "bbcgoodfood.com", Rating: ptr(4.6), Votes: 400}, Draft: draft(300, 30)},
		{Source: finds.Source{URL: "c", Site: "budgetbytes.com", Rating: ptr(5.0), Votes: 3}, Draft: draft(300, 30)},    // under 10 votes, not editorial
		{Source: finds.Source{URL: "d", Site: "eatsmarter.de", Rating: nil, Votes: 0}, Draft: draft(300, 30)},           // editorial: kept on the prior
		{Source: finds.Source{URL: "e", Site: "malteskitchen.de", Rating: ptr(4.6), Votes: 400}, Draft: draft(300, 30)}, // vetted blog
	}
	got := finds.Rank(cands, nil)
	var urls []string
	for _, c := range got {
		urls = append(urls, c.Source.URL)
	}
	require.Equal(t, []string{"b", "e", "d", "a"}, urls) // user tier 0.8 sinks 4.9
	require.InDelta(t, (4.0*30+4.6*400)/430, got[0].Source.Score, 1e-9)

	got = finds.Rank(cands, map[string]int{"bbcgoodfood.com": 2})
	require.Equal(t, "e", got[0].Source.URL) // two "looks wrong" lower the site
}

func TestCrossCheck_FlagsOutliersNeverAverages(t *testing.T) {
	t.Parallel()
	base := draft(900, 30)
	versions := []*importer.Draft{base, draft(300, 30), draft(320, 35)}
	flags := finds.CrossCheck(base, versions)
	require.Len(t, flags, 2)
	require.Equal(t, "outlier_grams_per_serving", flags[0].Code)
	require.Equal(t, 452.5, flags[0].Value)
	require.Equal(t, "outlier_salt_per_kg", flags[1].Code)
	require.Equal(t, 900.0, base.Ingredients[0].Amount) // untouched

	require.Empty(t, finds.CrossCheck(versions[1], versions))
	require.Empty(t, finds.CrossCheck(base, versions[:2])) // needs three versions
}

func TestSingleSourceEnough(t *testing.T) {
	t.Parallel()
	good := finds.Source{Rating: ptr(4.5), Votes: 200}
	require.True(t, finds.SingleSourceEnough(good, draft(300, 30)))
	require.False(t, finds.SingleSourceEnough(finds.Source{Rating: ptr(4.4), Votes: 900}, draft(300, 30)))
	require.False(t, finds.SingleSourceEnough(finds.Source{Rating: ptr(4.8), Votes: 199}, draft(300, 30)))
	require.False(t, finds.SingleSourceEnough(good, &importer.Draft{Name: "Banana bread"}))
}

func TestOriginOf(t *testing.T) {
	t.Parallel()
	require.Equal(t, finds.OriginAdaptedLight, finds.OriginOf(0, 0, 0))
	require.Equal(t, finds.OriginAdaptedLight, finds.OriginOf(2, 0, 0.15))
	require.Equal(t, finds.OriginAdaptedMedium, finds.OriginOf(3, 0, 0.1))
	require.Equal(t, finds.OriginAdaptedMedium, finds.OriginOf(1, 1, 0.3))
	require.Equal(t, finds.OriginInspired, finds.OriginOf(5, 0, 0.1))
	require.Equal(t, finds.OriginInspired, finds.OriginOf(2, 0, 0.31))
	require.Equal(t, finds.OriginInspired, finds.OriginOf(2, 2, 0.2))
}

func TestKitchenFit(t *testing.T) {
	t.Parallel()
	k := profile.Kitchen{MinutesWeekday: 30, MinutesWeekend: 60, Equipment: []string{"oven", "stovetop"}, Skill: ""}
	p, q := finds.KitchenFit(finds.Requirements{
		Equipment: []string{"oven", "wok"}, ActiveMinutes: 61, Advanced: true,
		Specialty: []string{"gochujang", "yuzu", "sumac", "miso"},
	}, k, []string{"Weiße Miso-Paste"}, nil)
	require.Equal(t, []string{"equipment:wok", "too_long", "skill", "specialty"}, p)
	require.Equal(t, []string{"equipment:wok", "skill"}, q)

	_, q = finds.KitchenFit(finds.Requirements{Equipment: []string{"wok"}, Advanced: true}, k, nil, []string{"equipment:wok", "skill"})
	require.Empty(t, q) // asked once

	k.Skill = profile.SkillConfident
	p, _ = finds.KitchenFit(finds.Requirements{Equipment: []string{"stovetop"}, ActiveMinutes: 60, Advanced: true, Specialty: []string{"yuzu", "sumac"}}, k, nil, nil)
	require.Empty(t, p)
}
