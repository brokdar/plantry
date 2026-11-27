package food_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/fake"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

// stubFDC returns fixed candidates for every query.
type stubFDC struct{ cs []food.Candidate }

func (s stubFDC) SearchByName(context.Context, string, int) ([]food.Candidate, error) {
	return append([]food.Candidate(nil), s.cs...), nil
}

func kcal(v float64) *float64 { return &v }

func addLeaf(t *testing.T, repo *sqlite.FoodRepo, name string, kcal100 float64) int64 {
	t.Helper()
	src := food.SourceManual
	f := &food.Food{Name: name, Kind: food.KindLeaf, Source: &src, Kcal100g: kcal(kcal100), Protein100g: kcal(1), Fat100g: kcal(1), Carbs100g: kcal(1)}
	require.NoError(t, repo.Create(context.Background(), f))
	return f.ID
}

// TestMatch_Confidence table-tests MATCH-2: same food in the same state,
// plausible kcal for its group, an amount in grams.
func TestMatch_Confidence(t *testing.T) {
	ctx := context.Background()
	repo := sqlite.NewFoodRepo(testhelper.NewTestDB(t))
	addLeaf(t, repo, "Rice (cooked)", 130)
	addLeaf(t, repo, "Basmati rice (dry)", 350)
	addLeaf(t, repo, "Wheat flour", 364)
	addLeaf(t, repo, "Onion", 40)
	lentils116 := stubFDC{cs: []food.Candidate{{Name: "Lentils, pink or red, raw", Source: food.SourceFDC, FdcID: 1, Kcal100g: kcal(116)}}}

	cases := []struct {
		name       string
		fdc        food.FoodProvider
		line       food.MatchLine
		lang       string
		status     string
		reason     string
		pick       string
		gramsAbout float64
	}{
		{"dry rice is not cooked rice", nil, food.MatchLine{Name: "Basmatireis", Amount: 300, Unit: "g"}, "de", food.MatchMatched, "", "Basmati rice (dry)", 300},
		{"cooked rice only matches cooked", nil, food.MatchLine{Name: "gekochter Reis", Amount: 200, Unit: "g"}, "de", food.MatchMatched, "", "Rice (cooked)", 200},
		{"116-kcal dry lentils block", lentils116, food.MatchLine{Name: "rote Linsen", Amount: 200, Unit: "g"}, "de", food.MatchUnmatched, food.ReasonKcal, "", 200},
		{"a cup of flour is about 125 g", nil, food.MatchLine{Name: "flour", Amount: 240, Unit: "ml"}, "en", food.MatchMatched, "", "Wheat flour", 127},
		{"across languages", nil, food.MatchLine{Name: "Zwiebel", Amount: 1, Unit: "piece"}, "de", food.MatchMatched, "", "Onion", 110},
		{"a count without a weight blocks", nil, food.MatchLine{Name: "Fantasiefrucht", Amount: 1, Unit: "piece"}, "de", food.MatchUnmatched, food.ReasonNoCandidate, "", 0},
		{"to taste needs no amount", nil, food.MatchLine{Name: "Zwiebel", ToTaste: true}, "de", food.MatchMatched, "", "Onion", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := food.NewMatcher(repo, c.fdc, nil)
			res, err := m.Match(ctx, food.MatchRequest{Lang: c.lang, Lines: []food.MatchLine{c.line}})
			require.NoError(t, err)
			got := res.Lines[0]
			assert.Equal(t, c.status, got.Status, "tried %v", got.Tried)
			assert.Equal(t, c.reason, got.Reason)
			if c.pick != "" {
				require.GreaterOrEqual(t, got.Pick, 0)
				assert.Equal(t, c.pick, got.Candidates[got.Pick].Name)
			}
			if c.gramsAbout > 0 {
				require.NotNil(t, got.Grams)
				assert.InDelta(t, c.gramsAbout, *got.Grams, c.gramsAbout*0.05)
			}
		})
	}
}

