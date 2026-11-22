package agent_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/agent"
)

// C-4: "add wholegrain bread to Wednesday lunch" puts the bread on the meal even
// beside a starch side already there; an add the user did not name is still
// refused as a second starch (tune2 g-undo-07: the named bread was refused, so
// turn 1 changed nothing and there was nothing to undo).
func TestApplyChanges_NamedAddOfASecondStarchIsApplied(t *testing.T) {
	t.Parallel()
	e := newPlanToolEnv(t)
	ctx, conv := e.chat()
	rice := e.insert(`INSERT INTO foods (name, kind, role, source) VALUES ('Couscous plain', 'leaf', 'side_starch', 'manual')`)
	bread := e.insert(`INSERT INTO foods (name, kind, role, source) VALUES ('Wholegrain bread', 'leaf', 'side_starch', 'manual')`)
	p, _ := e.freshMain()
	add := func(id int64) string {
		return fmt.Sprintf(`{"ops":[{"op":"add","plate_id":%d,"food_id":%d,"amount":100,"unit":"g"}]}`, p.ID, id)
	}
	_, _, _, err := e.ts.Execute(agent.WithTurn(ctx, conv), "apply_changes", []byte(add(rice)))
	require.NoError(t, err)

	_, _, _, err = e.ts.Execute(agent.WithTurn(ctx, conv), "apply_changes", []byte(add(bread)))
	require.Error(t, err, "an unnamed second starch is refused")
	assert.Contains(t, err.Error(), "second starch")

	_, _, _, err = e.ts.Execute(agent.WithTurnNamed(ctx, conv, bread), "apply_changes", []byte(add(bread)))
	require.NoError(t, err, "the bread the user named is added")
}
