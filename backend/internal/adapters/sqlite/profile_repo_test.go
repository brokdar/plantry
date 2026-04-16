package sqlite_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

func ptr64(v float64) *float64 { return &v }
func ptrStr(v string) *string  { return &v }

func TestProfileRepo_GetDefault(t *testing.T) {
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewProfileRepo(db)

	p, err := repo.Get(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "en", p.Locale)
	assert.Nil(t, p.KcalTarget)
	assert.Nil(t, p.ProteinPct)
	assert.Nil(t, p.FatPct)
	assert.Nil(t, p.CarbsPct)
	assert.Nil(t, p.SystemPrompt)
	assert.Equal(t, []string{}, p.DietaryRestrictions)
	assert.NotNil(t, p.Preferences)
	assert.False(t, p.UpdatedAt.IsZero())
}

func TestProfileRepo_Update(t *testing.T) {
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewProfileRepo(db)
	ctx := context.Background()

	in := &profile.Profile{
		KcalTarget:          ptr64(1800),
		ProteinPct:          ptr64(35),
		FatPct:              ptr64(30),
		CarbsPct:            ptr64(35),
		DietaryRestrictions: []string{"vegetarian"},
		Preferences:         map[string]any{},
		SystemPrompt:        ptrStr("Prefer quick meals"),
		Locale:              "de",
	}
	updated, err := repo.Update(ctx, in)
	require.NoError(t, err)
	assert.Equal(t, ptr64(1800), updated.KcalTarget)
	assert.Equal(t, ptr64(35), updated.ProteinPct)
	assert.Equal(t, []string{"vegetarian"}, updated.DietaryRestrictions)
	assert.Equal(t, ptrStr("Prefer quick meals"), updated.SystemPrompt)
	assert.Equal(t, "de", updated.Locale)

	// Verify via Get
	got, err := repo.Get(ctx)
	require.NoError(t, err)
	assert.Equal(t, ptr64(1800), got.KcalTarget)
	assert.Equal(t, []string{"vegetarian"}, got.DietaryRestrictions)
	assert.Equal(t, "de", got.Locale)
}

func TestProfileRepo_ClearTargets(t *testing.T) {
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewProfileRepo(db)
	ctx := context.Background()

	// Set targets first
	_, err := repo.Update(ctx, &profile.Profile{
		KcalTarget:          ptr64(2000),
		DietaryRestrictions: []string{},
		Preferences:         map[string]any{},
		Locale:              "en",
	})
	require.NoError(t, err)

	// Clear by setting nil
	_, err = repo.Update(ctx, &profile.Profile{
		KcalTarget:          nil,
		DietaryRestrictions: []string{},
		Preferences:         map[string]any{},
		Locale:              "en",
	})
	require.NoError(t, err)

	got, err := repo.Get(ctx)
	require.NoError(t, err)
	assert.Nil(t, got.KcalTarget)
}
