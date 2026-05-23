package preset_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/preset"
)

// --- stubs ---

type stubRepo struct {
	created   *preset.Preset
	creating  error
	createErr error
	storage   map[int64]*preset.Preset

	// optional overrides; when nil the defaults apply
	getFn func(ctx context.Context, id int64) (*preset.Preset, error)

	// call recorders
	addTagCalls    []string
	removeTagCalls []string
	createCount    int
	updateNameCnt  int
}

func (r *stubRepo) Create(_ context.Context, p *preset.Preset) error {
	if r.createErr != nil {
		return r.createErr
	}
	if r.storage == nil {
		r.storage = map[int64]*preset.Preset{}
	}
	p.ID = int64(len(r.storage) + 1)
	r.storage[p.ID] = p
	r.created = p
	r.createCount++
	return nil
}

func (r *stubRepo) Get(ctx context.Context, id int64) (*preset.Preset, error) {
	if r.getFn != nil {
		return r.getFn(ctx, id)
	}
	if p, ok := r.storage[id]; ok {
		return p, nil
	}
	return nil, domain.ErrNotFound
}

func (r *stubRepo) List(_ context.Context, _ preset.ListFilter) (*preset.ListResult, error) {
	return &preset.ListResult{Items: nil, Total: 0}, nil
}

func (r *stubRepo) UpdateName(_ context.Context, _ int64, _ string) (*preset.Preset, error) {
	r.updateNameCnt++
	return nil, nil
}
func (r *stubRepo) ReplacePlates(_ context.Context, _ int64, _ []preset.Plate) error { return nil }
func (r *stubRepo) ReplaceTags(_ context.Context, _ int64, _ []string) error         { return nil }
func (r *stubRepo) AddTag(_ context.Context, _ int64, tag string) error {
	r.addTagCalls = append(r.addTagCalls, tag)
	return nil
}

func (r *stubRepo) RemoveTag(_ context.Context, _ int64, tag string) error {
	r.removeTagCalls = append(r.removeTagCalls, tag)
	return nil
}
func (r *stubRepo) TouchLastUsed(_ context.Context, _ int64) error { return nil }
func (r *stubRepo) Delete(_ context.Context, _ int64) error        { return nil }
func (r *stubRepo) KnownTags(_ context.Context, _ int) ([]preset.TagUsage, error) {
	return nil, nil
}
func (r *stubRepo) CountUsingFood(_ context.Context, _ int64) (int64, error) { return 0, nil }

type stubFoodLookup struct {
	kinds map[int64]string
}

func (s *stubFoodLookup) Exists(_ context.Context, foodID int64) (bool, error) {
	_, ok := s.kinds[foodID]
	return ok, nil
}

func (s *stubFoodLookup) KindOf(_ context.Context, foodID int64) (string, error) {
	if k, ok := s.kinds[foodID]; ok {
		return k, nil
	}
	return "", domain.ErrNotFound
}

type stubFoodGetter struct {
	foods map[int64]*food.Food
}

func (s *stubFoodGetter) Get(_ context.Context, id int64) (*food.Food, error) {
	if f, ok := s.foods[id]; ok {
		return f, nil
	}
	return nil, domain.ErrNotFound
}

type stubPortionLookup struct{}

func (stubPortionLookup) ListPortions(_ context.Context, _ int64) ([]food.Portion, error) {
	return nil, nil
}

type stubPlateService struct {
	plates map[int64]*plate.Plate
}

func (s *stubPlateService) Get(_ context.Context, id int64) (*plate.Plate, error) {
	if p, ok := s.plates[id]; ok {
		return p, nil
	}
	return nil, domain.ErrNotFound
}

type stubTxRunner struct{}

func (stubTxRunner) RunInPresetTx(_ context.Context, _ func(preset.Repository, plate.Repository) error) error {
	return nil
}

func makeService(foods map[int64]*food.Food, plates map[int64]*plate.Plate) (*preset.Service, *stubRepo) {
	repo := &stubRepo{}
	kinds := make(map[int64]string, len(foods))
	for id, f := range foods {
		kinds[id] = string(f.Kind)
	}
	return preset.NewService(repo, &stubFoodLookup{kinds: kinds}, &stubPlateService{plates: plates}, stubTxRunner{}, stubPortionLookup{}, &stubFoodGetter{foods: foods}), repo
}

// --- tests ---

func TestNormalizeTag(t *testing.T) {
	cases := map[string]string{
		"Quick":        "quick",
		"  Vegan  ":    "vegan",
		"":             "",
		"\t\n":         "",
		"HIGH-PROTEIN": "high-protein",
	}
	for in, want := range cases {
		assert.Equal(t, want, preset.NormalizeTag(in), "input %q", in)
	}
}

