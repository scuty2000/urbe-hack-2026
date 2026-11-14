package item_test

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/item"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/testutil"
)

func newService(t *testing.T, deps item.Deps) (item.Service, repository.Database) {
	t.Helper()
	db := testutil.NewTestDB(t)
	return item.New(db, deps), db
}

func baseInput() item.CreateItemInput {
	return item.CreateItemInput{
		Source:         item.SourceFinderSelf,
		FinderID:       "finder-demo",
		IntermediaryID: "bar-x",
		LegalFinderID:  "bar-x",
		RightsAssigned: true,
		Category:       "smartphone",
		Brand:          "Apple",
		Model:          "iPhone 17",
		Color:          "black",
		Condition:      "good",
		PrivateProfile: map[string]any{"serial": "ABC123", "notes": "cracked corner"},
		Photos:         []string{"uploads/a.jpg"},
		FoundAt:        time.Date(2026, 11, 13, 18, 30, 0, 0, time.UTC),
		FoundLat:       41.9,
		FoundLng:       12.5,
		FoundZone:      "San Lorenzo",
		FoundNotes:     "near the station",
	}
}

func TestCreateItemRequiresRightsAssignment(t *testing.T) {
	svc, _ := newService(t, item.Deps{})

	in := baseInput()
	in.RightsAssigned = false

	_, err := svc.CreateItem(context.Background(), in)
	if !errors.Is(err, errs.ErrInvalidInput) {
		t.Fatalf("want invalid input, got %v", err)
	}
}

func TestCreateItemRequiresIntermediaryAsLegalFinder(t *testing.T) {
	svc, _ := newService(t, item.Deps{})

	in := baseInput()
	in.LegalFinderID = "portineria-sapienza"

	_, err := svc.CreateItem(context.Background(), in)
	if !errors.Is(err, errs.ErrInvalidInput) {
		t.Fatalf("want invalid input, got %v", err)
	}
}

func TestCreateItemPersistsItemCustodyEventAndAudit(t *testing.T) {
	svc, db := newService(t, item.Deps{})
	ctx := context.Background()

	created, err := svc.CreateItem(ctx, baseInput())
	if err != nil {
		t.Fatalf("CreateItem() error = %v", err)
	}

	if created.ID == "" {
		t.Fatal("item id is empty")
	}
	if !regexp.MustCompile(`^LF-[0-9A-F]{8}$`).MatchString(created.PublicCode) {
		t.Fatalf("public code = %q, want LF- plus 8 uppercase hex", created.PublicCode)
	}
	if created.Status != item.StatusPendingCustody {
		t.Fatalf("status = %q, want %q", created.Status, item.StatusPendingCustody)
	}
	if created.CustodyMode != item.CustodyModeIntermediary {
		t.Fatalf("custody mode = %q, want %q", created.CustodyMode, item.CustodyModeIntermediary)
	}
	if created.LegalFinderID != "bar-x" || created.CustodianID != "bar-x" {
		t.Fatalf("legal finder/custodian = %q/%q, want bar-x/bar-x", created.LegalFinderID, created.CustodianID)
	}
	if created.RiskTier != item.RiskHigh || created.Sensitive {
		t.Fatalf("risk = %q sensitive=%v, want high/false", created.RiskTier, created.Sensitive)
	}
	if created.RightsAssignedAt == nil {
		t.Fatal("rights assigned timestamp is nil")
	}

	row, err := db.GetItemByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetItemByID() error = %v", err)
	}
	if row.PrivateProfile == "" || row.PrivateProfile == "{}" {
		t.Fatal("private profile was not persisted")
	}
	if row.Photos == "" || row.Photos == "[]" {
		t.Fatal("photos were not persisted")
	}

	custody, err := db.GetCustodyByItemID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetCustodyByItemID() error = %v", err)
	}
	if custody.CustodianID != "bar-x" || custody.CustodianType != item.CustodyModeIntermediary {
		t.Fatalf("custody = %+v, want bar-x/intermediary", custody)
	}
	if custody.MaxCustodyDays != 7 {
		t.Fatalf("max custody days = %d, want 7", custody.MaxCustodyDays)
	}

	events, err := db.ListCustodyEvents(ctx, created.ID)
	if err != nil {
		t.Fatalf("ListCustodyEvents() error = %v", err)
	}
	if len(events) != 1 || events[0].Type != item.EventRightsAssigned {
		t.Fatalf("events = %+v, want one rights_assigned event", events)
	}
	if events[0].ActorID != "finder-demo" {
		t.Fatalf("event actor = %q, want finder-demo", events[0].ActorID)
	}

	logs, err := db.ListAuditLogs(ctx, repository.AuditLogFilter{Entity: "item", EntityID: created.ID})
	if err != nil {
		t.Fatalf("ListAuditLogs() error = %v", err)
	}
	if len(logs) != 1 || logs[0].Action != "item.enrolled" || logs[0].ActorID != "finder-demo" {
		t.Fatalf("audit logs = %+v, want one item.enrolled by finder-demo", logs)
	}
}

