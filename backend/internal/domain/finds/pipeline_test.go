package finds_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/fake"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/jsonld"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/finds"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/importer"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
	"github.com/jaltszeimer/plantry/backend/internal/domain/settings"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

// DATA-3 recorded pages (C1), served under URLs of enabled sites.
var pages = map[string]string{
	"https://www.bbcgoodfood.com/recipes/chicken-curry":   "en_chicken_curry_1.html",
	"https://www.budgetbytes.com/chicken-curry/":          "en_chicken_curry_2.html",
	"https://www.thewoksoflife.com/chicken-curry/":        "en_chicken_curry_3.html",
	"https://www.chefkoch.de/rezepte/1/linsensuppe.html":  "de_linsensuppe_1.html",
	"https://www.eatsmarter.de/rezepte/linsensuppe":       "de_linsensuppe_2.html",
	"https://www.kuechengoetter.de/rezepte/linsensuppe-1": "de_linsensuppe_3.html",
}

type pageFetcher struct{ t *testing.T }

func (f pageFetcher) Fetch(_ context.Context, u string) (string, string, error) {
	name, ok := pages[u]
	if !ok {
		return "", "", errors.New("not recorded")
	}
	b, err := os.ReadFile(filepath.Join("..", "..", "adapters", "jsonld", "testdata", "finds", name))
	require.NoError(f.t, err)
	return string(b), u, nil
}

// lineMatcher matches every line to a catalogue food except the names in
// unmatched.
type lineMatcher struct{ unmatched []string }

func (m lineMatcher) Match(_ context.Context, req food.MatchRequest) (*food.MatchResult, error) {
	res := &food.MatchResult{}
	for i, l := range req.Lines {
		id := int64(100 + i)
		lm := food.LineMatch{Status: food.MatchMatched, Name: l.Name, Pick: 0, Candidates: []food.Candidate{{Name: l.Name, ExistingID: &id}}}
		for _, u := range m.unmatched {
			if strings.Contains(strings.ToLower(l.Name), u) {
				lm = food.LineMatch{Status: "unmatched", Name: l.Name, Pick: -1}
			}
		}
		res.Lines = append(res.Lines, lm)
	}
	return res, nil
}

func toolTurn(tool, input string) fake.Turn {
	return fake.Turn{
		ForTool: tool, StopReason: "tool_use",
		Message: fake.ScriptedMessage{Content: []fake.ScriptedBlock{{
			Type: "tool_use", ToolUseID: "toolu_" + tool, ToolName: tool, ToolInput: json.RawMessage(input),
		}}},
		Usage: fake.ScriptedUsage{InputTokens: 800, OutputTokens: 300},
	}
}

func searchTurn(urls ...string) fake.Turn {
	b, _ := json.Marshal(map[string][]string{"urls": urls})
	return toolTurn(finds.SearchTool, string(b))
}

// adaptTurn: own steps; with swap, line 0 becomes tofu (a protein swap).
func adaptTurn(swap bool, req string) fake.Turn {
	swaps := `[]`
	if swap {
		swaps = `[{"line":0,"to":"600 g firm tofu","reason":"vegetarian household","protein":true}]`
	}
	return toolTurn(finds.AdaptTool, `{"name":"Mild curry","steps":["Fry the onion until soft, about 5 minutes.","Add the paste and the rest, simmer 20 minutes."],`+
		`"swaps":`+swaps+`,"requirements":`+req+`}`)
}

const fitsKitchen = `{"equipment":["stovetop"],"active_minutes":25}`

type rig struct {
	svc    *finds.Service
	store  *sqlite.FindRepo
	client *fake.Client
	db     *sql.DB
	// house sets the Cuisines, FirstWeek and Recipes of the household the rig
	// returns.
	house finds.Household
	// robots, when set, decides which sites the search may use.
	robots func(domain string) bool
}

func newRig(t *testing.T, web bool, m importer.Matcher, kitchen profile.Kitchen, turns ...fake.Turn) *rig {
	t.Helper()
	return newRigWrapped(t, nil, web, m, kitchen, turns...)
}

