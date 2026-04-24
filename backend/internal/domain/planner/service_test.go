package planner_test

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planner"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/stretchr/testify/require"
)

func sortByPlate(plates []plate.Plate) {
	sort.SliceStable(plates, func(i, j int) bool {
		if plates[i].Day != plates[j].Day {
			return plates[i].Day < plates[j].Day
		}
		return plates[i].ID < plates[j].ID
	})
	for i := range plates {
		sort.SliceStable(plates[i].Components, func(a, b int) bool {
			return plates[i].Components[a].SortOrder < plates[i].Components[b].SortOrder
		})
	}
}

type fakeWeekRepo struct {
	items  map[int64]*planner.Week
	nextID int64
}

func newFakeWeekRepo() *fakeWeekRepo {
	return &fakeWeekRepo{items: map[int64]*planner.Week{}, nextID: 1}
}

func (r *fakeWeekRepo) Create(_ context.Context, w *planner.Week) error {
	w.ID = r.nextID
	r.nextID++
	w.CreatedAt = time.Now()
	cp := *w
	cp.Plates = nil
	r.items[w.ID] = &cp
	return nil
}

func (r *fakeWeekRepo) Get(_ context.Context, id int64) (*planner.Week, error) {
	w, ok := r.items[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *w
	return &cp, nil
}

func (r *fakeWeekRepo) GetByYearAndNumber(_ context.Context, year, week int) (*planner.Week, error) {
	for _, w := range r.items {
		if w.Year == year && w.WeekNumber == week {
			cp := *w
			return &cp, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (r *fakeWeekRepo) List(_ context.Context, limit, offset int) ([]planner.Week, int64, error) {
	out := make([]planner.Week, 0, len(r.items))
	for _, w := range r.items {
		out = append(out, *w)
	}
	total := int64(len(out))
	if offset >= len(out) {
		return []planner.Week{}, total, nil
	}
	end := offset + limit
	if end > len(out) {
		end = len(out)
	}
	return out[offset:end], total, nil
}

type fakePlateRepo struct {
	plates    map[int64]*plate.Plate
	pcs       map[int64]*plate.PlateComponent
	nextPlate int64
	nextPC    int64
}

func newFakePlateRepo() *fakePlateRepo {
	return &fakePlateRepo{
		plates:    map[int64]*plate.Plate{},
		pcs:       map[int64]*plate.PlateComponent{},
		nextPlate: 1,
		nextPC:    1,
	}
}

func (r *fakePlateRepo) Create(_ context.Context, p *plate.Plate) error {
	p.ID = r.nextPlate
	r.nextPlate++
	cp := *p
	cp.Components = nil
	r.plates[p.ID] = &cp
	for i := range p.Components {
		pc := p.Components[i]
		pc.ID = r.nextPC
		pc.PlateID = p.ID
		r.nextPC++
		pcCopy := pc
		r.pcs[pc.ID] = &pcCopy
		p.Components[i] = pc
	}
	return nil
}

func (r *fakePlateRepo) Get(_ context.Context, id int64) (*plate.Plate, error) {
	p, ok := r.plates[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *p
	for _, pc := range r.pcs {
		if pc.PlateID == id {
			cp.Components = append(cp.Components, *pc)
		}
	}
	sort.SliceStable(cp.Components, func(a, b int) bool {
		return cp.Components[a].SortOrder < cp.Components[b].SortOrder
	})
	return &cp, nil
}

func (r *fakePlateRepo) Update(_ context.Context, p *plate.Plate) error {
	if _, ok := r.plates[p.ID]; !ok {
		return domain.ErrNotFound
	}
	cp := *p
	cp.Components = nil
	r.plates[p.ID] = &cp
	return nil
}

func (r *fakePlateRepo) Delete(_ context.Context, id int64) error {
	delete(r.plates, id)
	for pcid, pc := range r.pcs {
		if pc.PlateID == id {
			delete(r.pcs, pcid)
		}
	}
	return nil
}

func (r *fakePlateRepo) ListByWeek(_ context.Context, weekID int64) ([]plate.Plate, error) {
	var out []plate.Plate
	for _, p := range r.plates {
		if p.WeekID == weekID {
			cp := *p
			for _, pc := range r.pcs {
				if pc.PlateID == p.ID {
					cp.Components = append(cp.Components, *pc)
				}
			}
			out = append(out, cp)
		}
	}
	sortByPlate(out)
	return out, nil
}

func (r *fakePlateRepo) CreateComponent(_ context.Context, pc *plate.PlateComponent) error {
	pc.ID = r.nextPC
	r.nextPC++
	cp := *pc
	r.pcs[pc.ID] = &cp
	return nil
}

func (r *fakePlateRepo) GetComponent(_ context.Context, id int64) (*plate.PlateComponent, error) {
	pc, ok := r.pcs[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *pc
	return &cp, nil
}

func (r *fakePlateRepo) UpdateComponent(_ context.Context, pc *plate.PlateComponent) error {
	cp := *pc
	r.pcs[pc.ID] = &cp
	return nil
}

func (r *fakePlateRepo) DeleteComponent(_ context.Context, id int64) error {
	delete(r.pcs, id)
	return nil
}

func (r *fakePlateRepo) ListComponentsByPlate(_ context.Context, plateID int64) ([]plate.PlateComponent, error) {
	var out []plate.PlateComponent
	for _, pc := range r.pcs {
		if pc.PlateID == plateID {
			out = append(out, *pc)
		}
	}
	return out, nil
}

func (r *fakePlateRepo) CountUsingFood(_ context.Context, _ int64) (int64, error) {
	return 0, nil
}

func (r *fakePlateRepo) CountUsingTimeSlot(_ context.Context, _ int64) (int64, error) {
	return 0, nil
}

func (r *fakePlateRepo) SetSkipped(_ context.Context, _ int64, _ bool, _ *string) (*plate.Plate, error) {
	return nil, nil
}

func (r *fakePlateRepo) DeleteByWeek(_ context.Context, _ int64) (int64, error) {
	return 0, nil
}

// inlineTxRunner runs the closure with the same fake repos (no isolation).
type inlineTxRunner struct {
	weeks  planner.WeekRepository
	plates plate.Repository
}

func (t *inlineTxRunner) RunInTx(ctx context.Context, fn func(planner.WeekRepository, plate.Repository) error) error {
	return fn(t.weeks, t.plates)
}

func newSvc() (*planner.Service, *fakeWeekRepo, *fakePlateRepo) {
	weeks := newFakeWeekRepo()
	plates := newFakePlateRepo()
	tx := &inlineTxRunner{weeks: weeks, plates: plates}
	return planner.NewService(weeks, plates, tx), weeks, plates
}

func TestCurrent_GetOrCreate(t *testing.T) {
	svc, weeks, _ := newSvc()
	now := time.Date(2026, 4, 16, 12, 0, 0, 0, time.UTC)
	w1, err := svc.Current(context.Background(), now)
	require.NoError(t, err)
	require.NotZero(t, w1.ID)

	w2, err := svc.Current(context.Background(), now)
	require.NoError(t, err)
	require.Equal(t, w1.ID, w2.ID, "second call returns same week")
	require.Len(t, weeks.items, 1)
}

func TestByDate_OutOfRange(t *testing.T) {
	svc, _, _ := newSvc()
	_, err := svc.ByDate(context.Background(), 2026, 0)
	require.True(t, errors.Is(err, domain.ErrInvalidInput))
	_, err = svc.ByDate(context.Background(), 2026, 54)
	require.True(t, errors.Is(err, domain.ErrInvalidInput))
}

func TestByDate_GetOrCreate(t *testing.T) {
	svc, _, _ := newSvc()
	w, err := svc.ByDate(context.Background(), 2026, 16)
	require.NoError(t, err)
	require.Equal(t, 2026, w.Year)
	require.Equal(t, 16, w.WeekNumber)
}

func TestGet_LoadsPlates(t *testing.T) {
	svc, weeks, plates := newSvc()
	w := &planner.Week{Year: 2026, WeekNumber: 16}
	require.NoError(t, weeks.Create(context.Background(), w))
	p := &plate.Plate{WeekID: w.ID, Day: 0, SlotID: 1}
	require.NoError(t, plates.Create(context.Background(), p))

	got, err := svc.Get(context.Background(), w.ID)
	require.NoError(t, err)
	require.Len(t, got.Plates, 1)
}

func TestCopy_DeepClones(t *testing.T) {
	svc, weeks, plates := newSvc()
	source := &planner.Week{Year: 2026, WeekNumber: 16}
	require.NoError(t, weeks.Create(context.Background(), source))

	for i := 0; i < 3; i++ {
		p := &plate.Plate{
			WeekID: source.ID,
			Day:    i,
			SlotID: 1,
			Components: []plate.PlateComponent{
				{FoodID: int64(10 + i), Portions: 1, SortOrder: 0},
				{FoodID: int64(20 + i), Portions: 2, SortOrder: 1},
				{FoodID: int64(30 + i), Portions: 1, SortOrder: 2},
			},
		}
		require.NoError(t, plates.Create(context.Background(), p))
	}

	target, err := svc.Copy(context.Background(), source.ID, 2026, 17)
	require.NoError(t, err)
	require.Equal(t, 17, target.WeekNumber)
	require.NotEqual(t, source.ID, target.ID)
	require.Len(t, target.Plates, 3)
	for _, p := range target.Plates {
		require.Len(t, p.Components, 3)
		require.Equal(t, target.ID, p.WeekID)
		for i, pc := range p.Components {
			require.Equal(t, i, pc.SortOrder, "sort order preserved")
		}
	}

	// Source untouched.
	srcLoaded, err := svc.Get(context.Background(), source.ID)
	require.NoError(t, err)
	require.Len(t, srcLoaded.Plates, 3)
}

func TestCopy_TargetWeekOutOfRange(t *testing.T) {
	svc, weeks, _ := newSvc()
	source := &planner.Week{Year: 2026, WeekNumber: 16}
	require.NoError(t, weeks.Create(context.Background(), source))
	_, err := svc.Copy(context.Background(), source.ID, 2026, 0)
	require.True(t, errors.Is(err, domain.ErrInvalidInput))
}
