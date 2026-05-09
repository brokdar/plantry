package template_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/template"
)

// ── mock implementations ────────────────────────────────────────────────────

type mockRepo struct {
	createFn              func(ctx context.Context, t *template.Template) error
	getFn                 func(ctx context.Context, id int64) (*template.Template, error)
	updateNameFn          func(ctx context.Context, id int64, name string) (*template.Template, error)
	deleteFn              func(ctx context.Context, id int64) error
	listFn                func(ctx context.Context) ([]template.Template, error)
	listByScopeFn         func(ctx context.Context, scope template.Scope) ([]template.Template, error)
	replaceEntriesFn      func(ctx context.Context, templateID int64, entries []template.TemplateEntry) error
	listEntriesByTemplate func(ctx context.Context, templateID int64) ([]template.TemplateEntry, error)
	countUsingFoodFn      func(ctx context.Context, foodID int64) (int64, error)
}

func (m *mockRepo) Create(ctx context.Context, t *template.Template) error {
	if m.createFn != nil {
		return m.createFn(ctx, t)
	}
	t.ID = 1
	return nil
}

func (m *mockRepo) Get(ctx context.Context, id int64) (*template.Template, error) {
	if m.getFn != nil {
		return m.getFn(ctx, id)
	}
	return nil, nil
}

func (m *mockRepo) UpdateName(ctx context.Context, id int64, name string) (*template.Template, error) {
	if m.updateNameFn != nil {
		return m.updateNameFn(ctx, id, name)
	}
	return nil, nil
}

func (m *mockRepo) Delete(ctx context.Context, id int64) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return nil
}

func (m *mockRepo) List(ctx context.Context) ([]template.Template, error) {
	if m.listFn != nil {
		return m.listFn(ctx)
	}
	return nil, nil
}

func (m *mockRepo) ListByScope(ctx context.Context, scope template.Scope) ([]template.Template, error) {
	if m.listByScopeFn != nil {
		return m.listByScopeFn(ctx, scope)
	}
	return nil, nil
}

func (m *mockRepo) ReplaceEntries(ctx context.Context, templateID int64, entries []template.TemplateEntry) error {
	if m.replaceEntriesFn != nil {
		return m.replaceEntriesFn(ctx, templateID, entries)
	}
	return nil
}

func (m *mockRepo) ListEntriesByTemplate(ctx context.Context, templateID int64) ([]template.TemplateEntry, error) {
	if m.listEntriesByTemplate != nil {
		return m.listEntriesByTemplate(ctx, templateID)
	}
	return nil, nil
}

func (m *mockRepo) CountUsingFood(ctx context.Context, foodID int64) (int64, error) {
	if m.countUsingFoodFn != nil {
		return m.countUsingFoodFn(ctx, foodID)
	}
	return 0, nil
}

type mockFoodChecker struct {
	existsFn func(ctx context.Context, foodID int64) (bool, error)
	kindFn   func(ctx context.Context, foodID int64) (string, error)
}

func (m *mockFoodChecker) Exists(ctx context.Context, foodID int64) (bool, error) {
	if m.existsFn != nil {
		return m.existsFn(ctx, foodID)
	}
	return true, nil
}

func (m *mockFoodChecker) KindOf(ctx context.Context, foodID int64) (string, error) {
	if m.kindFn != nil {
		return m.kindFn(ctx, foodID)
	}
	// Default: composed (most existing template tests use composed-style integers).
	return "composed", nil
}

type mockPlateComponentSource struct {
	listFn func(ctx context.Context, plateID int64) ([]plate.PlateComponent, error)
}

func (m *mockPlateComponentSource) ListComponentsByPlate(ctx context.Context, plateID int64) ([]plate.PlateComponent, error) {
	if m.listFn != nil {
		return m.listFn(ctx, plateID)
	}
	return nil, nil
}

type mockTxRunner struct {
	pr plate.Repository
}

