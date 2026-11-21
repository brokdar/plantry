//go:build spectest

package spectest

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"

	appdb "github.com/jaltszeimer/plantry/backend/db"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/fake"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/agent"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
	"github.com/jaltszeimer/plantry/backend/internal/domain/slot"
	"github.com/jaltszeimer/plantry/backend/internal/domain/window"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper/testsvc"

	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

// households are the four start databases (the dev household and the three
// profile-matrix households), slimmed copies in testdata/.
var households = []string{"dev", "veg1700", "two2600", "newuser"}

var (
	today      = time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC)
	chatWeek   = time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC)
	plannerWks = []time.Time{
		time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC), time.Date(2026, 11, 28, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 12, 5, 0, 0, 0, 0, time.UTC), time.Date(2026, 12, 12, 0, 0, 0, 0, time.UTC),
	}
)

// env wires the chat tool set and the planner over a COPY of a start db.
type env struct {
	t       *testing.T
	db      *sql.DB
	ctx     context.Context
	ts      *agent.ToolSet
	planner *planning.Service
	deps    planning.Deps
	rec     *airun.Recorder
	win     window.Window
	state   func(context.Context, window.Window, time.Time) (planning.State, error)
	prof    *profile.Service
	set     agent.SettingsReader
	undo    *agent.Changes
	slots   *slot.Service
	final   string // the scripted final reply of the next turns (default "Done.")
	text    string // the assistant text of the last turn
	conv    *int64 // the conversation of the turns so far (undo is per conversation)
}

func newEnv(t *testing.T, dbName string, start time.Time) *env {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", dbName+".db"))
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "r.db")
	require.NoError(t, os.WriteFile(path, data, 0o600))
	db, err := sql.Open("sqlite", testhelper.DSN(path))
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	// the start databases are dumps at an older schema: bring them to the current one
	migrate(t, db)
	e := &env{t: t, db: db, win: window.Window{Start: start, End: start.AddDate(0, 0, 6)}}
	set := testsvc.Settings(db)
	e.deps = testsvc.PlanningDeps(db)
	e.rec = airun.NewRecorder(sqlite.NewAIRunRepo(db), slog.New(slog.DiscardHandler), time.Now)
	e.deps.RunLog, e.deps.Runs = planning.RecorderLog{Recorder: e.rec}, sqlite.NewAIRunRepo(db)
	e.deps.Go = func(f func()) { f() }
	e.planner = planning.NewService(e.deps)
	plates, avail := testsvc.PlanAt(db, nil, func() time.Time { return today })
	changes := agent.NewChanges(sqlite.NewAIChangeRepo(db), plates, avail, sqlite.NewPlanLightRepo(db), sqlite.NewTxRunner(db), func() time.Time { return today })
	foods := sqlite.NewFoodRepo(db)
	e.state = testsvc.PlanState(db)
	e.ts, err = agent.NewToolSet(agent.Services{
		Foods: food.NewService(foods), NutritionResolver: food.NewNutritionResolver(foods),
		Plates: plates, Profile: profile.NewService(sqlite.NewProfileRepo(db)),
		Slots: slot.NewService(sqlite.NewSlotRepo(db)), Settings: set,
		PlanView: testsvc.PlanView(db), Planner: e.planner, Cells: avail, Availability: avail,
		Changes: sqlite.NewAIChangeRepo(db),
		State:   e.state, Tx: sqlite.NewTxRunner(db), Light: sqlite.NewPlanLightRepo(db), Undo: changes,
	})
	require.NoError(t, err)
	e.ctx = context.Background()
	e.slots = slot.NewService(sqlite.NewSlotRepo(db))
	e.prof = profile.NewService(sqlite.NewProfileRepo(db))
	e.set = set
	e.undo = changes
	return e
}

// tc is one scripted tool call of a chat turn.
type tc struct {
	Name string
	In   any
}

// res is the result of one tool call.
type res struct {
	Name string
	Out  map[string]any
	Err  bool
	Raw  string
}

func toolTurn(i int, c tc) fake.Turn {
	var raw json.RawMessage
	switch v := c.In.(type) {
	case string:
		raw = json.RawMessage(v)
	default:
		raw, _ = json.Marshal(v)
	}
	return fake.Turn{
		StopReason: "tool_use",
		Message:    fake.ScriptedMessage{Content: []fake.ScriptedBlock{{Type: "tool_use", ToolUseID: fmt.Sprintf("tu_%d", i), ToolName: c.Name, ToolInput: raw}}},
		Usage:      fake.ScriptedUsage{InputTokens: 10, OutputTokens: 1},
	}
}