func TestCreateItemSensitiveCategoryGetsTightCustody(t *testing.T) {
	svc, db := newService(t, item.Deps{})

	in := baseInput()
	in.Category = "passport"
	created, err := svc.CreateItem(context.Background(), in)
	if err != nil {
		t.Fatalf("CreateItem() error = %v", err)
	}
	if !created.Sensitive || created.RiskTier != item.RiskSensitive {
		t.Fatalf("risk = %q sensitive=%v, want sensitive/true", created.RiskTier, created.Sensitive)
	}

	custody, err := db.GetCustodyByItemID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetCustodyByItemID() error = %v", err)
	}
	if custody.MaxCustodyDays != 1 {
		t.Fatalf("max custody days = %d, want 1 for sensitive items", custody.MaxCustodyDays)
	}
}

func TestCreateItemOperatorUsesInstitutionAsLegalFinder(t *testing.T) {
	svc, _ := newService(t, item.Deps{})

	in := baseInput()
	in.Source = item.SourceOperator
	in.FinderID = "institution-demo"
	in.IntermediaryID = ""
	in.LegalFinderID = "institution-demo"

	created, err := svc.CreateItem(context.Background(), in)
	if err != nil {
		t.Fatalf("CreateItem() error = %v", err)
	}
	if created.LegalFinderID != "institution-demo" || created.CustodianID != "institution-demo" {
		t.Fatalf("legal finder/custodian = %q/%q, want institution-demo", created.LegalFinderID, created.CustodianID)
	}
	if created.CustodyMode != item.CustodyModeInstitution {
		t.Fatalf("custody mode = %q, want %q", created.CustodyMode, item.CustodyModeInstitution)
	}
}

func TestGetItemAuthorizesOnlyCustodiansAndInstitutions(t *testing.T) {
	svc, _ := newService(t, item.Deps{})
	ctx := context.Background()

	created, err := svc.CreateItem(ctx, baseInput())
	if err != nil {
		t.Fatalf("CreateItem() error = %v", err)
	}

	if _, err := svc.GetItem(ctx, created.ID, item.Actor{ID: "bar-x", Role: item.ActorRoleIntermediary}); err != nil {
		t.Fatalf("custodian intermediary denied: %v", err)
	}
	if _, err := svc.GetItem(ctx, created.ID, item.Actor{ID: "institution-demo", Role: item.ActorRoleInstitution}); err != nil {
		t.Fatalf("institution denied: %v", err)
	}
	if _, err := svc.GetItem(ctx, created.ID, item.Actor{ID: "finder-demo", Role: item.ActorRoleFinder}); !errors.Is(err, errs.ErrForbidden) {
		t.Fatalf("finder access err = %v, want ErrForbidden", err)
	}
	if _, err := svc.GetItem(ctx, created.ID, item.Actor{ID: "other-shop", Role: item.ActorRoleIntermediary}); !errors.Is(err, errs.ErrForbidden) {
		t.Fatalf("other intermediary err = %v, want ErrForbidden", err)
	}
	if _, err := svc.GetItem(ctx, "missing", item.Actor{ID: "institution-demo", Role: item.ActorRoleInstitution}); !errors.Is(err, errs.ErrNotFound) {
		t.Fatalf("missing item err = %v, want ErrNotFound", err)
	}
}

