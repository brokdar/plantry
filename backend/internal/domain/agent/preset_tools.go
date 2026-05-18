package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/preset"
)

// presetTools returns the seven preset-related agent tools defined in
// feature.md §6.8. They replace the legacy apply_template tool with a
// proper list/get/create/update/delete/apply/copy_week surface.
func presetTools(svc Services) []Tool {
	return []Tool{
		toolListPresets(svc),
		toolGetPreset(svc),
		toolCreatePresetFromPlates(svc),
		toolUpdatePreset(svc),
		toolDeletePreset(svc),
		toolApplyPreset(svc),
		toolCopyWeek(svc),
	}
}

// PresetService abstracts the preset.Service surface used by agent tools so
// tests can stub it.
type PresetService interface {
	List(ctx context.Context, filter preset.ListFilter) (*preset.ListResult, error)
	Get(ctx context.Context, id int64) (*preset.Preset, error)
	CreateFromPlates(ctx context.Context, in preset.CreateFromPlatesInput) (*preset.Preset, error)
	Patch(ctx context.Context, id int64, in preset.PatchInput) (*preset.Preset, error)
	Delete(ctx context.Context, id int64) error
	Apply(ctx context.Context, id int64, req preset.ApplyRequest) (*preset.ApplyResult, error)
	CopyWeek(ctx context.Context, req preset.CopyWeekRequest) (*preset.ApplyResult, error)
	KnownTags(ctx context.Context, limit int) ([]preset.TagUsage, error)
}

func toolListPresets(svc Services) Tool {
	schema := json.RawMessage(`{
      "type":"object",
      "properties":{
        "search":{"type":"string"},
        "slot_id":{"type":"integer","minimum":1},
        "tag":{"type":"string"},
        "limit":{"type":"integer","minimum":1,"maximum":100},
        "offset":{"type":"integer","minimum":0}
      },
      "additionalProperties":false
    }`)
	return Tool{
		Name:        "list_presets",
		Description: "List the user's saved presets, optionally filtered by name, slot_id, or tag. Returns id, name, plate count, slot_ids, tags, last_used_at, plus the top tags by frequency so you can prefer existing tags over coining duplicates.",
		Schema:      schema,
		Handler: func(ctx context.Context, input json.RawMessage) (json.RawMessage, ToolEffect, error) {
			var in struct {
				Search string `json:"search"`
				SlotID int64  `json:"slot_id"`
				Tag    string `json:"tag"`
				Limit  int    `json:"limit"`
				Offset int    `json:"offset"`
			}
			if err := json.Unmarshal(input, &in); err != nil {
				return nil, ToolEffectNone, fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
			}
			filter := preset.ListFilter{
				Search: in.Search,
				Limit:  in.Limit,
				Offset: in.Offset,
				Sort:   preset.SortName,
			}
			if in.SlotID > 0 {
				filter.SlotIDs = []int64{in.SlotID}
			}
			if in.Tag != "" {
				filter.Tags = []string{in.Tag}
			}
			res, err := svc.Presets.List(ctx, filter)
			if err != nil {
				return nil, ToolEffectNone, err
			}
			known, err := svc.Presets.KnownTags(ctx, 20)
			if err != nil {
				return nil, ToolEffectNone, err
			}
			items := make([]map[string]any, len(res.Items))
			for i, p := range res.Items {
				items[i] = presetSummary(&p)
			}
			knownOut := make([]map[string]any, len(known))
			for i, kt := range known {
				knownOut[i] = map[string]any{"tag": kt.Tag, "count": kt.Count}
			}
			return mustJSON(map[string]any{
				"presets":    items,
				"total":      res.Total,
				"known_tags": knownOut,
			}), ToolEffectNone, nil
		},
	}
}

