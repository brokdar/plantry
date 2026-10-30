package availability_test

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// --- fakes ---

type overrideKey struct {
	date string
	slot int64
}

type defaultKey struct {
	weekday int
	slot    int64
}

type fakeRepo struct {
	slotOrder []int64
	active    map[int64]bool
	defaults  map[defaultKey]availability.Default
	overrides map[overrideKey]availability.Override
	presets   map[int64]string // used to fill PresetName on read
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		active:    map[int64]bool{},
		defaults:  map[defaultKey]availability.Default{},
		overrides: map[overrideKey]availability.Override{},
		presets:   map[int64]string{},
	}
}

func (r *fakeRepo) addSlot(id int64, active bool) {
	r.slotOrder = append(r.slotOrder, id)
	r.active[id] = active
}

func (r *fakeRepo) name(id *int64) *string {
	if id == nil {
		return nil
	}
	n, ok := r.presets[*id]
	if !ok {
		return nil
	}
	return &n
}

func (r *fakeRepo) ActiveSlotIDs(context.Context) ([]int64, error) {
	var out []int64
	for _, id := range r.slotOrder {
		if r.active[id] {
			out = append(out, id)
		}
	}
	return out, nil
}

func (r *fakeRepo) SlotState(_ context.Context, id int64) (bool, bool, error) {
	a, ok := r.active[id]
	return ok, a, nil
}

func (r *fakeRepo) ListDefaults(context.Context) ([]availability.Default, error) {
	out := make([]availability.Default, 0, len(r.defaults))
	for _, d := range r.defaults {
		d.PresetName = r.name(d.PresetID)
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Weekday != out[j].Weekday {
			return out[i].Weekday < out[j].Weekday
		}
		return out[i].SlotID < out[j].SlotID
	})
	return out, nil
}

func (r *fakeRepo) GetDefault(_ context.Context, weekday int, slotID int64) (*availability.Default, error) {
	d, ok := r.defaults[defaultKey{weekday, slotID}]
	if !ok {
		return nil, nil
	}
	d.PresetName = r.name(d.PresetID)
	return &d, nil
}

func (r *fakeRepo) ReplaceActiveDefaults(_ context.Context, rows []availability.Default) error {
	for k := range r.defaults {
		if r.active[k.slot] {
			delete(r.defaults, k)
		}
	}
	for _, d := range rows {
		r.defaults[defaultKey{d.Weekday, d.SlotID}] = d
	}
	return nil
}

func (r *fakeRepo) ListOverrides(_ context.Context, from, to time.Time) ([]availability.Override, error) {
	var out []availability.Override
	for _, o := range r.overrides {
		if o.Date.Before(from) || o.Date.After(to) {
			continue
		}
		o.PresetName = r.name(o.PresetID)
		out = append(out, o)
	}
	return out, nil
}

func (r *fakeRepo) GetOverride(_ context.Context, date time.Time, slotID int64) (*availability.Override, error) {
	o, ok := r.overrides[overrideKey{date.Format("2006-01-02"), slotID}]
	if !ok {
		return nil, nil
	}
	o.PresetName = r.name(o.PresetID)
	return &o, nil
}

func (r *fakeRepo) UpsertOverride(_ context.Context, o availability.Override) error {
	r.overrides[overrideKey{o.Date.Format("2006-01-02"), o.SlotID}] = o
	return nil
}

func (r *fakeRepo) DeleteOverride(_ context.Context, date time.Time, slotID int64) error {
	delete(r.overrides, overrideKey{date.Format("2006-01-02"), slotID})
	return nil
}

// fakePlates implements availability.CellPlates.
type fakePlates struct {
	plates []plate.Plate
}

func (p *fakePlates) Range(_ context.Context, from, to time.Time) ([]plate.Plate, error) {
	var out []plate.Plate
	for _, pl := range p.plates {
		if !pl.Date.Before(from) && !pl.Date.After(to) {
			out = append(out, pl)
		}
	}
	return out, nil
}

func (p *fakePlates) Delete(_ context.Context, id int64) (plate.PlanSnapshot, error) {
	for i, pl := range p.plates {
		if pl.ID == id {
			p.plates = append(p.plates[:i], p.plates[i+1:]...)
			return plate.PlanSnapshot{}, nil
		}
	}
	return plate.PlanSnapshot{}, domain.ErrNotFound
}

type fakeTx struct {
	calls int
}

func (t *fakeTx) InTx(ctx context.Context, fn func(context.Context) error) error {
	t.calls++
	return fn(ctx)
}

