package window

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShoppingDefault(t *testing.T) {
	// Window Sat 31 Oct to Fri 6 Nov; the next one starts Sat 7 Nov.
	tests := []struct {
		name     string
		today    string
		from, to string
	}{
		{"mid-window starts today", "2026-11-04", "2026-11-04", "2026-11-06"},
		{"on the shopping day", "2026-10-31", "2026-10-31", "2026-11-06"},
		{"before the window starts at its start", "2026-10-29", "2026-10-31", "2026-11-06"},
		{"last day shops the next window", "2026-11-06", "2026-11-07", "2026-11-13"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, from, to := ShoppingDefault(d(tt.today), saturday)
			assert.Equal(t, d(tt.from), from)
			assert.Equal(t, d(tt.to), to)
		})
	}
}
