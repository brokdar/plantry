package agent

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/actor"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/preset"
)

// stubPresetSvc is a no-op PresetService used to register preset tools so we
// can introspect their schemas without spinning up a real service.
type stubPresetSvc struct {
	listFn      func(ctx context.Context, f preset.ListFilter) (*preset.ListResult, error)
	getFn       func(ctx context.Context, id int64) (*preset.Preset, error)
	createFn    func(ctx context.Context, in preset.CreateFromPlatesInput) (*preset.Preset, error)
	patchFn     func(ctx context.Context, id int64, in preset.PatchInput) (*preset.Preset, error)
	deleteFn    func(ctx context.Context, id int64) error
	applyFn     func(ctx context.Context, id int64, req preset.ApplyRequest) (*preset.ApplyResult, error)
	copyWeekFn  func(ctx context.Context, req preset.CopyWeekRequest) (*preset.ApplyResult, error)
	knownTagsFn func(ctx context.Context, limit int) ([]preset.TagUsage, error)
}

func (s *stubPresetSvc) List(ctx context.Context, f preset.ListFilter) (*preset.ListResult, error) {
	if s.listFn != nil {
		return s.listFn(ctx, f)
	}
	return &preset.ListResult{}, nil
}

func (s *stubPresetSvc) Get(ctx context.Context, id int64) (*preset.Preset, error) {
	if s.getFn != nil {
		return s.getFn(ctx, id)
	}
	return &preset.Preset{ID: id}, nil
}

func (s *stubPresetSvc) CreateFromPlates(ctx context.Context, in preset.CreateFromPlatesInput) (*preset.Preset, error) {
	if s.createFn != nil {
		return s.createFn(ctx, in)
	}
	return &preset.Preset{ID: 1, Name: in.Name}, nil
}

func (s *stubPresetSvc) Patch(ctx context.Context, id int64, in preset.PatchInput) (*preset.Preset, error) {
	if s.patchFn != nil {
		return s.patchFn(ctx, id, in)
	}
	return &preset.Preset{ID: id}, nil
}

func (s *stubPresetSvc) Delete(ctx context.Context, id int64) error {
	if s.deleteFn != nil {
		return s.deleteFn(ctx, id)
	}
	return nil
}

func (s *stubPresetSvc) Apply(ctx context.Context, id int64, req preset.ApplyRequest) (*preset.ApplyResult, error) {
	if s.applyFn != nil {
		return s.applyFn(ctx, id, req)
	}
	return &preset.ApplyResult{}, nil
}

func (s *stubPresetSvc) CopyWeek(ctx context.Context, req preset.CopyWeekRequest) (*preset.ApplyResult, error) {
	if s.copyWeekFn != nil {
		return s.copyWeekFn(ctx, req)
	}
	return &preset.ApplyResult{}, nil
}

func (s *stubPresetSvc) KnownTags(ctx context.Context, limit int) ([]preset.TagUsage, error) {
	if s.knownTagsFn != nil {
		return s.knownTagsFn(ctx, limit)
	}
	return nil, nil
}

func TestPresetTools_RegisteredWhenServicePresent(t *testing.T) {
	ts, err := NewToolSet(Services{Presets: &stubPresetSvc{}})
	require.NoError(t, err)

	names := []string{
		"list_presets",
		"get_preset",
		"create_preset_from_plates",
		"update_preset",
		"delete_preset",
		"apply_preset",
		"copy_week",
	}
	for _, name := range names {
		_, ok := ts.byName[name]
		assert.True(t, ok, "tool %s should be registered", name)
	}
}

func TestPresetTools_NotRegisteredWithoutService(t *testing.T) {
	ts, err := NewToolSet(Services{})
	require.NoError(t, err)

	for _, name := range []string{"list_presets", "apply_preset", "copy_week"} {
		_, ok := ts.byName[name]
		assert.False(t, ok, "tool %s should NOT be registered without a preset service", name)
	}
}