func (m *mockTxRunner) RunInTemplateTx(ctx context.Context, fn func(template.Repository, plate.Repository) error) error {
	return fn(nil, m.pr)
}

type mockPlateRepo struct {
	createFn      func(ctx context.Context, p *plate.Plate) error
	deleteFn      func(ctx context.Context, id int64) error
	listByRangeFn func(ctx context.Context, from, to time.Time) ([]plate.Plate, error)
	nextID        int64
	deletedPlates []int64
}

func (m *mockPlateRepo) Create(ctx context.Context, p *plate.Plate) error {
	if m.createFn != nil {
		return m.createFn(ctx, p)
	}
	m.nextID++
	p.ID = m.nextID
	return nil
}
func (m *mockPlateRepo) Get(_ context.Context, _ int64) (*plate.Plate, error) { return nil, nil }
func (m *mockPlateRepo) Update(_ context.Context, _ *plate.Plate) error       { return nil }
func (m *mockPlateRepo) Delete(ctx context.Context, id int64) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	m.deletedPlates = append(m.deletedPlates, id)
	return nil
}
func (m *mockPlateRepo) CreateComponent(_ context.Context, _ *plate.PlateComponent) error { return nil }
func (m *mockPlateRepo) GetComponent(_ context.Context, _ int64) (*plate.PlateComponent, error) {
	return nil, nil
}
func (m *mockPlateRepo) UpdateComponent(_ context.Context, _ *plate.PlateComponent) error { return nil }
func (m *mockPlateRepo) DeleteComponent(_ context.Context, _ int64) error                 { return nil }
func (m *mockPlateRepo) ListComponentsByPlate(_ context.Context, _ int64) ([]plate.PlateComponent, error) {
	return nil, nil
}
func (m *mockPlateRepo) CountUsingFood(_ context.Context, _ int64) (int64, error)     { return 0, nil }
func (m *mockPlateRepo) CountUsingTimeSlot(_ context.Context, _ int64) (int64, error) { return 0, nil }
func (m *mockPlateRepo) RecentUnitForFood(_ context.Context, _ int64) (string, error) {
	return "", nil
}

func (m *mockPlateRepo) SetSkipped(_ context.Context, _ int64, _ bool, _ *string) (*plate.Plate, error) {
	return nil, nil
}

func (m *mockPlateRepo) ListByDateRange(ctx context.Context, from, to time.Time) ([]plate.Plate, error) {
	if m.listByRangeFn != nil {
		return m.listByRangeFn(ctx, from, to)
	}
	return nil, nil
}

// ── helpers ─────────────────────────────────────────────────────────────────

