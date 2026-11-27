package chefsets

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/fdc"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/jsonld"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/off"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/importer"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

func loadMatching(t *testing.T) *Matching {
	t.Helper()
	m, err := LoadMatching()
	require.NoError(t, err)
	return m
}

func (s MatchSet) kinds() map[string]int {
	out := map[string]int{}
	for _, l := range s.Lines {
		out[l.Kind]++
	}
	return out
}

// TestMatchingSetsLoad: the German set is the 72 real lines of the repo's test
// pages, the English set is similar in size, and every line is labelled
// consistently.
func TestMatchingSetsLoad(t *testing.T) {
	m := loadMatching(t)
	assert.Len(t, m.DE.Lines, 72)
	assert.GreaterOrEqual(t, len(m.EN.Lines), 72)
	t.Logf("DE %d lines, kinds %v", len(m.DE.Lines), m.DE.kinds())
	t.Logf("EN %d lines, kinds %v", len(m.EN.Lines), m.EN.kinds())

	catalogue := map[string]bool{}
	for _, l := range m.Catalogue {
		catalogue[l.Name] = true
	}
	ids := map[string]bool{}
	for _, set := range []MatchSet{m.DE, m.EN} {
		for _, l := range set.Lines {
			require.False(t, ids[l.ID], "duplicate id %s", l.ID)
			ids[l.ID] = true
			assert.Contains(t, LineKinds, l.Kind, l.ID)
			assert.Contains(t, []string{"de", "en"}, l.Lang, l.ID)
			assert.NotEmpty(t, l.Raw, l.ID)
			assert.NotEmpty(t, l.Name, l.ID)
			assert.NotEmpty(t, l.Source, l.ID)
			if len(l.Expect) == 0 {
				assert.NotEmpty(t, l.Note, "%s: a line with no expected food explains why", l.ID)
			}
			if l.Kind == "compound" {
				assert.GreaterOrEqual(t, len(l.Expect), 2, "%s: a compound line splits", l.ID)
				assert.False(t, l.AnyOf, l.ID)
			}
			if l.AnyOf {
				assert.GreaterOrEqual(t, len(l.Expect), 2, "%s: any_of needs alternatives", l.ID)
			}
			for _, e := range l.Expect {
				assert.NotEmpty(t, e.Identity, l.ID)
				sources := 0
				if e.Catalogue != "" {
					sources++
					assert.True(t, catalogue[e.Catalogue], "%s: %q is not a dev catalogue leaf", l.ID, e.Catalogue)
				}
				if e.FDCID != 0 {
					sources++
					assert.NotEmpty(t, e.FDCQuery, l.ID)
				}
				if e.OFFQuery != "" {
					sources++
					assert.NotEmpty(t, e.OFFMatch, l.ID)
					assert.Contains(t, []string{"de", "en"}, e.OFFLang, l.ID)
				}
				assert.Equal(t, 1, sources, "%s: %q names exactly one source", l.ID, e.Identity)
				if e.Grams != nil {
					assert.Positive(t, *e.Grams, l.ID)
				}
			}
		}
	}
	// The hard kinds the brief asks for are present in the English set.
	for _, k := range []string{"alternatives", "compound", "to_taste", "container_range", "branded", "vague_count"} {
		assert.Positive(t, m.EN.kinds()[k], "EN set has kind %s", k)
	}
}

// TestGermanSetIsTheRepoTestPages: the German set's raw lines are exactly what
// the JSON-LD parser extracts from the test pages the set names as its source,
// so the 72 cannot drift from the pages.
func TestGermanSetIsTheRepoTestPages(t *testing.T) {
	m := loadMatching(t)
	want := map[string][]string{}
	var order []string
	for _, l := range m.DE.Lines {
		if _, ok := want[l.Source]; !ok {
			order = append(order, l.Source)
		}
		want[l.Source] = append(want[l.Source], l.Raw)
	}
	for _, src := range order {
		path := filepath.Join("..", "..", "..", strings.TrimPrefix(src, "repo:"))
		page, err := os.ReadFile(path)
		require.NoError(t, err, src)
		rec, err := jsonld.ExtractRecipe(string(page))
		require.NoError(t, err, src)
		assert.Equal(t, rec.RecipeIngredient, want[src], src)
	}
}