func TestCreateFromPlates_RequiresName(t *testing.T) {
	svc, _ := makeService(nil, nil)
	_, err := svc.CreateFromPlates(context.Background(), preset.CreateFromPlatesInput{PlateIDs: []int64{1}})
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrInvalidInput))
}

func TestCreateFromPlates_RequiresPlateIDs(t *testing.T) {
	svc, _ := makeService(nil, nil)
	_, err := svc.CreateFromPlates(context.Background(), preset.CreateFromPlatesInput{Name: "X"})
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrInvalidInput))
}

func TestCreateFromPlates_ComposedCopy(t *testing.T) {
	portions := 2
	foods := map[int64]*food.Food{
		7: {ID: 7, Kind: food.KindComposed, Name: "Pasta"},
	}
	plates := map[int64]*plate.Plate{
		11: {
			ID:     11,
			SlotID: 3,
			Components: []plate.PlateComponent{
				{FoodID: 7, Portions: &portions, SortOrder: 0},
			},
		},
	}
	svc, repo := makeService(foods, plates)
	got, err := svc.CreateFromPlates(context.Background(), preset.CreateFromPlatesInput{
		Name: "My Pasta", Tags: []string{"Italian", "italian"}, PlateIDs: []int64{11},
	})
	require.NoError(t, err)
	require.NotNil(t, repo.created)
	assert.Equal(t, "My Pasta", got.Name)
	assert.Equal(t, []string{"italian"}, got.Tags)
	require.Len(t, got.Plates, 1)
	assert.Equal(t, int64(3), got.Plates[0].SlotID)
	require.Len(t, got.Plates[0].Components, 1)
	c := got.Plates[0].Components[0]
	require.NotNil(t, c.Portions)
	assert.Equal(t, 2, *c.Portions)
	assert.Nil(t, c.Amount)
}

func TestCreateFromPlates_LeafCopyResolvesGrams(t *testing.T) {
	amount := 150.0
	unit := "g"
	grams := 150.0
	src := "direct"
	foods := map[int64]*food.Food{
		9: {ID: 9, Kind: food.KindLeaf, Name: "Chicken"},
	}
	plates := map[int64]*plate.Plate{
		20: {
			ID:     20,
			SlotID: 1,
			Components: []plate.PlateComponent{
				{FoodID: 9, Amount: &amount, Unit: &unit, Grams: &grams, GramsSource: &src},
			},
		},
	}
	svc, _ := makeService(foods, plates)
	got, err := svc.CreateFromPlates(context.Background(), preset.CreateFromPlatesInput{
		Name: "Chicken", PlateIDs: []int64{20},
	})
	require.NoError(t, err)
	require.Len(t, got.Plates[0].Components, 1)
	c := got.Plates[0].Components[0]
	require.NotNil(t, c.Amount)
	assert.Equal(t, 150.0, *c.Amount)
	require.NotNil(t, c.Grams)
	assert.Equal(t, 150.0, *c.Grams)
}

func TestCreateFromPlates_MultiPlate_OrdersByDateThenSlot(t *testing.T) {
	portions := 1
	foods := map[int64]*food.Food{
		1: {ID: 1, Kind: food.KindComposed, Name: "X"},
	}
	// pl3 (early date), pl1 (late date), pl2 (mid date) — should sort to pl3, pl2, pl1.
	plates := map[int64]*plate.Plate{
		1: {ID: 1, SlotID: 2, Date: dayUTC(2026, 5, 3), Components: []plate.PlateComponent{{FoodID: 1, Portions: &portions}}},
		2: {ID: 2, SlotID: 1, Date: dayUTC(2026, 5, 2), Components: []plate.PlateComponent{{FoodID: 1, Portions: &portions}}},
		3: {ID: 3, SlotID: 1, Date: dayUTC(2026, 5, 1), Components: []plate.PlateComponent{{FoodID: 1, Portions: &portions}}},
	}
	svc, _ := makeService(foods, plates)
	got, err := svc.CreateFromPlates(context.Background(), preset.CreateFromPlatesInput{
		Name: "Three", PlateIDs: []int64{1, 2, 3},
	})
	require.NoError(t, err)
	require.Len(t, got.Plates, 3)
	assert.Equal(t, int64(1), got.Plates[0].SlotID, "first plate slot — earliest date")
	assert.Equal(t, int64(1), got.Plates[1].SlotID)
	assert.Equal(t, int64(2), got.Plates[2].SlotID)
}

func dayUTC(y, m, d int) time.Time {
	return time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
}

// --- Update tests ---

func ptr[T any](v T) *T { return &v }

// seedExistingPreset directly registers a preset into the stub repo's storage
// without going through repo.Create (which would invoke the validate path).
func seedExistingPreset(r *stubRepo, p *preset.Preset) {
	if r.storage == nil {
		r.storage = map[int64]*preset.Preset{}
	}
	if p.ID == 0 {
		p.ID = int64(len(r.storage) + 1)
	}
	r.storage[p.ID] = p
}