func mustDate(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func ptrInt64(v int64) *int64 { return &v }
func ptrInt(v int) *int       { return &v }

func makeService(repo *mockRepo, pr *mockPlateRepo) *template.Service {
	return template.NewService(repo, &mockFoodChecker{}, &mockPlateComponentSource{}, &mockTxRunner{pr: pr})
}

// ── Apply (slot scope) tests ────────────────────────────────────────────────

func TestApplySlot_HappyPath(t *testing.T) {
	// Slot template with 2 entries, both at offset 0 → 1 plate.
	start := mustDate("2026-04-25")

	tmpl := &template.Template{
		ID:    10,
		Name:  "Lunch",
		Scope: template.ScopeSlot,
		Entries: []template.TemplateEntry{
			{FoodID: 1, Portions: 1, DayOffset: 0, SortOrder: 0},
			{FoodID: 2, Portions: 2, DayOffset: 0, SortOrder: 1},
		},
	}
	repo := &mockRepo{
		getFn: func(_ context.Context, _ int64) (*template.Template, error) { return tmpl, nil },
	}
	pr := &mockPlateRepo{}
	svc := makeService(repo, pr)

	slot := int64(1)
	res, err := svc.Apply(context.Background(), 10, template.ApplyPayload{Date: &start, SlotID: &slot})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if len(res.Created) != 1 {
		t.Fatalf("expected 1 plate, got %d", len(res.Created))
	}
	if len(res.Created[0].Components) != 2 {
		t.Errorf("plate components = %d; want 2", len(res.Created[0].Components))
	}
}

func TestApplySlot_LegacyMultiOffset(t *testing.T) {
	// Legacy slot template with multiple day_offsets → groups into multiple plates.
	start := mustDate("2026-04-25")
	tmpl := &template.Template{
		ID:    11,
		Scope: template.ScopeSlot,
		Entries: []template.TemplateEntry{
			{FoodID: 1, Portions: 1, DayOffset: 0, SortOrder: 0},
			{FoodID: 2, Portions: 1, DayOffset: 6, SortOrder: 0},
		},
	}
	repo := &mockRepo{
		getFn: func(_ context.Context, _ int64) (*template.Template, error) { return tmpl, nil },
	}
	pr := &mockPlateRepo{}
	svc := makeService(repo, pr)

	slot := int64(1)
	res, err := svc.Apply(context.Background(), 11, template.ApplyPayload{Date: &start, SlotID: &slot})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if len(res.Created) != 2 {
		t.Fatalf("expected 2 plates, got %d", len(res.Created))
	}
	if res.Created[1].Date != mustDate("2026-05-01") {
		t.Errorf("second plate date = %s; want 2026-05-01", res.Created[1].Date.Format("2006-01-02"))
	}
}

func TestApplySlot_MissingSlotID(t *testing.T) {
	tmpl := &template.Template{ID: 1, Scope: template.ScopeSlot, Entries: []template.TemplateEntry{{FoodID: 1}}}
	repo := &mockRepo{
		getFn: func(_ context.Context, _ int64) (*template.Template, error) { return tmpl, nil },
	}
	svc := makeService(repo, &mockPlateRepo{})
	d := mustDate("2026-04-25")
	_, err := svc.Apply(context.Background(), 1, template.ApplyPayload{Date: &d})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput, got %v", err)
	}
}

func TestApplySlot_ConflictPropagated(t *testing.T) {
	tmpl := &template.Template{ID: 12, Scope: template.ScopeSlot, Entries: []template.TemplateEntry{{FoodID: 1}}}
	repo := &mockRepo{
		getFn: func(_ context.Context, _ int64) (*template.Template, error) { return tmpl, nil },
	}
	conflictErr := fmt.Errorf("%w: plate already exists", domain.ErrDuplicateName)
	pr := &mockPlateRepo{
		createFn: func(_ context.Context, _ *plate.Plate) error { return conflictErr },
	}
	svc := makeService(repo, pr)
	d := mustDate("2026-04-25")
	slot := int64(1)
	_, err := svc.Apply(context.Background(), 12, template.ApplyPayload{Date: &d, SlotID: &slot})
	if !errors.Is(err, domain.ErrDuplicateName) {
		t.Errorf("error = %v; want to wrap ErrDuplicateName", err)
	}
}

// ── Apply (day scope) tests ─────────────────────────────────────────────────

func TestApplyDay_HappyPath(t *testing.T) {
	tmpl := &template.Template{
		ID:    20,
		Scope: template.ScopeDay,
		Entries: []template.TemplateEntry{
			{FoodID: 1, Portions: 1, SlotID: ptrInt64(1)},
			{FoodID: 2, Portions: 1, SlotID: ptrInt64(2)},
		},
	}
	repo := &mockRepo{
		getFn: func(_ context.Context, _ int64) (*template.Template, error) { return tmpl, nil },
	}
	pr := &mockPlateRepo{}
	svc := makeService(repo, pr)
	d := mustDate("2026-04-25")
	res, err := svc.Apply(context.Background(), 20, template.ApplyPayload{Date: &d})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if len(res.Created) != 2 {
		t.Fatalf("expected 2 plates, got %d", len(res.Created))
	}
	if res.Created[0].SlotID != 1 || res.Created[1].SlotID != 2 {
		t.Errorf("slots = %d,%d; want 1,2", res.Created[0].SlotID, res.Created[1].SlotID)
	}
}

