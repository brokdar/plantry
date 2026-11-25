-- +goose Up
-- +goose StatementBegin
-- Kitchen data (KIT): the profile gets the cooking time allowances, skill,
-- equipment, heat tolerance and leanings; basics_at_home and never_items
-- hold foods by reference. skill, heat and the lists are checked in Go.
ALTER TABLE user_profile ADD COLUMN minutes_weekday INTEGER NOT NULL DEFAULT 30
    CHECK (minutes_weekday BETWEEN 5 AND 480);
ALTER TABLE user_profile ADD COLUMN minutes_weekend INTEGER NOT NULL DEFAULT 60
    CHECK (minutes_weekend BETWEEN 5 AND 480);
ALTER TABLE user_profile ADD COLUMN skill TEXT;
ALTER TABLE user_profile ADD COLUMN equipment TEXT NOT NULL DEFAULT '["oven","stovetop"]';
ALTER TABLE user_profile ADD COLUMN heat TEXT;
ALTER TABLE user_profile ADD COLUMN leanings TEXT NOT NULL DEFAULT '[]';

CREATE TABLE basics_at_home (
    food_id INTEGER PRIMARY KEY REFERENCES foods(id) ON DELETE CASCADE
);

CREATE TABLE never_items (
    food_id INTEGER PRIMARY KEY REFERENCES foods(id) ON DELETE CASCADE
);

-- Prefill from the catalogue: salt, pepper, oil, sugar, flour and common
-- dried spices, by name in English and German.
INSERT INTO basics_at_home (food_id)
SELECT id FROM foods
WHERE kind = 'leaf' AND lower(name) IN (
    'salt', 'salz', 'pepper', 'pfeffer', 'black pepper', 'schwarzer pfeffer',
    'oil', 'öl', 'olive oil', 'olivenöl', 'vegetable oil', 'pflanzenöl', 'sunflower oil', 'sonnenblumenöl', 'rapeseed oil', 'rapsöl',
    'sugar', 'zucker', 'flour', 'mehl', 'wheat flour', 'weizenmehl',
    'oregano', 'thyme', 'thymian', 'dried basil', 'basilikum getrocknet', 'paprika powder', 'paprikapulver',
    'cumin', 'kreuzkümmel', 'cinnamon', 'zimt', 'curry powder', 'currypulver', 'bay leaf', 'bay leaves', 'lorbeerblatt', 'lorbeerblätter',
    'chili flakes', 'chiliflocken', 'nutmeg', 'muskat', 'muskatnuss', 'turmeric', 'kurkuma', 'garlic powder', 'knoblauchpulver'
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE never_items;
DROP TABLE basics_at_home;
ALTER TABLE user_profile DROP COLUMN leanings;
ALTER TABLE user_profile DROP COLUMN heat;
ALTER TABLE user_profile DROP COLUMN equipment;
ALTER TABLE user_profile DROP COLUMN skill;
ALTER TABLE user_profile DROP COLUMN minutes_weekend;
ALTER TABLE user_profile DROP COLUMN minutes_weekday;
-- +goose StatementEnd