// newRigWrapped is newRig with the provider client wrapped (nil: as is).
func newRigWrapped(t *testing.T, wrap func(*fake.Client) llm.Client, web bool, m importer.Matcher, kitchen profile.Kitchen, turns ...fake.Turn) *rig {
	t.Helper()
	db := testhelper.NewTestDB(t)
	rec := airun.NewRecorder(sqlite.NewAIRunRepo(db), slog.New(slog.DiscardHandler), time.Now)
	client := fake.NewFromScript(&fake.Script{Turns: turns, Capabilities: fake.ScriptedCapabilities{WebSearch: web}})
	var provider llm.Client = client
	if wrap != nil {
		provider = wrap(client)
	}
	store := sqlite.NewFindRepo(db)
	r := &rig{store: store, client: client, db: db}
	r.svc = finds.NewService(finds.Deps{
		Store: store, LLM: llm.StaticResolver(rec.Meter(provider, "fake"), "fake-model"), Runs: rec,
		Fetcher: pageFetcher{t}, Extractor: importer.NewService(nil, jsonld.Extractor{}, nil, nil, nil), Matcher: m,
		Household: func(context.Context) (finds.Household, error) {
			return finds.Household{Lang: "en", Diet: "vegetarian", Portions: 2, Kitchen: kitchen, Cuisines: r.house.Cuisines, RatherNot: r.house.RatherNot, FirstWeek: r.house.FirstWeek, Recipes: r.house.Recipes}, nil
		},
		Sites: func(context.Context) ([]settings.ChefSite, error) {
			return []settings.ChefSite{
				{Domain: "bbcgoodfood.com", Lang: "en", Enabled: true},
				{Domain: "budgetbytes.com", Lang: "en", Enabled: true},
				{Domain: "thewoksoflife.com", Lang: "en", Enabled: true},
				{Domain: "bonappetit.com", Lang: "en"},
				{Domain: "chefkoch.de", Lang: "de", Enabled: true},
			}, nil
		},
		Enqueue: func(context.Context, int64, bool) error { return nil },
		Robots:  func(_ context.Context, d string) bool { return r.robots == nil || r.robots(d) },
	})
	return r
}

var curryURLs = []string{
	"https://www.bbcgoodfood.com/recipes/chicken-curry", "https://www.budgetbytes.com/chicken-curry/",
	"https://www.thewoksoflife.com/chicken-curry/",
}

// kitchen allows 90 min at the weekend: every recorded curry fits a cell.
var kitchen = profile.Kitchen{MinutesWeekday: 30, MinutesWeekend: 90, Equipment: []string{"oven", "stovetop"}}

func (r *rig) run(t *testing.T, trigger finds.Trigger) *finds.Find {
	t.Helper()
	ctx := context.Background()
	f, err := r.svc.Request(ctx, "chicken curry", trigger)
	require.NoError(t, err)
	require.Equal(t, finds.StateSearching, f.State)
	_ = r.svc.Run(ctx, f.ID)
	got, err := r.svc.Get(ctx, f.ID)
	require.NoError(t, err)
	return got
}