func TestApplyDay_ConflictSkip(t *testing.T) {
	tmpl := &template.Template{
		ID:    21,
		Scope: template.ScopeDay,
		Entries: []template.TemplateEntry{
			{FoodID: 1, Portions: 1, SlotID: ptrInt64(1)},
			{FoodID: 2, Portions: 1, SlotID: ptrInt64(2)},
		},
	}
	d := mustDate("2026-04-25")
	repo := &mockRepo{
		getFn: func(_ context.Context, _ int64) (*template.Template, error) { return tmpl, nil },
	}
	pr := &mockPlateRepo{
		listByRangeFn: func(_ context.Context, _, _ time.Time) ([]plate.Plate, error) {
			return []plate.Plate{{ID: 99, Date: d, SlotID: 1}}, nil
		},
	}
	svc := makeService(repo, pr)
	res, err := svc.Apply(context.Background(), 21, template.ApplyPayload{Date: &d, Conflict: template.ConflictSkip})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if len(res.Created) != 1 || res.Created[0].SlotID != 2 {
		t.Errorf("expected only slot 2 created; got %+v", res.Created)
	}
	if len(res.Skipped) != 1 || res.Skipped[0].SlotID != 1 {
		t.Errorf("expected slot 1 skipped; got %+v", res.Skipped)
	}
	if len(pr.deletedPlates) != 0 {
		t.Errorf("skip should not delete; got %v", pr.deletedPlates)
	}
}

func TestApplyDay_ConflictOverwrite(t *testing.T) {
	tmpl := &template.Template{
		ID:    22,
		Scope: template.ScopeDay,
		Entries: []template.TemplateEntry{
			{FoodID: 1, Portions: 1, SlotID: ptrInt64(1)},
		},
	}
	d := mustDate("2026-04-25")
	repo := &mockRepo{
		getFn: func(_ context.Context, _ int64) (*template.Template, error) { return tmpl, nil },
	}
	pr := &mockPlateRepo{
		listByRangeFn: func(_ context.Context, _, _ time.Time) ([]plate.Plate, error) {
			return []plate.Plate{{ID: 77, Date: d, SlotID: 1}}, nil
		},
	}
	svc := makeService(repo, pr)
	res, err := svc.Apply(context.Background(), 22, template.ApplyPayload{Date: &d, Conflict: template.ConflictOverwrite})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if len(res.Created) != 1 {
		t.Fatalf("expected 1 plate, got %d", len(res.Created))
	}
	if len(pr.deletedPlates) != 1 || pr.deletedPlates[0] != 77 {
		t.Errorf("expected plate 77 deleted; got %v", pr.deletedPlates)
	}
}

// ── Apply (week scope) tests ────────────────────────────────────────────────

func TestApplyWeek_DistributesByDayOffset(t *testing.T) {
	tmpl := &template.Template{
		ID:    30,
		Scope: template.ScopeWeek,
		Entries: []template.TemplateEntry{
			{FoodID: 1, Portions: 1, DayOffset: 0, SlotID: ptrInt64(1)},
			{FoodID: 2, Portions: 1, DayOffset: 6, SlotID: ptrInt64(1)},
		},
	}
	repo := &mockRepo{
		getFn: func(_ context.Context, _ int64) (*template.Template, error) { return tmpl, nil },
	}
	pr := &mockPlateRepo{}
	svc := makeService(repo, pr)
	start := mustDate("2026-04-25")
	res, err := svc.Apply(context.Background(), 30, template.ApplyPayload{StartDate: &start})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if len(res.Created) != 2 {
		t.Fatalf("expected 2 plates, got %d", len(res.Created))
	}
	if res.Created[1].Date != mustDate("2026-05-01") {
		t.Errorf("second plate date = %s; want 2026-05-01", res.Created[1].Date.Format("2006-01-02"))
	}
}