func TestListItemsByCustodianOnlyReturnsOpenStatuses(t *testing.T) {
	svc, db := newService(t, item.Deps{})
	ctx := context.Background()

	first, err := svc.CreateItem(ctx, baseInput())
	if err != nil {
		t.Fatalf("CreateItem() error = %v", err)
	}
	second, err := svc.CreateItem(ctx, baseInput())
	if err != nil {
		t.Fatalf("CreateItem() error = %v", err)
	}
	if err := db.UpdateItemFields(ctx, second.ID, map[string]any{"status": item.StatusReturned}); err != nil {
		t.Fatalf("UpdateItemFields() error = %v", err)
	}
	custody, err := db.GetCustodyByItemID(ctx, first.ID)
	if err != nil {
		t.Fatalf("GetCustodyByItemID() error = %v", err)
	}
	dueAt := time.Now().UTC().Add(48 * time.Hour)
	if err := db.UpdateCustodyFields(ctx, custody.ID, map[string]any{"due_at": dueAt}); err != nil {
		t.Fatalf("UpdateCustodyFields() error = %v", err)
	}

	items, err := svc.ListItemsByCustodian(ctx, "bar-x")
	if err != nil {
		t.Fatalf("ListItemsByCustodian() error = %v", err)
	}
	if len(items) != 1 || items[0].ID != first.ID {
		t.Fatalf("items = %+v, want only %s", items, first.ID)
	}
	if items[0].DueAt == nil || !items[0].DueAt.Equal(dueAt) {
		t.Fatalf("first.DueAt = %v, want %v", items[0].DueAt, dueAt)
	}
}

func TestRiskPolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		category  string
		wantTier  string
		sensitive bool
	}{
		{"identity document", item.RiskSensitive, true},
		{"passport", item.RiskSensitive, true},
		{"credit card", item.RiskSensitive, true},
		{"keys", item.RiskSensitive, true},
		{"medical device", item.RiskSensitive, true},
		{"SIM card", item.RiskSensitive, true},
		{"phone", item.RiskHigh, false},
		{"laptop", item.RiskHigh, false},
		{"wallet", item.RiskHigh, false},
		{"jewelry", item.RiskHigh, false},
		{"watch", item.RiskHigh, false},
		{"camera", item.RiskHigh, false},
		{"backpack", item.RiskMedium, false},
		{"glasses", item.RiskMedium, false},
		{"headphones", item.RiskMedium, false},
		{"umbrella", item.RiskLow, false},
		{"bottle", item.RiskLow, false},
		{"book", item.RiskLow, false},
		{"clothing", item.RiskLow, false},
		{"unknown thing", item.RiskMedium, false},
	}

	for _, tt := range tests {
		t.Run(tt.category, func(t *testing.T) {
			t.Parallel()
			tier, sensitive := item.RiskForCategory(tt.category)
			if tier != tt.wantTier || sensitive != tt.sensitive {
				t.Fatalf("RiskForCategory(%q) = %q/%v, want %q/%v", tt.category, tier, sensitive, tt.wantTier, tt.sensitive)
			}
		})
	}
}

// fakeCataloger lets the item service tests exercise both the d3 and the
// fallback path without any network access.
type fakeCataloger struct {
	responses []map[string]item.Answer
	err       error
	calls     int
	questions []map[string]item.Question
	images    [][]string
}

func (f *fakeCataloger) Ask(_ context.Context, _ string, images []string, questions map[string]item.Question) (map[string]item.Answer, error) {
	f.calls++
	f.images = append(f.images, images)
	f.questions = append(f.questions, questions)
	if f.err != nil {
		return nil, f.err
	}
	if len(f.responses) == 0 {
		return map[string]item.Answer{}, nil
	}
	answer := f.responses[0]
	if len(f.responses) > 1 {
		f.responses = f.responses[1:]
	}
	return answer, nil
}

func TestCatalogFallsBackToFormValues(t *testing.T) {
	cataloger := &fakeCataloger{err: errs.ErrUnavailable}
	svc, _ := newService(t, item.Deps{D3: cataloger})

	form := item.CatalogForm{
		Category:  "wallet",
		Color:     "brown",
		Condition: "worn",
	}

	got, err := svc.Catalog(context.Background(), []item.ImageInput{{Path: "uploads/a.jpg"}}, form)
	if err != nil {
		t.Fatalf("Catalog() error = %v", err)
	}
	if got.Source != item.CatalogSourceForm {
		t.Fatalf("source = %q, want %q", got.Source, item.CatalogSourceForm)
	}
	if got.Category != "wallet" || got.Color != "brown" || got.Condition != "worn" {
		t.Fatalf("catalog = %+v, want form values preserved", got)
	}
}

