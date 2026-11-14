package seed_test

import (
	"context"
	"testing"

	itemdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/item"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/seed"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/testutil"
)

func TestDemoSeedsDeterministicDataset(t *testing.T) {
	db := testutil.NewTestDB(t)
	ctx := context.Background()

	// Demo runs twice: the reset/seed path must be repeatable.
	seed.Demo(t, db)
	seed.Demo(t, db)

	users, err := db.ListUsers(ctx)
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if len(users) != 7 {
		t.Fatalf("users = %d, want 7", len(users))
	}

	intermediaries, err := db.ListIntermediaries(ctx)
	if err != nil {
		t.Fatalf("ListIntermediaries: %v", err)
	}
	if len(intermediaries) != 2 {
		t.Fatalf("intermediaries = %d, want 2", len(intermediaries))
	}
	if _, err := db.GetIntermediaryByID(ctx, "bar-x"); err != nil {
		t.Fatalf("bar-x missing: %v", err)
	}
	if _, err := db.GetIntermediaryByID(ctx, "portineria-sapienza"); err != nil {
		t.Fatalf("portineria-sapienza missing: %v", err)
	}

	tariff, err := db.GetActiveTariff(ctx)
	if err != nil {
		t.Fatalf("GetActiveTariff: %v", err)
	}
	if tariff.DailyRateCents != 500 || tariff.FixedFeeCents != 1000 || tariff.Currency != "EUR" {
		t.Fatalf("tariff = %+v", tariff)
	}

	items, err := db.SearchItems(ctx, repository.ItemFilter{Limit: 50})
	if err != nil {
		t.Fatalf("SearchItems: %v", err)
	}
	if len(items) != 4 {
		t.Fatalf("items = %d, want 4", len(items))
	}
	statuses := map[string]string{}
	for _, row := range items {
		statuses[row.ID] = row.Status
	}
	if statuses["item-phone"] != itemdomain.StatusInCustody {
		t.Fatalf("item-phone status = %q, want in_custody", statuses["item-phone"])
	}
	if statuses["item-wallet"] != "transferred" {
		t.Fatalf("item-wallet status = %q, want transferred", statuses["item-wallet"])
	}
	if statuses["item-umbrella"] != itemdomain.StatusPendingCustody {
		t.Fatalf("item-umbrella status = %q", statuses["item-umbrella"])
	}
	if statuses["item-document"] != itemdomain.StatusPendingCustody {
		t.Fatalf("item-document status = %q", statuses["item-document"])
	}

	phoneRow, err := db.GetItemByID(ctx, "item-phone")
	if err != nil {
		t.Fatalf("GetItemByID(item-phone): %v", err)
	}
	phone, err := itemdomain.FromRecord(phoneRow)
	if err != nil {
		t.Fatalf("mapping item-phone: %v", err)
	}
	if phone.CustodianID != "bar-x" || phone.FoundZone != "San Lorenzo" || phone.Category != "smartphone" {
		t.Fatalf("item-phone = %+v", phone)
	}
	if phone.PrivateProfile["case_color"] != "blu" {
		t.Fatalf("item-phone private profile = %v", phone.PrivateProfile)
	}

	documentRow, err := db.GetItemByID(ctx, "item-document")
	if err != nil {
		t.Fatalf("GetItemByID(item-document): %v", err)
	}
	if !documentRow.Sensitive || documentRow.RiskTier != itemdomain.RiskSensitive {
		t.Fatalf("item-document = %+v, want sensitive", documentRow)
	}

	custodyRow, err := db.GetCustodyByItemID(ctx, "item-phone")
	if err != nil {
		t.Fatalf("GetCustodyByItemID(item-phone): %v", err)
	}
	if custodyRow.AcceptedAt == nil || custodyRow.DueAt == nil || custodyRow.MaxCustodyDays != 7 {
		t.Fatalf("item-phone custody = %+v", custodyRow)
	}

	events, err := db.ListCustodyEvents(ctx, "item-phone")
	if err != nil {
		t.Fatalf("ListCustodyEvents(item-phone): %v", err)
	}
	if len(events) < 2 {
		t.Fatalf("item-phone events = %+v, want rights_assigned + accepted", events)
	}

	claim, err := db.GetClaimByID(ctx, "claim-phone")
	if err != nil {
		t.Fatalf("GetClaimByID(claim-phone): %v", err)
	}
	if claim.ItemID != "item-phone" || claim.SearcherID != "searcher-alt" || claim.Status != "approved" {
		t.Fatalf("claim-phone = %+v", claim)
	}
	if claim.ReturnChannel != "intermediary_direct" || claim.ApprovalBy != "bar-x" {
		t.Fatalf("claim-phone = %+v", claim)
	}
	if claim.Evidence != `["colore nero","crepa sullo schermo"]` {
		t.Fatalf("claim-phone evidence = %q, want the safe JSON array", claim.Evidence)
	}

	wallet, err := db.GetItemByID(ctx, "item-wallet")
	if err != nil {
		t.Fatalf("GetItemByID(item-wallet): %v", err)
	}
	if wallet.TransferredAt == nil || wallet.LegalTermStart == nil || wallet.CustodianID != "institution-demo" {
		t.Fatalf("item-wallet = %+v", wallet)
	}
}
