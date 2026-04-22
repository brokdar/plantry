-- +goose Up
-- Extended nutrients: nullable so "no data" is distinct from 0.
-- Units match upstream sources (FDC + OFF); UI renders the correct suffix.
ALTER TABLE ingredients ADD COLUMN saturated_fat_100g REAL; -- g
ALTER TABLE ingredients ADD COLUMN trans_fat_100g     REAL; -- g
ALTER TABLE ingredients ADD COLUMN cholesterol_100g   REAL; -- mg
ALTER TABLE ingredients ADD COLUMN sugar_100g         REAL; -- g
ALTER TABLE ingredients ADD COLUMN potassium_100g     REAL; -- mg
ALTER TABLE ingredients ADD COLUMN calcium_100g       REAL; -- mg
ALTER TABLE ingredients ADD COLUMN iron_100g          REAL; -- mg
ALTER TABLE ingredients ADD COLUMN magnesium_100g     REAL; -- mg
ALTER TABLE ingredients ADD COLUMN phosphorus_100g    REAL; -- mg
ALTER TABLE ingredients ADD COLUMN zinc_100g          REAL; -- mg
ALTER TABLE ingredients ADD COLUMN vitamin_a_100g     REAL; -- µg RAE
ALTER TABLE ingredients ADD COLUMN vitamin_c_100g     REAL; -- mg
ALTER TABLE ingredients ADD COLUMN vitamin_d_100g     REAL; -- µg
ALTER TABLE ingredients ADD COLUMN vitamin_b12_100g   REAL; -- µg
ALTER TABLE ingredients ADD COLUMN vitamin_b6_100g    REAL; -- mg
ALTER TABLE ingredients ADD COLUMN folate_100g        REAL; -- µg DFE

-- +goose Down
ALTER TABLE ingredients DROP COLUMN folate_100g;
ALTER TABLE ingredients DROP COLUMN vitamin_b6_100g;
ALTER TABLE ingredients DROP COLUMN vitamin_b12_100g;
ALTER TABLE ingredients DROP COLUMN vitamin_d_100g;
ALTER TABLE ingredients DROP COLUMN vitamin_c_100g;
ALTER TABLE ingredients DROP COLUMN vitamin_a_100g;
ALTER TABLE ingredients DROP COLUMN zinc_100g;
ALTER TABLE ingredients DROP COLUMN phosphorus_100g;
ALTER TABLE ingredients DROP COLUMN magnesium_100g;
ALTER TABLE ingredients DROP COLUMN iron_100g;
ALTER TABLE ingredients DROP COLUMN calcium_100g;
ALTER TABLE ingredients DROP COLUMN potassium_100g;
ALTER TABLE ingredients DROP COLUMN sugar_100g;
ALTER TABLE ingredients DROP COLUMN cholesterol_100g;
ALTER TABLE ingredients DROP COLUMN trans_fat_100g;
ALTER TABLE ingredients DROP COLUMN saturated_fat_100g;
