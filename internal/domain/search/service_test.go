package search_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	itemdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/item"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/search"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/platform/qdrant"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/seed"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/testutil"
)

func TestSearchFallsBackToSQLiteAndHidesPrivateData(t *testing.T) {
	db := testutil.NewTestDB(t)
	seed.Demo(t, db)

	svc := search.New(db, search.Deps{QdrantURL: "", EmbeddingsURL: ""})
	got, err := svc.Search(context.Background(), search.Query{
		Text:     "iphone nero",
		Category: "smartphone",
		Zone:     "San Lorenzo",
	})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(got) == 0 {
		t.Fatal("Search() returned no results")
	}
	if got[0].ItemID != "item-phone" {
		t.Fatalf("first result = %+v, want item-phone", got[0])
	}
	if got[0].Category != "smartphone" || got[0].FoundZone != "San Lorenzo" {
		t.Fatalf("result fields = %+v", got[0])
	}
	if got[0].CustodianType != itemdomain.CustodyModeIntermediary {
		t.Fatalf("custodian type = %q", got[0].CustodianType)
	}

	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshalling results: %v", err)
	}
	payload := strings.ToLower(string(encoded))
	for _, forbidden := range []string{"private", "custodia blu", "blu", "41.90", "41.9012", "12.51"} {
		if strings.Contains(payload, strings.ToLower(forbidden)) {
			t.Fatalf("search result leaks %q: %s", forbidden, payload)
		}
	}
}

func TestSearchHidesPendingItems(t *testing.T) {
	db := testutil.NewTestDB(t)
	seed.Demo(t, db)

	svc := search.New(db, search.Deps{})
	ctx := context.Background()

	for _, tc := range []struct {
		name string
		text string
	}{
		{name: "umbrella", text: "ombrello"},
		{name: "document", text: "documento identità"},
	} {
		got, err := svc.Search(ctx, search.Query{Text: tc.text})
		if err != nil {
			t.Fatalf("%s: Search() error = %v", tc.name, err)
		}
		for _, result := range got {
			if result.ItemID == "item-umbrella" || result.ItemID == "item-document" {
				t.Fatalf("%s: pending item leaked into search: %+v", tc.name, result)
			}
		}
	}
}

func TestSearchFiltersByCategoryZoneAndTime(t *testing.T) {
	db := testutil.NewTestDB(t)
	seed.Demo(t, db)
	now := time.Now().UTC()

	svc := search.New(db, search.Deps{})
	ctx := context.Background()

	byCategory, err := svc.Search(ctx, search.Query{Category: "wallet"})
	if err != nil {
		t.Fatalf("category search: %v", err)
	}
	if len(byCategory) != 1 || byCategory[0].ItemID != "item-wallet" {
		t.Fatalf("category results = %+v", byCategory)
	}

	byZone, err := svc.Search(ctx, search.Query{Zone: "San Lorenzo"})
	if err != nil {
		t.Fatalf("zone search: %v", err)
	}
	if len(byZone) != 2 {
		t.Fatalf("zone results = %+v, want phone and wallet", byZone)
	}

	recent, err := svc.Search(ctx, search.Query{FoundAfter: now.Add(-24 * time.Hour)})
	if err != nil {
		t.Fatalf("found-after search: %v", err)
	}
	if len(recent) != 1 || recent[0].ItemID != "item-phone" {
		t.Fatalf("found-after results = %+v, want only item-phone", recent)
	}

	older, err := svc.Search(ctx, search.Query{FoundBefore: now.Add(-24 * time.Hour)})
	if err != nil {
		t.Fatalf("found-before search: %v", err)
	}
	if len(older) != 1 || older[0].ItemID != "item-wallet" {
		t.Fatalf("found-before results = %+v, want only item-wallet", older)
	}
}

type fakeEmbedder struct {
	text   string
	vector []float32
	err    error
}

func (f *fakeEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	f.text = text
	if f.err != nil {
		return nil, f.err
	}
	if f.vector == nil {
		return make([]float32, qdrant.VectorSize), nil
	}
	return f.vector, nil
}