// The scripted pipeline on the recorded pages reaches every FIND-4 state.
func TestPipeline_EveryState(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("ready, then in_library", func(t *testing.T) {
		t.Parallel()
		r := newRig(t, true, lineMatcher{}, kitchen, searchTurn(curryURLs...), adaptTurn(true, fitsKitchen))
		f := r.run(t, finds.TriggerPlanning)
		require.Equal(t, finds.StateReady, f.State, f.Steps)
		require.Len(t, f.Sources, 3)
		require.True(t, f.Sources[0].Base)
		require.Equal(t, "bbcgoodfood.com", f.Sources[0].Site) // test kitchen, 4.8 from 2310
		require.NotNil(t, f.Sources[0].Rating)
		require.Equal(t, 2310, f.Sources[0].Votes)
		require.Equal(t, f.Sources[0].URL, f.SourceURL)
		require.Equal(t, finds.OriginInspired, *f.Origin) // the chicken is most of the weight
		require.Equal(t, "protein_swap", f.Changes[0].Kind)
		require.Empty(t, f.Draft.ImageURL)
		require.Equal(t, "Fry the onion until soft, about 5 minutes.", f.Draft.Instructions[0])
		require.Contains(t, f.Draft.Ingredients[0].Name, "tofu")
		require.NotNil(t, f.RunID)
		require.Equal(t, "searched", f.Steps[0].Key)
		require.Equal(t, "checks_passed", f.Steps[len(f.Steps)-1].Key)

		// The search carried the allowed domains of the enabled English sites
		// and nothing about the household beyond diet and language (NFR-5).
		req := r.client.Requests()[0]
		require.Equal(t, []string{"bbcgoodfood.com", "budgetbytes.com", "thewoksoflife.com"}, req.WebSearch.AllowedDomains)
		require.NotContains(t, req.Messages[0].Content[0].Text, "stovetop")

		res, err := r.db.ExecContext(ctx, `INSERT INTO foods (name, kind, role, reference_portions) VALUES ('Mild curry', 'composed', 'main', 2)`)
		require.NoError(t, err)
		foodID, err := res.LastInsertId()
		require.NoError(t, err)
		kept, err := r.svc.Keep(ctx, f.ID, foodID)
		require.NoError(t, err)
		require.Equal(t, finds.StateInLibrary, kept.State)
		_, err = r.svc.Keep(ctx, f.ID, foodID)
		require.Error(t, err)
	})

	t.Run("requested find with an unmatched line needs a check", func(t *testing.T) {
		t.Parallel()
		r := newRig(t, true, lineMatcher{unmatched: []string{"curry paste"}}, kitchen, searchTurn(curryURLs...), adaptTurn(false, fitsKitchen))
		f := r.run(t, finds.TriggerChat)
		require.Equal(t, finds.StateNeedsCheck, f.State)
		require.Equal(t, "unmatched_ingredient", f.Gates.Blocks[0].Code)
	})

	t.Run("autonomous find with an unmatched line tries the next and fails", func(t *testing.T) {
		t.Parallel()
		r := newRig(t, true, lineMatcher{unmatched: []string{"curry paste", "curry powder", "garam"}}, kitchen,
			searchTurn(curryURLs...), adaptTurn(false, fitsKitchen), adaptTurn(false, fitsKitchen), adaptTurn(false, fitsKitchen))
		f := r.run(t, finds.TriggerPlanning)
		require.Equal(t, finds.StateFailed, f.State)
		tried := 0
		for _, s := range f.Steps {
			if s.Key == "adapted" {
				tried++
			}
		}
		require.Equal(t, 3, tried, f.Steps)
	})

	t.Run("autonomous find without web search fails at once (FIND-8)", func(t *testing.T) {
		t.Parallel()
		r := newRig(t, false, lineMatcher{}, kitchen)
		f := r.run(t, finds.TriggerPlanning)
		require.Equal(t, finds.StateFailed, f.State)
		require.Equal(t, "no_search", f.Steps[0].Key)
		require.Empty(t, r.client.Requests())
	})

	t.Run("a search reply cut off at the token limit fails as an error, not as nothing found", func(t *testing.T) {
		t.Parallel()
		cut := toolTurn(finds.SearchTool, `{}`)
		cut.StopReason = "max_tokens"
		r := newRig(t, true, lineMatcher{}, kitchen, cut)
		f := r.run(t, finds.TriggerChat)
		require.Equal(t, finds.StateFailed, f.State)
		require.Equal(t, "error", f.Steps[len(f.Steps)-1].Key, f.Steps)
	})

	t.Run("a missing tool asks once and keeps a requested find for a check", func(t *testing.T) {
		t.Parallel()
		r := newRig(t, true, lineMatcher{}, kitchen, searchTurn(curryURLs...),
			adaptTurn(false, `{"equipment":["wok"],"active_minutes":25,"advanced":true}`))
		f := r.run(t, finds.TriggerChat)
		require.Equal(t, finds.StateNeedsCheck, f.State)
		require.Equal(t, []string{"equipment:wok", "skill"}, f.Questions)
		asked, err := r.store.Asked(ctx)
		require.NoError(t, err)
		require.Equal(t, []string{"equipment:wok", "skill"}, asked)
		// The problem is a step in words, with no empty "checks_failed" line.
		var keys []string
		for _, s := range f.Steps {
			keys = append(keys, s.Key)
			require.NotEqual(t, "checks_failed", s.Key, "no gate block: %v", f.Steps)
			if s.Key == "kitchen_equipment" {
				require.Equal(t, "wok", s.Params["tool"])
			}
		}
		require.Contains(t, keys, "kitchen_equipment")
		require.Contains(t, keys, "kitchen_skill")
	})

	t.Run("a tool asked about before still shows as a step", func(t *testing.T) {
		t.Parallel()
		r := newRig(t, true, lineMatcher{}, kitchen, searchTurn(curryURLs...),
			adaptTurn(false, `{"equipment":["wok"],"active_minutes":25,"advanced":false}`))
		require.NoError(t, r.store.MarkAsked(ctx, "equipment:wok"))
		f := r.run(t, finds.TriggerChat)
		require.Equal(t, finds.StateNeedsCheck, f.State)
		require.Empty(t, f.Questions)
		last := f.Steps[len(f.Steps)-1]
		require.Equal(t, "kitchen_equipment", last.Key, f.Steps)
		require.Equal(t, "wok", last.Params["tool"])
	})

	t.Run("discarded as looks wrong lowers the source", func(t *testing.T) {
		t.Parallel()
		r := newRig(t, true, lineMatcher{}, kitchen, searchTurn(curryURLs...), adaptTurn(false, fitsKitchen))
		f := r.run(t, finds.TriggerDiscover)
		require.Equal(t, finds.StateReady, f.State)
		d, err := r.svc.Discard(ctx, f.ID, finds.ReasonLooksWrong, nil)
		require.NoError(t, err)
		require.Equal(t, finds.StateDiscarded, d.State)
		pen, err := r.store.SourcePenalties(ctx)
		require.NoError(t, err)
		require.Equal(t, map[string]int{"bbcgoodfood.com": 1}, pen)
	})
}

