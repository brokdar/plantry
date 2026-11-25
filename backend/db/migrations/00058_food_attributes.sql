-- +goose Up
-- Stored recipe attributes (spec §3, ATTR): the assistant's (auto) and the
-- user's values of a food. Rule values derived from the ingredients
-- (ATTR-4) are computed on read and never stored. values_json holds the
-- attribute fields (cuisine, kind, heroes, texture, temperature, saucy,
-- richness, colour family, taste, technique, format); sources_json maps
-- each stored field to its source (auto or user); version is the attribute
-- schema version the row was written under (flavor.Version). Vocabularies
-- are checked in Go.
CREATE TABLE food_attributes (
    food_id      INTEGER PRIMARY KEY REFERENCES foods (id) ON DELETE CASCADE,
    version      INTEGER NOT NULL,
    values_json  TEXT    NOT NULL,
    sources_json TEXT    NOT NULL
);

-- +goose Down
DROP TABLE food_attributes;
