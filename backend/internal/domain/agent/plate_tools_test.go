package agent_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/agent"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
	"github.com/jaltszeimer/plantry/backend/internal/domain/slot"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper/testsvc"
)

// plateToolEnv runs the plate tools against the real write path: a dinner
// slot, leaf Rice, composed Chili, household portions 2.
type plateToolEnv struct {
	ctx    context.Context
	db     *sql.DB
	ts     *agent.ToolSet
	plates *plate.Service
	slotID int64
	rice   int64
	chili  int64
}

func newPlateToolEnv(t *testing.T) *plateToolEnv {
	t.Helper()
	ctx := context.Background()
	db := testhelper.NewTestDB(t)
	insert := func(q string) int64 {
		res, err := db.ExecContext(ctx, q)
		require.NoError(t, err, q)
		id, err := res.LastInsertId()
		require.NoError(t, err)
		return id
	}
	e := &plateToolEnv{ctx: ctx, db: db}
	e.slotID = insert(`INSERT INTO time_slots (name_key, icon, sort_order, active) VALUES ('slot.dinner', 'moon', 1, 1)`)
	e.rice = insert(`INSERT INTO foods (name, kind, source) VALUES ('Rice', 'leaf', 'manual')`)
	e.chili = insert(`INSERT INTO foods (name, kind, role, reference_portions) VALUES ('Chili', 'composed', 'main', 4)`)
	_, err := db.ExecContext(ctx, `UPDATE user_profile SET household_portions = 2 WHERE id = 1`)
	require.NoError(t, err)
	e.plates = testsvc.Plates(db)
	e.ts, err = agent.NewToolSet(agent.Services{
		Foods:   food.NewService(sqlite.NewFoodRepo(db)),
		Plates:  e.plates,
		Profile: profile.NewService(sqlite.NewProfileRepo(db)),
		Slots:   slot.NewService(sqlite.NewSlotRepo(db)),
	})
	require.NoError(t, err)
	return e
}

func (e *plateToolEnv) exec(t *testing.T, name, input string) (map[string]any, error) {
	t.Helper()
	out, _, err := e.ts.Execute(e.ctx, name, json.RawMessage(input))
	if err != nil {
		return nil, err
	}
	var m map[string]any
	require.NoError(t, json.Unmarshal(out, &m))
	return m, nil
}

func (e *plateToolEnv) mustExec(t *testing.T, name, input string) map[string]any {
	t.Helper()
	m, err := e.exec(t, name, input)
	require.NoError(t, err)
	return m
}

func (e *plateToolEnv) newPlate(t *testing.T) int64 {
	t.Helper()
	p := e.mustExec(t, "create_plate", fmt.Sprintf(`{"date":"2026-11-02","slot_id":%d}`, e.slotID))
	return int64(p["id"].(float64))
}

// B2-AC5
func TestTool_AddFoodToPlate_TakesPerEaterQuantities(t *testing.T) {
	e := newPlateToolEnv(t)
	plateID := e.newPlate(t)

	leaf := e.mustExec(t, "add_food_to_plate", fmt.Sprintf(`{"plate_id":%d,"food_id":%d,"amount":150,"unit":"g"}`, plateID, e.rice))
	assert.Equal(t, 150.0, leaf["amount"])
	assert.Equal(t, "g", leaf["unit"])
	assert.Equal(t, 150.0, leaf["grams"], "150 g, not portions × 100 g")
	assert.NotContains(t, leaf, "cook_session_id")

	composed := e.mustExec(t, "add_food_to_plate", fmt.Sprintf(`{"plate_id":%d,"food_id":%d,"portions":1.5}`, plateID, e.chili))
	assert.Equal(t, 1.5, composed["portions"])
	require.Contains(t, composed, "cook_session_id")

	p, err := e.plates.Get(e.ctx, plateID)
	require.NoError(t, err)
	require.Len(t, p.Components, 2)
	assert.InDelta(t, 150.0, *p.Components[0].Grams, 1e-9)
	assert.InDelta(t, 1.5, *p.Components[1].Portions, 1e-9)
	v, err := e.plates.Session(e.ctx, *p.Components[1].CookSessionID)
	require.NoError(t, err)
	assert.InDelta(t, 3.0, v.Portions, 1e-9, "1.5 per eater × 2 eaters")

	for name, input := range map[string]string{
		"leaf without amount":  fmt.Sprintf(`{"plate_id":%d,"food_id":%d}`, plateID, e.rice),
		"leaf without unit":    fmt.Sprintf(`{"plate_id":%d,"food_id":%d,"amount":150}`, plateID, e.rice),
		"leaf with portions":   fmt.Sprintf(`{"plate_id":%d,"food_id":%d,"portions":1}`, plateID, e.rice),
		"composed with amount": fmt.Sprintf(`{"plate_id":%d,"food_id":%d,"amount":150,"unit":"g"}`, plateID, e.chili),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := e.exec(t, "add_food_to_plate", input)
			assert.ErrorIs(t, err, domain.ErrInvalidInput)
		})
	}
	p, err = e.plates.Get(e.ctx, plateID)
	require.NoError(t, err)
	assert.Len(t, p.Components, 2, "a rejected call writes nothing")
}