type fakePresets map[int64]string

func (f fakePresets) PresetName(_ context.Context, id int64) (string, bool, error) {
	n, ok := f[id]
	return n, ok, nil
}

// --- helpers ---

const (
	slotBreakfast int64 = 1
	slotLunch     int64 = 2
	slotDinner    int64 = 3
	slotInactive  int64 = 4

	presetOatmeal int64 = 10
)

type fixture struct {
	repo   *fakeRepo
	plates *fakePlates
	tx     *fakeTx
	svc    *availability.Service
}

func newFixture() *fixture {
	repo := newFakeRepo()
	repo.addSlot(slotBreakfast, true)
	repo.addSlot(slotLunch, true)
	repo.addSlot(slotDinner, true)
	repo.addSlot(slotInactive, false)
	repo.presets[presetOatmeal] = "Oatmeal"
	plates := &fakePlates{}
	tx := &fakeTx{}
	svc := availability.NewService(repo, tx, fakePresets{presetOatmeal: "Oatmeal"}).WithPlates(plates)
	return &fixture{repo: repo, plates: plates, tx: tx, svc: svc}
}

func day(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func ptr[T any](v T) *T { return &v }

// 2026-10-26 is a Monday.
const monday = "2026-10-26"

// --- tests ---

func TestAvailabilityWeekdayOf_MondayIsZero(t *testing.T) {
	assert.Equal(t, 0, availability.WeekdayOf(day("2026-10-26")))
	assert.Equal(t, 6, availability.WeekdayOf(day("2026-11-01")))
}

func TestAvailabilityResolve_Precedence(t *testing.T) {
	type level struct {
		mode   availability.Mode // "" = no row
		preset bool
	}
	defaults := []level{{"", false}, {availability.ModeNone, false}, {availability.ModeRoutine, true}}
	overrides := []level{{"", false}, {availability.ModePlan, false}, {availability.ModeNone, false}, {availability.ModeRoutine, true}}

	for _, d := range defaults {
		for _, o := range overrides {
			name := fmt.Sprintf("default=%q/override=%q", d.mode, o.mode)
			t.Run(name, func(t *testing.T) {
				f := newFixture()
				if d.mode != "" {
					row := availability.Default{Weekday: 0, SlotID: slotLunch, Mode: d.mode}
					if d.preset {
						row.PresetID = ptr(presetOatmeal)
					}
					f.repo.defaults[defaultKey{0, slotLunch}] = row
				}
				if o.mode != "" {
					row := availability.Override{Date: day(monday), SlotID: slotLunch, Mode: o.mode}
					if o.preset {
						row.PresetID = ptr(presetOatmeal)
					}
					if o.mode == availability.ModeNone {
						row.Note = ptr("away")
					}
					f.repo.overrides[overrideKey{monday, slotLunch}] = row
				}

				cells, err := f.svc.Resolve(context.Background(), day(monday), day(monday))
				require.NoError(t, err)
				var got *availability.Cell
				for i := range cells {
					if cells[i].SlotID == slotLunch {
						got = &cells[i]
					}
				}
				require.NotNil(t, got)

				wantMode, wantSource, wantPreset := availability.ModePlan, availability.SourceFallback, false
				switch {
				case o.mode != "":
					wantMode, wantSource, wantPreset = o.mode, availability.SourceOverride, o.preset
				case d.mode != "":
					wantMode, wantSource, wantPreset = d.mode, availability.SourceDefault, d.preset
				}
				assert.Equal(t, wantMode, got.Mode)
				assert.Equal(t, wantSource, got.Source)
				if wantPreset {
					require.NotNil(t, got.PresetID)
					assert.Equal(t, presetOatmeal, *got.PresetID)
					require.NotNil(t, got.PresetName)
					assert.Equal(t, "Oatmeal", *got.PresetName)
				} else {
					assert.Nil(t, got.PresetID)
					assert.Nil(t, got.PresetName)
				}
				if o.mode == availability.ModeNone {
					require.NotNil(t, got.Note)
					assert.Equal(t, "away", *got.Note)
				} else {
					assert.Nil(t, got.Note)
				}

				mode, err := f.svc.ModeAt(context.Background(), day(monday), slotLunch)
				require.NoError(t, err)
				assert.Equal(t, wantMode, mode)
				cm, err := f.svc.CellMode(context.Background(), day(monday), slotLunch)
				require.NoError(t, err)
				assert.Equal(t, string(wantMode), cm)
			})
		}
	}
}

func TestAvailabilityResolve_GridShapeAndInactiveSlotExcluded(t *testing.T) {
	f := newFixture()
	f.repo.overrides[overrideKey{monday, slotInactive}] = availability.Override{Date: day(monday), SlotID: slotInactive, Mode: availability.ModeNone}
	// Sunday default applies only on Sunday.
	f.repo.defaults[defaultKey{6, slotDinner}] = availability.Default{Weekday: 6, SlotID: slotDinner, Mode: availability.ModeNone}

	cells, err := f.svc.Resolve(context.Background(), day(monday), day("2026-11-01"))
	require.NoError(t, err)
	require.Len(t, cells, 7*3)
	for _, c := range cells {
		assert.NotEqual(t, slotInactive, c.SlotID)
	}
	assert.Equal(t, day(monday), cells[0].Date)
	assert.Equal(t, slotBreakfast, cells[0].SlotID)
	assert.Equal(t, slotLunch, cells[1].SlotID)
	last := cells[len(cells)-1]
	assert.Equal(t, day("2026-11-01"), last.Date)
	assert.Equal(t, slotDinner, last.SlotID)
	assert.Equal(t, availability.ModeNone, last.Mode)
	assert.Equal(t, availability.ModePlan, cells[2].Mode, "Monday dinner falls back to plan")
}

func TestAvailabilityResolve_ToBeforeFrom(t *testing.T) {
	f := newFixture()
	_, err := f.svc.Resolve(context.Background(), day("2026-10-27"), day(monday))
	assert.ErrorIs(t, err, domain.ErrInvalidInput)
}

func TestAvailabilityDefaults_FullGridOverActiveSlots(t *testing.T) {
	f := newFixture()
	f.repo.defaults[defaultKey{2, slotBreakfast}] = availability.Default{Weekday: 2, SlotID: slotBreakfast, Mode: availability.ModeRoutine, PresetID: ptr(presetOatmeal)}
	f.repo.defaults[defaultKey{2, slotInactive}] = availability.Default{Weekday: 2, SlotID: slotInactive, Mode: availability.ModeNone}

	got, err := f.svc.Defaults(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 7*3)
	for _, d := range got {
		assert.NotEqual(t, slotInactive, d.SlotID)
		if d.Weekday == 2 && d.SlotID == slotBreakfast {
			assert.Equal(t, availability.ModeRoutine, d.Mode)
			require.NotNil(t, d.PresetID)
			assert.Equal(t, presetOatmeal, *d.PresetID)
		} else {
			assert.Equal(t, availability.ModePlan, d.Mode)
			assert.Nil(t, d.PresetID)
		}
	}
	assert.Equal(t, 0, got[0].Weekday)
	assert.Equal(t, slotBreakfast, got[0].SlotID)
	assert.Equal(t, slotLunch, got[1].SlotID)
}

func TestAvailabilitySetDefaults_DropsPlanRowsAndReplaces(t *testing.T) {
	f := newFixture()
	f.repo.defaults[defaultKey{5, slotDinner}] = availability.Default{Weekday: 5, SlotID: slotDinner, Mode: availability.ModeNone}
	f.repo.defaults[defaultKey{5, slotInactive}] = availability.Default{Weekday: 5, SlotID: slotInactive, Mode: availability.ModeNone}

	err := f.svc.SetDefaults(context.Background(), []availability.Default{
		{Weekday: 0, SlotID: slotBreakfast, Mode: availability.ModeRoutine, PresetID: ptr(presetOatmeal)},
		{Weekday: 0, SlotID: slotLunch, Mode: availability.ModePlan},
		{Weekday: 6, SlotID: slotDinner, Mode: availability.ModeNone},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, f.tx.calls)

	assert.Len(t, f.repo.defaults, 3, "plan row dropped, old active row replaced, inactive row kept")
	_, hasPlan := f.repo.defaults[defaultKey{0, slotLunch}]
	assert.False(t, hasPlan)
	_, hasOld := f.repo.defaults[defaultKey{5, slotDinner}]
	assert.False(t, hasOld)
	_, hasInactive := f.repo.defaults[defaultKey{5, slotInactive}]
	assert.True(t, hasInactive)
}

func TestAvailabilitySetDefaults_Validation(t *testing.T) {
	tests := []struct {
		name string
		rows []availability.Default
		want error
	}{
		{"weekday below range", []availability.Default{{Weekday: -1, SlotID: slotLunch, Mode: availability.ModeNone}}, domain.ErrInvalidInput},
		{"weekday above range", []availability.Default{{Weekday: 7, SlotID: slotLunch, Mode: availability.ModeNone}}, domain.ErrInvalidInput},
		{"duplicate cell", []availability.Default{
			{Weekday: 1, SlotID: slotLunch, Mode: availability.ModeNone},
			{Weekday: 1, SlotID: slotLunch, Mode: availability.ModePlan},
		}, domain.ErrInvalidInput},
		{"bad mode", []availability.Default{{Weekday: 1, SlotID: slotLunch, Mode: "away"}}, domain.ErrInvalidInput},
		{"unknown slot", []availability.Default{{Weekday: 1, SlotID: 99, Mode: availability.ModeNone}}, domain.ErrAvailabilitySlotUnknown},
		{"inactive slot", []availability.Default{{Weekday: 1, SlotID: slotInactive, Mode: availability.ModeNone}}, domain.ErrSlotInactive},
		{"routine without preset", []availability.Default{{Weekday: 1, SlotID: slotLunch, Mode: availability.ModeRoutine}}, domain.ErrPresetRequired},
		{"routine unknown preset", []availability.Default{{Weekday: 1, SlotID: slotLunch, Mode: availability.ModeRoutine, PresetID: ptr(int64(99))}}, domain.ErrAvailabilityPresetUnknown},
		{"none with preset", []availability.Default{{Weekday: 1, SlotID: slotLunch, Mode: availability.ModeNone, PresetID: ptr(presetOatmeal)}}, domain.ErrPresetNotAllowed},
		{"plan with preset", []availability.Default{{Weekday: 1, SlotID: slotLunch, Mode: availability.ModePlan, PresetID: ptr(presetOatmeal)}}, domain.ErrPresetNotAllowed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture()
			f.repo.defaults[defaultKey{3, slotDinner}] = availability.Default{Weekday: 3, SlotID: slotDinner, Mode: availability.ModeNone}
			err := f.svc.SetDefaults(context.Background(), tt.rows)
			assert.ErrorIs(t, err, tt.want)
			assert.Len(t, f.repo.defaults, 1, "nothing written on validation error")
		})
	}
}

func TestAvailabilitySetOverride_Validation(t *testing.T) {
	tests := []struct {
		name string
		in   availability.OverrideInput
		want error
	}{
		{"zero date", availability.OverrideInput{SlotID: slotLunch, Mode: availability.ModeNone}, domain.ErrInvalidInput},
		{"bad mode", availability.OverrideInput{Date: day(monday), SlotID: slotLunch, Mode: "away"}, domain.ErrInvalidInput},
		{"empty mode", availability.OverrideInput{Date: day(monday), SlotID: slotLunch}, domain.ErrInvalidInput},
		{"unknown slot", availability.OverrideInput{Date: day(monday), SlotID: 99, Mode: availability.ModeNone}, domain.ErrNotFound},
		{"inactive slot", availability.OverrideInput{Date: day(monday), SlotID: slotInactive, Mode: availability.ModeNone}, domain.ErrSlotInactive},
		{"routine without preset", availability.OverrideInput{Date: day(monday), SlotID: slotLunch, Mode: availability.ModeRoutine}, domain.ErrPresetRequired},
		{"routine unknown preset", availability.OverrideInput{Date: day(monday), SlotID: slotLunch, Mode: availability.ModeRoutine, PresetID: ptr(int64(99))}, domain.ErrAvailabilityPresetUnknown},
		{"none with preset", availability.OverrideInput{Date: day(monday), SlotID: slotLunch, Mode: availability.ModeNone, PresetID: ptr(presetOatmeal)}, domain.ErrPresetNotAllowed},
		{"plan with preset", availability.OverrideInput{Date: day(monday), SlotID: slotLunch, Mode: availability.ModePlan, PresetID: ptr(presetOatmeal)}, domain.ErrPresetNotAllowed},
		{"note too long", availability.OverrideInput{Date: day(monday), SlotID: slotLunch, Mode: availability.ModeNone, Note: ptr(strings.Repeat("x", 201))}, domain.ErrInvalidInput},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture()
			_, _, err := f.svc.SetOverride(context.Background(), tt.in)
			assert.ErrorIs(t, err, tt.want)
			assert.Empty(t, f.repo.overrides)
		})
	}
}

