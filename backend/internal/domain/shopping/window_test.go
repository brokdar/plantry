package shopping_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/settings"
	"github.com/jaltszeimer/plantry/backend/internal/domain/shopping"
	"github.com/jaltszeimer/plantry/backend/internal/domain/window"
)

type fakeSettings map[string]string

func (f fakeSettings) Get(_ context.Context, key string) (settings.Value, error) {
	return settings.Value{Key: key, Raw: f[key]}, nil
}

func TestService_Range(t *testing.T) {
	sat := fakeSettings{settings.KeyPlanShoppingDay: "5", settings.KeyPlanAnchor: window.AnchorToday}
	next := fakeSettings{settings.KeyPlanShoppingDay: "5", settings.KeyPlanAnchor: window.AnchorNextShoppingDay}
	tests := []struct {
		name       string
		set        fakeSettings
		from, to   string // empty: default range
		today      string
		wantFrom   string
		wantTo     string
		wantWindow string
	}{
		{"AC6 default mid-window starts today", sat, "", "", "2026-11-04", "2026-11-04", "2026-11-06", "2026-10-31"},
		{"default on the shopping day", sat, "", "", "2026-10-31", "2026-10-31", "2026-11-06", "2026-10-31"},
		{"default next shopping day anchor", next, "", "", "2026-11-04", "2026-11-07", "2026-11-13", "2026-11-07"},
		{"explicit range keeps it, window contains from", sat, "2026-11-08", "2026-11-10", "2026-11-04", "2026-11-08", "2026-11-10", "2026-11-07"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := shopping.NewService(nil, nil, tt.set)
			var r *shopping.Range
			if tt.from != "" {
				r = &shopping.Range{From: day(tt.from), To: day(tt.to)}
			}
			got, err := svc.Range(context.Background(), r, day(tt.today))
			require.NoError(t, err)
			assert.Equal(t, day(tt.wantFrom), got.From)
			assert.Equal(t, day(tt.wantTo), got.To)
			assert.Equal(t, day(tt.wantWindow), got.Window.Start)
			assert.Equal(t, day(tt.wantWindow).AddDate(0, 0, 6), got.Window.End)
		})
	}
}
