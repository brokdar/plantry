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

func TestTool_GetPlatesRange_Callable(t *testing.T) {
	tools := toolsByName()

	tool, ok := tools["get_plates_range"]
	if !ok {
		t.Fatal("get_plates_range tool is not registered")
	}

	var schema map[string]any
	if err := json.Unmarshal(tool.Schema, &schema); err != nil {
		t.Fatalf("schema is not valid JSON: %v", err)
	}

	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("schema missing 'properties' object")
	}
	if _, hasFrom := props["from"]; !hasFrom {
		t.Error("get_plates_range schema missing 'from' parameter")
	}
	if _, hasTo := props["to"]; !hasTo {
		t.Error("get_plates_range schema missing 'to' parameter")
	}
}

func TestTool_CreatePlate_AcceptsDate(t *testing.T) {
	tools := toolsByName()

	tool, ok := tools["create_plate"]
	if !ok {
		t.Fatal("create_plate tool is not registered")
	}

	var schema map[string]any
	if err := json.Unmarshal(tool.Schema, &schema); err != nil {
		t.Fatalf("schema is not valid JSON: %v", err)
	}

	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("schema missing 'properties' object")
	}
	if _, hasDate := props["date"]; !hasDate {
		t.Error("create_plate schema missing 'date' parameter")
	}
}

func TestTool_GetWeek_NotExposed(t *testing.T) {
	tools := toolsByName()
	if _, ok := tools["get_week"]; ok {
		t.Error("get_week must not be exposed in defaultTools")
	}
}

func TestTool_CreatePlate_RequiresDate(t *testing.T) {
	tools := toolsByName()
	tool, ok := tools["create_plate"]
	if !ok {
		t.Fatal("create_plate tool is not registered")
	}
	var schema map[string]any
	if err := json.Unmarshal(tool.Schema, &schema); err != nil {
		t.Fatalf("schema is not valid JSON: %v", err)
	}
	required, ok := schema["required"].([]any)
	if !ok {
		t.Fatal("schema missing 'required' array")
	}
	found := false
	for _, r := range required {
		if r == "date" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("create_plate schema required array must contain 'date'; got %v", required)
	}
}

func TestTool_CreatePlate_NoWeekIdProp(t *testing.T) {
	tools := toolsByName()
	tool, ok := tools["create_plate"]
	if !ok {
		t.Fatal("create_plate tool is not registered")
	}
	var schema map[string]any
	if err := json.Unmarshal(tool.Schema, &schema); err != nil {
		t.Fatalf("schema is not valid JSON: %v", err)
	}
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("schema missing 'properties' object")
	}
	if _, hasWeekID := props["week_id"]; hasWeekID {
		t.Error("create_plate schema must not contain 'week_id' property")
	}
}

func TestTool_ApplyTemplate_Callable(t *testing.T) {
	tools := toolsByName()
	tool, ok := tools["apply_template"]
	if !ok {
		t.Fatal("apply_template tool is not registered")
	}
	var schema map[string]any
	if err := json.Unmarshal(tool.Schema, &schema); err != nil {
		t.Fatalf("schema is not valid JSON: %v", err)
	}
	required, ok := schema["required"].([]any)
	if !ok {
		t.Fatal("schema missing 'required' array")
	}
	want := map[string]bool{"template_id": true, "start_date": true, "slot_id": true}
	for _, r := range required {
		delete(want, r.(string))
	}
	if len(want) > 0 {
		missing := make([]string, 0, len(want))
		for k := range want {
			missing = append(missing, k)
		}
		t.Errorf("apply_template schema required missing: %v", missing)
	}
}
