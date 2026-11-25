-- name: EnqueueChefJob :one
INSERT INTO chef_jobs (kind, payload, requested, created_at, updated_at)
VALUES (sqlc.arg(kind), sqlc.arg(payload), sqlc.arg(requested), sqlc.arg(now), sqlc.arg(now))
RETURNING id;

-- name: ClaimChefJob :one
UPDATE chef_jobs SET status = 'running', attempts = attempts + 1, updated_at = sqlc.arg(now)
WHERE id = (SELECT q.id FROM chef_jobs q WHERE q.status = 'queued' ORDER BY q.id LIMIT 1)
RETURNING id, kind, payload, requested, attempts;

-- name: FinishChefJob :exec
UPDATE chef_jobs SET status = sqlc.arg(status), error = sqlc.arg(error), updated_at = sqlc.arg(now)
WHERE id = sqlc.arg(id);

-- name: RecoverChefJobs :execrows
UPDATE chef_jobs
SET status = CASE WHEN attempts >= sqlc.arg(max_attempts) THEN 'failed' ELSE 'queued' END,
    error = CASE WHEN attempts >= sqlc.arg(max_attempts) THEN 'interrupted ' || attempts || ' times' ELSE error END,
    updated_at = sqlc.arg(now)
WHERE status = 'running';