func TestCatalogUsesD3Answers(t *testing.T) {
	cataloger := &fakeCataloger{responses: []map[string]item.Answer{
		{
			"category": {Type: "choice", Choice: "smartphone", Probabilities: map[string]float64{"smartphone": 0.9}},
			"color":    {Type: "choice", Choice: "black"},
			"condition": {
				Type: "choice", Probabilities: map[string]float64{"good": 0.7, "worn": 0.3},
			},
			"has_crack": {Type: "noul", Noul: 0.8},
			"is_document": {
				Type: "noul", Noul: 0.9,
			},
		},
		{
			"document_type": {Type: "choice", Choice: "passport"},
		},
	}}
	svc, _ := newService(t, item.Deps{D3: cataloger})

	got, err := svc.Catalog(context.Background(), nil, item.CatalogForm{Category: "other"})
	if err != nil {
		t.Fatalf("Catalog() error = %v", err)
	}
	if got.Source != item.CatalogSourceD3 {
		t.Fatalf("source = %q, want %q", got.Source, item.CatalogSourceD3)
	}
	if got.Category != "smartphone" || got.Color != "black" || got.Condition != "good" {
		t.Fatalf("catalog = %+v, want d3 values", got)
	}
	if !got.HasCrack || !got.IsDocument || got.DocumentType != "passport" {
		t.Fatalf("flags = %+v, want crack+document+passport", got)
	}
	if cataloger.calls != 2 {
		t.Fatalf("catalog calls = %d, want 2 (base + document type)", cataloger.calls)
	}
}

func TestCatalogSkipsImagesByDefault(t *testing.T) {
	cataloger := &fakeCataloger{}
	svc, _ := newService(t, item.Deps{D3: cataloger})

	if _, err := svc.Catalog(context.Background(), []item.ImageInput{{
		Data: []byte{0xFF, 0xD8, 0xFF, 0xE0}, MediaType: "image/jpeg",
	}}, item.CatalogForm{Category: "smartphone"}); err != nil {
		t.Fatalf("Catalog() error = %v", err)
	}
	if len(cataloger.images) != 1 || len(cataloger.images[0]) != 0 {
		t.Fatalf("D3 images = %+v, want none by default", cataloger.images)
	}
}

func TestCatalogSendsImagesWhenAllowedForNonDocuments(t *testing.T) {
	cataloger := &fakeCataloger{}
	svc, _ := newService(t, item.Deps{D3: cataloger, AllowD3Images: true})

	if _, err := svc.Catalog(context.Background(), []item.ImageInput{{
		Data: []byte{0xFF, 0xD8, 0xFF, 0xE0}, MediaType: "image/jpeg",
	}}, item.CatalogForm{Category: "smartphone"}); err != nil {
		t.Fatalf("Catalog() error = %v", err)
	}
	if len(cataloger.images) != 1 || len(cataloger.images[0]) != 1 {
		t.Fatalf("D3 images = %+v, want one smartphone image", cataloger.images)
	}
}

func TestCatalogNeverSendsDocumentImagesToD3(t *testing.T) {
	for _, category := range []string{
		"document", "documento", "documenti", "documents", "identity_document",
		"passport", "carta di credito", "bancomat", "chiavi", "dispositivo medico", "",
	} {
		cataloger := &fakeCataloger{}
		svc, _ := newService(t, item.Deps{D3: cataloger, AllowD3Images: true})

		if _, err := svc.Catalog(context.Background(), []item.ImageInput{{
			Data: []byte{0xFF, 0xD8, 0xFF, 0xE0}, MediaType: "image/jpeg",
		}}, item.CatalogForm{Category: category}); err != nil {
			t.Fatalf("Catalog(%q) error = %v", category, err)
		}
		for _, images := range cataloger.images {
			if len(images) != 0 {
				t.Fatalf("D3 images for category %q = %+v, want none", category, images)
			}
		}
	}
}

func TestAnalyzeSendsImagesWithoutEnrollmentFlag(t *testing.T) {
	cataloger := &fakeCataloger{}
	svc, _ := newService(t, item.Deps{D3: cataloger})

	if _, err := svc.Analyze(context.Background(), []item.ImageInput{{
		Data: []byte{0xFF, 0xD8, 0xFF, 0xE0}, MediaType: "image/jpeg",
	}}, item.CatalogForm{}); err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if len(cataloger.images) != 1 || len(cataloger.images[0]) != 1 {
		t.Fatalf("D3 images = %+v, want the first wizard photo", cataloger.images)
	}
}

