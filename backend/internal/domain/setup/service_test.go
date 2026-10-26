package setup_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/settings"
	"github.com/jaltszeimer/plantry/backend/internal/domain/setup"
)

var errBoom = errors.New("boom")

type fakeSettings struct {
	vals   map[string]string
	getErr error
	setErr error
}

func (f *fakeSettings) Get(_ context.Context, key string) (settings.Value, error) {
	if f.getErr != nil {
		return settings.Value{}, f.getErr
	}
	raw, ok := f.vals[key]
	if !ok {
		return settings.Value{Key: key, Raw: "false", Source: settings.SourceDefault}, nil
	}
	return settings.Value{Key: key, Raw: raw, Source: settings.SourceDB}, nil
}

func (f *fakeSettings) Set(_ context.Context, key, raw string) error {
	if f.setErr != nil {
		return f.setErr
	}
	f.vals[key] = raw
	return nil
}

// fakeFoods stores food kinds and counts only the composed ones, the way
// the SQL query does.
type fakeFoods struct {
	kinds []string
	err   error
}

func (f *fakeFoods) CountComposedFoods(_ context.Context) (int64, error) {
	if f.err != nil {
		return 0, f.err
	}
	var n int64
	for _, k := range f.kinds {
		if k == "composed" {
			n++
		}
	}
	return n, nil
}

func TestSetupService_Status(t *testing.T) {
	tests := []struct {
		name     string
		stored   map[string]string
		kinds    []string
		getErr   error
		countErr error
		want     setup.Status
		wantErr  error
	}{
		{
			name: "fresh",
			want: setup.Status{Completed: false, RecipeCount: 0, RecipeTarget: setup.RecipeTarget},
		},
		{
			name:   "stored true reads back as completed",
			stored: map[string]string{settings.KeySetupCompleted: "true"},
			want:   setup.Status{Completed: true, RecipeCount: 0, RecipeTarget: setup.RecipeTarget},
		},
		{
			name:  "counts composed foods only",
			kinds: []string{"leaf", "composed", "leaf", "composed", "composed"},
			want:  setup.Status{Completed: false, RecipeCount: 3, RecipeTarget: setup.RecipeTarget},
		},
		{
			name:    "settings get error is wrapped",
			getErr:  errBoom,
			wantErr: errBoom,
		},
		{
			name:     "count error is wrapped",
			countErr: errBoom,
			wantErr:  errBoom,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vals := map[string]string{}
			for k, v := range tt.stored {
				vals[k] = v
			}
			svc := setup.NewService(
				&fakeSettings{vals: vals, getErr: tt.getErr},
				&fakeFoods{kinds: tt.kinds, err: tt.countErr},
			)
			got, err := svc.Status(context.Background())
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.NotEqual(t, tt.wantErr.Error(), err.Error(), "error should carry context")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSetupService_Complete(t *testing.T) {
	tests := []struct {
		name    string
		calls   int
		setErr  error
		wantErr error
	}{
		{name: "sets completed", calls: 1},
		{name: "idempotent", calls: 2},
		{name: "settings set error is wrapped", calls: 1, setErr: errBoom, wantErr: errBoom},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &fakeSettings{vals: map[string]string{}, setErr: tt.setErr}
			svc := setup.NewService(s, &fakeFoods{})
			ctx := context.Background()

			var got setup.Status
			var err error
			for range tt.calls {
				got, err = svc.Complete(ctx)
			}
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.NotEqual(t, tt.wantErr.Error(), err.Error(), "error should carry context")
				assert.NotContains(t, s.vals, settings.KeySetupCompleted)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, setup.Status{Completed: true, RecipeTarget: setup.RecipeTarget}, got)
			assert.Equal(t, "true", s.vals[settings.KeySetupCompleted])

			status, err := svc.Status(ctx)
			require.NoError(t, err)
			assert.True(t, status.Completed)
		})
	}
}

func TestSetupService_RecipeTarget(t *testing.T) {
	assert.Equal(t, 10, setup.RecipeTarget)
}