// TestMatch_DryRiceBlocksCookedLeaf: "Reis" in a recipe is dry rice; the
// catalogue's only rice is cooked (3x fewer kcal), so the line is unmatched.
func TestMatch_DryRiceBlocksCookedLeaf(t *testing.T) {
	repo := sqlite.NewFoodRepo(testhelper.NewTestDB(t))
	addLeaf(t, repo, "Rice (cooked)", 130)
	res, err := food.NewMatcher(repo, nil, nil).Match(context.Background(), food.MatchRequest{
		Lang:  "de",
		Lines: []food.MatchLine{{Name: "Reis", Amount: 200, Unit: "g"}},
	})
	require.NoError(t, err)
	assert.Equal(t, food.MatchUnmatched, res.Lines[0].Status)
	assert.Equal(t, food.ReasonState, res.Lines[0].Reason)
	assert.Equal(t, []string{"catalogue"}, res.Lines[0].Tried)
}

// TestEnsureLeaf_DuplicateCheckAndCleanup: a candidate the catalogue
// already holds (by synonym, across languages) is reused; a new one is
// created with its origin, and removed on reject only while unused.
func TestEnsureLeaf_DuplicateCheckAndCleanup(t *testing.T) {
	ctx := context.Background()
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewFoodRepo(db)
	origins := sqlite.NewFoodOriginRepo(db)
	zwiebel := addLeaf(t, repo, "Zwiebel", 40)
	o := food.Origin{Kind: food.OriginImport, Ref: "https://example.org/curry"}

	id, created, err := food.EnsureLeaf(ctx, repo, origins, food.Candidate{Name: "Onions, raw", Source: food.SourceFDC, FdcID: 170000, Kcal100g: kcal(40)}, o)
	require.NoError(t, err)
	assert.False(t, created, "Onions, raw is the catalogue's Zwiebel")
	assert.Equal(t, zwiebel, id)

	saffron, created, err := food.EnsureLeaf(ctx, repo, origins, food.Candidate{Name: "Spices, saffron", Source: food.SourceFDC, FdcID: 170934, Kcal100g: kcal(310)}, o)
	require.NoError(t, err)
	require.True(t, created)
	again, created, err := food.EnsureLeaf(ctx, repo, origins, food.Candidate{Name: "Spices, saffron", Source: food.SourceFDC, FdcID: 170934, Kcal100g: kcal(310)}, o)
	require.NoError(t, err)
	assert.False(t, created, "same FDC id")
	assert.Equal(t, saffron, again)
	cumin, _, err := food.EnsureLeaf(ctx, repo, origins, food.Candidate{Name: "Spices, cumin seed", Source: food.SourceFDC, FdcID: 170923, Kcal100g: kcal(375)}, o)
	require.NoError(t, err)

	// Cumin is used by a saved recipe; saffron is not.
	portions := 2.0
	recipe := &food.Food{
		Name: "Curry", Kind: food.KindComposed, Role: ptrRole(food.RoleMain), ReferencePortions: &portions,
		Children: []food.FoodComponent{{ChildID: cumin, Amount: 2, Unit: "g", Grams: 2}},
	}
	require.NoError(t, repo.Create(ctx, recipe))

	removed, err := food.RemoveOrphanLeaves(ctx, repo, origins, o)
	require.NoError(t, err)
	assert.Equal(t, 1, removed)
	_, err = repo.Get(ctx, saffron)
	require.Error(t, err, "unused leaf removed")
	_, err = repo.Get(ctx, cumin)
	require.NoError(t, err, "referenced leaf stays")
	_, err = repo.Get(ctx, zwiebel)
	require.NoError(t, err, "a leaf the import did not create stays")
}