// TestRecordedProviderResponses: every expected FDC identity is in its recorded
// response, every expected OFF product fragment is in its response, and both
// load through the real clients.
func TestRecordedProviderResponses(t *testing.T) {
	m := loadMatching(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var rec Recorded
		var ok bool
		if strings.Contains(r.URL.Path, "/fdc/") {
			rec, ok = m.FDCResponse(r.URL.Query().Get("query"))
		} else {
			rec, ok = m.OFFResponse(r.URL.Query().Get("lc"), r.URL.Query().Get("search_terms"))
		}
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(rec.Response)
	}))
	defer srv.Close()
	fdcClient := fdc.New("test", fdc.WithBaseURL(srv.URL))
	offClient := off.New(off.WithBaseURL(srv.URL))
	ctx := context.Background()

	seenFDC, seenOFF := map[string]bool{}, map[string]bool{}
	for _, set := range []MatchSet{m.DE, m.EN} {
		for _, l := range set.Lines {
			for _, e := range l.Expect {
				switch {
				case e.FDCID != 0:
					seenFDC[e.FDCQuery] = true
					got, err := fdcClient.SearchByName(ctx, e.FDCQuery, []string{"Foundation", "SR Legacy"}, 5)
					require.NoError(t, err, l.ID)
					i := slices.IndexFunc(got, func(c fdc.Candidate) bool { return c.FdcID == e.FDCID })
					require.GreaterOrEqual(t, i, 0, "%s: fdc %d (%s) not in response %q", l.ID, e.FDCID, e.Identity, e.FDCQuery)
					assert.NotNil(t, got[i].Kcal100g, "%s: no energy in %d", l.ID, e.FDCID)
				case e.OFFQuery != "":
					seenOFF[e.OFFLang+":"+e.OFFQuery] = true
					got, err := offClient.SearchByName(ctx, e.OFFQuery, e.OFFLang, 5)
					require.NoError(t, err, l.ID)
					assert.True(t, slices.ContainsFunc(got, func(c off.Candidate) bool {
						return strings.Contains(strings.ToLower(c.Name), e.OFFMatch)
					}), "%s: no OFF product with %q in response %q", l.ID, e.OFFMatch, e.OFFQuery)
				}
			}
		}
	}
	for _, q := range m.FDCQueries() {
		rec, _ := m.FDCResponse(q)
		assert.Contains(t, []string{OriginLive, OriginBulk}, rec.Origin, q)
	}
	for _, k := range m.OFFKeys() {
		rec, _ := m.OFFResponse(offKey(k))
		assert.Equal(t, OriginLive, rec.Origin, k)
	}
	live := 0
	for _, q := range m.FDCQueries() {
		if rec, _ := m.FDCResponse(q); rec.Origin == OriginLive {
			live++
		}
	}
	assert.Positive(t, live, "some FDC responses are live-recorded")
	assert.NotEmpty(t, seenFDC)
	assert.NotEmpty(t, seenOFF)
}

// lineMatched reports whether a resolver result for the line is right: it
// found the expected catalogue leaf (any one for an any_of line, all for a
// compound line). A line that expects no food, or a food outside the
// catalogue, is not matchable from the catalogue.
func lineMatched(l MatchLine, hit func(name string) bool) bool {
	if len(l.Expect) == 0 {
		return false
	}
	hits := 0
	for _, e := range l.Expect {
		if e.Catalogue != "" && hit(e.Catalogue) {
			hits++
		}
	}
	if l.AnyOf {
		return hits > 0
	}
	return hits == len(l.Expect)
}

type baselineRate struct{ matched, scored, unscored int }

func (b baselineRate) percent() float64 { return 100 * float64(b.matched) / float64(b.scored) }

// baseline runs today's import path without keys: the line parser, then the
// resolver's local catalogue step, over the leaves selected by keep. Lines
// that expect no food are left out of the denominator.
func baseline(t *testing.T, m *Matching, set MatchSet, keep func(CatalogueLeaf) bool) baselineRate {
	t.Helper()
	ctx := context.Background()
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewFoodRepo(db)
	src := food.SourceManual
	for _, leaf := range m.Catalogue {
		if !keep(leaf) {
			continue
		}
		f := &food.Food{
			Name: leaf.Name, Kind: food.KindLeaf, Source: &src,
			Kcal100g: leaf.Kcal100g, Protein100g: leaf.Protein, Fat100g: leaf.Fat, Carbs100g: leaf.Carbs,
		}
		require.NoError(t, repo.Create(ctx, f))
	}
	match, err := food.NewResolver(repo, nil, nil, nil).LocalMatcher(ctx)
	require.NoError(t, err)

	var b baselineRate
	for _, l := range set.Lines {
		if len(l.Expect) == 0 {
			b.unscored++
			continue
		}
		parse := importer.ParseLineDE
		if l.Lang == "en" {
			parse = importer.ParseLineEN
		}
		got := ""
		if c := match(parse(l.Raw).Name); c != nil {
			got = c.Name
		}
		b.scored++
		if lineMatched(l, func(name string) bool { return got == name }) {
			b.matched++
		}
	}
	return b
}

// TestBaselineLocalMatchRates pins the no-key match rate of today's resolver
// (parse line, then local catalogue only) on both sets. B1 must beat it: the
// targets are DE 60 percent and EN 80 percent. The dev catalogue was seeded
// from some of these very recipes, so the rate is generous; the 106-leaf
// seeded backup and the empty catalogue of a fresh install bracket it.
func TestBaselineLocalMatchRates(t *testing.T) {
	m := loadMatching(t)
	all := func(CatalogueLeaf) bool { return true }
	seeded := func(l CatalogueLeaf) bool { return l.Seeded }
	none := func(CatalogueLeaf) bool { return false }

	cases := []struct {
		name string
		set  MatchSet
		keep func(CatalogueLeaf) bool
		want baselineRate
	}{
		{"DE dev catalogue", m.DE, all, baselineRate{10, 72, 0}},
		{"EN dev catalogue", m.EN, all, baselineRate{21, 86, 4}},
		{"DE seeded 106", m.DE, seeded, baselineRate{9, 72, 0}},
		{"EN seeded 106", m.EN, seeded, baselineRate{21, 86, 4}},
		{"DE empty catalogue", m.DE, none, baselineRate{0, 72, 0}},
		{"EN empty catalogue", m.EN, none, baselineRate{0, 86, 4}},
	}
	for _, c := range cases {
		got := baseline(t, m, c.set, c.keep)
		t.Logf("%-20s %d of %d lines (%.1f %%), %d lines expect no food", c.name, got.matched, got.scored, got.percent(), got.unscored)
		assert.Equal(t, c.want, got, c.name)
	}
}

func offKey(k string) (lang, query string) {
	lang, query, _ = strings.Cut(k, ":")
	return lang, query
}
