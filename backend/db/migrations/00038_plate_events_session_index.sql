-- +goose Up
-- plate_events is append-only and grows forever. CountPlateEventsAfter and
-- AgentRunsOfRows filter on cook_session_id (OR plate_id); only plate_id
-- (00030, ix_plate_events_plate) and ai_run_id were indexed.
CREATE INDEX ix_plate_events_session ON plate_events(cook_session_id);

-- +goose Down
DROP INDEX ix_plate_events_session;
