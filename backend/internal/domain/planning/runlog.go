package planning

import (
	"context"
	"errors"

	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
)

// RecorderLog is the RunLog over the AI run recorder: every generation is
// an ai_runs row (D9), the heuristic's too.
type RecorderLog struct {
	Recorder *airun.Recorder
}

// Start creates the run's row; a row the recorder could not create is an
// error, since the run's plates reference it.
func (l RecorderLog) Start(ctx context.Context, s RunStart) (context.Context, Run, error) {
	ws, we := s.WindowStart, s.WindowEnd
	var instruction *string
	if s.Instruction != "" {
		instruction = &s.Instruction
	}
	ctx, run := l.Recorder.Start(ctx, airun.Start{
		Kind: s.Kind, PromptVersion: s.PromptVersion, WindowStart: &ws, WindowEnd: &we, Instruction: instruction,
		Chooser: s.Chooser, ScopeJSON: s.ScopeJSON,
	})
	if run.ID() == 0 {
		return nil, nil, errors.New("ai run row not created")
	}
	return ctx, recorderRun{run}, nil
}

type recorderRun struct{ run *airun.Run }

func (r recorderRun) ID() int64 { return r.run.ID() }

func (r recorderRun) Finish(ctx context.Context, end RunEnd) {
	r.run.FinishWith(ctx, end.Status, end.ErrorKey, airun.Result{
		RepairAttempts: end.RepairAttempts, ViolationsJSON: end.ViolationsJSON, ScoreJSON: end.ScoreJSON, Notes: end.Notes,
	})
}