func TestAvailabilitySetOverride_NoteRules(t *testing.T) {
	t.Run("200 chars kept for none", func(t *testing.T) {
		f := newFixture()
		note := strings.Repeat("ü", 200)
		cell, _, err := f.svc.SetOverride(context.Background(), availability.OverrideInput{Date: day(monday), SlotID: slotLunch, Mode: availability.ModeNone, Note: &note})
		require.NoError(t, err)
		require.NotNil(t, cell.Note)
		assert.Equal(t, note, *cell.Note)
		assert.Equal(t, note, *f.repo.overrides[overrideKey{monday, slotLunch}].Note)
	})
	t.Run("dropped for routine", func(t *testing.T) {
		f := newFixture()
		cell, _, err := f.svc.SetOverride(context.Background(), availability.OverrideInput{Date: day(monday), SlotID: slotLunch, Mode: availability.ModeRoutine, PresetID: ptr(presetOatmeal), Note: ptr("ignored")})
		require.NoError(t, err)
		assert.Nil(t, cell.Note)
		assert.Nil(t, f.repo.overrides[overrideKey{monday, slotLunch}].Note)
	})
	t.Run("blank note stored as null", func(t *testing.T) {
		f := newFixture()
		cell, _, err := f.svc.SetOverride(context.Background(), availability.OverrideInput{Date: day(monday), SlotID: slotLunch, Mode: availability.ModeNone, Note: ptr("  ")})
		require.NoError(t, err)
		assert.Nil(t, cell.Note)
	})
}

