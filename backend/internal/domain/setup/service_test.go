package setup_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/settings"
	"github.com/jaltszeimer/plantry/backend/internal/domain/setup"
)

type fakeSettings struct{ vals map[string]string }

func (f *fakeSettings) Get(_ context.Context, key string) (settings.Value, error) {
	raw, ok := f.vals[key]
	if !ok {
		return settings.Value{Key: key, Raw: "false", Source: settings.SourceDefault}, nil
	}
	return settings.Value{Key: key, Raw: raw, Source: settings.SourceDB}, nil
}

func (f *fakeSettings) Set(_ context.Context, key, raw string) error {
	f.vals[key] = raw
	return nil
}

// fakeFoods stores food kinds and counts only the composed ones, the way
// the SQL query does.
type fakeFoods struct{ kinds []string }

func (f *fakeFoods) CountComposedFoods(_ context.Context) (int64, error) {
	var n int64
	for _, k := range f.kinds {
		if k == "composed" {
			n++
		}
	}
	return n, nil
}

func newService(kinds ...string) (*setup.Service, *fakeSettings) {
	s := &fakeSettings{vals: map[string]string{}}
	return setup.NewService(s, &fakeFoods{kinds: kinds}), s
}

func TestSetupService_Fresh(t *testing.T) {
	svc, _ := newService()
	got, err := svc.Status(context.Background())
	require.NoError(t, err)
	assert.Equal(t, setup.Status{Completed: false, RecipeCount: 0, RecipeTarget: setup.RecipeTarget}, got)
	assert.Equal(t, 10, setup.RecipeTarget)
}

func TestSetupService_Complete(t *testing.T) {
	svc, s := newService()
	ctx := context.Background()

	got, err := svc.Complete(ctx)
	require.NoError(t, err)
	assert.True(t, got.Completed)
	assert.Equal(t, "true", s.vals[settings.KeySetupCompleted])

	got, err = svc.Status(ctx)
	require.NoError(t, err)
	assert.True(t, got.Completed)

	// Idempotent.
	got, err = svc.Complete(ctx)
	require.NoError(t, err)
	assert.True(t, got.Completed)
}

func TestSetupService_CountsComposedOnly(t *testing.T) {
	svc, _ := newService("leaf", "composed", "leaf", "composed", "composed")
	got, err := svc.Status(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 3, got.RecipeCount)
}
