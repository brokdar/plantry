package profile_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
)

// fakeRepo is an in-memory Repository for unit tests.
type fakeRepo struct {
	stored *profile.Profile
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{stored: &profile.Profile{
		DietaryRestrictions: []string{},
		Preferences:         map[string]any{},
		DietType:            profile.DietOmnivore,
		HouseholdPortions:   1,
		Locale:              "en",
	}}
}

func (r *fakeRepo) Get(_ context.Context) (*profile.Profile, error) {
	cp := *r.stored
	cp.DietaryRestrictions = append([]string{}, r.stored.DietaryRestrictions...)
	return &cp, nil
}

func (r *fakeRepo) Update(_ context.Context, p *profile.Profile) (*profile.Profile, error) {
	cp := *p
	cp.DietaryRestrictions = append([]string{}, p.DietaryRestrictions...)
	r.stored = &cp
	return &cp, nil
}

func ptr[T any](v T) *T { return &v }

func TestService_Get_ReturnsDefault(t *testing.T) {
	svc := profile.NewService(newFakeRepo())
	p, err := svc.Get(context.Background())
	require.NoError(t, err)
	require.Equal(t, "en", p.Locale)
	require.Nil(t, p.KcalTarget)
}

func TestService_Update_NilTargetsPass(t *testing.T) {
	svc := profile.NewService(newFakeRepo())
	in := &profile.Profile{Locale: "en", DietaryRestrictions: []string{}, Preferences: map[string]any{}, DietType: profile.DietOmnivore, HouseholdPortions: 1}
	_, err := svc.Update(context.Background(), in)
	require.NoError(t, err)
}

func TestService_Update_Validation(t *testing.T) {
	cases := []struct {
		name string
		in   profile.Profile
	}{
		{
			name: "kcal_target zero",
			in:   profile.Profile{KcalTarget: ptr(0.0), Locale: "en", DietaryRestrictions: []string{}, Preferences: map[string]any{}, DietType: profile.DietOmnivore, HouseholdPortions: 1},
		},
		{
			name: "kcal_target negative",
			in:   profile.Profile{KcalTarget: ptr(-100.0), Locale: "en", DietaryRestrictions: []string{}, Preferences: map[string]any{}, DietType: profile.DietOmnivore, HouseholdPortions: 1},
		},
		{
			name: "pct sum over 100",
			in: profile.Profile{
				ProteinPct: ptr(50.0), FatPct: ptr(30.0), CarbsPct: ptr(30.0),
				Locale: "en", DietaryRestrictions: []string{}, Preferences: map[string]any{}, DietType: profile.DietOmnivore, HouseholdPortions: 1,
			},
		},
		{
			name: "negative pct",
			in: profile.Profile{
				ProteinPct: ptr(-1.0),
				Locale:     "en", DietaryRestrictions: []string{}, Preferences: map[string]any{}, DietType: profile.DietOmnivore, HouseholdPortions: 1,
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := profile.NewService(newFakeRepo())
			_, err := svc.Update(context.Background(), &tc.in)
			require.Error(t, err)
			require.True(t, errors.Is(err, domain.ErrInvalidMacros), "expected ErrInvalidMacros, got: %v", err)
		})
	}
}

func TestService_Update_ValidPctSum(t *testing.T) {
	svc := profile.NewService(newFakeRepo())
	in := &profile.Profile{
		ProteinPct: ptr(35.0), FatPct: ptr(30.0), CarbsPct: ptr(35.0),
		Locale: "en", DietaryRestrictions: []string{}, Preferences: map[string]any{}, DietType: profile.DietOmnivore, HouseholdPortions: 1,
	}
	_, err := svc.Update(context.Background(), in)
	require.NoError(t, err)
}

func TestService_Update_PctSumExactly100(t *testing.T) {
	svc := profile.NewService(newFakeRepo())
	in := &profile.Profile{
		ProteinPct: ptr(40.0), FatPct: ptr(30.0), CarbsPct: ptr(30.0),
		Locale: "en", DietaryRestrictions: []string{}, Preferences: map[string]any{}, DietType: profile.DietOmnivore, HouseholdPortions: 1,
	}
	_, err := svc.Update(context.Background(), in)
	require.NoError(t, err)
}

func TestService_Update_PartialPctPass(t *testing.T) {
	svc := profile.NewService(newFakeRepo())
	in := &profile.Profile{
		ProteinPct: ptr(35.0), // only protein set — valid
		Locale:     "en", DietaryRestrictions: []string{}, Preferences: map[string]any{}, DietType: profile.DietOmnivore, HouseholdPortions: 1,
	}
	_, err := svc.Update(context.Background(), in)
	require.NoError(t, err)
}

func TestService_Update_Idempotent(t *testing.T) {
	svc := profile.NewService(newFakeRepo())
	in := &profile.Profile{
		KcalTarget: ptr(2000.0), ProteinPct: ptr(30.0),
		Locale: "de", DietaryRestrictions: []string{"vegan"}, Preferences: map[string]any{}, DietType: profile.DietOmnivore, HouseholdPortions: 1,
	}
	_, err := svc.Update(context.Background(), in)
	require.NoError(t, err)

	got, err := svc.Get(context.Background())
	require.NoError(t, err)
	require.Equal(t, ptr(2000.0), got.KcalTarget)
	require.Equal(t, ptr(30.0), got.ProteinPct)
	require.Equal(t, "de", got.Locale)
	require.Equal(t, []string{"vegan"}, got.DietaryRestrictions)
}

