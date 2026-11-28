-- +goose Up
-- Chef E2: the opt-in monthly library review (LIB-1..3). library_review is
-- the one card: the month it was computed for, its findings (JSON, each
-- with a dismissed flag) and whether the user dismissed the card.
-- library_gaps queues the gaps the assistant fills itself at the next
-- planning session (LIB-3, FIND-1b): dish is the find query, find_id the
-- find started for it, offered whether this planning session took one.
CREATE TABLE library_review (
    id             INTEGER PRIMARY KEY CHECK (id = 1),
    month          TEXT    NOT NULL,
    findings_json  TEXT    NOT NULL,
    card_dismissed INTEGER NOT NULL DEFAULT 0 CHECK (card_dismissed IN (0, 1))
);
CREATE TABLE library_gaps (
    kind       TEXT    PRIMARY KEY,
    dish       TEXT    NOT NULL,
    created_at TEXT    NOT NULL,
    find_id    INTEGER REFERENCES recipe_suggestions(id) ON DELETE SET NULL,
    offered    INTEGER NOT NULL DEFAULT 0 CHECK (offered IN (0, 1))
);

-- +goose Down
DROP TABLE library_gaps;
DROP TABLE library_review;
