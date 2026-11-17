package sqlite

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/agent"
)

// availability_before_json holds a list of cells; rows written before a
// change could hold several cells hold the single cell object.
func TestDecodeCells(t *testing.T) {
	none := &agent.CellOverride{CellMode: "none"}
	tests := []struct {
		name    string
		raw     string
		want    []agent.CellBefore
		wantErr bool
	}{
		{
			name: "legacy single object",
			raw:  `{"date":"2026-11-08","slot_id":2,"override":null,"after":{"mode":"none"}}`,
			want: []agent.CellBefore{{Date: "2026-11-08", SlotID: 2, After: none}},
		},
		{
			name: "legacy single object with surrounding space",
			raw:  "\n {\"date\":\"2026-11-08\",\"slot_id\":2,\"override\":{\"mode\":\"none\"},\"after\":null}",
			want: []agent.CellBefore{{Date: "2026-11-08", SlotID: 2, Override: none}},
		},
		{
			name: "list of two cells",
			raw:  `[{"date":"2026-11-08","slot_id":2,"override":null,"after":{"mode":"none"}},{"date":"2026-11-09","slot_id":3,"override":{"mode":"none"},"after":null}]`,
			want: []agent.CellBefore{
				{Date: "2026-11-08", SlotID: 2, After: none},
				{Date: "2026-11-09", SlotID: 3, Override: none},
			},
		},
		{name: "empty list", raw: `[]`, want: []agent.CellBefore{}},
		{name: "null", raw: `null`, want: nil},
		{name: "empty string", raw: ``, wantErr: true},
		{name: "broken object", raw: `{"date":`, wantErr: true},
		{name: "broken list", raw: `[{"date":1}]`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decodeCells(tt.raw)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