func toolGetPreset(svc Services) Tool {
	schema := json.RawMessage(`{
      "type":"object",
      "required":["preset_id"],
      "properties":{
        "preset_id":{"type":"integer","minimum":1}
      },
      "additionalProperties":false
    }`)
	return Tool{
		Name:        "get_preset",
		Description: "Return a single preset with its plates and components fully expanded. Call this before apply_preset when you need to know which slots the preset covers or which foods it contains.",
		Schema:      schema,
		Handler: func(ctx context.Context, input json.RawMessage) (json.RawMessage, ToolEffect, error) {
			var in struct {
				PresetID int64 `json:"preset_id"`
			}
			if err := json.Unmarshal(input, &in); err != nil {
				return nil, ToolEffectNone, fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
			}
			p, err := svc.Presets.Get(ctx, in.PresetID)
			if err != nil {
				return nil, ToolEffectNone, err
			}
			return mustJSON(presetDetail(p)), ToolEffectNone, nil
		},
	}
}

func toolCreatePresetFromPlates(svc Services) Tool {
	schema := json.RawMessage(`{
      "type":"object",
      "required":["name","plate_ids"],
      "properties":{
        "name":{"type":"string","minLength":1},
        "plate_ids":{"type":"array","items":{"type":"integer","minimum":1},"minItems":1},
        "tags":{"type":"array","items":{"type":"string","minLength":1}}
      },
      "additionalProperties":false
    }`)
	return Tool{
		Name:        "create_preset_from_plates",
		Description: "Create a new preset from one or more existing planner plates. Each plate becomes a PresetPlate bound to its slot, with components copied verbatim. Prefer tags already returned by list_presets' known_tags over coining near-duplicates.",
		Schema:      schema,
		Handler: func(ctx context.Context, input json.RawMessage) (json.RawMessage, ToolEffect, error) {
			var in preset.CreateFromPlatesInput
			if err := json.Unmarshal(input, &in); err != nil {
				return nil, ToolEffectNone, fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
			}
			p, err := svc.Presets.CreateFromPlates(ctx, in)
			if err != nil {
				return nil, ToolEffectNone, err
			}
			return mustJSON(presetSummary(p)), ToolEffectNone, nil
		},
	}
}

func toolUpdatePreset(svc Services) Tool {
	schema := json.RawMessage(`{
      "type":"object",
      "required":["preset_id"],
      "properties":{
        "preset_id":{"type":"integer","minimum":1},
        "name":{"type":"string","minLength":1},
        "add_tags":{"type":"array","items":{"type":"string","minLength":1}},
        "remove_tags":{"type":"array","items":{"type":"string","minLength":1}}
      },
      "additionalProperties":false
    }`)
	return Tool{
		Name:        "update_preset",
		Description: "Rename a preset and/or add/remove tags. Plate and component edits inside a preset are not exposed to the agent in v1 — use create_preset_from_plates to capture a new variant instead.",
		Schema:      schema,
		Handler: func(ctx context.Context, input json.RawMessage) (json.RawMessage, ToolEffect, error) {
			var in struct {
				PresetID   int64    `json:"preset_id"`
				Name       *string  `json:"name,omitempty"`
				AddTags    []string `json:"add_tags,omitempty"`
				RemoveTags []string `json:"remove_tags,omitempty"`
			}
			if err := json.Unmarshal(input, &in); err != nil {
				return nil, ToolEffectNone, fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
			}
			p, err := svc.Presets.Patch(ctx, in.PresetID, preset.PatchInput{
				Name: in.Name, AddTags: in.AddTags, RemoveTags: in.RemoveTags,
			})
			if err != nil {
				return nil, ToolEffectNone, err
			}
			return mustJSON(presetSummary(p)), ToolEffectNone, nil
		},
	}
}

