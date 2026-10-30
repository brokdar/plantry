package importer_test

import (
	"context"
	"database/sql"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/fake"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/importer"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

type stubExtractor struct{ rec *importer.RawRecipe }

func (s stubExtractor) ExtractRecipe(string) (*importer.RawRecipe, error) { return s.rec, nil }

func TestService_Extract_PassesSourceKcalThrough(t *testing.T) {
	kcal := 512.0
	rec := &importer.RawRecipe{
		Name:                 "Spaghetti",
		RecipeIngredient:     []string{"200 g Spaghetti"},
		SourceKcalPerServing: &kcal,
	}
	svc := importer.NewService(nil, stubExtractor{rec: rec}, nil, nil, nil)

	d, err := svc.Extract(context.Background(), importer.ExtractInput{HTML: "<html></html>"})
	require.NoError(t, err)
	require.NotNil(t, d.SourceKcalPerServing)
	require.InDelta(t, 512.0, *d.SourceKcalPerServing, 0.001)
}

const importModel = "fake-import"

// newRecordedImport wires the LLM fallback to a metered fake client whose
// runs land in a real SQLite DB. The JSON-LD stub finds no recipe.
func newRecordedImport(t *testing.T, turns ...fake.Turn) (*importer.Service, *sql.DB) {
	t.Helper()
	db := testhelper.NewTestDB(t)
	rec := airun.NewRecorder(sqlite.NewAIRunRepo(db), slog.New(slog.DiscardHandler), time.Now)
	client := rec.Meter(fake.NewFromScript(&fake.Script{Turns: turns}), "fake")
	svc := importer.NewService(nil, stubExtractor{rec: &importer.RawRecipe{}}, llm.StaticResolver(client, importModel), nil, nil).
		WithRecorder(rec)
	return svc, db
}

// textTurn returns a scripted reply. stopReason must not be end_turn for a
// turn followed by another, since the fake rewinds after a terminal turn.
func textTurn(text, stopReason string, in, out int) fake.Turn {
	return fake.Turn{
		StopReason: stopReason,
		Message:    fake.ScriptedMessage{Content: []fake.ScriptedBlock{{Type: "text", Text: text}}},
		Usage:      fake.ScriptedUsage{InputTokens: in, OutputTokens: out},
	}
}

type importRun struct {
	kind, provider, model, promptVersion, status string
	errorKey                                     sql.NullString
	iterations                                   int
	in, out                                      int64
	steps                                        []string
}

func onlyImportRun(t *testing.T, db *sql.DB) importRun {
	t.Helper()
	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM ai_runs`).Scan(&n))
	require.Equal(t, 1, n, "want exactly one ai_runs row")
	var r importRun
	require.NoError(t, db.QueryRow(`SELECT kind, provider, model, prompt_version, status, error_key,
		iterations, input_tokens, output_tokens FROM ai_runs`).Scan(
		&r.kind, &r.provider, &r.model, &r.promptVersion, &r.status, &r.errorKey, &r.iterations, &r.in, &r.out))
	rows, err := db.Query(`SELECT type || ':' || name || ':' || is_error FROM ai_run_steps ORDER BY seq`)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var s string
		require.NoError(t, rows.Scan(&s))
		r.steps = append(r.steps, s)
	}
	require.NoError(t, rows.Err())
	return r
}

func TestService_Extract_LLMFallbackRecordsOneRun(t *testing.T) {
	validJSON := `{"name":"Soup","servings":2,"instructions":["Cook"],"ingredient_lines":["1 l Water"],"language":"en"}`
	svc, db := newRecordedImport(t,
		textTurn("not json", "max_tokens", 100, 10),
		textTurn(validJSON, "end_turn", 200, 20),
	)

	d, err := svc.Extract(context.Background(), importer.ExtractInput{HTML: "<html>soup</html>"})
	require.NoError(t, err)
	require.Equal(t, "Soup", d.Name)

	r := onlyImportRun(t, db)
	assert.Equal(t, "import_fallback", r.kind)
	assert.Equal(t, "fake", r.provider)
	assert.Equal(t, importModel, r.model)
	assert.Equal(t, importer.PromptVersion, r.promptVersion)
	assert.Equal(t, "ok", r.status)
	assert.False(t, r.errorKey.Valid)
	assert.Equal(t, 2, r.iterations)
	assert.Equal(t, int64(300), r.in)
	assert.Equal(t, int64(30), r.out)
	assert.Equal(t, []string{"llm_call:" + importModel + ":0", "llm_call:" + importModel + ":0"}, r.steps)
}

func TestService_Extract_LLMFallbackFailureStatus(t *testing.T) {
	tests := []struct {
		name    string
		turns   []fake.Turn
		wantErr error
		wantKey string
		steps   int
	}{
		{
			name:    "not a recipe",
			turns:   []fake.Turn{textTurn(`{"not_a_recipe": true}`, "end_turn", 5, 1)},
			wantErr: domain.ErrImportNoRecipe,
			wantKey: "error.import.no_recipe",
			steps:   1,
		},
		{
			name:    "invalid json twice",
			turns:   []fake.Turn{textTurn("nope", "max_tokens", 5, 1), textTurn("still nope", "end_turn", 5, 1)},
			wantErr: domain.ErrImportLLMFailed,
			wantKey: "error.import.llm_failed",
			steps:   2,
		},
		{
			name:    "client error",
			turns:   nil, // empty script: every call fails
			wantErr: domain.ErrImportLLMFailed,
			wantKey: "error.import.llm_failed",
			steps:   1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, db := newRecordedImport(t, tt.turns...)
			_, err := svc.Extract(context.Background(), importer.ExtractInput{HTML: "<html></html>"})
			require.ErrorIs(t, err, tt.wantErr)

			r := onlyImportRun(t, db)
			assert.Equal(t, "error", r.status)
			assert.Equal(t, tt.wantKey, r.errorKey.String)
			assert.Len(t, r.steps, tt.steps)
		})
	}
}