// A source already in the library or a find is not fetched again.
func TestPipeline_SkipsKnownSources(t *testing.T) {
	t.Parallel()
	r := newRig(t, true, lineMatcher{}, kitchen, searchTurn(curryURLs...), adaptTurn(false, fitsKitchen),
		searchTurn(curryURLs...), adaptTurn(false, fitsKitchen))
	first := r.run(t, finds.TriggerChat)
	require.Equal(t, finds.StateReady, first.State)
	second := r.run(t, finds.TriggerChat)
	require.Len(t, second.Sources, 2)
	for _, s := range second.Sources {
		require.NotEqual(t, first.SourceURL, s.URL)
	}
	// Two versions are fewer than FIND-11 wants, but the base (4.5 from 880
	// votes, not baked, a light adaptation) suffices alone: no flag.
	require.Equal(t, finds.StateReady, second.State)
	require.Empty(t, second.Flags)
}

// AC-7: whatever the scenario, a find that reaches ready has no unmatched
// line, no blocking gate and no copied photo; a find with an unmatched line
// never reaches ready, and none carries an image.
func TestPipeline_AC7(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		m       lineMatcher
		trigger finds.Trigger
		adapt   fake.Turn
	}{
		"clean":      {lineMatcher{}, finds.TriggerPlanning, adaptTurn(true, fitsKitchen)},
		"unmatched":  {lineMatcher{unmatched: []string{"curry paste"}}, finds.TriggerChat, adaptTurn(false, fitsKitchen)},
		"discover":   {lineMatcher{}, finds.TriggerDiscover, adaptTurn(false, fitsKitchen)},
		"needs tool": {lineMatcher{}, finds.TriggerChat, adaptTurn(false, `{"equipment":["wok"],"active_minutes":25}`)},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newRig(t, true, tc.m, kitchen, searchTurn(curryURLs...), tc.adapt)
			f := r.run(t, tc.trigger)
			if f.Draft != nil {
				require.Empty(t, f.Draft.ImageURL, "no copied photo")
			}
			if f.State != finds.StateReady {
				return
			}
			require.Empty(t, f.Gates.Blocks, "no GATE-1 block on a ready find")
			require.NotContains(t, name, "unmatched")
		})
	}
}