func toolDeletePreset(svc Services) Tool {
	schema := json.RawMessage(`{
      "type":"object",
      "required":["preset_id"],
      "properties":{
        "preset_id":{"type":"integer","minimum":1}
      },
      "additionalProperties":false
    }`)
	return Tool{
		Name:        "delete_preset",
		Description: "Delete a preset by id. Already-applied plates on the planner are not affected.",
		Schema:      schema,
		Handler: func(ctx context.Context, input json.RawMessage) (json.RawMessage, ToolEffect, error) {
			var in struct {
				PresetID int64 `json:"preset_id"`
			}
			if err := json.Unmarshal(input, &in); err != nil {
				return nil, ToolEffectNone, fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
			}
			if err := svc.Presets.Delete(ctx, in.PresetID); err != nil {
				return nil, ToolEffectNone, err
			}
			return mustJSON(map[string]any{"deleted": true}), ToolEffectNone, nil
		},
	}
}

func toolApplyPreset(svc Services) Tool {
	schema := json.RawMessage(`{
      "type":"object",
      "required":["preset_id","target_date"],
      "properties":{
        "preset_id":{"type":"integer","minimum":1},
        "target_date":{"type":"string","pattern":"^[0-9]{4}-[0-9]{2}-[0-9]{2}$"},
        "on_conflict":{"type":"string","enum":["skip","overwrite"]},
        "slot_ids_filter":{"type":"array","items":{"type":"integer","minimum":1}}
      },
      "additionalProperties":false
    }`)
	return Tool{
		Name:        "apply_preset",
		Description: "Apply a preset's plates onto target_date. Default on_conflict is 'skip' — leaves existing plates untouched. Use 'overwrite' only when the user explicitly asks to replace / rebuild / wipe. If the user's request targets a specific slot, set slot_ids_filter so multi-plate presets only land the relevant plate. The result enumerates created, replaced, skipped_occupied, and skipped_no_slot — surface any non-empty skip list to the user.",
		Schema:      schema,
		Handler: func(ctx context.Context, input json.RawMessage) (json.RawMessage, ToolEffect, error) {
			var in struct {
				PresetID      int64   `json:"preset_id"`
				TargetDate    string  `json:"target_date"`
				OnConflict    string  `json:"on_conflict"`
				SlotIDsFilter []int64 `json:"slot_ids_filter"`
			}
			if err := json.Unmarshal(input, &in); err != nil {
				return nil, ToolEffectNone, fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
			}
			date, err := time.Parse("2006-01-02", in.TargetDate)
			if err != nil {
				return nil, ToolEffectNone, fmt.Errorf("%w: target_date: %v", domain.ErrInvalidInput, err)
			}
			result, err := svc.Presets.Apply(ctx, in.PresetID, preset.ApplyRequest{
				TargetDate:    date,
				OnConflict:    preset.ConflictMode(in.OnConflict),
				SlotIDsFilter: in.SlotIDsFilter,
			})
			if err != nil {
				return nil, ToolEffectNone, err
			}
			return mustJSON(applyResultSummary(result)), ToolEffectPlateChanged, nil
		},
	}
}

func toolCopyWeek(svc Services) Tool {
	schema := json.RawMessage(`{
      "type":"object",
      "required":["source_start","target_start"],
      "properties":{
        "source_start":{"type":"string","pattern":"^[0-9]{4}-[0-9]{2}-[0-9]{2}$"},
        "target_start":{"type":"string","pattern":"^[0-9]{4}-[0-9]{2}-[0-9]{2}$"},
        "on_conflict":{"type":"string","enum":["skip","overwrite"]}
      },
      "additionalProperties":false
    }`)
	return Tool{
		Name:        "copy_week",
		Description: "Copy every plate in [source_start, source_start+6 days] onto the corresponding day of [target_start, target_start+6 days]. Use this for 'apply my usual routine' style requests by copying a representative past week. Default on_conflict is 'skip'.",
		Schema:      schema,
		Handler: func(ctx context.Context, input json.RawMessage) (json.RawMessage, ToolEffect, error) {
			var in struct {
				SourceStart string `json:"source_start"`
				TargetStart string `json:"target_start"`
				OnConflict  string `json:"on_conflict"`
			}
			if err := json.Unmarshal(input, &in); err != nil {
				return nil, ToolEffectNone, fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
			}
			src, err := time.Parse("2006-01-02", in.SourceStart)
			if err != nil {
				return nil, ToolEffectNone, fmt.Errorf("%w: source_start: %v", domain.ErrInvalidInput, err)
			}
			tgt, err := time.Parse("2006-01-02", in.TargetStart)
			if err != nil {
				return nil, ToolEffectNone, fmt.Errorf("%w: target_start: %v", domain.ErrInvalidInput, err)
			}
			result, err := svc.Presets.CopyWeek(ctx, preset.CopyWeekRequest{
				SourceStart: src,
				TargetStart: tgt,
				OnConflict:  preset.ConflictMode(in.OnConflict),
			})
			if err != nil {
				return nil, ToolEffectNone, err
			}
			return mustJSON(applyResultSummary(result)), ToolEffectPlateChanged, nil
		},
	}
}

