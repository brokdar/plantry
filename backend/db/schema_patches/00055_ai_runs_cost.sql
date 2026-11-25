-- Schema patch: informs sqlc of the column Go migration 00055 adds (the run's
-- cost in USD at the time of the run; the same migration drops the kind
-- CHECK). Read by sqlc only; NOT a goose migration.
ALTER TABLE ai_runs ADD COLUMN cost_usd REAL;