// ── Create validation per scope ─────────────────────────────────────────────

func TestCreate_DayScopeRejectsMissingSlotID(t *testing.T) {
	repo := &mockRepo{}
	svc := makeService(repo, &mockPlateRepo{})
	_, err := svc.Create(context.Background(), "Day", template.ScopeDay, nil, []template.TemplateEntry{
		{FoodID: 1, Portions: 1}, // no SlotID
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput, got %v", err)
	}
}

func TestCreate_WeekScopeRejectsBadDayOffset(t *testing.T) {
	repo := &mockRepo{}
	svc := makeService(repo, &mockPlateRepo{})
	_, err := svc.Create(context.Background(), "Week", template.ScopeWeek, nil, []template.TemplateEntry{
		{FoodID: 1, Portions: 1, DayOffset: 99, SlotID: ptrInt64(1)},
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput, got %v", err)
	}
}

func TestCreate_SlotScopeRejectsSlotID(t *testing.T) {
	repo := &mockRepo{}
	svc := makeService(repo, &mockPlateRepo{})
	_, err := svc.Create(context.Background(), "Slot", template.ScopeSlot, nil, []template.TemplateEntry{
		{FoodID: 1, Portions: 1, SlotID: ptrInt64(1)},
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput, got %v", err)
	}
}

func TestCreate_DefaultsToSlotScope(t *testing.T) {
	var captured *template.Template
	repo := &mockRepo{
		createFn: func(_ context.Context, t *template.Template) error {
			captured = t
			t.ID = 5
			return nil
		},
	}
	svc := makeService(repo, &mockPlateRepo{})
	_, err := svc.Create(context.Background(), "X", "", nil, []template.TemplateEntry{{FoodID: 1, Portions: 1}})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if captured.Scope != template.ScopeSlot {
		t.Errorf("Scope = %q; want slot", captured.Scope)
	}
}

// ── SaveAsTemplate tests ────────────────────────────────────────────────────

func TestSaveAsTemplate_HappyPath(t *testing.T) {
	anchor := mustDate("2026-04-25")
	plates := []plate.Plate{
		{Date: mustDate("2026-04-25"), SlotID: 1, Components: []plate.PlateComponent{{FoodID: 1, Portions: ptrInt(1)}}},
		{Date: mustDate("2026-04-26"), SlotID: 2, Components: []plate.PlateComponent{{FoodID: 2, Portions: ptrInt(2)}}},
		{Date: mustDate("2026-04-27"), SlotID: 3, Components: []plate.PlateComponent{{FoodID: 3, Portions: ptrInt(1)}}},
	}
	var created *template.Template
	repo := &mockRepo{
		createFn: func(_ context.Context, t *template.Template) error {
			t.ID = 99
			created = t
			return nil
		},
	}
	svc := makeService(repo, &mockPlateRepo{})

	tmpl, err := svc.SaveAsTemplate(context.Background(), "My Pattern", plates, anchor)
	if err != nil {
		t.Fatalf("SaveAsTemplate returned error: %v", err)
	}
	if tmpl.ID != 99 {
		t.Errorf("tmpl.ID = %d; want 99", tmpl.ID)
	}
	if created.Scope != template.ScopeWeek {
		t.Errorf("Scope = %q; want week", created.Scope)
	}
	if len(created.Entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(created.Entries))
	}
	wantOffsets := []int{0, 1, 2}
	wantSlots := []int64{1, 2, 3}
	for i, e := range created.Entries {
		if e.DayOffset != wantOffsets[i] {
			t.Errorf("entry[%d].DayOffset = %d; want %d", i, e.DayOffset, wantOffsets[i])
		}
		if e.SlotID == nil || *e.SlotID != wantSlots[i] {
			t.Errorf("entry[%d].SlotID = %v; want %d", i, e.SlotID, wantSlots[i])
		}
	}
}

func TestSaveAsTemplate_RejectsPlateBeforeAnchor(t *testing.T) {
	anchor := mustDate("2026-04-25")
	plates := []plate.Plate{
		{Date: mustDate("2026-04-24"), Components: []plate.PlateComponent{{FoodID: 1, Portions: ptrInt(1)}}},
	}
	repo := &mockRepo{}
	svc := makeService(repo, &mockPlateRepo{})

	_, err := svc.SaveAsTemplate(context.Background(), "Bad Pattern", plates, anchor)
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput, got %v", err)
	}
}

func TestSaveAsTemplate_EmptyPlates(t *testing.T) {
	repo := &mockRepo{}
	svc := makeService(repo, &mockPlateRepo{})

	_, err := svc.SaveAsTemplate(context.Background(), "Empty", []plate.Plate{}, mustDate("2026-04-25"))
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput, got %v", err)
	}
}

