package sqlite_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

// B1-AC6 (store side): the D25 outcome machine, one event after another.
func TestAIRunRepo_RecordOutcome(t *testing.T) {
	const (
		acc   = plate.OutcomeAccepted
		dis   = plate.OutcomeDiscarded
		reset = plate.OutcomeReset
	)
	tests := []struct {
		name   string
		events []plate.OutcomeEvent
		want   string // "" = NULL
	}{
		{"no event", nil, ""},
		{"accept", []plate.OutcomeEvent{acc}, "accepted"},
		{"discard", []plate.OutcomeEvent{dis}, "discarded"},
		{"accept twice", []plate.OutcomeEvent{acc, acc}, "accepted"},
		{"accept then discard", []plate.OutcomeEvent{acc, dis}, "partially_accepted"},
		{"discard then accept", []plate.OutcomeEvent{dis, acc}, "partially_accepted"},
		{"partial stays partial", []plate.OutcomeEvent{acc, dis, acc, dis}, "partially_accepted"},
		{"accept then reset", []plate.OutcomeEvent{acc, reset}, "open"},
		{"reset of a partial keeps the discard", []plate.OutcomeEvent{acc, dis, reset}, "discarded"},
		{"reset after discard", []plate.OutcomeEvent{dis, reset}, "discarded"},
		{"reset of a fresh run", []plate.OutcomeEvent{reset}, ""},
		{"open then accept", []plate.OutcomeEvent{acc, reset, acc}, "accepted"},
		{"open then discard", []plate.OutcomeEvent{acc, reset, dis}, "discarded"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			db := testhelper.NewTestDB(t)
			repo := sqlite.NewAIRunRepo(db)
			id, err := repo.CreateRun(ctx, airun.RunStart{Kind: airun.KindGenerate, StartedAt: time.Now(), PromptVersion: "v1"})
			require.NoError(t, err)
			for _, ev := range tt.events {
				require.NoError(t, repo.Record(ctx, id, ev))
			}
			var outcome, at sql.NullString
			require.NoError(t, db.QueryRow(`SELECT outcome, outcome_at FROM ai_runs WHERE id = ?`, id).Scan(&outcome, &at))
			assert.Equal(t, tt.want, outcome.String)
			assert.Equal(t, tt.want != "", outcome.Valid)
			assert.Equal(t, len(tt.events) > 0, at.Valid, "outcome_at set on every record")
		})
	}
}

func TestAIRunRepo_RecordOutcomeRejectsUnknownEvent(t *testing.T) {
	ctx := context.Background()
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewAIRunRepo(db)
	id, err := repo.CreateRun(ctx, airun.RunStart{Kind: airun.KindGenerate, StartedAt: time.Now(), PromptVersion: "v1"})
	require.NoError(t, err)
	require.Error(t, repo.Record(ctx, id, "kept"))
}
