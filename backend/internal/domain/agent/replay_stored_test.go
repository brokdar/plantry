package agent_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	_ "github.com/jaltszeimer/plantry/backend/db/migrations"
	"github.com/jaltszeimer/plantry/backend/internal/domain/agent"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"

	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

// TestReplayStored is the M0 replay harness (see replay_stored_README.md).
// It is skipped unless PLANTRY_REPLAY_ROUNDS names the loop's rounds directory.
// Every stored chat tool call of the named rounds runs through the current
// tool layer on a copy of the run's start database, turn by turn in one
// conversation, with no model. Each call is "identical" or "differs"; the
// report goes to PLANTRY_REPLAY_OUT (JSON).
func TestReplayStored(t *testing.T) {
	rounds := os.Getenv("PLANTRY_REPLAY_ROUNDS")
	if rounds == "" {
		t.Skip("PLANTRY_REPLAY_ROUNDS not set")
	}
	names := strings.Fields(os.Getenv("PLANTRY_REPLAY_NAMES"))
	if len(names) == 0 {
		names = []string{"a1-fca42e7", "a2-0b8ca4c", "ho1-0b8ca4c", "hh1-0b8ca4c"}
	}
	variants := scenarioVariants(t, filepath.Dir(filepath.Dir(rounds)))
	var (
		mu  sync.Mutex
		all []replayCall
	)
	t.Run("rounds", func(t *testing.T) {
		for _, rd := range names {
			runs, err := filepath.Glob(filepath.Join(rounds, rd, "runs", "*", "run.db"))
			require.NoError(t, err)
			for _, rdb := range runs {
				if !isChatRun(t, filepath.Join(filepath.Dir(rdb), "meta.json")) {
					continue // generation runs use the planner, which needs a model
				}
				t.Run(rd+"/"+filepath.Base(filepath.Dir(rdb)), func(t *testing.T) {
					t.Parallel()
					label := filepath.Base(filepath.Dir(rdb))
					label = label[:strings.LastIndex(label, "-r")]
					calls := replayRun(t, filepath.Join(rounds, rd), rdb, variants[label])
					for i := range calls {
						calls[i].Round = strings.SplitN(rd, "-", 2)[0]
						calls[i].Run = filepath.Base(filepath.Dir(rdb))
					}
					mu.Lock()
					all = append(all, calls...)
					mu.Unlock()
				})
			}
		}
	})
	sort.Slice(all, func(i, j int) bool {
		a, b := all[i], all[j]
		if a.Round != b.Round {
			return a.Round < b.Round
		}
		if a.Run != b.Run {
			return a.Run < b.Run
		}
		return a.Seq < b.Seq
	})
	if out := os.Getenv("PLANTRY_REPLAY_OUT"); out != "" {
		b, err := json.MarshalIndent(all, "", " ")
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(out, b, 0o644))
	}
	type key struct{ round, status string }
	n := map[key]int{}
	paths := map[string]int{}
	for _, c := range all {
		n[key{c.Round, c.Status}]++
		for _, p := range c.Paths {
			paths[c.Tool+" "+p]++
		}
	}
	t.Logf("calls: %d", len(all))
	for k, v := range n {
		t.Logf("%s %s: %d", k.round, k.status, v)
	}
	var ps []string
	for p, v := range paths {
		ps = append(ps, fmt.Sprintf("%5d  %s", v, p))
	}
	sort.Sort(sort.Reverse(sort.StringSlice(ps)))
	for _, p := range ps {
		t.Log(p)
	}
}

type replayCall struct {
	Round  string   `json:"round"`
	Run    string   `json:"run"`
	AIRun  int64    `json:"ai_run"`
	Seq    int      `json:"seq"`
	Tool   string   `json:"tool"`
	Status string   `json:"status"` // identical, differs, skipped
	Note   string   `json:"note,omitempty"`
	Paths  []string `json:"paths,omitempty"` // differing field paths, indexes as []
	Diffs  []string `json:"diffs,omitempty"` // first few "path: stored -> now"
	NDiffs int      `json:"n_diffs,omitempty"`
	// Args and Got (the replayed result) are set when PLANTRY_REPLAY_FULL is
	// set: the census reads them.
	Args json.RawMessage `json:"args,omitempty"`
	Got  json.RawMessage `json:"got,omitempty"`
}

