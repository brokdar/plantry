package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite/sqlcgen"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/chef"
)

// ChefJobRepo is the persistent queue of capped Chef work (chef.JobStore).
type ChefJobRepo struct {
	conn
}

// NewChefJobRepo creates a SQLite-backed job queue.
func NewChefJobRepo(db *sql.DB) *ChefJobRepo {
	return &ChefJobRepo{conn: newConn(db)}
}

var _ chef.JobStore = (*ChefJobRepo)(nil)

// Enqueue inserts a queued job.
func (r *ChefJobRepo) Enqueue(ctx context.Context, kind airun.Kind, payload string, requested bool, now time.Time) (int64, error) {
	if !chef.Capped(kind) {
		return 0, fmt.Errorf("enqueue chef job: %q is not a capped kind", kind)
	}
	if payload == "" {
		payload = "{}"
	}
	var req int64
	if requested {
		req = 1
	}
	id, err := r.queriesFor(ctx).EnqueueChefJob(ctx, sqlcgen.EnqueueChefJobParams{
		Kind: string(kind), Payload: payload, Requested: req, Now: formatTime(now),
	})
	if err != nil {
		return 0, fmt.Errorf("enqueue chef job: %w", err)
	}
	return id, nil
}

// Claim marks the oldest queued job running.
func (r *ChefJobRepo) Claim(ctx context.Context, now time.Time) (chef.Job, bool, error) {
	row, err := r.queriesFor(ctx).ClaimChefJob(ctx, formatTime(now))
	if errors.Is(err, sql.ErrNoRows) {
		return chef.Job{}, false, nil
	}
	if err != nil {
		return chef.Job{}, false, fmt.Errorf("claim chef job: %w", err)
	}
	return chef.Job{
		ID: row.ID, Kind: airun.Kind(row.Kind), Payload: row.Payload,
		Requested: row.Requested == 1, Attempts: int(row.Attempts),
	}, true, nil
}

// Finish sets a job's end status.
func (r *ChefJobRepo) Finish(ctx context.Context, id int64, status chef.Status, errText string, now time.Time) error {
	if err := r.queriesFor(ctx).FinishChefJob(ctx, sqlcgen.FinishChefJobParams{
		Status: string(status), Error: errText, Now: formatTime(now), ID: id,
	}); err != nil {
		return fmt.Errorf("finish chef job %d: %w", id, err)
	}
	return nil
}

// Recover puts running jobs back to queued, failing those at maxAttempts.
func (r *ChefJobRepo) Recover(ctx context.Context, maxAttempts int, now time.Time) (int64, error) {
	n, err := r.queriesFor(ctx).RecoverChefJobs(ctx, sqlcgen.RecoverChefJobsParams{
		MaxAttempts: int64(maxAttempts), Now: formatTime(now),
	})
	if err != nil {
		return 0, fmt.Errorf("recover chef jobs: %w", err)
	}
	return n, nil
}