func TestApplyPreset_SchemaValidatesTargetDate(t *testing.T) {
	ts, err := NewToolSet(Services{Presets: &stubPresetSvc{}})
	require.NoError(t, err)

	// Bad date pattern should fail schema validation.
	_, _, err = ts.Execute(context.Background(), "apply_preset",
		json.RawMessage(`{"preset_id":1,"target_date":"nope"}`))
	require.Error(t, err)

	// Good date passes.
	out, eff, err := ts.Execute(context.Background(), "apply_preset",
		json.RawMessage(`{"preset_id":1,"target_date":"2026-05-20"}`))
	require.NoError(t, err)
	assert.Equal(t, ToolEffectPlateChanged, eff)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(out, &parsed))
	assert.Equal(t, "plate_changed", parsed["effect"])
}

func TestApplyPreset_OverwriteFlag(t *testing.T) {
	var captured preset.ApplyRequest
	svc := &stubPresetSvc{
		applyFn: func(_ context.Context, _ int64, req preset.ApplyRequest) (*preset.ApplyResult, error) {
			captured = req
			return &preset.ApplyResult{}, nil
		},
	}
	ts, err := NewToolSet(Services{Presets: svc})
	require.NoError(t, err)

	_, _, err = ts.Execute(context.Background(), "apply_preset",
		json.RawMessage(`{"preset_id":1,"target_date":"2026-05-20","on_conflict":"overwrite","slot_ids_filter":[2,3]}`))
	require.NoError(t, err)
	assert.Equal(t, preset.ConflictOverwrite, captured.OnConflict)
	assert.Equal(t, []int64{2, 3}, captured.SlotIDsFilter)
}

// Preset apply and copy week write plates through the plate service, so they
// run as the agent too (D20), over the user actor of the HTTP request.
func TestPresetTools_RunAsAgent(t *testing.T) {
	var got []actor.Actor
	svc := &stubPresetSvc{
		applyFn: func(ctx context.Context, _ int64, _ preset.ApplyRequest) (*preset.ApplyResult, error) {
			got = append(got, actor.From(ctx))
			return &preset.ApplyResult{}, nil
		},
		copyWeekFn: func(ctx context.Context, _ preset.CopyWeekRequest) (*preset.ApplyResult, error) {
			got = append(got, actor.From(ctx))
			return &preset.ApplyResult{}, nil
		},
	}
	ts, err := NewToolSet(Services{Presets: svc})
	require.NoError(t, err)
	ctx := actor.With(context.Background(), actor.User, 0)

	_, _, err = ts.Execute(ctx, "apply_preset", json.RawMessage(`{"preset_id":1,"target_date":"2026-05-20"}`))
	require.NoError(t, err)
	_, _, err = ts.Execute(ctx, "copy_week", json.RawMessage(`{"source_start":"2026-05-11","target_start":"2026-05-18"}`))
	require.NoError(t, err)
	assert.Equal(t, []actor.Actor{{Kind: actor.Agent}, {Kind: actor.Agent}}, got)
}

func TestApplyResultSummary_ReplacedAndSkippedOverrides(t *testing.T) {
	r := &preset.ApplyResult{
		Removed: plate.PlanSnapshot{Plates: []plate.SnapshotPlate{
			{Date: "2026-05-18", SlotID: 3},
			// Not a date: kept as is, never turned into 0001-01-01.
			{Date: "bogus", SlotID: 2},
		}},
		SkippedOverrides: []plate.CellKey{{Date: time.Date(2026, 5, 19, 0, 0, 0, 0, time.UTC), SlotID: 1}},
	}
	var got map[string]any
	require.NoError(t, json.Unmarshal(mustJSON(applyResultSummary(r)), &got))
	assert.Equal(t, []any{
		map[string]any{"date": "2026-05-18", "slot_id": float64(3)},
		map[string]any{"date": "bogus", "slot_id": float64(2)},
	}, got["replaced"])
	assert.Equal(t, []any{map[string]any{"date": "2026-05-19", "slot_id": float64(1)}}, got["skipped_overrides"])
}
