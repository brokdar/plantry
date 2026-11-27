package chefsets

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/fake"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/fdc"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/off"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/importer"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

// recordedServer answers FDC and OFF searches from the DATA-2 recordings;
// an unrecorded query has no results.
func recordedServer(t *testing.T, m *Matching) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/fdc/") {
			if rec, ok := m.FDCResponse(r.URL.Query().Get("query")); ok {
				_, _ = w.Write(rec.Response)
				return
			}
			_, _ = w.Write([]byte(`{"foods":[]}`))
			return
		}
		if rec, ok := m.OFFResponse(r.URL.Query().Get("lc"), r.URL.Query().Get("search_terms")); ok {
			_, _ = w.Write(rec.Response)
			return
		}
		_, _ = w.Write([]byte(`{"products":[]}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// matchRig is the matcher over a catalogue and the recordings.
type matchRig struct {
	db      *sql.DB
	matcher *food.Matcher
}

func newMatchRig(t *testing.T, m *Matching, keep func(CatalogueLeaf) bool) *matchRig {
	t.Helper()
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewFoodRepo(db)
	src := food.SourceManual
	for _, leaf := range m.Catalogue {
		if keep(leaf) {
			require.NoError(t, repo.Create(context.Background(), &food.Food{
				Name: leaf.Name, Kind: food.KindLeaf, Source: &src,
				Kcal100g: leaf.Kcal100g, Protein100g: leaf.Protein, Fat100g: leaf.Fat, Carbs100g: leaf.Carbs,
			}))
		}
	}
	srv := recordedServer(t, m)
	return &matchRig{db: db, matcher: food.NewMatcher(repo,
		fdc.NewProvider(fdc.New("test", fdc.WithBaseURL(srv.URL))),
		off.NewProvider(off.New(off.WithBaseURL(srv.URL))))}
}

// recipeLines is one recipe: its lines and their cleaned draft rows.
type recipeLines struct {
	lang  string
	lines []MatchLine
	rows  [][]importer.DraftIngredient
}

func recipesOf(set MatchSet) []recipeLines {
	var out []recipeLines
	for _, l := range set.Lines {
		if len(out) == 0 || out[len(out)-1].lines[0].Recipe != l.Recipe {
			out = append(out, recipeLines{lang: l.Lang})
		}
		r := &out[len(out)-1]
		r.lines = append(r.lines, l)
		r.rows = append(r.rows, importer.ParseLine(l.Raw, l.Lang))
	}
	return out
}

func (r recipeLines) request() food.MatchRequest {
	req := food.MatchRequest{Lang: r.lang, Requested: true}
	for _, rows := range r.rows {
		for _, row := range rows {
			req.Lines = append(req.Lines, importer.MatchLineOf(row))
		}
	}
	return req
}

// isExpected reports whether a candidate is the expected food.
func isExpected(c food.Candidate, e Expect) bool {
	switch {
	case e.Catalogue != "":
		return c.Source == food.SourceLocal && c.Name == e.Catalogue
	case e.FDCID != 0:
		return c.FdcID == e.FDCID
	case e.OFFMatch != "":
		return c.Source == food.SourceOFF && strings.Contains(strings.ToLower(c.Name), e.OFFMatch)
	}
	return false
}

// lineOK scores one line over its rows' results: a compound line needs all
// its foods, an any_of line one of them, a plain line its food. ok picks
// whether a row result has the expected food.
func lineOK(l MatchLine, res []food.LineMatch, ok func(food.LineMatch, Expect) bool) bool {
	hit := func(e Expect) bool {
		for _, r := range res {
			if ok(r, e) {
				return true
			}
		}
		return false
	}
	n := 0
	for _, e := range l.Expect {
		if hit(e) {
			n++
		}
	}
	if l.AnyOf {
		return n > 0
	}
	return n == len(l.Expect)
}

func matchedTo(r food.LineMatch, e Expect) bool {
	return r.Status != food.MatchUnmatched && r.Pick >= 0 && isExpected(r.Candidates[r.Pick], e)
}

func candidateOf(r food.LineMatch, e Expect) bool {
	for _, c := range r.Candidates {
		if isExpected(c, e) {
			return true
		}
	}
	return false
}

type rate struct {
	ok, scored, gramsOK, gramsScored int
	failed                           map[string][]string
}

func (r rate) percent() float64 { return 100 * float64(r.ok) / float64(r.scored) }

// score runs fn per recipe and scores every line that expects a food.
func score(recipes []recipeLines, results [][]food.LineMatch, ok func(food.LineMatch, Expect) bool) rate {
	rt := rate{failed: map[string][]string{}}
	for ri, rec := range recipes {
		at := 0
		for li, l := range rec.lines {
			res := results[ri][at : at+len(rec.rows[li])]
			at += len(rec.rows[li])
			if len(l.Expect) == 0 {
				continue
			}
			rt.scored++
			if lineOK(l, res, ok) {
				rt.ok++
				if g := l.Expect[0].Grams; g != nil && res[0].Grams != nil {
					rt.gramsScored++
					if math.Abs(*res[0].Grams-*g) <= 0.35**g {
						rt.gramsOK++
					}
				}
				continue
			}
			why := res[0].Reason
			if res[0].Status != food.MatchUnmatched {
				why = "other food"
			}
			rt.failed[l.Kind] = append(rt.failed[l.Kind], fmt.Sprintf("%s %q (%s)", l.ID, l.Name, why))
		}
	}
	return rt
}

func (rig *matchRig) gatherAll(t *testing.T, recipes []recipeLines) [][]food.LineMatch {
	t.Helper()
	out := make([][]food.LineMatch, len(recipes))
	for i, rec := range recipes {
		got, err := rig.matcher.Gather(context.Background(), rec.request())
		require.NoError(t, err)
		out[i] = got
	}
	return out
}

func (rig *matchRig) decideAll(t *testing.T, recipes []recipeLines, gathered [][]food.LineMatch) [][]food.LineMatch {
	t.Helper()
	out := make([][]food.LineMatch, len(recipes))
	for i, rec := range recipes {
		res, err := rig.matcher.Decide(context.Background(), rec.request(), gathered[i])
		require.NoError(t, err)
		out[i] = res.Lines
	}
	return out
}

func logRate(t *testing.T, name string, r rate) {
	t.Logf("%-28s %d of %d lines (%.1f %%), grams within 35 %% on %d of %d matched lines", name, r.ok, r.scored, r.percent(), r.gramsOK, r.gramsScored)
	for kind, lines := range r.failed {
		t.Logf("    unmatched %s: %s", kind, strings.Join(lines, "; "))
	}
}

// TestMatchRates_NoKey measures the no-key rate of the matching path on
// DATA-2 (MATCH-7): the importer's line cleaning, the synonym table, the
// catalogue, then FDC and OFF from the recordings. A line counts when its
// expected food is the confident (or closest) pick. Targets: DE 60 %, EN
// 80 % with the dev catalogue; the empty catalogue of a fresh install is
// measured and reported (the recordings cover only DATA-2's FDC/OFF foods).
func TestMatchRates_NoKey(t *testing.T) {
	m := loadMatching(t)
	all := func(CatalogueLeaf) bool { return true }
	none := func(CatalogueLeaf) bool { return false }
	cases := []struct {
		name   string
		set    MatchSet
		keep   func(CatalogueLeaf) bool
		target float64
	}{
		{"DE dev catalogue", m.DE, all, 60},
		{"EN dev catalogue", m.EN, all, 80},
		{"DE empty catalogue", m.DE, none, 0},
		{"EN empty catalogue", m.EN, none, 0},
	}
	for _, c := range cases {
		rig := newMatchRig(t, m, c.keep)
		recipes := recipesOf(c.set)
		got := score(recipes, rig.decideAll(t, recipes, rig.gatherAll(t, recipes)), matchedTo)
		logRate(t, c.name, got)
		assert.GreaterOrEqual(t, got.percent(), c.target, c.name)
	}
}

// TestMatchRates_ScriptedKey: with a provider the expected food is among
// the candidates of at least 95 % of lines (candidate recall), and a fake
// provider that picks it whenever present makes one chef_match run per
// recipe and matches those lines.
func TestMatchRates_ScriptedKey(t *testing.T) {
	m := loadMatching(t)
	for _, set := range []MatchSet{m.DE, m.EN} {
		rig := newMatchRig(t, m, func(CatalogueLeaf) bool { return true })
		recipes := recipesOf(set)
		gathered := rig.gatherAll(t, recipes)
		recall := score(recipes, gathered, candidateOf)
		logRate(t, set.Set+" candidate recall", recall)
		assert.GreaterOrEqual(t, recall.percent(), 95.0, set.Set)

		var turns []fake.Turn
		for ri, rec := range recipes {
			var picks []map[string]any
			at := 0
			for li, l := range rec.lines {
				for k := range rec.rows[li] {
					pick := -1
					for ci, c := range gathered[ri][at+k].Candidates {
						if pick < 0 && slicesAny(l.Expect, func(e Expect) bool { return isExpected(c, e) }) {
							pick = ci
						}
					}
					picks = append(picks, map[string]any{"key": fmt.Sprintf("l%d", at+k+1), "pick": pick})
				}
				at += len(rec.rows[li])
			}
			input, err := json.Marshal(map[string]any{"lines": picks})
			require.NoError(t, err)
			turns = append(turns, fake.Turn{
				ForTool: food.MatchTool, StopReason: "tool_use",
				Message: fake.ScriptedMessage{Content: []fake.ScriptedBlock{{Type: "tool_use", ToolUseID: fmt.Sprintf("toolu_%d", ri), ToolName: food.MatchTool, ToolInput: input}}},
				Usage:   fake.ScriptedUsage{InputTokens: 1000, OutputTokens: 50},
			})
		}
		runs := sqlite.NewAIRunRepo(rig.db)
		rec := airun.NewRecorder(runs, slog.New(slog.NewTextHandler(io.Discard, nil)), time.Now)
		rig.matcher.WithPicker(llm.StaticResolver(rec.Meter(fake.NewFromScript(&fake.Script{Turns: turns}), "fake"), "fake-model"), rec,
			func(context.Context, bool) (bool, bool, error) { return true, false, nil })
		keyed := score(recipes, rig.decideAll(t, recipes, gathered), matchedTo)
		logRate(t, set.Set+" scripted key", keyed)
		rec.Wait()
		rows, err := runs.ListRuns(context.Background(), airun.RunQuery{Kind: airun.KindChefMatch, Limit: 100})
		require.NoError(t, err)
		assert.Len(t, rows, len(recipes), "one chef_match run per recipe")
		assert.GreaterOrEqual(t, keyed.ok, recall.ok-len(recipes), "the provider's picks hold where the food was fetched")
	}
}

func slicesAny[T any](s []T, f func(T) bool) bool {
	for _, v := range s {
		if f(v) {
			return true
		}
	}
	return false
}