func TestTool_UpdatePlateComponent_TakesPerEaterQuantities(t *testing.T) {
	e := newPlateToolEnv(t)
	plateID := e.newPlate(t)
	leaf := e.mustExec(t, "add_food_to_plate", fmt.Sprintf(`{"plate_id":%d,"food_id":%d,"amount":100,"unit":"g"}`, plateID, e.rice))
	composed := e.mustExec(t, "add_food_to_plate", fmt.Sprintf(`{"plate_id":%d,"food_id":%d}`, plateID, e.chili))
	assert.Equal(t, 1.0, composed["portions"], "portions default to 1 per eater")

	got := e.mustExec(t, "update_plate_component", fmt.Sprintf(`{"plate_component_id":%v,"amount":80,"unit":"g"}`, leaf["id"]))
	assert.Equal(t, 80.0, got["grams"])
	got = e.mustExec(t, "update_plate_component", fmt.Sprintf(`{"plate_component_id":%v,"portions":0.5}`, composed["id"]))
	assert.Equal(t, 0.5, got["portions"])

	_, err := e.exec(t, "update_plate_component", fmt.Sprintf(`{"plate_component_id":%v}`, composed["id"]))
	assert.ErrorIs(t, err, domain.ErrInvalidInput, "a quantity is required")
	_, err = e.exec(t, "update_plate_component", fmt.Sprintf(`{"plate_component_id":%v,"portions":2}`, leaf["id"]))
	assert.ErrorIs(t, err, domain.ErrInvalidInput, "a leaf takes amount + unit")
}

func TestTool_SwapFood_TakesPerEaterQuantities(t *testing.T) {
	e := newPlateToolEnv(t)
	plateID := e.newPlate(t)
	composed := e.mustExec(t, "add_food_to_plate", fmt.Sprintf(`{"plate_id":%d,"food_id":%d}`, plateID, e.chili))

	_, err := e.exec(t, "swap_food", fmt.Sprintf(`{"plate_component_id":%v,"new_food_id":%d}`, composed["id"], e.rice))
	assert.ErrorIs(t, err, domain.ErrInvalidInput, "portions do not fit a leaf")

	got := e.mustExec(t, "swap_food", fmt.Sprintf(`{"plate_component_id":%v,"new_food_id":%d,"amount":120,"unit":"g"}`, composed["id"], e.rice))
	assert.Equal(t, float64(e.rice), got["food_id"])
	assert.Equal(t, 120.0, got["grams"])
	assert.NotContains(t, got, "cook_session_id")
}

func TestTool_CreatePlate_EatersAndPlateSummary(t *testing.T) {
	e := newPlateToolEnv(t)
	p := e.mustExec(t, "create_plate", fmt.Sprintf(`{"date":"2026-11-02","slot_id":%d,"eaters":3}`, e.slotID))
	assert.Equal(t, 3.0, p["eaters"])
	e.mustExec(t, "add_food_to_plate", fmt.Sprintf(`{"plate_id":%v,"food_id":%d}`, p["id"], e.chili))
	defaulted := e.mustExec(t, "create_plate", fmt.Sprintf(`{"date":"2026-11-03","slot_id":%d}`, e.slotID))
	assert.Equal(t, 2.0, defaulted["eaters"], "eaters default to the household")

	_, err := e.exec(t, "create_plate", fmt.Sprintf(`{"date":"2026-11-04","slot_id":%d,"eaters":0}`, e.slotID))
	assert.ErrorIs(t, err, domain.ErrInvalidInput)
	_, err = e.exec(t, "create_plate", fmt.Sprintf(`{"date":"2026-11-04","slot_id":%d,"eaters":21}`, e.slotID))
	assert.ErrorIs(t, err, domain.ErrInvalidInput)

	got := e.mustExec(t, "get_plates_range", `{"from":"2026-11-02","to":"2026-11-03"}`)
	plates := got["plates"].([]any)
	require.Len(t, plates, 2)
	first := plates[0].(map[string]any)
	assert.Equal(t, 3.0, first["eaters"])
	comp := first["components"].([]any)[0].(map[string]any)
	assert.Contains(t, comp, "cook_session_id")
	assert.Equal(t, 1.0, comp["portions"])
}

func TestTool_GetProfile_HasHouseholdPortions(t *testing.T) {
	e := newPlateToolEnv(t)
	got := e.mustExec(t, "get_profile", `{}`)
	assert.Equal(t, 2.0, got["household_portions"])
}

func TestPrompt_SaysPortionsPerEater(t *testing.T) {
	prompt := agent.ComposePrompt(nil, nil, nil, nil, nil)
	assert.Contains(t, prompt, "portions per eater")
	assert.Equal(t, "v4", agent.PromptVersion)
}

// B2-AC6: the chat modes and week ids are gone from the agent package.
func TestAgentPackage_HasNoChatModeSymbols(t *testing.T) {
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)
	fset := token.NewFileSet()
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		src, err := os.ReadFile(path)
		require.NoError(t, err)
		f, err := parser.ParseFile(fset, path, src, 0)
		require.NoError(t, err)
		ast.Inspect(f, func(n ast.Node) bool {
			var names []*ast.Ident
			switch d := n.(type) {
			case *ast.Field:
				names = d.Names
			case *ast.ValueSpec:
				names = d.Names
			case *ast.TypeSpec:
				names = []*ast.Ident{d.Name}
			case *ast.FuncDecl:
				names = []*ast.Ident{d.Name}
			}
			for _, id := range names {
				if id.Name == "Mode" || id.Name == "WeekID" {
					t.Errorf("%s: declares %s", fset.Position(id.Pos()), id.Name)
				}
			}
			return true
		})
		assert.NotContains(t, string(src), "modeHint", path)
		assert.NotContains(t, string(src), "toolQuantity", path)
	}
}