func TestSaveAsTemplate_SingleSlotSingleDayIsScopeSlot(t *testing.T) {
	anchor := mustDate("2026-04-25")
	plates := []plate.Plate{
		{
			Date:       anchor,
			SlotID:     3,
			Components: []plate.PlateComponent{{FoodID: 1, Portions: ptrInt(1)}, {FoodID: 2, Portions: ptrInt(2)}},
		},
	}
	var created *template.Template
	repo := &mockRepo{
		createFn: func(_ context.Context, t *template.Template) error {
			t.ID = 42
			created = t
			return nil
		},
	}
	svc := makeService(repo, &mockPlateRepo{})

	tmpl, err := svc.SaveAsTemplate(context.Background(), "Slot Pattern", plates, anchor)
	if err != nil {
		t.Fatalf("SaveAsTemplate returned error: %v", err)
	}
	if tmpl.ID != 42 {
		t.Errorf("tmpl.ID = %d; want 42", tmpl.ID)
	}
	if created.Scope != template.ScopeSlot {
		t.Errorf("Scope = %q; want slot", created.Scope)
	}
	for i, e := range created.Entries {
		if e.SlotID != nil {
			t.Errorf("entries[%d].SlotID = %v; want nil (slot scope)", i, e.SlotID)
		}
	}
}

// ── IsZero date guards ───────────────────────────────────────────────────────

func TestApplySlot_RejectsZeroDate(t *testing.T) {
	tmpl := &template.Template{
		ID:    1,
		Scope: template.ScopeSlot,
		Entries: []template.TemplateEntry{
			{FoodID: 1, Portions: 1},
		},
	}
	repo := &mockRepo{
		getFn: func(_ context.Context, _ int64) (*template.Template, error) { return tmpl, nil },
	}
	svc := makeService(repo, &mockPlateRepo{})
	zero := time.Time{}
	slotID := int64(1)
	_, err := svc.Apply(context.Background(), 1, template.ApplyPayload{Date: &zero, SlotID: &slotID})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for zero date, got %v", err)
	}
}

func TestApplyDay_RejectsZeroDate(t *testing.T) {
	tmpl := &template.Template{
		ID:    2,
		Scope: template.ScopeDay,
		Entries: []template.TemplateEntry{
			{FoodID: 1, Portions: 1, SlotID: ptrInt64(1)},
		},
	}
	repo := &mockRepo{
		getFn: func(_ context.Context, _ int64) (*template.Template, error) { return tmpl, nil },
	}
	svc := makeService(repo, &mockPlateRepo{})
	zero := time.Time{}
	_, err := svc.Apply(context.Background(), 2, template.ApplyPayload{Date: &zero})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for zero date, got %v", err)
	}
}

