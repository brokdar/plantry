-- +goose Up
-- Phase 2 D18: a leaf food's store section override. NULL means the section
-- is classified from the name (shopping.Classify), so nothing is backfilled.
ALTER TABLE foods ADD COLUMN shop_section TEXT
    CHECK (shop_section IS NULL OR (kind = 'leaf' AND shop_section IN
        ('produce','bakery','dairy_eggs','meat_fish','dry_goods','cans_jars','spices_oils','frozen','other')));

-- +goose Down
ALTER TABLE foods DROP COLUMN shop_section;