type fakeVectors struct {
	point   qdrant.Point
	deleted string
}

func (f *fakeVectors) Upsert(_ context.Context, point qdrant.Point) error {
	f.point = point
	return nil
}

func (f *fakeVectors) Search(context.Context, []float32, qdrant.Filter, int) ([]qdrant.ScoredPoint, error) {
	return nil, nil
}

func (f *fakeVectors) Delete(_ context.Context, itemID string) error {
	f.deleted = itemID
	return nil
}

func TestIndexItemOnlyEmbedsPublicFields(t *testing.T) {
	db := testutil.NewTestDB(t)
	seed.Demo(t, db)

	row, err := db.GetItemByID(context.Background(), "item-phone")
	if err != nil {
		t.Fatalf("loading item-phone: %v", err)
	}
	phone, err := itemdomain.FromRecord(row)
	if err != nil {
		t.Fatalf("mapping item-phone: %v", err)
	}

	embedder := &fakeEmbedder{}
	vectors := &fakeVectors{}
	svc := search.New(db, search.Deps{Embedder: embedder, Vectors: vectors})
	if err := svc.IndexItem(context.Background(), phone); err != nil {
		t.Fatalf("IndexItem() error = %v", err)
	}

	text := strings.ToLower(embedder.text)
	for _, want := range []string{"iphone", "nero", "san lorenzo"} {
		if !strings.Contains(text, want) {
			t.Fatalf("index text %q does not contain %q", embedder.text, want)
		}
	}
	for _, forbidden := range []string{"custodia", "blu", "crepa", "adesivo", "41.9012", "12.5155"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("index text leaks %q: %s", forbidden, embedder.text)
		}
	}

	if len(vectors.point.Vector) != qdrant.VectorSize {
		t.Fatalf("vector length = %d", len(vectors.point.Vector))
	}
	if vectors.point.ItemID != "item-phone" {
		t.Fatalf("point item id = %q", vectors.point.ItemID)
	}
	wantPayload := map[string]bool{
		"item_id": true, "category": true, "zone": true,
		"status": true, "custodian_type": true, "description": true,
	}
	if len(vectors.point.Payload) != len(wantPayload) {
		t.Fatalf("payload keys = %v", vectors.point.Payload)
	}
	for key := range wantPayload {
		if _, ok := vectors.point.Payload[key]; !ok {
			t.Fatalf("payload missing %q: %v", key, vectors.point.Payload)
		}
	}
	if _, leaked := vectors.point.Payload["private_profile"]; leaked {
		t.Fatal("payload leaked private_profile")
	}
	if _, leaked := vectors.point.Payload["found_lat"]; leaked {
		t.Fatal("payload leaked coordinates")
	}
}

func TestIndexItemSkipsPendingAndDeletesIndex(t *testing.T) {
	db := testutil.NewTestDB(t)
	seed.Demo(t, db)

	row, err := db.GetItemByID(context.Background(), "item-umbrella")
	if err != nil {
		t.Fatalf("loading item-umbrella: %v", err)
	}
	umbrella, err := itemdomain.FromRecord(row)
	if err != nil {
		t.Fatalf("mapping item-umbrella: %v", err)
	}

	embedder := &fakeEmbedder{}
	vectors := &fakeVectors{}
	svc := search.New(db, search.Deps{Embedder: embedder, Vectors: vectors})
	if err := svc.IndexItem(context.Background(), umbrella); err != nil {
		t.Fatalf("IndexItem() error = %v", err)
	}
	if embedder.text != "" {
		t.Fatalf("pending item was embedded: %q", embedder.text)
	}
	if vectors.deleted != "item-umbrella" {
		t.Fatalf("deleted = %q, want item-umbrella", vectors.deleted)
	}

	if err := svc.DeleteItem(context.Background(), "item-wallet"); err != nil {
		t.Fatalf("DeleteItem() error = %v", err)
	}
	if vectors.deleted != "item-wallet" {
		t.Fatalf("deleted = %q, want item-wallet", vectors.deleted)
	}
}