func TestApplyWeek_RejectsZeroStartDate(t *testing.T) {
	tmpl := &template.Template{
		ID:    3,
		Scope: template.ScopeWeek,
		Entries: []template.TemplateEntry{
			{FoodID: 1, Portions: 1, DayOffset: 0, SlotID: ptrInt64(1)},
		},
	}
	repo := &mockRepo{
		getFn: func(_ context.Context, _ int64) (*template.Template, error) { return tmpl, nil },
	}
	svc := makeService(repo, &mockPlateRepo{})
	zero := time.Time{}
	_, err := svc.Apply(context.Background(), 3, template.ApplyPayload{StartDate: &zero})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for zero start_date, got %v", err)
	}
}

// ── Atomicity: mid-tx error returns (nil, err) ───────────────────────────────

func TestApplySlot_ErrorReturnsNilPlates(t *testing.T) {
	tmpl := &template.Template{
		ID:    10,
		Scope: template.ScopeSlot,
		// Two entries at different offsets → two plates; fail on the second.
		Entries: []template.TemplateEntry{
			{FoodID: 1, Portions: 1, DayOffset: 0},
			{FoodID: 2, Portions: 1, DayOffset: 1},
		},
	}
	repo := &mockRepo{
		getFn: func(_ context.Context, _ int64) (*template.Template, error) { return tmpl, nil },
	}
	calls := 0
	pr := &mockPlateRepo{
		createFn: func(_ context.Context, _ *plate.Plate) error {
			calls++
			if calls >= 2 {
				return fmt.Errorf("db full")
			}
			return nil
		},
	}
	svc := makeService(repo, pr)
	d := mustDate("2026-04-25")
	slotID := int64(1)
	res, err := svc.Apply(context.Background(), 10, template.ApplyPayload{Date: &d, SlotID: &slotID})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if res != nil {
		t.Errorf("expected nil result on error, got %+v", res)
	}
}

func TestApplyDay_ErrorReturnsNilPlates(t *testing.T) {
	tmpl := &template.Template{
		ID:    11,
		Scope: template.ScopeDay,
		Entries: []template.TemplateEntry{
			{FoodID: 1, Portions: 1, SlotID: ptrInt64(1)},
			{FoodID: 2, Portions: 1, SlotID: ptrInt64(2)},
		},
	}
	repo := &mockRepo{
		getFn: func(_ context.Context, _ int64) (*template.Template, error) { return tmpl, nil },
	}
	calls := 0
	pr := &mockPlateRepo{
		createFn: func(_ context.Context, _ *plate.Plate) error {
			calls++
			if calls >= 2 {
				return fmt.Errorf("db full")
			}
			return nil
		},
	}
	svc := makeService(repo, pr)
	d := mustDate("2026-04-25")
	res, err := svc.Apply(context.Background(), 11, template.ApplyPayload{Date: &d})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if res != nil {
		t.Errorf("expected nil result on error, got %+v", res)
	}
}

func TestApplyWeek_ErrorReturnsNilPlates(t *testing.T) {
	tmpl := &template.Template{
		ID:    12,
		Scope: template.ScopeWeek,
		Entries: []template.TemplateEntry{
			{FoodID: 1, Portions: 1, DayOffset: 0, SlotID: ptrInt64(1)},
			{FoodID: 2, Portions: 1, DayOffset: 6, SlotID: ptrInt64(1)},
		},
	}
	repo := &mockRepo{
		getFn: func(_ context.Context, _ int64) (*template.Template, error) { return tmpl, nil },
	}
	calls := 0
	pr := &mockPlateRepo{
		createFn: func(_ context.Context, _ *plate.Plate) error {
			calls++
			if calls >= 2 {
				return fmt.Errorf("db full")
			}
			return nil
		},
	}
	svc := makeService(repo, pr)
	start := mustDate("2026-04-25")
	res, err := svc.Apply(context.Background(), 12, template.ApplyPayload{StartDate: &start})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if res != nil {
		t.Errorf("expected nil result on error, got %+v", res)
	}
}
