package slot_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/slot"
	"github.com/stretchr/testify/require"
)

type fakeRepo struct {
	items     map[int64]*slot.TimeSlot
	usage     map[int64]int64
	nextID    int64
	createErr error
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{items: map[int64]*slot.TimeSlot{}, usage: map[int64]int64{}, nextID: 1}
}

func (r *fakeRepo) Create(_ context.Context, t *slot.TimeSlot) error {
	if r.createErr != nil {
		return r.createErr
	}
	t.ID = r.nextID
	r.nextID++
	cp := *t
	r.items[t.ID] = &cp
	return nil
}

func (r *fakeRepo) Get(_ context.Context, id int64) (*slot.TimeSlot, error) {
	t, ok := r.items[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *t
	return &cp, nil
}

func (r *fakeRepo) Update(_ context.Context, t *slot.TimeSlot) error {
	if _, ok := r.items[t.ID]; !ok {
		return domain.ErrNotFound
	}
	cp := *t
	r.items[t.ID] = &cp
	return nil
}

func (r *fakeRepo) Delete(_ context.Context, id int64) error {
	if _, ok := r.items[id]; !ok {
		return domain.ErrNotFound
	}
	delete(r.items, id)
	return nil
}

func (r *fakeRepo) List(_ context.Context, _ bool) ([]slot.TimeSlot, error) {
	out := make([]slot.TimeSlot, 0, len(r.items))
	for _, t := range r.items {
		out = append(out, *t)
	}
	return out, nil
}

func (r *fakeRepo) CountPlatesUsing(_ context.Context, id int64) (int64, error) {
	return r.usage[id], nil
}

func TestService_Create_Validation(t *testing.T) {
	cases := []struct {
		name string
		in   slot.TimeSlot
		want error
	}{
		{"empty name_key", slot.TimeSlot{NameKey: "", Icon: "Coffee"}, domain.ErrInvalidInput},
		{"empty icon", slot.TimeSlot{NameKey: "slot.breakfast", Icon: ""}, domain.ErrInvalidInput},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeRepo()
			svc := slot.NewService(repo)
			err := svc.Create(context.Background(), &tc.in)
			require.Error(t, err)
			require.True(t, errors.Is(err, tc.want))
		})
	}
}

func TestService_Create_AssignsID(t *testing.T) {
	repo := newFakeRepo()
	svc := slot.NewService(repo)
	in := &slot.TimeSlot{NameKey: "slot.breakfast", Icon: "Coffee", SortOrder: 1, Active: true}
	require.NoError(t, svc.Create(context.Background(), in))
	require.Equal(t, int64(1), in.ID)
}

func TestService_Delete_InUse(t *testing.T) {
	repo := newFakeRepo()
	repo.items[5] = &slot.TimeSlot{ID: 5, NameKey: "slot.lunch", Icon: "Sun"}
	repo.usage[5] = 3
	svc := slot.NewService(repo)
	err := svc.Delete(context.Background(), 5)
	require.Error(t, err)
	require.True(t, errors.Is(err, domain.ErrInUse))
}

func TestService_Delete_NotInUse(t *testing.T) {
	repo := newFakeRepo()
	repo.items[5] = &slot.TimeSlot{ID: 5, NameKey: "slot.lunch", Icon: "Sun"}
	svc := slot.NewService(repo)
	require.NoError(t, svc.Delete(context.Background(), 5))
	_, ok := repo.items[5]
	require.False(t, ok)
}

func TestService_Update_Validation(t *testing.T) {
	repo := newFakeRepo()
	repo.items[1] = &slot.TimeSlot{ID: 1, NameKey: "slot.breakfast", Icon: "Coffee"}
	svc := slot.NewService(repo)
	err := svc.Update(context.Background(), &slot.TimeSlot{ID: 1, NameKey: "", Icon: "Coffee"})
	require.True(t, errors.Is(err, domain.ErrInvalidInput))
}
