package wizard_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/item"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/wizard"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository"
)

type fakeAnalyzer struct {
	result item.CatalogResult
	err    error
	calls  int
	photos []item.ImageInput
	form   item.CatalogForm
}

func (f *fakeAnalyzer) Analyze(_ context.Context, photos []item.ImageInput, form item.CatalogForm) (item.CatalogResult, error) {
	f.calls++
	f.photos = photos
	f.form = form
	return f.result, f.err
}

type fakePlanner struct {
	plan  wizard.Plan
	err   error
	calls int
	input wizard.PlannerInput
}

func (f *fakePlanner) Plan(_ context.Context, input wizard.PlannerInput) (wizard.Plan, error) {
	f.calls++
	f.input = input
	return f.plan, f.err
}

func photo() item.ImageInput {
	return item.ImageInput{Data: []byte{0xFF, 0xD8, 0xFF, 0xE0}, MediaType: "image/jpeg"}
}

func TestAnalyzeUsesD3AndPlanner(t *testing.T) {
	analyzer := &fakeAnalyzer{result: item.CatalogResult{
		Source: item.CatalogSourceD3, Category: "smartphone", Color: "black",
		CaseColor: "blue", Condition: "good", HasCrack: true,
	}}
	planner := &fakePlanner{plan: wizard.Plan{
		PhotoPlan: []wizard.PhotoSlot{{
			Key: "back", Label: "Retro", Instruction: "Fotografa il retro", Required: true,
		}},
		Prefill:        wizard.Prefill{Category: "smartphone", Brand: "Apple", Model: "iPhone 17", Color: "nero"},
		PrivatePrefill: "Custodia blu, crepa visibile sull'angolo.",
		Warnings:       []string{"Controlla il colore."},
	}}
	svc := wizard.New(repository.Database{}, wizard.Deps{Items: analyzer, Planner: planner})

	got, err := svc.Analyze(context.Background(), wizard.AnalyzeInput{Photos: []item.ImageInput{photo()}})
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if got.Source != "d3+deepseek" {
		t.Fatalf("source = %q", got.Source)
	}
	if got.Prefill.Category != "smartphone" || got.Prefill.Color != "nero" || got.Prefill.Condition != "good" {
		t.Fatalf("prefill = %+v", got.Prefill)
	}
	if got.Prefill.Brand != "Apple" || got.Prefill.Model != "iPhone 17" {
		t.Fatalf("prefill did not use the vision planner: %+v", got.Prefill)
	}
	if got.PrivatePrefill != "Custodia blu, crepa visibile sull'angolo." {
		t.Fatalf("privatePrefill = %q", got.PrivatePrefill)
	}
	if len(got.PhotoPlan) != 1 || got.PhotoPlan[0].Key != "back" {
		t.Fatalf("photoPlan = %+v", got.PhotoPlan)
	}
	if analyzer.calls != 1 || planner.calls != 1 {
		t.Fatalf("calls analyzer=%d planner=%d", analyzer.calls, planner.calls)
	}
	if planner.input.CaseColor != "blue" || planner.input.PhotoCount != 1 || planner.input.HasCrack != true {
		t.Fatalf("planner input = %+v", planner.input)
	}
	if len(planner.input.Photos) != 1 || len(planner.input.Photos[0].Data) == 0 {
		t.Fatalf("planner did not receive the photo: %+v", planner.input.Photos)
	}
}

func TestAnalyzeDocumentSkipsPlanner(t *testing.T) {
	analyzer := &fakeAnalyzer{result: item.CatalogResult{
		Source: item.CatalogSourceD3, Category: "document", IsDocument: true,
	}}
	planner := &fakePlanner{}
	svc := wizard.New(repository.Database{}, wizard.Deps{Items: analyzer, Planner: planner})

	got, err := svc.Analyze(context.Background(), wizard.AnalyzeInput{Photos: []item.ImageInput{photo()}})
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if !got.IsDocument || !got.Sensitive || got.Prefill.Category != "documenti" {
		t.Fatalf("analysis = %+v", got)
	}
	if len(got.PhotoPlan) != 0 {
		t.Fatalf("photoPlan = %+v, want none for a document", got.PhotoPlan)
	}
	if planner.calls != 0 {
		t.Fatalf("planner calls = %d, want 0", planner.calls)
	}
	if got.PrivatePrefill != "" {
		t.Fatalf("privatePrefill = %q, want empty", got.PrivatePrefill)
	}
	if len(got.Warnings) == 0 || !strings.Contains(strings.ToLower(got.Warnings[0]), "documento") {
		t.Fatalf("warnings = %+v", got.Warnings)
	}
}

func TestAnalyzeFallsBackWhenPlannerFails(t *testing.T) {
	analyzer := &fakeAnalyzer{result: item.CatalogResult{
		Source: item.CatalogSourceD3, Category: "wallet", Color: "brown", Condition: "worn",
	}}
	planner := &fakePlanner{err: errs.ErrUnavailable}
	svc := wizard.New(repository.Database{}, wizard.Deps{Items: analyzer, Planner: planner})

	got, err := svc.Analyze(context.Background(), wizard.AnalyzeInput{Photos: []item.ImageInput{photo()}})
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if got.Source != "d3+fallback" {
		t.Fatalf("source = %q", got.Source)
	}
	if got.Prefill.Category != "portafoglio" || got.Prefill.Color != "marrone" || got.Prefill.Condition != "fair" {
		t.Fatalf("prefill = %+v", got.Prefill)
	}
	if len(got.PhotoPlan) == 0 {
		t.Fatal("fallback plan is empty")
	}
	if len(got.Warnings) == 0 {
		t.Fatal("fallback warning is empty")
	}
}