// turn runs one real chat turn (agent.Service.Chat) whose model is scripted
// to make exactly these tool calls and then end; it returns the tool results
// and the change events. Window and today are the service's own.
func (e *env) turn(text string, calls ...tc) ([]res, []llm.ChangePayload) {
	e.t.Helper()
	turns := []fake.Turn{}
	for i, c := range calls {
		turns = append(turns, toolTurn(i, c))
	}
	turns = append(turns, fake.Turn{StopReason: "end_turn", Message: fake.ScriptedMessage{Content: []fake.ScriptedBlock{{Type: "text", Text: e.reply()}}}, Usage: fake.ScriptedUsage{}})
	client := fake.NewFromScript(&fake.Script{Turns: turns})
	svc := agent.NewService(sqlite.NewAIRepo(e.db), llm.StaticResolver(e.rec.Meter(client, "fake"), "fake-model"), e.ts,
		e.prof, e.set, agent.PlanContext{Build: e.deps.Builder.Build, Summary: planning.RenderPlanSummary},
		func() time.Time { return today }).WithRecorder(e.rec)
	out := make(chan llm.Event, 64)
	errCh := make(chan error, 1)
	go func() {
		defer close(out)
		errCh <- svc.Chat(context.Background(), agent.ChatRequest{ConversationID: e.conv, UserText: text, Today: today, Start: e.win.Start}, out)
	}()
	var rs []res
	var chs []llm.ChangePayload
	acc := ""
	for ev := range out {
		switch ev.Type {
		case llm.EventToolResult:
			p, ok := ev.Payload.(llm.ToolResultPayload)
			require.True(e.t, ok, "tool result payload %T", ev.Payload)
			r := res{Name: p.Name, Err: p.IsError, Raw: string(p.Output)}
			_ = json.Unmarshal(p.Output, &r.Out)
			rs = append(rs, r)
		case llm.EventConversationReady:
			if p, ok := ev.Payload.(llm.ConversationReadyPayload); ok && e.conv == nil {
				id := p.ConversationID
				e.conv = &id
			}
		case llm.EventAssistantDelta:
			if p, ok := ev.Payload.(llm.AssistantDeltaPayload); ok {
				acc += p.Text
			}
		case llm.EventChange:
			p, ok := ev.Payload.(llm.ChangePayload)
			require.True(e.t, ok, "change payload %T", ev.Payload)
			chs = append(chs, p)
		}
	}
	require.NoError(e.t, <-errCh)
	e.text = acc
	if e.conv != nil { // the stored final assistant message is what the household reads
		var content string
		if err := e.db.QueryRow(`SELECT content FROM ai_messages WHERE conversation_id=? AND role='assistant' ORDER BY id DESC LIMIT 1`, *e.conv).Scan(&content); err == nil {
			var blocks []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}
			if json.Unmarshal([]byte(content), &blocks) == nil {
				e.text = ""
				for _, b := range blocks {
					if b.Type == "text" {
						e.text += b.Text
					}
				}
			} else {
				e.text = content
			}
		}
	}
	return rs, chs
}

func (e *env) st() planning.State {
	e.t.Helper()
	s, err := e.state(e.ctx, e.win, today)
	require.NoError(e.t, err)
	return s
}

// violations are the hard violations of the persisted window, as keys.
func (e *env) violations() map[string]bool {
	s := e.st()
	out := map[string]bool{}
	for _, is := range planning.Check(s, planning.WindowTarget(s)) {
		if is.Severity == planning.SeverityViolation {
			out[issueKey(is)] = true
		}
	}
	return out
}

func issueKey(is planning.Issue) string {
	k := is.Rule + "/" + is.Code + "/" + is.Date
	if is.SlotID != nil {
		k += "/s" + itoa(*is.SlotID)
	}
	if is.PlateID != nil {
		k += "/p" + itoa(*is.PlateID)
	}
	return k
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func (e *env) reply() string {
	if e.final == "" {
		return "Done."
	}
	return e.final
}

var gooseMu sync.Mutex // goose keeps package-level state

func migrate(t *testing.T, db *sql.DB) {
	t.Helper()
	gooseMu.Lock()
	defer gooseMu.Unlock()
	goose.SetLogger(goose.NopLogger())
	goose.SetBaseFS(appdb.Migrations)
	require.NoError(t, goose.SetDialect("sqlite3"))
	require.NoError(t, goose.Up(db, "migrations"))
}