func TestUpdate_EmptyName_IsRejected(t *testing.T) {
	svc, repo := makeService(nil, nil)
	seedExistingPreset(repo, &preset.Preset{ID: 1, Name: "Existing", Tags: []string{}})

	_, err := svc.Update(context.Background(), 1, preset.UpdateInput{Name: ptr("")})
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrInvalidInput))
	// No tx work should have occurred.
	assert.Zero(t, repo.updateNameCnt)
}

func TestUpdate_ZeroSlotID_IsRejected(t *testing.T) {
	foods := map[int64]*food.Food{
		7: {ID: 7, Kind: food.KindComposed, Name: "Pasta"},
	}
	svc, repo := makeService(foods, nil)
	seedExistingPreset(repo, &preset.Preset{ID: 1, Name: "Existing", Tags: []string{}})

	portions := 1
	badPlates := []preset.Plate{{
		SlotID: 0, // invalid
		Components: []preset.Component{
			{FoodID: 7, Portions: &portions},
		},
	}}
	_, err := svc.Update(context.Background(), 1, preset.UpdateInput{Plates: &badPlates})
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrInvalidInput))
}

func TestUpdate_PresetNotFound_IsRejected(t *testing.T) {
	svc, repo := makeService(nil, nil)
	repo.getFn = func(_ context.Context, _ int64) (*preset.Preset, error) {
		return nil, domain.ErrNotFound
	}
	_, err := svc.Update(context.Background(), 42, preset.UpdateInput{Name: ptr("X")})
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrNotFound))
}

// --- Patch tests ---

func TestPatch_BlankName_IsRejected(t *testing.T) {
	svc, repo := makeService(nil, nil)
	seedExistingPreset(repo, &preset.Preset{ID: 1, Name: "Existing", Tags: []string{}})

	_, err := svc.Patch(context.Background(), 1, preset.PatchInput{Name: ptr("  ")})
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrInvalidInput))
	assert.Zero(t, repo.updateNameCnt)
}

func TestPatch_PresetNotFound(t *testing.T) {
	svc, repo := makeService(nil, nil)
	repo.getFn = func(_ context.Context, _ int64) (*preset.Preset, error) {
		return nil, domain.ErrNotFound
	}
	_, err := svc.Patch(context.Background(), 42, preset.PatchInput{AddTags: []string{"quick"}})
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrNotFound))
	assert.Empty(t, repo.addTagCalls)
}

func TestPatch_TagsNormalised(t *testing.T) {
	svc, repo := makeService(nil, nil)
	seedExistingPreset(repo, &preset.Preset{ID: 1, Name: "Existing", Tags: []string{}})

	_, err := svc.Patch(context.Background(), 1, preset.PatchInput{
		AddTags: []string{"QUICK", " Vegan "},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"quick", "vegan"}, repo.addTagCalls)
}

func TestPatch_BlankAddTag_IsSkipped(t *testing.T) {
	svc, repo := makeService(nil, nil)
	seedExistingPreset(repo, &preset.Preset{ID: 1, Name: "Existing", Tags: []string{}})

	_, err := svc.Patch(context.Background(), 1, preset.PatchInput{AddTags: []string{"  "}})
	require.NoError(t, err)
	assert.Empty(t, repo.addTagCalls)
}

// --- Duplicate tests ---

func TestDuplicate_CopiesWithSuffix(t *testing.T) {
	svc, repo := makeService(nil, nil)
	portions := 2
	last := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	seedExistingPreset(repo, &preset.Preset{
		ID:   1,
		Name: "Pasta",
		Tags: []string{"italian", "quick"},
		Plates: []preset.Plate{{
			SlotID: 3,
			Components: []preset.Component{
				{FoodID: 7, Portions: &portions, SortOrder: 0},
			},
		}},
		LastUsedAt: &last,
	})

	got, err := svc.Duplicate(context.Background(), 1)
	require.NoError(t, err)
	assert.Equal(t, "Pasta (copy)", got.Name)
	assert.Equal(t, []string{"italian", "quick"}, got.Tags)
	require.Len(t, got.Plates, 1)
	assert.Equal(t, int64(3), got.Plates[0].SlotID)
	require.Len(t, got.Plates[0].Components, 1)
	c := got.Plates[0].Components[0]
	require.NotNil(t, c.Portions)
	assert.Equal(t, 2, *c.Portions)
	assert.Nil(t, got.LastUsedAt)
}

func TestDuplicate_NotFound(t *testing.T) {
	svc, repo := makeService(nil, nil)
	repo.getFn = func(_ context.Context, _ int64) (*preset.Preset, error) {
		return nil, domain.ErrNotFound
	}
	_, err := svc.Duplicate(context.Background(), 99)
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrNotFound))
	assert.Zero(t, repo.createCount)
}