// TestMatch_OneChefMatchRunPerRecipe: with a provider the recipe's lines go
// to one chef_match call that picks among the fetched candidates; a pick
// that fails the confidence checks does not stand.
func TestMatch_OneChefMatchRunPerRecipe(t *testing.T) {
	ctx := context.Background()
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewFoodRepo(db)
	addLeaf(t, repo, "Light coconut milk", 74)
	fdc := stubFDC{cs: []food.Candidate{
		{Name: "Nuts, coconut milk, canned", Source: food.SourceFDC, FdcID: 170173, Kcal100g: kcal(197)},
		{Name: "Beverages, coconut milk, sweetened", Source: food.SourceFDC, FdcID: 174116, Kcal100g: kcal(31)},
	}}
	client := fake.NewFromScript(&fake.Script{Turns: []fake.Turn{{
		ForTool: food.MatchTool, StopReason: "tool_use",
		Message: fake.ScriptedMessage{Content: []fake.ScriptedBlock{{
			Type: "tool_use", ToolUseID: "toolu_1", ToolName: food.MatchTool,
			ToolInput: json.RawMessage(`{"lines":[{"key":"l1","pick":0},{"key":"l2","pick":1}]}`),
		}}},
		Usage: fake.ScriptedUsage{InputTokens: 900, OutputTokens: 40},
	}}})
	runs := sqlite.NewAIRunRepo(db)
	rec := airun.NewRecorder(runs, slog.New(slog.NewTextHandler(io.Discard, nil)), time.Now)
	guarded := 0
	m := food.NewMatcher(repo, fdc, nil).WithPicker(llm.StaticResolver(rec.Meter(client, "fake"), "fake-model"), rec,
		func(_ context.Context, requested bool) (food.MatchPermit, error) {
			guarded++
			assert.True(t, requested)
			return food.MatchPermit{Run: true, Warn: true}, nil
		})

	res, err := m.Match(ctx, food.MatchRequest{Lang: "de", Requested: true, Lines: []food.MatchLine{
		{Name: "Kokosmilch", Amount: 400, Unit: "ml"},
		{Name: "Kokosmilch", Amount: 200, Unit: "ml"},
	}})
	require.NoError(t, err)
	rec.Wait()
	assert.Equal(t, 1, guarded)
	assert.True(t, res.CapWarning, "requested work at the cap runs with a warning")
	got := res.Lines[0]
	require.Equal(t, food.MatchMatched, got.Status)
	assert.Equal(t, 170173, got.Candidates[got.Pick].FdcID, "the picked candidate")
	// Line 2 picks the sweetened drink (31 kcal), implausible for the
	// group: the ladder's confident match stands.
	assert.Equal(t, 170173, res.Lines[1].Candidates[res.Lines[1].Pick].FdcID)

	rows, err := runs.ListRuns(ctx, airun.RunQuery{Kind: airun.KindChefMatch, Limit: 10})
	require.NoError(t, err)
	require.Len(t, rows, 1, "one chef_match run per recipe")
	assert.Equal(t, airun.StatusOK, rows[0].Status)
	assert.Equal(t, food.MatchVersion, rows[0].PromptVersion)
	assert.Equal(t, int64(900), rows[0].Tokens.Input, "the run carries its metered usage (its cost)")
}

// queryFDC answers only the queries it knows and records every query.
type queryFDC struct {
	known   map[string][]food.Candidate
	queries *[]string
}

func (s queryFDC) SearchByName(_ context.Context, q string, _ int) ([]food.Candidate, error) {
	*s.queries = append(*s.queries, q)
	return append([]food.Candidate(nil), s.known[q]...), nil
}

func translateTurn(input string) fake.Turn {
	return fake.Turn{
		ForTool: food.TranslateTool, StopReason: "tool_use",
		Message: fake.ScriptedMessage{Content: []fake.ScriptedBlock{{
			Type: "tool_use", ToolUseID: "toolu_t", ToolName: food.TranslateTool, ToolInput: json.RawMessage(input),
		}}},
		Usage: fake.ScriptedUsage{InputTokens: 200, OutputTokens: 20},
	}
}