func TestAvailabilitySetOverride_Routine(t *testing.T) {
	f := newFixture()
	cell, removed, err := f.svc.SetOverride(context.Background(), availability.OverrideInput{Date: day(monday), SlotID: slotBreakfast, Mode: availability.ModeRoutine, PresetID: ptr(presetOatmeal)})
	require.NoError(t, err)
	assert.Nil(t, removed)
	assert.Equal(t, availability.Cell{
		Date: day(monday), SlotID: slotBreakfast, Mode: availability.ModeRoutine, Source: availability.SourceOverride,
		PresetID: ptr(presetOatmeal), PresetName: ptr("Oatmeal"),
	}, cell)
}

func TestAvailabilitySetOverride_NoneOnPlate(t *testing.T) {
	seed := func(f *fixture) {
		f.plates.plates = []plate.Plate{
			{ID: 7, Date: day(monday), SlotID: slotLunch, Components: []plate.PlateComponent{{ID: 1, PlateID: 7, FoodID: 3}}},
			{ID: 8, Date: day(monday), SlotID: slotDinner},
		}
	}

	t.Run("without remove_plate returns ErrCellHasPlate", func(t *testing.T) {
		f := newFixture()
		seed(f)
		_, removed, err := f.svc.SetOverride(context.Background(), availability.OverrideInput{Date: day(monday), SlotID: slotLunch, Mode: availability.ModeNone})
		assert.ErrorIs(t, err, domain.ErrCellHasPlate)
		assert.Nil(t, removed)
		assert.Empty(t, f.repo.overrides)
		assert.Len(t, f.plates.plates, 2)
	})

	t.Run("with remove_plate deletes and returns the plate", func(t *testing.T) {
		f := newFixture()
		seed(f)
		cell, removed, err := f.svc.SetOverride(context.Background(), availability.OverrideInput{Date: day(monday), SlotID: slotLunch, Mode: availability.ModeNone, RemovePlate: true, Note: ptr("restaurant")})
		require.NoError(t, err)
		require.NotNil(t, removed)
		assert.Equal(t, int64(7), removed.ID)
		assert.Len(t, removed.Components, 1, "plate loaded with components before delete")
		require.Len(t, f.plates.plates, 1)
		assert.Equal(t, int64(8), f.plates.plates[0].ID)
		assert.Equal(t, availability.ModeNone, cell.Mode)
		assert.Equal(t, "restaurant", *cell.Note)
		assert.Equal(t, 1, f.tx.calls)
	})

	t.Run("with remove_plate deletes every plate in the cell", func(t *testing.T) {
		f := newFixture()
		seed(f)
		f.plates.plates = append(f.plates.plates, plate.Plate{ID: 9, Date: day(monday), SlotID: slotLunch})
		_, removed, err := f.svc.SetOverride(context.Background(), availability.OverrideInput{Date: day(monday), SlotID: slotLunch, Mode: availability.ModeNone, RemovePlate: true})
		require.NoError(t, err)
		require.NotNil(t, removed)
		assert.Equal(t, int64(7), removed.ID, "first plate returned")
		require.Len(t, f.plates.plates, 1)
		assert.Equal(t, int64(8), f.plates.plates[0].ID, "only the other cell's plate remains")
	})

	t.Run("remove_plate on empty cell removes nothing", func(t *testing.T) {
		f := newFixture()
		_, removed, err := f.svc.SetOverride(context.Background(), availability.OverrideInput{Date: day(monday), SlotID: slotLunch, Mode: availability.ModeNone, RemovePlate: true})
		require.NoError(t, err)
		assert.Nil(t, removed)
	})

	t.Run("plan and routine ignore the plate", func(t *testing.T) {
		f := newFixture()
		seed(f)
		_, removed, err := f.svc.SetOverride(context.Background(), availability.OverrideInput{Date: day(monday), SlotID: slotLunch, Mode: availability.ModePlan})
		require.NoError(t, err)
		assert.Nil(t, removed)
		assert.Len(t, f.plates.plates, 2)
	})
}

