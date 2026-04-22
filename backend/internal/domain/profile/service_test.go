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
	in := &profile.Profile{Locale: "en", DietaryRestrictions: []string{}, Preferences: map[string]any{}}
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
			in:   profile.Profile{KcalTarget: ptr(0.0), Locale: "en", DietaryRestrictions: []string{}, Preferences: map[string]any{}},
		},
		{
			name: "kcal_target negative",
			in:   profile.Profile{KcalTarget: ptr(-100.0), Locale: "en", DietaryRestrictions: []string{}, Preferences: map[string]any{}},
		},
		{
			name: "pct sum over 100",
			in: profile.Profile{
				ProteinPct: ptr(50.0), FatPct: ptr(30.0), CarbsPct: ptr(30.0),
				Locale: "en", DietaryRestrictions: []string{}, Preferences: map[string]any{},
			},
		},
		{
			name: "negative pct",
			in: profile.Profile{
				ProteinPct: ptr(-1.0),
				Locale:     "en", DietaryRestrictions: []string{}, Preferences: map[string]any{},
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
		Locale: "en", DietaryRestrictions: []string{}, Preferences: map[string]any{},
	}
	_, err := svc.Update(context.Background(), in)
	require.NoError(t, err)
}

func TestService_Update_PctSumExactly100(t *testing.T) {
	svc := profile.NewService(newFakeRepo())
	in := &profile.Profile{
		ProteinPct: ptr(40.0), FatPct: ptr(30.0), CarbsPct: ptr(30.0),
		Locale: "en", DietaryRestrictions: []string{}, Preferences: map[string]any{},
	}
	_, err := svc.Update(context.Background(), in)
	require.NoError(t, err)
}

func TestService_Update_PartialPctPass(t *testing.T) {
	svc := profile.NewService(newFakeRepo())
	in := &profile.Profile{
		ProteinPct: ptr(35.0), // only protein set — valid
		Locale:     "en", DietaryRestrictions: []string{}, Preferences: map[string]any{},
	}
	_, err := svc.Update(context.Background(), in)
	require.NoError(t, err)
}

func TestService_Update_Idempotent(t *testing.T) {
	svc := profile.NewService(newFakeRepo())
	in := &profile.Profile{
		KcalTarget: ptr(2000.0), ProteinPct: ptr(30.0),
		Locale: "de", DietaryRestrictions: []string{"vegan"}, Preferences: map[string]any{},
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