// TestMatch_TranslatesLinesOutsideTheTable: a German line the synonym table
// does not know gets its English FDC term from one batched call per recipe
// (inside the chef_match run with the Chef on, as the uncapped lookup run
// with the Chef off); the candidates still come from FDC.
func TestMatch_TranslatesLinesOutsideTheTable(t *testing.T) {
	for _, tc := range []struct {
		name   string
		permit food.MatchPermit
		kind   airun.Kind
	}{
		{"chef on", food.MatchPermit{Run: true}, airun.KindChefMatch},
		{"chef off, provider set", food.MatchPermit{Uncapped: true}, airun.KindLookup},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			db := testhelper.NewTestDB(t)
			repo := sqlite.NewFoodRepo(db)
			var queries []string
			fdc := queryFDC{queries: &queries, known: map[string][]food.Candidate{
				"sumac":               {{Name: "Spices, sumac", Source: food.SourceFDC, FdcID: 1, Kcal100g: kcal(239)}},
				"jerusalem artichoke": {{Name: "Kale, raw", Source: food.SourceFDC, FdcID: 2, Kcal100g: kcal(35)}},
			}}
			client := fake.NewFromScript(&fake.Script{Turns: []fake.Turn{
				translateTurn(`{"terms":[{"key":"l1","term":"sumac"},{"key":"l2","term":"jerusalem artichoke"}]}`),
				{
					ForTool: food.MatchTool, StopReason: "tool_use",
					Message: fake.ScriptedMessage{Content: []fake.ScriptedBlock{{
						Type: "tool_use", ToolUseID: "toolu_1", ToolName: food.MatchTool,
						ToolInput: json.RawMessage(`{"lines":[{"key":"l1","pick":0},{"key":"l2","pick":0}]}`),
					}}},
				},
			}})
			rec := airun.NewRecorder(sqlite.NewAIRunRepo(db), slog.New(slog.NewTextHandler(io.Discard, nil)), time.Now)
			m := food.NewMatcher(repo, fdc, nil).WithPicker(llm.StaticResolver(rec.Meter(client, "fake"), "fake-model"), rec,
				func(context.Context, bool) (food.MatchPermit, error) { return tc.permit, nil })
			lines := []food.MatchLine{{Name: "Sumach", Amount: 5, Unit: "g"}, {Name: "Topinambur", Amount: 200, Unit: "g"}, {Name: "Salz", ToTaste: true}}

			res, err := m.Match(ctx, food.MatchRequest{Lang: "de", Requested: true, Lines: lines})
			require.NoError(t, err)
			rec.Wait()
			assert.Contains(t, queries, "sumac")
			assert.Contains(t, queries, "jerusalem artichoke")
			for i, want := range []int{1, 2} {
				got := res.Lines[i]
				require.Equal(t, food.MatchMatched, got.Status, got.Name)
				assert.Equal(t, want, got.Candidates[got.Pick].FdcID)
			}
			rows, err := sqlite.NewAIRunRepo(db).ListRuns(ctx, airun.RunQuery{Kind: tc.kind, Limit: 10})
			require.NoError(t, err)
			require.Len(t, rows, 1, "one AI run per recipe, translation inside it")
			assert.Equal(t, int64(200), rows[0].Tokens.Input)
		})
	}
}

// TestEnsureLeaf_ImplausibleLeafIsNotADuplicate: a catalogue leaf of the
// same name whose kcal is far outside the food's band does not swallow the
// FDC candidate.
func TestEnsureLeaf_ImplausibleLeafIsNotADuplicate(t *testing.T) {
	ctx := context.Background()
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewFoodRepo(db)
	bad := addLeaf(t, repo, "Zwiebel", 900)
	o := food.Origin{Kind: food.OriginImport, Ref: "https://example.org/x"}
	id, created, err := food.EnsureLeaf(ctx, repo, sqlite.NewFoodOriginRepo(db), food.Candidate{Name: "Onions, raw", Source: food.SourceFDC, FdcID: 170000, Kcal100g: kcal(40)}, o)
	require.NoError(t, err)
	assert.True(t, created)
	assert.NotEqual(t, bad, id)
}