// A web search the provider pauses is resent with the paused reply until the
// search tool is called (the discovery converse loop's rule).
func TestPipeline_PausedSearchContinues(t *testing.T) {
	t.Parallel()
	pause := fake.Turn{
		ForTool: finds.SearchTool, StopReason: "pause_turn",
		Message: fake.ScriptedMessage{Content: []fake.ScriptedBlock{{Type: "text", Text: "Still searching."}}},
		Usage:   fake.ScriptedUsage{InputTokens: 400, OutputTokens: 50},
	}
	r := newRig(t, true, lineMatcher{}, kitchen, pause, searchTurn(curryURLs...), adaptTurn(false, fitsKitchen))
	f := r.run(t, finds.TriggerPlanning)
	require.Equal(t, finds.StateReady, f.State, f.Steps)
	reqs := r.client.Requests()
	require.Len(t, reqs[1].Messages, 2, "the paused reply went back")
}

// A find never waits on a job that will not run: no provider refuses the
// request, a failed queueing, a skipped job and a lost job end it failed.
func TestPipeline_NeverStrandedSearching(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	stepKey := func(f *finds.Find) string {
		require.Equal(t, finds.StateFailed, f.State)
		require.Len(t, f.Steps, 1)
		return f.Steps[0].Key
	}
	t.Run("no provider refuses the request", func(t *testing.T) {
		t.Parallel()
		r := newRig(t, true, lineMatcher{}, kitchen)
		svc := finds.NewService(finds.Deps{Store: r.store, LLM: noProvider{}, Enqueue: func(context.Context, int64, bool) error { return nil }})
		_, err := svc.Request(ctx, "pho", finds.TriggerChat)
		require.ErrorIs(t, err, llm.ErrProviderMissing)
		all, err := r.store.List(ctx, nil, 10)
		require.NoError(t, err)
		require.Empty(t, all)
	})
	t.Run("enqueue error fails the find", func(t *testing.T) {
		t.Parallel()
		r := newRig(t, true, lineMatcher{}, kitchen)
		svc := finds.NewService(finds.Deps{Store: r.store, Enqueue: func(context.Context, int64, bool) error { return errors.New("db full") }})
		_, err := svc.Request(ctx, "pho", finds.TriggerChat)
		require.Error(t, err)
		all, err := r.store.List(ctx, nil, 10)
		require.NoError(t, err)
		require.Len(t, all, 1)
		require.Equal(t, "error", stepKey(&all[0]))
	})
	t.Run("a skipped job fails the find with the guard's reason", func(t *testing.T) {
		t.Parallel()
		r := newRig(t, true, lineMatcher{}, kitchen)
		f, err := r.svc.Request(ctx, "pho", finds.TriggerChat)
		require.NoError(t, err)
		r.svc.Unfinished(ctx, strconv.FormatInt(f.ID, 10), true, "no_provider")
		got, err := r.svc.Get(ctx, f.ID)
		require.NoError(t, err)
		require.Equal(t, "no_provider", stepKey(got))
	})
	t.Run("a finished find stays when its job is reported", func(t *testing.T) {
		t.Parallel()
		r := newRig(t, true, lineMatcher{}, kitchen, searchTurn(curryURLs...), adaptTurn(false, fitsKitchen))
		f := r.run(t, finds.TriggerChat)
		require.Equal(t, finds.StateReady, f.State)
		r.svc.Unfinished(ctx, strconv.FormatInt(f.ID, 10), false, "boom")
		got, err := r.svc.Get(ctx, f.ID)
		require.NoError(t, err)
		require.Equal(t, finds.StateReady, got.State)
	})
	t.Run("a find without a job fails at boot", func(t *testing.T) {
		t.Parallel()
		r := newRig(t, true, lineMatcher{}, kitchen)
		f, err := r.svc.Request(ctx, "pho", finds.TriggerChat)
		require.NoError(t, err)
		require.NoError(t, r.svc.FailStranded(ctx))
		got, err := r.svc.Get(ctx, f.ID)
		require.NoError(t, err)
		require.Equal(t, "error", stepKey(got))
	})
	t.Run("a find with a queued job keeps searching", func(t *testing.T) {
		t.Parallel()
		r := newRig(t, true, lineMatcher{}, kitchen)
		f, err := r.svc.Request(ctx, "pho", finds.TriggerChat)
		require.NoError(t, err)
		_, err = sqlite.NewChefJobRepo(r.db).Enqueue(ctx, airun.KindChefFind, strconv.FormatInt(f.ID, 10), true, time.Now())
		require.NoError(t, err)
		require.NoError(t, r.svc.FailStranded(ctx))
		got, err := r.svc.Get(ctx, f.ID)
		require.NoError(t, err)
		require.Equal(t, finds.StateSearching, got.State)
	})
}