func validProfile() profile.Profile {
	return profile.Profile{
		Locale:              "en",
		DietaryRestrictions: []string{},
		Preferences:         map[string]any{},
		DietType:            profile.DietOmnivore,
		HouseholdPortions:   1,
	}
}

func TestProfileService_Update_PlanningPrefsValidation(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(p *profile.Profile)
		wantErr error
	}{
		{"household 0", func(p *profile.Profile) { p.HouseholdPortions = 0 }, domain.ErrInvalidHouseholdPortions},
		{"household negative", func(p *profile.Profile) { p.HouseholdPortions = -1 }, domain.ErrInvalidHouseholdPortions},
		{"household 0.5", func(p *profile.Profile) { p.HouseholdPortions = 0.5 }, nil},
		{"household 20", func(p *profile.Profile) { p.HouseholdPortions = 20 }, nil},
		{"household 20.5", func(p *profile.Profile) { p.HouseholdPortions = 20.5 }, domain.ErrInvalidHouseholdPortions},
		{"meat -1", func(p *profile.Profile) { p.MeatDaysPerWeekMax = ptr(-1) }, domain.ErrInvalidMeatDays},
		{"meat 0", func(p *profile.Profile) { p.MeatDaysPerWeekMax = ptr(0) }, nil},
		{"meat 7", func(p *profile.Profile) { p.MeatDaysPerWeekMax = ptr(7) }, nil},
		{"meat 8", func(p *profile.Profile) { p.MeatDaysPerWeekMax = ptr(8) }, domain.ErrInvalidMeatDays},
		{"meat nil", func(p *profile.Profile) { p.MeatDaysPerWeekMax = nil }, nil},
		{"diet keto", func(p *profile.Profile) { p.DietType = "keto" }, domain.ErrInvalidDietType},
		{"diet empty", func(p *profile.Profile) { p.DietType = "" }, domain.ErrInvalidDietType},
		{"diet vegan", func(p *profile.Profile) { p.DietType = profile.DietVegan }, nil},
		{"diet vegetarian", func(p *profile.Profile) { p.DietType = profile.DietVegetarian }, nil},
		{"diet pescetarian", func(p *profile.Profile) { p.DietType = profile.DietPescetarian }, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := profile.NewService(newFakeRepo())
			in := validProfile()
			tc.mutate(&in)
			_, err := svc.Update(context.Background(), &in)
			if tc.wantErr == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tc.wantErr)
		})
	}
}

func TestProfileService_Replace_MergesAbsentDietAndHousehold(t *testing.T) {
	ctx := context.Background()
	svc := profile.NewService(newFakeRepo())

	vegan := profile.DietVegan
	in := profile.ReplaceInput{Profile: validProfile(), DietType: &vegan, HouseholdPortions: ptr(2.5)}
	in.Profile.MeatDaysPerWeekMax = ptr(3)
	_, err := svc.Replace(ctx, in)
	require.NoError(t, err)

	// Nil DietType/HouseholdPortions keep the stored values; the rest is replaced.
	in = profile.ReplaceInput{Profile: validProfile()}
	in.Profile.KcalTarget = ptr(2000.0)
	got, err := svc.Replace(ctx, in)
	require.NoError(t, err)
	require.Equal(t, profile.DietVegan, got.DietType)
	require.InDelta(t, 2.5, got.HouseholdPortions, 1e-9)
	require.Nil(t, got.MeatDaysPerWeekMax)
	require.Equal(t, ptr(2000.0), got.KcalTarget)

	// Profile.DietType/HouseholdPortions are ignored in favour of the pointers.
	in = profile.ReplaceInput{Profile: validProfile()}
	in.Profile.DietType = "keto"
	in.Profile.HouseholdPortions = 99
	got, err = svc.Replace(ctx, in)
	require.NoError(t, err)
	require.Equal(t, profile.DietVegan, got.DietType)
	require.InDelta(t, 2.5, got.HouseholdPortions, 1e-9)
}

func TestProfileService_Replace_ValidatesMergedValues(t *testing.T) {
	svc := profile.NewService(newFakeRepo())
	keto := profile.DietType("keto")

	_, err := svc.Replace(context.Background(), profile.ReplaceInput{Profile: validProfile(), DietType: &keto})
	require.ErrorIs(t, err, domain.ErrInvalidDietType)

	_, err = svc.Replace(context.Background(), profile.ReplaceInput{Profile: validProfile(), HouseholdPortions: ptr(20.5)})
	require.ErrorIs(t, err, domain.ErrInvalidHouseholdPortions)
}

func TestProfileService_Replace_GetErrorPropagates(t *testing.T) {
	svc := profile.NewService(&failingGetRepo{err: domain.ErrNotFound})
	_, err := svc.Replace(context.Background(), profile.ReplaceInput{Profile: validProfile()})
	require.ErrorIs(t, err, domain.ErrNotFound)
}

type failingGetRepo struct {
	fakeRepo
	err error
}

func (r *failingGetRepo) Get(_ context.Context) (*profile.Profile, error) { return nil, r.err }

func TestProfileDietType_Valid(t *testing.T) {
	for _, d := range []profile.DietType{profile.DietVegan, profile.DietVegetarian, profile.DietPescetarian, profile.DietOmnivore} {
		require.Truef(t, d.Valid(), "%q should be valid", d)
	}
	for _, d := range []profile.DietType{"", "keto", "Vegan"} {
		require.Falsef(t, d.Valid(), "%q should be invalid", d)
	}
}
