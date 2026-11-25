package sqlite_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/db"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/kitchen"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

func TestKitchenSettings_DefaultsAndRoundTrip(t *testing.T) {
	conn := testhelper.NewTestDB(t)
	svc := profile.NewService(sqlite.NewProfileRepo(conn))
	ctx := context.Background()

	p, err := svc.Get(ctx)
	require.NoError(t, err)
	assert.Equal(t, profile.Kitchen{
		MinutesWeekday: 30, MinutesWeekend: 60, Equipment: []string{"oven", "stovetop"}, Leanings: []string{},
	}, p.Kitchen)

	want := profile.Kitchen{
		MinutesWeekday: 25, MinutesWeekend: 90, Skill: profile.SkillConfident,
		Equipment: []string{"oven", "wok"}, Heat: profile.HeatHot, Leanings: []string{"garlic", "lemon"},
	}
	got, err := svc.UpdateKitchen(ctx, want)
	require.NoError(t, err)
	assert.Equal(t, want, got.Kitchen)

	// The planning-preference save leaves the kitchen alone.
	_, err = svc.Replace(ctx, profile.ReplaceInput{Profile: profile.Profile{Locale: "de"}})
	require.NoError(t, err)
	p, err = svc.Get(ctx)
	require.NoError(t, err)
	assert.Equal(t, want, p.Kitchen)
}

func TestKitchenSettings_Validation(t *testing.T) {
	svc := profile.NewService(sqlite.NewProfileRepo(testhelper.NewTestDB(t)))
	ok := profile.Kitchen{MinutesWeekday: 30, MinutesWeekend: 60, Equipment: []string{"oven"}}
	for name, mutate := range map[string]func(*profile.Kitchen){
		"minutes low":   func(k *profile.Kitchen) { k.MinutesWeekday = 4 },
		"minutes high":  func(k *profile.Kitchen) { k.MinutesWeekend = 481 },
		"skill":         func(k *profile.Kitchen) { k.Skill = "chef" },
		"heat":          func(k *profile.Kitchen) { k.Heat = "lava" },
		"unknown tool":  func(k *profile.Kitchen) { k.Equipment = []string{"deep_fryer"} },
		"repeated tool": func(k *profile.Kitchen) { k.Equipment = []string{"oven", "oven"} },
		"six leanings":  func(k *profile.Kitchen) { k.Leanings = []string{"a", "b", "c", "d", "e", "f"} },
		"long leaning":  func(k *profile.Kitchen) { k.Leanings = []string{string(make([]rune, 41))} },
	} {
		t.Run(name, func(t *testing.T) {
			k := ok
			mutate(&k)
			_, err := svc.UpdateKitchen(context.Background(), k)
			assert.ErrorIs(t, err, domain.ErrInvalidKitchen)
		})
	}
	// Blank and repeated leanings are dropped, so they do not count toward five.
	k := ok
	k.Leanings = []string{" a ", "a", "", "b", "c", "d", "e"}
	got, err := svc.UpdateKitchen(context.Background(), k)
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b", "c", "d", "e"}, got.Kitchen.Leanings)
}

func TestKitchenLists_BasicsAndNeverItems(t *testing.T) {
	conn := testhelper.NewTestDB(t)
	repo := sqlite.NewKitchenRepo(conn)
	svc := kitchen.NewService(repo)
	ctx := context.Background()
	mustExec(t, conn, `INSERT INTO foods (id, name, kind, source) VALUES (1, 'Zimt', 'leaf', 'manual'), (2, 'Apfel', 'leaf', 'manual')`)

	require.NoError(t, svc.AddBasic(ctx, 1))
	require.NoError(t, svc.AddBasic(ctx, 1), "idempotent")
	require.NoError(t, svc.AddBasic(ctx, 2))
	got, err := svc.Basics(ctx)
	require.NoError(t, err)
	assert.Equal(t, []kitchen.Food{{ID: 2, Name: "Apfel"}, {ID: 1, Name: "Zimt"}}, got)

	assert.ErrorIs(t, svc.AddBasic(ctx, 99), domain.ErrNotFound)
	require.NoError(t, svc.RemoveBasic(ctx, 1))
	require.NoError(t, svc.RemoveBasic(ctx, 1), "absent is fine")

	require.NoError(t, repo.AddNeverItem(ctx, 2))
	assert.ErrorIs(t, repo.AddNeverItem(ctx, 99), domain.ErrNotFound)
	never, err := repo.NeverItems(ctx)
	require.NoError(t, err)
	assert.Equal(t, []kitchen.Food{{ID: 2, Name: "Apfel"}}, never)

	// Deleting a food drops its references.
	mustExec(t, conn, `DELETE FROM foods WHERE id = 2`)
	got, err = svc.Basics(ctx)
	require.NoError(t, err)
	assert.Empty(t, got)
	never, err = repo.NeverItems(ctx)
	require.NoError(t, err)
	assert.Empty(t, never)
	require.NoError(t, repo.RemoveNeverItem(ctx, 2))
}

// Migration 00059 prefills the basics from the catalogue's leaves by name and
// leaves other foods (and a recipe called Salz) out.
func TestMigration00059_PrefillsBasicsFromCatalogue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "migration59.db")
	conn, err := sql.Open("sqlite", testhelper.DSN(path))
	require.NoError(t, err)
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })

	goose.SetBaseFS(db.Migrations)
	require.NoError(t, goose.SetDialect("sqlite3"))
	require.NoError(t, goose.UpTo(conn, "migrations", 58))
	mustExec(t, conn, `INSERT INTO foods (id, name, kind, source) VALUES
		(1, 'Salz', 'leaf', 'manual'), (2, 'Olive Oil', 'leaf', 'manual'), (3, 'Gurke', 'leaf', 'manual'), (4, 'Paprikapulver', 'leaf', 'manual')`)
	mustExec(t, conn, `INSERT INTO foods (id, name, kind, reference_portions) VALUES (5, 'Sugar', 'composed', 2)`)
	require.NoError(t, goose.UpTo(conn, "migrations", 59))

	rows, err := conn.Query(`SELECT food_id FROM basics_at_home ORDER BY food_id`)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	var ids []int64
	for rows.Next() {
		var id int64
		require.NoError(t, rows.Scan(&id))
		ids = append(ids, id)
	}
	assert.Equal(t, []int64{1, 2, 4}, ids)
}