type noProvider struct{}

func (noProvider) Current(context.Context) (llm.Client, string, error) {
	return nil, "", llm.ErrProviderMissing
}

type failingTaste struct{}

func (failingTaste) NotForMeIngredients(context.Context, []string) error {
	return errors.New("taste down")
}

// A discard whose learning fails leaves the find undecided.
func TestDiscard_LearnFailureKeepsFindOpen(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	r := newRig(t, true, lineMatcher{}, kitchen, searchTurn(curryURLs...), adaptTurn(false, fitsKitchen))
	f := r.run(t, finds.TriggerChat)
	require.Equal(t, finds.StateReady, f.State)
	svc := finds.NewService(finds.Deps{Store: r.store, Taste: failingTaste{}, Tx: sqlite.NewTxRunner(r.db)})
	_, err := svc.Discard(ctx, f.ID, finds.ReasonIngredients, []string{"cilantro"})
	require.Error(t, err)
	got, err := r.svc.Get(ctx, f.ID)
	require.NoError(t, err)
	require.Equal(t, finds.StateReady, got.State)
}

// COLD-4: in week one a find the user did not name, and Fill my library, adapts
// for a mild, familiar dish (whatever heat the kitchen sets) and Fill searches within
// the household's cuisines; a named find and later weeks are unchanged.
func TestPipeline_WeekOneIsMildAndFamiliar(t *testing.T) {
	t.Parallel()
	run := func(t *testing.T, trigger finds.Trigger, firstWeek bool, k profile.Kitchen) (search, adapt string) {
		t.Helper()
		r := newRig(t, true, lineMatcher{}, k, searchTurn(curryURLs...), adaptTurn(false, fitsKitchen))
		r.house = finds.Household{Cuisines: []string{"german"}, FirstWeek: firstWeek}
		f := r.run(t, trigger)
		require.Equal(t, finds.StateReady, f.State, f.Steps)
		reqs := r.client.Requests()
		return reqs[0].Messages[0].Content[0].Text, reqs[len(reqs)-1].Messages[0].Content[0].Text
	}
	search, adapt := run(t, finds.TriggerFill, true, kitchen)
	assert.Contains(t, search, "german")
	assert.Contains(t, adapt, "Heat tolerance: mild")
	assert.Contains(t, adapt, "first week")

	search, adapt = run(t, finds.TriggerPlanning, true, kitchen)
	assert.NotContains(t, search, "german", "only Fill's generic dishes need a cuisine")
	assert.Contains(t, adapt, "Heat tolerance: mild")

	hot := kitchen
	hot.Heat = profile.HeatHot
	_, adapt = run(t, finds.TriggerPlanning, true, hot)
	assert.Contains(t, adapt, "Heat tolerance: mild", "week one is mild whatever the kitchen heat")

	for _, c := range []struct {
		trigger finds.Trigger
		week    bool
	}{{finds.TriggerChat, true}, {finds.TriggerFill, false}} {
		search, adapt = run(t, c.trigger, c.week, kitchen)
		assert.NotContains(t, search, "german")
		assert.NotContains(t, adapt, "mild")
	}
}
