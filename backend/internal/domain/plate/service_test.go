package plate_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/stretchr/testify/require"
)

type fakeRepo struct {
	plates    map[int64]*plate.Plate
	pcs       map[int64]*plate.PlateComponent
	usageComp map[int64]int64
	usageSlot map[int64]int64
	nextPlate int64
	nextPC    int64
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		plates:    map[int64]*plate.Plate{},
		pcs:       map[int64]*plate.PlateComponent{},
		usageComp: map[int64]int64{},
		usageSlot: map[int64]int64{},
		nextPlate: 1,
		nextPC:    1,
	}
}

func (r *fakeRepo) Create(_ context.Context, p *plate.Plate) error {
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

func (r *fakeRepo) Get(_ context.Context, id int64) (*plate.Plate, error) {
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
	return &cp, nil
}

func (r *fakeRepo) Update(_ context.Context, p *plate.Plate) error {
	if _, ok := r.plates[p.ID]; !ok {
		return domain.ErrNotFound
	}
	cp := *p
	cp.Components = nil
	r.plates[p.ID] = &cp
	return nil
}

func (r *fakeRepo) Delete(_ context.Context, id int64) error {
	if _, ok := r.plates[id]; !ok {
		return domain.ErrNotFound
	}
	delete(r.plates, id)
	for pcid, pc := range r.pcs {
		if pc.PlateID == id {
			delete(r.pcs, pcid)
		}
	}
	return nil
}

func (r *fakeRepo) ListByWeek(_ context.Context, weekID int64) ([]plate.Plate, error) {
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
	return out, nil
}

func (r *fakeRepo) CreateComponent(_ context.Context, pc *plate.PlateComponent) error {
	pc.ID = r.nextPC
	r.nextPC++
	cp := *pc
	r.pcs[pc.ID] = &cp
	return nil
}

func (r *fakeRepo) GetComponent(_ context.Context, id int64) (*plate.PlateComponent, error) {
	pc, ok := r.pcs[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *pc
	return &cp, nil
}

func (r *fakeRepo) UpdateComponent(_ context.Context, pc *plate.PlateComponent) error {
	if _, ok := r.pcs[pc.ID]; !ok {
		return domain.ErrNotFound
	}
	cp := *pc
	r.pcs[pc.ID] = &cp
	return nil
}

func (r *fakeRepo) DeleteComponent(_ context.Context, id int64) error {
	if _, ok := r.pcs[id]; !ok {
		return domain.ErrNotFound
	}
	delete(r.pcs, id)
	return nil
}

func (r *fakeRepo) ListComponentsByPlate(_ context.Context, plateID int64) ([]plate.PlateComponent, error) {
	var out []plate.PlateComponent
	for _, pc := range r.pcs {
		if pc.PlateID == plateID {
			out = append(out, *pc)
		}
	}
	return out, nil
}

func (r *fakeRepo) CountUsingComponent(_ context.Context, componentID int64) (int64, error) {
	return r.usageComp[componentID], nil
}

func (r *fakeRepo) CountUsingTimeSlot(_ context.Context, slotID int64) (int64, error) {
	return r.usageSlot[slotID], nil
}

type fakeSlots struct{ exists map[int64]bool }

func (f *fakeSlots) Exists(_ context.Context, id int64) (bool, error) { return f.exists[id], nil }

type fakeComponents struct{ exists map[int64]bool }

func (f *fakeComponents) Exists(_ context.Context, id int64) (bool, error) {
	return f.exists[id], nil
}

func newSvc() (*plate.Service, *fakeRepo, *fakeSlots, *fakeComponents) {
	repo := newFakeRepo()
	slots := &fakeSlots{exists: map[int64]bool{1: true, 2: true}}
	comps := &fakeComponents{exists: map[int64]bool{10: true, 20: true, 30: true, 40: true}}
	return plate.NewService(repo, slots, comps), repo, slots, comps
}

func TestCreate_HappyPath(t *testing.T) {
	svc, _, _, _ := newSvc()
	p := &plate.Plate{WeekID: 1, Day: 0, SlotID: 1}
	require.NoError(t, svc.Create(context.Background(), p))
	require.Equal(t, int64(1), p.ID)
}

func TestCreate_InvalidDay(t *testing.T) {
	svc, _, _, _ := newSvc()
	err := svc.Create(context.Background(), &plate.Plate{WeekID: 1, Day: 7, SlotID: 1})
	require.True(t, errors.Is(err, domain.ErrInvalidDay), "got %v", err)
}

func TestCreate_SlotUnknown(t *testing.T) {
	svc, _, _, _ := newSvc()
	err := svc.Create(context.Background(), &plate.Plate{WeekID: 1, Day: 0, SlotID: 999})
	require.True(t, errors.Is(err, domain.ErrSlotUnknown), "got %v", err)
}

func TestCreate_WithInitialComponents(t *testing.T) {
	svc, repo, _, _ := newSvc()
	p := &plate.Plate{
		WeekID: 1,
		Day:    1,
		SlotID: 1,
		Components: []plate.PlateComponent{
			{ComponentID: 10, Portions: 1},
			{ComponentID: 20, Portions: 2},
		},
	}
	require.NoError(t, svc.Create(context.Background(), p))
	require.Len(t, repo.pcs, 2)
	// SortOrder assigned in order
	for _, pc := range p.Components {
		stored := repo.pcs[pc.ID]
		require.NotNil(t, stored)
		require.Equal(t, pc.SortOrder, stored.SortOrder)
	}
}

func TestCreate_UnknownComponent(t *testing.T) {
	svc, _, _, _ := newSvc()
	p := &plate.Plate{
		WeekID: 1, Day: 0, SlotID: 1,
		Components: []plate.PlateComponent{{ComponentID: 999, Portions: 1}},
	}
	err := svc.Create(context.Background(), p)
	require.True(t, errors.Is(err, domain.ErrNotFound), "got %v", err)
}

func TestAddComponent_AssignsNextSortOrder(t *testing.T) {
	svc, repo, _, _ := newSvc()
	p := &plate.Plate{WeekID: 1, Day: 0, SlotID: 1}
	require.NoError(t, svc.Create(context.Background(), p))
	pc1, err := svc.AddComponent(context.Background(), p.ID, 10, 1)
	require.NoError(t, err)
	require.Equal(t, 0, pc1.SortOrder)
	pc2, err := svc.AddComponent(context.Background(), p.ID, 20, 1)
	require.NoError(t, err)
	require.Equal(t, 1, pc2.SortOrder)
	pc3, err := svc.AddComponent(context.Background(), p.ID, 30, 1)
	require.NoError(t, err)
	require.Equal(t, 2, pc3.SortOrder)
	require.Len(t, repo.pcs, 3)
}

func TestSwapComponent_PreservesSortOrder(t *testing.T) {
	svc, _, _, _ := newSvc()
	p := &plate.Plate{
		WeekID: 1, Day: 0, SlotID: 1,
		Components: []plate.PlateComponent{
			{ComponentID: 10, Portions: 1},
			{ComponentID: 20, Portions: 2},
		},
	}
	require.NoError(t, svc.Create(context.Background(), p))
	original := p.Components[1]
	swapped, err := svc.SwapComponent(context.Background(), original.ID, 30, nil)
	require.NoError(t, err)
	require.Equal(t, int64(30), swapped.ComponentID)
	require.Equal(t, original.SortOrder, swapped.SortOrder)
	require.Equal(t, original.Portions, swapped.Portions, "portions preserved when override nil")
}

func TestSwapComponent_OverridePortions(t *testing.T) {
	svc, _, _, _ := newSvc()
	p := &plate.Plate{
		WeekID: 1, Day: 0, SlotID: 1,
		Components: []plate.PlateComponent{{ComponentID: 10, Portions: 1}},
	}
	require.NoError(t, svc.Create(context.Background(), p))
	override := 3.5
	swapped, err := svc.SwapComponent(context.Background(), p.Components[0].ID, 20, &override)
	require.NoError(t, err)
	require.InDelta(t, 3.5, swapped.Portions, 1e-9)
}

func TestUpdateComponentPortions(t *testing.T) {
	svc, _, _, _ := newSvc()
	p := &plate.Plate{
		WeekID: 1, Day: 0, SlotID: 1,
		Components: []plate.PlateComponent{{ComponentID: 10, Portions: 1}},
	}
	require.NoError(t, svc.Create(context.Background(), p))
	pc, err := svc.UpdateComponentPortions(context.Background(), p.Components[0].ID, 2.5)
	require.NoError(t, err)
	require.InDelta(t, 2.5, pc.Portions, 1e-9)

	_, err = svc.UpdateComponentPortions(context.Background(), p.Components[0].ID, 0)
	require.True(t, errors.Is(err, domain.ErrInvalidInput))
}

func TestRemoveComponent(t *testing.T) {
	svc, repo, _, _ := newSvc()
	p := &plate.Plate{
		WeekID: 1, Day: 0, SlotID: 1,
		Components: []plate.PlateComponent{
			{ComponentID: 10, Portions: 1},
			{ComponentID: 20, Portions: 1},
		},
	}
	require.NoError(t, svc.Create(context.Background(), p))
	require.NoError(t, svc.RemoveComponent(context.Background(), p.Components[0].ID))
	require.Len(t, repo.pcs, 1)
}

func TestUpdate_InvalidDay(t *testing.T) {
	svc, repo, _, _ := newSvc()
	p := &plate.Plate{WeekID: 1, Day: 0, SlotID: 1}
	require.NoError(t, svc.Create(context.Background(), p))
	repo.plates[p.ID].Day = 7
	err := svc.Update(context.Background(), &plate.Plate{ID: p.ID, WeekID: 1, Day: 7, SlotID: 1})
	require.True(t, errors.Is(err, domain.ErrInvalidDay))
}