func presetSummary(p *preset.Preset) map[string]any {
	slotIDs := make([]int64, 0, len(p.Plates))
	seen := map[int64]struct{}{}
	for _, plate := range p.Plates {
		if _, ok := seen[plate.SlotID]; ok {
			continue
		}
		seen[plate.SlotID] = struct{}{}
		slotIDs = append(slotIDs, plate.SlotID)
	}
	out := map[string]any{
		"id":          p.ID,
		"name":        p.Name,
		"plate_count": len(p.Plates),
		"slot_ids":    slotIDs,
		"tags":        p.Tags,
	}
	if p.LastUsedAt != nil {
		out["last_used_at"] = p.LastUsedAt.UTC().Format(time.RFC3339)
	}
	return out
}

func presetDetail(p *preset.Preset) map[string]any {
	plates := make([]map[string]any, len(p.Plates))
	for i, plate := range p.Plates {
		comps := make([]map[string]any, len(plate.Components))
		for j, c := range plate.Components {
			cm := map[string]any{
				"food_id":    c.FoodID,
				"sort_order": c.SortOrder,
			}
			if c.Portions != nil {
				cm["portions"] = *c.Portions
			}
			if c.Amount != nil {
				cm["amount"] = *c.Amount
			}
			if c.Unit != nil {
				cm["unit"] = *c.Unit
			}
			if c.Grams != nil {
				cm["grams"] = *c.Grams
			}
			if c.Note != nil {
				cm["note"] = *c.Note
			}
			comps[j] = cm
		}
		plates[i] = map[string]any{
			"slot_id":    plate.SlotID,
			"sort_order": plate.SortOrder,
			"components": comps,
		}
	}
	out := presetSummary(p)
	out["plates"] = plates
	return out
}

func applyResultSummary(r *preset.ApplyResult) map[string]any {
	createdIDs := make([]int64, len(r.Created))
	for i, p := range r.Created {
		createdIDs[i] = p.ID
	}
	replaced := make([]map[string]any, len(r.Replaced))
	for i, ri := range r.Replaced {
		replaced[i] = map[string]any{
			"new_plate_id": ri.NewPlate.ID,
			"old_plate_id": ri.OldPlate.ID,
		}
	}
	skippedOcc := make([]map[string]any, len(r.SkippedOccupied))
	for i, s := range r.SkippedOccupied {
		skippedOcc[i] = map[string]any{
			"date":    s.Date.Format("2006-01-02"),
			"slot_id": s.SlotID,
		}
	}
	skippedNoSlot := make([]map[string]any, len(r.SkippedNoSlot))
	for i, s := range r.SkippedNoSlot {
		skippedNoSlot[i] = map[string]any{
			"slot_id": s.SlotID,
		}
	}
	return map[string]any{
		"created_plate_ids": createdIDs,
		"replaced":          replaced,
		"skipped_occupied":  skippedOcc,
		"skipped_no_slot":   skippedNoSlot,
		"effect":            "plate_changed",
	}
}
