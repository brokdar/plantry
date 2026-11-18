package agent

import (
	"encoding/json"
	"testing"
)

// toolsByName returns the default tool list keyed by tool name using nil
// services. Handlers are not invoked in these tests — only metadata is checked.
func toolsByName() map[string]Tool {
	tools := defaultTools(Services{})
	m := make(map[string]Tool, len(tools))
	for _, t := range tools {
		m[t.Name] = t
	}
	return m
}

func schemaOf(t *testing.T, name string) map[string]any {
	t.Helper()
	tool, ok := toolsByName()[name]
	if !ok {
		t.Fatalf("%s tool is not registered", name)
	}
	var schema map[string]any
	if err := json.Unmarshal(tool.Schema, &schema); err != nil {
		t.Fatalf("schema is not valid JSON: %v", err)
	}
	return schema
}

// opItemProps returns the properties of one apply_changes op.
func opItemProps(t *testing.T) map[string]any {
	t.Helper()
	props := schemaOf(t, "apply_changes")["properties"].(map[string]any)
	ops := props["ops"].(map[string]any)
	return ops["items"].(map[string]any)["properties"].(map[string]any)
}

func TestTool_ApplyChanges_Schema(t *testing.T) {
	schema := schemaOf(t, "apply_changes")
	props := schema["properties"].(map[string]any)
	for _, k := range []string{"ops", "dry_run", "goal", "light"} {
		if _, ok := props[k]; !ok {
			t.Errorf("apply_changes schema missing %q", k)
		}
	}
	// A call that only sets a light mark has no op: ops is optional (the
	// handler asks for it when neither ops nor light is given).
	if required, _ := schema["required"].([]any); len(required) != 0 {
		t.Errorf("apply_changes requires nothing in the schema; got %v", required)
	}
	item := opItemProps(t)
	enum := item["op"].(map[string]any)["enum"].([]any)
	want := []string{"add", "swap", "set_quantity", "remove", "create_plate", "delete_plate", "set_portions", "set_eaters", "set_availability", "move", "replace", "away", "move_cook", "copy"}
	if len(enum) != len(want) {
		t.Fatalf("op enum = %v, want %v", enum, want)
	}
	for i, w := range want {
		if enum[i] != w {
			t.Errorf("op enum[%d] = %v, want %s", i, enum[i], w)
		}
	}
	for _, k := range []string{"date", "slot_id", "session_id", "cook_at"} {
		if _, ok := item[k]; !ok {
			t.Errorf("op schema missing %q", k)
		}
	}
	if _, ok := item["week_id"]; ok {
		t.Error("op schema must not contain 'week_id'")
	}
}

func TestTool_FindFoods_Schema(t *testing.T) {
	props := schemaOf(t, "find_foods")["properties"].(map[string]any)
	for _, k := range []string{"roles", "kinds", "ids", "no_cook", "diet", "tags", "search", "max_kcal", "min_protein", "sort", "limit"} {
		if _, ok := props[k]; !ok {
			t.Errorf("find_foods schema missing %q", k)
		}
	}
}

func TestTool_RemovedToolsAreGone(t *testing.T) {
	tools := toolsByName()
	for _, name := range []string{
		"list_foods", "get_food", "get_plates_range", "create_plate", "add_food_to_plate", "swap_food",
		"update_plate_component", "remove_plate_component", "delete_plate", "set_session_portions",
		"set_plate_eaters", "set_availability",
	} {
		if _, ok := tools[name]; ok {
			t.Errorf("%s was replaced by apply_changes / find_foods / get_plan", name)
		}
	}
}

func TestTool_GetWeek_NotExposed(t *testing.T) {
	tools := toolsByName()
	if _, ok := tools["get_week"]; ok {
		t.Error("get_week must not be exposed in defaultTools")
	}
}

// apply_template was removed in the Presets cutover (phase 6); applying
// reusable plate bundles is now covered by apply_preset in preset_tools.go.
