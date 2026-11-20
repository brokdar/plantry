package agent

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// goldenRestore is a real final reply of the a2, hh1 and ho1 rounds (what the
// model wrote, before the restore) with the must-say facts of its saved change.
// OldAdded are the lines the regex restore appended then; WantAdded are the
// lines that are really missing (read by hand): the others repeat a fact the
// reply states, in a weekday name, a number word, "1,698" or a fraction.
type goldenRestore struct {
	Name      string     `json:"name"`
	Reply     string     `json:"reply"`
	Facts     []mustFact `json:"facts"`
	OldAdded  []string   `json:"old_added"`
	WantAdded []string   `json:"want_added"`
}

func TestRestoreMust_RealReplies(t *testing.T) {
	b, err := os.ReadFile("testdata/restore_golden.json")
	require.NoError(t, err)
	var cases []goldenRestore
	require.NoError(t, json.Unmarshal(b, &cases))
	require.GreaterOrEqual(t, len(cases), 8)

	var before, after, dupBefore int
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			got := restoreText(c.Reply, c.Facts...)
			added := []string{}
			for _, l := range strings.Split(got, "\n") {
				if strings.HasPrefix(l, "- ") && !strings.Contains(c.Reply, l) {
					added = append(added, l[2:])
				}
			}
			assert.Equal(t, c.WantAdded, added, "only the facts the reply lacks are added")
			before += len(c.OldAdded)
			after += len(added)
			for _, o := range c.OldAdded {
				if !strings.Contains(strings.Join(c.WantAdded, "\n"), strings.TrimSuffix(o, ".")) {
					dupBefore++
				}
			}
			// Placement: the reply's last paragraph stays last.
			paras := strings.Split(strings.TrimRight(c.Reply, "\n"), "\n\n")
			if len(added) > 0 && closing(paras[len(paras)-1]) {
				assert.True(t, strings.HasSuffix(got, paras[len(paras)-1]), "the lines come before the closing question or offer")
			}
			assert.Equal(t, got, restoreText(got, c.Facts...), "restoring twice adds nothing")
		})
	}
	t.Logf("appended lines: %d before (%d of them repeated a stated fact), %d now", before, dupBefore, after)
	assert.Less(t, after, before)
}