func TestAvailabilityNoneOverride(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	f.repo.overrides[overrideKey{monday, slotLunch}] = availability.Override{
		Date: day(monday), SlotID: slotLunch, Mode: availability.ModeNone, Note: ptr("canteen"),
	}
	f.repo.overrides[overrideKey{monday, slotDinner}] = availability.Override{
		Date: day(monday), SlotID: slotDinner, Mode: availability.ModeRoutine, PresetID: ptr(presetOatmeal),
	}

	note, ok, err := f.svc.NoneOverride(ctx, day(monday), slotLunch)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, ptr("canteen"), note)

	_, ok, err = f.svc.NoneOverride(ctx, day(monday), slotDinner)
	require.NoError(t, err)
	assert.False(t, ok, "a routine override is not a none override")

	_, ok, err = f.svc.NoneOverride(ctx, day(monday), slotBreakfast)
	require.NoError(t, err)
	assert.False(t, ok, "no override")
}

func TestAvailabilitySetNoneIfUnset(t *testing.T) {
	ctx := context.Background()

	t.Run("writes on a plain cell", func(t *testing.T) {
		f := newFixture()
		ok, err := f.svc.SetNoneIfUnset(ctx, day(monday), slotLunch, ptr("canteen"))
		require.NoError(t, err)
		assert.True(t, ok)
		got := f.repo.overrides[overrideKey{monday, slotLunch}]
		assert.Equal(t, availability.ModeNone, got.Mode)
		assert.Equal(t, ptr("canteen"), got.Note)
	})
	t.Run("keeps an existing override", func(t *testing.T) {
		f := newFixture()
		f.repo.overrides[overrideKey{monday, slotLunch}] = availability.Override{Date: day(monday), SlotID: slotLunch, Mode: availability.ModePlan}
		ok, err := f.svc.SetNoneIfUnset(ctx, day(monday), slotLunch, nil)
		require.NoError(t, err)
		assert.False(t, ok)
		assert.Equal(t, availability.ModePlan, f.repo.overrides[overrideKey{monday, slotLunch}].Mode)
	})
	t.Run("skips a cell that is none by default", func(t *testing.T) {
		f := newFixture()
		f.repo.defaults[defaultKey{0, slotLunch}] = availability.Default{Weekday: 0, SlotID: slotLunch, Mode: availability.ModeNone}
		ok, err := f.svc.SetNoneIfUnset(ctx, day(monday), slotLunch, nil)
		require.NoError(t, err)
		assert.False(t, ok)
		assert.Empty(t, f.repo.overrides)
	})
	t.Run("a plate in the cell is an error", func(t *testing.T) {
		f := newFixture()
		f.plates.plates = []plate.Plate{{ID: 1, Date: day(monday), SlotID: slotLunch}}
		_, err := f.svc.SetNoneIfUnset(ctx, day(monday), slotLunch, nil)
		require.ErrorIs(t, err, domain.ErrCellHasPlate)
	})
}

func TestAvailabilityClearOverride_Idempotent(t *testing.T) {
	f := newFixture()
	f.repo.overrides[overrideKey{monday, slotLunch}] = availability.Override{Date: day(monday), SlotID: slotLunch, Mode: availability.ModeNone}
	require.NoError(t, f.svc.ClearOverride(context.Background(), day(monday), slotLunch))
	assert.Empty(t, f.repo.overrides)
	require.NoError(t, f.svc.ClearOverride(context.Background(), day(monday), slotLunch))
}
