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
	createFn                 func(ctx context.Context, t *template.Template) error
	getFn                    func(ctx context.Context, id int64) (*template.Template, error)
	updateNameFn             func(ctx context.Context, id int64, name string) (*template.Template, error)
	deleteFn                 func(ctx context.Context, id int64) error
	listFn                   func(ctx context.Context) ([]template.Template, error)
	replaceComponentsFn      func(ctx context.Context, templateID int64, comps []template.TemplateComponent) error
	listComponentsByTemplate func(ctx context.Context, templateID int64) ([]template.TemplateComponent, error)
	countUsingFoodFn         func(ctx context.Context, foodID int64) (int64, error)
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

func (m *mockRepo) ReplaceComponents(ctx context.Context, templateID int64, comps []template.TemplateComponent) error {
	if m.replaceComponentsFn != nil {
		return m.replaceComponentsFn(ctx, templateID, comps)
	}
	return nil
}

func (m *mockRepo) ListComponentsByTemplate(ctx context.Context, templateID int64) ([]template.TemplateComponent, error) {
	if m.listComponentsByTemplate != nil {
		return m.listComponentsByTemplate(ctx, templateID)
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
}

func (m *mockFoodChecker) Exists(ctx context.Context, foodID int64) (bool, error) {
	if m.existsFn != nil {
		return m.existsFn(ctx, foodID)
	}
	return true, nil
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

type mockTxRunner struct{}

func (m *mockTxRunner) RunInTemplateTx(ctx context.Context, fn func(template.Repository, plate.Repository) error) error {
	return fn(nil, nil)
}

type mockPlateCreator struct {
	createFn func(ctx context.Context, p *plate.Plate) error
	nextID   int64
}

func (m *mockPlateCreator) Create(ctx context.Context, p *plate.Plate) error {
	if m.createFn != nil {
		return m.createFn(ctx, p)
	}
	m.nextID++
	p.ID = m.nextID
	return nil
}

// ── helpers ─────────────────────────────────────────────────────────────────

func mustDate(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func makeService(repo *mockRepo, pc *mockPlateCreator) *template.Service {
	return template.NewService(repo, &mockFoodChecker{}, &mockPlateComponentSource{}, &mockTxRunner{}, pc)
}

// ── Apply tests ──────────────────────────────────────────────────────────────

func TestApply_HappyPath(t *testing.T) {
	// Template with 3 entries: 2 at offset 0, 1 at offset 1.
	// Start = Saturday 2026-04-25.
	// Expect: plate at 2026-04-25 (offset 0) and 2026-04-26 (offset 1).
	start := mustDate("2026-04-25")

	tmpl := &template.Template{
		ID:   10,
		Name: "Test",
		Components: []template.TemplateComponent{
			{FoodID: 1, Portions: 1, DayOffset: 0, SortOrder: 0},
			{FoodID: 2, Portions: 2, DayOffset: 0, SortOrder: 1},
			{FoodID: 3, Portions: 1, DayOffset: 1, SortOrder: 0},
		},
	}

	repo := &mockRepo{
		getFn: func(_ context.Context, id int64) (*template.Template, error) {
			return tmpl, nil
		},
	}
	pc := &mockPlateCreator{}
	svc := makeService(repo, pc)

	plates, err := svc.Apply(context.Background(), 10, start, 1)
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}

	if len(plates) != 2 {
		t.Fatalf("expected 2 plates, got %d", len(plates))
	}

	// First group (offset 0) → Saturday.
	if plates[0].Date != mustDate("2026-04-25") {
		t.Errorf("plates[0].Date = %s; want 2026-04-25", plates[0].Date.Format("2006-01-02"))
	}
	if len(plates[0].Components) != 2 {
		t.Errorf("plates[0] components = %d; want 2", len(plates[0].Components))
	}

	// Second group (offset 1) → Sunday.
	if plates[1].Date != mustDate("2026-04-26") {
		t.Errorf("plates[1].Date = %s; want 2026-04-26", plates[1].Date.Format("2006-01-02"))
	}
	if len(plates[1].Components) != 1 {
		t.Errorf("plates[1] components = %d; want 1", len(plates[1].Components))
	}
}

func TestApply_ISOWeekBoundary(t *testing.T) {
	// Start = Saturday 2026-04-25, offset 6 = following Friday 2026-05-01.
	start := mustDate("2026-04-25")
	expected := mustDate("2026-05-01")

	tmpl := &template.Template{
		ID:   11,
		Name: "Week Span",
		Components: []template.TemplateComponent{
			{FoodID: 1, Portions: 1, DayOffset: 6, SortOrder: 0},
		},
	}
	repo := &mockRepo{
		getFn: func(_ context.Context, _ int64) (*template.Template, error) { return tmpl, nil },
	}
	pc := &mockPlateCreator{}
	svc := makeService(repo, pc)

	plates, err := svc.Apply(context.Background(), 11, start, 1)
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if len(plates) != 1 {
		t.Fatalf("expected 1 plate, got %d", len(plates))
	}
	if plates[0].Date != expected {
		t.Errorf("plate date = %s; want %s", plates[0].Date.Format("2006-01-02"), expected.Format("2006-01-02"))
	}
}

func TestApply_ConflictPropagated(t *testing.T) {
	tmpl := &template.Template{
		ID:   12,
		Name: "Conflict",
		Components: []template.TemplateComponent{
			{FoodID: 1, Portions: 1, DayOffset: 0, SortOrder: 0},
		},
	}
	repo := &mockRepo{
		getFn: func(_ context.Context, _ int64) (*template.Template, error) { return tmpl, nil },
	}
	conflictErr := fmt.Errorf("%w: plate already exists", domain.ErrDuplicateName)
	pc := &mockPlateCreator{
		createFn: func(_ context.Context, _ *plate.Plate) error {
			return conflictErr
		},
	}
	svc := makeService(repo, pc)

	_, err := svc.Apply(context.Background(), 12, mustDate("2026-04-25"), 1)
	if err == nil {
		t.Fatal("expected error from Apply, got nil")
	}
	if !errors.Is(err, domain.ErrDuplicateName) {
		t.Errorf("error = %v; want to wrap ErrDuplicateName", err)
	}
}

func TestApply_MissingSlotID(t *testing.T) {
	repo := &mockRepo{}
	pc := &mockPlateCreator{}
	svc := makeService(repo, pc)

	_, err := svc.Apply(context.Background(), 1, mustDate("2026-04-25"), 0)
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput, got %v", err)
	}
}

// ── SaveAsTemplate tests ─────────────────────────────────────────────────────

func TestSaveAsTemplate_HappyPath(t *testing.T) {
	// 3 plates on 3 consecutive days, anchorDate = first day.
	anchor := mustDate("2026-04-25")
	plates := []plate.Plate{
		{Date: mustDate("2026-04-25"), Components: []plate.PlateComponent{{FoodID: 1, Portions: 1}}},
		{Date: mustDate("2026-04-26"), Components: []plate.PlateComponent{{FoodID: 2, Portions: 2}}},
		{Date: mustDate("2026-04-27"), Components: []plate.PlateComponent{{FoodID: 3, Portions: 1}}},
	}

	var created *template.Template
	repo := &mockRepo{
		createFn: func(_ context.Context, t *template.Template) error {
			t.ID = 99
			created = t
			return nil
		},
	}
	svc := makeService(repo, &mockPlateCreator{})

	tmpl, err := svc.SaveAsTemplate(context.Background(), "My Pattern", plates, anchor)
	if err != nil {
		t.Fatalf("SaveAsTemplate returned error: %v", err)
	}
	if tmpl.ID != 99 {
		t.Errorf("tmpl.ID = %d; want 99", tmpl.ID)
	}
	if len(created.Components) != 3 {
		t.Fatalf("expected 3 components, got %d", len(created.Components))
	}
	offsets := make([]int, len(created.Components))
	for i, c := range created.Components {
		offsets[i] = c.DayOffset
	}
	for i, want := range []int{0, 1, 2} {
		if offsets[i] != want {
			t.Errorf("component[%d].DayOffset = %d; want %d", i, offsets[i], want)
		}
	}
}

func TestSaveAsTemplate_RejectsPlateBeforeAnchor(t *testing.T) {
	anchor := mustDate("2026-04-25")
	plates := []plate.Plate{
		{Date: mustDate("2026-04-24"), Components: []plate.PlateComponent{{FoodID: 1, Portions: 1}}},
	}
	repo := &mockRepo{}
	svc := makeService(repo, &mockPlateCreator{})

	_, err := svc.SaveAsTemplate(context.Background(), "Bad Pattern", plates, anchor)
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput, got %v", err)
	}
}

func TestSaveAsTemplate_EmptyPlates(t *testing.T) {
	repo := &mockRepo{}
	svc := makeService(repo, &mockPlateCreator{})

	_, err := svc.SaveAsTemplate(context.Background(), "Empty", []plate.Plate{}, mustDate("2026-04-25"))
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput, got %v", err)
	}
}