func isChatRun(t *testing.T, meta string) bool {
	t.Helper()
	b, err := os.ReadFile(meta)
	require.NoError(t, err)
	var m struct {
		Kind string `json:"kind"`
	}
	require.NoError(t, json.Unmarshal(b, &m))
	return m.Kind == "chat"
}

// scenarioVariants maps a scenario label to its start DB variant, from the
// capability, held-out and households scenario files.
func scenarioVariants(t *testing.T, plantryDev string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, f := range []string{"loop/scenarios.json", "heldout/scenarios.json", "households/scenarios.json"} {
		b, err := os.ReadFile(filepath.Join(plantryDev, f))
		if err != nil {
			continue
		}
		var s struct {
			Scenarios []struct {
				Label        string `json:"label"`
				StartVariant string `json:"start_variant"`
			} `json:"scenarios"`
		}
		require.NoError(t, json.Unmarshal(b, &s))
		for _, sc := range s.Scenarios {
			out[sc.Label] = sc.StartVariant
		}
	}
	return out
}

// replayTools are the chat tools a model calls directly; plan.* steps are
// recordings of the planner and of the turn's before/after state.
var replayTools = map[string]bool{
	"apply_changes": true, "find_foods": true, "get_plan": true, "find_alternatives": true,
	"find_ways": true, "undo_last_change": true, "regenerate": true,
}

func replayRun(t *testing.T, roundDir, runDB, variant string) []replayCall {
	t.Helper()
	start := filepath.Join(roundDir, "start-chat.db")
	if variant != "" {
		start = filepath.Join(roundDir, "start-chat-"+variant+".db")
	}
	// The start database may predate newer migrations: bring a copy up first.
	b, err := os.ReadFile(start)
	require.NoError(t, err)
	mig := filepath.Join(t.TempDir(), "start.db")
	require.NoError(t, os.WriteFile(mig, b, 0o600))
	mdb, err := sql.Open("sqlite", testhelper.DSN(mig))
	require.NoError(t, err)
	migrateCopy(t, mdb)
	var smax int64 // the run DB's first runs are the start DB's own history
	require.NoError(t, mdb.QueryRow(`SELECT COALESCE(MAX(id), 0) FROM ai_runs`).Scan(&smax))
	require.NoError(t, mdb.Close())

	e := replayEnv(t, mig)
	_, conv := e.chat() // one conversation for the whole run

	src, err := sql.Open("sqlite", "file:"+runDB+"?mode=ro")
	require.NoError(t, err)
	defer func() { _ = src.Close() }()
	turns, err := src.Query(`SELECT id FROM ai_runs WHERE kind = 'chat' AND id > ? ORDER BY id`, smax)
	require.NoError(t, err)
	var ids []int64
	for turns.Next() {
		var id int64
		require.NoError(t, turns.Scan(&id))
		ids = append(ids, id)
	}
	require.NoError(t, turns.Err())

	var out []replayCall
	full := os.Getenv("PLANTRY_REPLAY_FULL") != ""
	diverged := false // a regenerate is not replayed, so later calls see another plan
	for turn, id := range ids {
		ctx, _ := e.rec.Start(e.ctx, airun.Start{Kind: airun.KindChat, ConversationID: &conv})
		ctx = agent.WithTurnSeq(ctx, conv, uint64(turn+1))
		rows, err := src.Query(`SELECT seq, name, COALESCE(args_json,''), COALESCE(result_json,'') FROM ai_run_steps
			WHERE run_id = ? AND type = 'tool_call' AND caller = 'direct' ORDER BY seq`, id)
		require.NoError(t, err)
		type step struct {
			seq              int
			name, args, want string
		}
		var steps []step
		for rows.Next() {
			var s step
			require.NoError(t, rows.Scan(&s.seq, &s.name, &s.args, &s.want))
			if replayTools[s.name] {
				steps = append(steps, s)
			}
		}
		require.NoError(t, rows.Err())
		_ = rows.Close()
		for _, s := range steps {
			c := replayCall{AIRun: id, Seq: s.seq, Tool: s.name}
			if s.name == "regenerate" {
				c.Status, c.Note = "skipped", "needs the planner model"
				diverged = true
				out = append(out, c)
				continue
			}
			got, _, _, err := e.ts.Execute(ctx, s.name, json.RawMessage(s.args))
			if err != nil {
				got, _ = json.Marshal(map[string]string{"error": err.Error()})
			}
			if full {
				c.Args, c.Got = json.RawMessage(s.args), got
			}
			var a, b any
			require.NoError(t, json.Unmarshal([]byte(s.want), &a))
			require.NoError(t, json.Unmarshal(got, &b))
			if m, ok := b.(map[string]any); ok {
				delete(m, "change_id") // recorded after the stored result was written
			}
			var diffs []string
			diffJSON("", a, b, &diffs)
			if len(diffs) == 0 {
				c.Status = "identical"
			} else {
				c.Status, c.NDiffs = "differs", len(diffs)
				seen := map[string]bool{}
				for _, d := range diffs {
					path, _, _ := strings.Cut(d, ": ")
					p := normPath(path)
					if !seen[p] {
						seen[p] = true
						c.Paths = append(c.Paths, p)
					}
				}
				sort.Strings(c.Paths)
				c.Diffs = diffs[:min(len(diffs), 6)]
			}
			if diverged && c.Status == "differs" {
				c.Note = "after a skipped regenerate: the plan state differs"
			}
			out = append(out, c)
		}
	}
	return out
}