func TestAnalyzeSkipsKnownDocumentCategory(t *testing.T) {
	cataloger := &fakeCataloger{}
	svc, _ := newService(t, item.Deps{D3: cataloger})

	if _, err := svc.Analyze(context.Background(), []item.ImageInput{{
		Data: []byte{0xFF, 0xD8, 0xFF, 0xE0}, MediaType: "image/jpeg",
	}}, item.CatalogForm{Category: "documenti"}); err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if len(cataloger.images) != 1 || len(cataloger.images[0]) != 0 {
		t.Fatalf("D3 images = %+v, want none for a known document category", cataloger.images)
	}
}

func TestAnalyzeReadsCaseColor(t *testing.T) {
	cataloger := &fakeCataloger{responses: []map[string]item.Answer{{
		"category":   {Type: "choice", Choice: "smartphone"},
		"color":      {Type: "choice", Choice: "black"},
		"case_color": {Type: "choice", Choice: "blue"},
		"condition":  {Type: "choice", Choice: "good"},
	}}}
	svc, _ := newService(t, item.Deps{D3: cataloger})

	got, err := svc.Analyze(context.Background(), []item.ImageInput{{
		Data: []byte{0xFF, 0xD8, 0xFF, 0xE0}, MediaType: "image/jpeg",
	}}, item.CatalogForm{})
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if got.CaseColor != "blue" || got.Category != "smartphone" {
		t.Fatalf("Analyze() = %+v, want case color blue and smartphone", got)
	}
}

func TestCreateItemDocumentSignalFloorsRiskToSensitive(t *testing.T) {
	cataloger := &fakeCataloger{responses: []map[string]item.Answer{
		{
			"category":    {Type: "choice", Choice: "book"},
			"color":       {Type: "choice", Choice: "black"},
			"condition":   {Type: "choice", Choice: "good"},
			"is_document": {Type: "noul", Noul: 0.9},
		},
		{
			"document_type": {Type: "choice", Choice: "passport"},
		},
	}}
	svc, db := newService(t, item.Deps{D3: cataloger})

	in := baseInput()
	in.Category = "book"
	in.Color = "black"
	in.Condition = "good"

	created, err := svc.CreateItem(context.Background(), in)
	if err != nil {
		t.Fatalf("CreateItem() error = %v", err)
	}
	if cataloger.calls != 2 {
		t.Fatalf("catalog calls = %d, want 2 (base + document type)", cataloger.calls)
	}
	if created.RiskTier != item.RiskSensitive || !created.Sensitive {
		t.Fatalf("risk = %q sensitive=%v, want sensitive/true", created.RiskTier, created.Sensitive)
	}
	if got := created.PrivateProfile["document_type"]; got != "passport" {
		t.Fatalf("document_type = %v, want passport", got)
	}
	if got := created.PrivateProfile["catalog_source"]; got != item.CatalogSourceD3 {
		t.Fatalf("catalog_source = %v, want d3", got)
	}

	row, err := db.GetItemByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetItemByID() error = %v", err)
	}
	if !strings.Contains(row.PrivateProfile, `"document_type":"passport"`) ||
		!strings.Contains(row.PrivateProfile, `"catalog_source":"d3"`) {
		t.Fatalf("persisted private profile = %s, want document_type and catalog_source", row.PrivateProfile)
	}
	if row.RiskTier != item.RiskSensitive || !row.Sensitive {
		t.Fatalf("persisted risk = %q sensitive=%v, want sensitive/true", row.RiskTier, row.Sensitive)
	}
}

func TestCreateItemFallsBackToFormCategoryWhenD3Unavailable(t *testing.T) {
	cataloger := &fakeCataloger{err: errs.ErrUnavailable}
	svc, _ := newService(t, item.Deps{D3: cataloger})

	in := baseInput()
	in.Category = ""
	in.Color = ""

	created, err := svc.CreateItem(context.Background(), in)
	if err != nil {
		t.Fatalf("CreateItem() error = %v", err)
	}
	// The form was empty too, so the item is still enrolled with a safe
	// default category rather than failing the whole intake.
	if created.Category == "" {
		t.Fatal("category is empty after fallback")
	}
	if created.RiskTier == "" {
		t.Fatal("risk tier is empty after fallback")
	}
}
