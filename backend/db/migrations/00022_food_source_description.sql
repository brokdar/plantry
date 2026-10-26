-- +goose Up
-- +goose StatementBegin
-- Foods remember where they were imported from and carry a free-text
-- description. Both are optional.
ALTER TABLE foods ADD COLUMN source_url TEXT;
ALTER TABLE foods ADD COLUMN description TEXT;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE foods DROP COLUMN description;
ALTER TABLE foods DROP COLUMN source_url;
-- +goose StatementEnd