var idxRe = regexp.MustCompile(`\[\d+\]`)

func normPath(p string) string { return idxRe.ReplaceAllString(p, "[]") }

func short(v any) string {
	b, _ := json.Marshal(v)
	if len(b) > 120 {
		return string(b[:120]) + "..."
	}
	return string(b)
}

// diffJSON appends "path: stored -> now" for every leaf that differs.
func diffJSON(path string, a, b any, out *[]string) {
	switch av := a.(type) {
	case map[string]any:
		bv, ok := b.(map[string]any)
		if !ok {
			*out = append(*out, fmt.Sprintf("%s: %s -> %s", path, short(a), short(b)))
			return
		}
		keys := map[string]bool{}
		for k := range av {
			keys[k] = true
		}
		for k := range bv {
			keys[k] = true
		}
		sorted := make([]string, 0, len(keys))
		for k := range keys {
			sorted = append(sorted, k)
		}
		sort.Strings(sorted)
		for _, k := range sorted {
			x, xok := av[k]
			y, yok := bv[k]
			switch {
			case !xok:
				*out = append(*out, fmt.Sprintf("%s.%s: (absent) -> %s", path, k, short(y)))
			case !yok:
				*out = append(*out, fmt.Sprintf("%s.%s: %s -> (absent)", path, k, short(x)))
			default:
				diffJSON(path+"."+k, x, y, out)
			}
		}
	case []any:
		bv, ok := b.([]any)
		if !ok {
			*out = append(*out, fmt.Sprintf("%s: %s -> %s", path, short(a), short(b)))
			return
		}
		for i := 0; i < max(len(av), len(bv)); i++ {
			p := fmt.Sprintf("%s[%d]", path, i)
			switch {
			case i >= len(av):
				*out = append(*out, fmt.Sprintf("%s: (absent) -> %s", p, short(bv[i])))
			case i >= len(bv):
				*out = append(*out, fmt.Sprintf("%s: %s -> (absent)", p, short(av[i])))
			default:
				diffJSON(p, av[i], bv[i], out)
			}
		}
	default:
		if !reflect.DeepEqual(a, b) {
			*out = append(*out, fmt.Sprintf("%s: %s -> %s", path, short(a), short(b)))
		}
	}
}