func TestAnalyzeFallsBackToManualWhenD3Fails(t *testing.T) {
	analyzer := &fakeAnalyzer{err: errs.ErrUnavailable}
	planner := &fakePlanner{}
	svc := wizard.New(repository.Database{}, wizard.Deps{Items: analyzer, Planner: planner})

	got, err := svc.Analyze(context.Background(), wizard.AnalyzeInput{Photos: []item.ImageInput{photo()}})
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if got.Source != "manual" || len(got.Warnings) == 0 {
		t.Fatalf("analysis = %+v", got)
	}
	if len(got.PhotoPlan) == 0 {
		t.Fatal("manual plan must suggest a detail photo")
	}
	if planner.calls != 0 {
		t.Fatalf("planner calls = %d, want 0 when D3 is down", planner.calls)
	}
}

func TestAnalyzeDocumentCategoryStopsPlannerEvenWithoutNoul(t *testing.T) {
	analyzer := &fakeAnalyzer{result: item.CatalogResult{
		Source: item.CatalogSourceD3, Category: "document", IsDocument: false,
	}}
	planner := &fakePlanner{}
	svc := wizard.New(repository.Database{}, wizard.Deps{Items: analyzer, Planner: planner})

	got, err := svc.Analyze(context.Background(), wizard.AnalyzeInput{Photos: []item.ImageInput{photo()}})
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if !got.IsDocument || !got.Sensitive || got.Prefill.Category != "documenti" {
		t.Fatalf("analysis = %+v, want a document", got)
	}
	if len(got.PhotoPlan) != 0 || planner.calls != 0 {
		t.Fatalf("plan = %+v planner calls=%d, want no document photos", got.PhotoPlan, planner.calls)
	}
}

func TestAnalyzeRejectsInvalidPhotos(t *testing.T) {
	svc := wizard.New(repository.Database{}, wizard.Deps{Items: &fakeAnalyzer{result: item.CatalogResult{Source: item.CatalogSourceD3}}})
	cases := []wizard.AnalyzeInput{
		{},
		{Photos: []item.ImageInput{{Data: nil, MediaType: "image/jpeg"}}},
		{Photos: []item.ImageInput{{Data: []byte("x"), MediaType: "text/plain"}}},
		{Photos: []item.ImageInput{photo(), photo(), photo(), photo(), photo()}},
	}
	for _, input := range cases {
		if _, err := svc.Analyze(context.Background(), input); !errors.Is(err, errs.ErrInvalidInput) {
			t.Fatalf("Analyze(%+v) error = %v, want ErrInvalidInput", input, err)
		}
	}
}

func TestAnalyzeMapsDocumentCategoriesToSensitive(t *testing.T) {
	analyzer := &fakeAnalyzer{result: item.CatalogResult{
		Source: item.CatalogSourceD3, Category: "document", IsDocument: true,
	}}
	svc := wizard.New(repository.Database{}, wizard.Deps{Items: analyzer})
	got, err := svc.Analyze(context.Background(), wizard.AnalyzeInput{Photos: []item.ImageInput{photo()}})
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if !got.Sensitive || got.Prefill.Category != "documenti" {
		t.Fatalf("analysis = %+v", got)
	}
}

func TestAnalyzeTruncatesPlannerSlots(t *testing.T) {
	analyzer := &fakeAnalyzer{result: item.CatalogResult{
		Source: item.CatalogSourceD3, Category: "smartphone",
	}}
	planner := &fakePlanner{plan: wizard.Plan{PhotoPlan: []wizard.PhotoSlot{
		{Key: "a", Label: "A", Instruction: "A"}, {Key: "b", Label: "B", Instruction: "B"},
		{Key: "c", Label: "C", Instruction: "C"}, {Key: "d", Label: "D", Instruction: "D"},
	}}}
	svc := wizard.New(repository.Database{}, wizard.Deps{Items: analyzer, Planner: planner})
	got, err := svc.Analyze(context.Background(), wizard.AnalyzeInput{Photos: []item.ImageInput{photo()}})
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if len(got.PhotoPlan) != 3 {
		t.Fatalf("photoPlan = %d slots, want 3", len(got.PhotoPlan))
	}
}

func TestAnalyzeRejectsUnsafeSlotKeys(t *testing.T) {
	analyzer := &fakeAnalyzer{result: item.CatalogResult{
		Source: item.CatalogSourceD3, Category: "smartphone",
	}}
	planner := &fakePlanner{plan: wizard.Plan{PhotoPlan: []wizard.PhotoSlot{
		{Key: "__proto__", Label: "Bad", Instruction: "Bad"},
		{Key: "back", Label: "Retro", Instruction: "Fotografa il retro"},
	}}}
	svc := wizard.New(repository.Database{}, wizard.Deps{Items: analyzer, Planner: planner})
	got, err := svc.Analyze(context.Background(), wizard.AnalyzeInput{Photos: []item.ImageInput{photo()}})
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if len(got.PhotoPlan) != 1 || got.PhotoPlan[0].Key != "back" {
		t.Fatalf("photoPlan = %+v, want only the safe key", got.PhotoPlan)
	}
}

func TestNormalizeCategoryTable(t *testing.T) {
	for raw, want := range map[string]string{
		"smartphone": "smartphone",
		"laptop":     "laptop",
		"wallet":     "portafoglio",
		"keys":       "chiavi",
		"document":   "documenti",
		"jewelry":    "gioielli",
		"watch":      "orologio",
		"book":       "libro",
		"bottle":     "bottiglia",
		"other":      "altro",
	} {
		if got := wizard.NormalizeCategory(raw); got != want {
			t.Fatalf("NormalizeCategory(%q) = %q, want %q", raw, got, want)
		}
	}
}
