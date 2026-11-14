/*
Package seed builds the deterministic demo dataset and resets it between demos.

The seed is intentionally idempotent: Reset deletes the registry tables and
re-inserts the same ids, users and timestamps relative to now. Users and
sessions are preserved so an admin running POST /api/demo/reset keeps their
session.

Demo data:
  - six demo users (finder, searcher, two intermediaries, institution, admin);
  - two civic points (Bar X and Portineria Sapienza);
  - one active PA tariff (500 cents/day, 1000 cents fixed, EUR);
  - item-phone in custody at Bar X (searchable, claim approved);
  - item-wallet transferred to the institution (searchable);
  - item-umbrella pending at Bar X (used by the custody accept demo);
  - item-document pending and sensitive.
*/
package seed

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/auth"
	itemdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/item"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository/record"
	"gorm.io/gorm"
)

// Indexer is the optional search index port. It matches search.Service without
// making the seed package depend on the search domain.
type Indexer interface {
	IndexItem(ctx context.Context, item itemdomain.Item) error
}

// resetTables are deleted before re-seeding, in foreign-key-safe order. Users
// and sessions are intentionally kept: the admin session survives a reset.
var resetTables = []string{
	"reward_notices",
	"payments",
	"fees",
	"pickup_passes",
	"verification_sessions",
	"claims",
	"custody_events",
	"custody",
	"notifications",
	"audit_logs",
	"lost_reports",
	"tariffs",
	"items",
	"intermediaries",
}

// Demo seeds the demo dataset into db and fails the test on error.
func Demo(t testing.TB, db repository.Database, indexers ...Indexer) {
	t.Helper()
	if err := Reset(context.Background(), db, indexers...); err != nil {
		t.Fatalf("seeding demo data: %v", err)
	}
}

// Reset replaces the registry dataset with the demo dataset. It is safe to run
// repeatedly; the users required by the demo are created when missing.
func Reset(ctx context.Context, db repository.Database, indexers ...Indexer) error {
	if err := auth.SeedDemoUsers(ctx, db); err != nil {
		return fmt.Errorf("seed demo users: %w", err)
	}

	now := time.Now().UTC()
	rows := demoRows(now)

	err := db.GormDB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, table := range resetTables {
			if err := tx.Exec("DELETE FROM " + table).Error; err != nil {
				return fmt.Errorf("clearing %s: %w", table, err)
			}
		}

		repo := repository.NewRepository(tx)
		for _, intermediary := range rows.intermediaries {
			if err := repo.CreateIntermediary(ctx, intermediary); err != nil {
				return fmt.Errorf("seeding intermediary %q: %w", intermediary.ID, err)
			}
		}
		if err := repo.CreateTariff(ctx, rows.tariff); err != nil {
			return fmt.Errorf("seeding tariff: %w", err)
		}
		for _, item := range rows.items {
			if err := repo.CreateItemWithCustody(ctx, item.item, item.custody[0], item.events[0]); err != nil {
				return fmt.Errorf("seeding item %q: %w", item.item.ID, err)
			}
			for _, custody := range item.custody[1:] {
				if err := repo.CreateCustody(ctx, custody); err != nil {
					return fmt.Errorf("seeding custody for %q: %w", item.item.ID, err)
				}
			}
			for _, event := range item.events[1:] {
				if err := repo.CreateCustodyEvent(ctx, event); err != nil {
					return fmt.Errorf("seeding event for %q: %w", item.item.ID, err)
				}
			}
		}
		for _, claim := range rows.claims {
			if err := repo.CreateClaim(ctx, claim); err != nil {
				return fmt.Errorf("seeding claim %q: %w", claim.ID, err)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("seeding demo data: %w", err)
	}

	// Index only after the transaction commits: Qdrant is best-effort and a
	// seed must still succeed with the vector store down.
	if len(indexers) == 0 {
		return nil
	}
	indexer := indexers[0]
	if indexer == nil {
		return nil
	}
	// Indexing is optional and bounded: the DB reset already committed, so a
	// slow vector store must not push the demo reset past the write timeout.
	indexCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	for _, itemRow := range rows.items {
		item, err := itemdomain.FromRecord(itemRow.item)
		if err != nil {
			return fmt.Errorf("mapping seeded item %q: %w", itemRow.item.ID, err)
		}
		if item.Status != itemdomain.StatusInCustody && item.Status != "transferred" {
			continue
		}
		if err := indexer.IndexItem(indexCtx, item); err != nil {
			break
		}
	}
	return nil
}

type seededItem struct {
	item    record.ItemRecord
	custody []record.CustodyRecord
	events  []record.CustodyEventRecord
}

type demoData struct {
	intermediaries []record.IntermediaryRecord
	tariff         record.TariffRecord
	items          []seededItem
	claims         []record.ClaimRecord
}

func demoRows(now time.Time) demoData {
	phoneFound := now.Add(-2 * time.Hour)
	phoneCreated := now.Add(-3 * time.Hour)
	phoneAccepted := now.Add(-1 * time.Hour)
	phoneDue := phoneAccepted.Add(7 * 24 * time.Hour)

	walletFound := now.Add(-30 * time.Hour)
	walletCreated := now.Add(-31 * time.Hour)
	walletAccepted := now.Add(-29 * time.Hour)
	walletClosed := now.Add(-20 * time.Hour)

	umbrellaFound := now.Add(-1 * time.Hour)
	documentFound := now.Add(-3 * time.Hour)

	phone := seededPhone(phoneCreated, phoneFound, phoneAccepted, phoneDue)
	wallet := seededWallet(walletCreated, walletFound, walletAccepted, walletClosed)
	umbrella := seededUmbrella(now.Add(-2*time.Hour), umbrellaFound)
	document := seededDocument(now.Add(-3*time.Hour), documentFound)

	return demoData{
		intermediaries: []record.IntermediaryRecord{
			{
				ID:           "bar-x",
				Name:         "Bar X",
				Address:      "Via dei Volsci 12, Roma",
				GeoLat:       41.9012,
				GeoLng:       12.5155,
				Hours:        "07:00-21:00",
				Capacity:     20,
				Status:       "active",
				AgreementRef: "conv-2026-001",
				Contact:      "bar-x@example.test",
			},
			{
				ID:           "portineria-sapienza",
				Name:         "Portineria Sapienza",
				Address:      "Piazzale Aldo Moro 5, Roma",
				GeoLat:       41.9028,
				GeoLng:       12.5145,
				Hours:        "08:00-20:00",
				Capacity:     40,
				Status:       "active",
				AgreementRef: "conv-2026-002",
				Contact:      "portineria-sapienza@example.test",
			},
		},
		tariff: record.TariffRecord{
			ID:             "tariff-demo",
			DailyRateCents: 500,
			FixedFeeCents:  1000,
			Currency:       "EUR",
			Active:         true,
		},
		items: []seededItem{phone, wallet, umbrella, document},
		claims: []record.ClaimRecord{
			{
				ID:            "claim-phone",
				ItemID:        "item-phone",
				SearcherID:    "searcher-alt",
				Status:        "approved",
				ReturnChannel: "intermediary_direct",
				Confidence:    0.92,
				Evidence:      `["colore nero","crepa sullo schermo"]`,
				ApprovalBy:    "bar-x",
				ApprovedAt:    timePtr(now.Add(-30 * time.Minute)),
				CreatedAt:     phoneFound,
			},
		},
	}
}

func seededPhone(createdAt, foundAt, acceptedAt, dueAt time.Time) seededItem {
	item := baseItem(record.ItemRecord{
		ID:             "item-phone",
		PublicCode:     "LF-00000001",
		Status:         itemdomain.StatusInCustody,
		LegalFinderID:  "bar-x",
		Category:       "smartphone",
		Brand:          "Apple",
		Model:          "iPhone 17",
		Color:          "nero",
		Condition:      "good",
		FoundAt:        foundAt,
		FoundLat:       41.9012,
		FoundLng:       12.5155,
		FoundZone:      "San Lorenzo",
		FoundNotes:     "trovato davanti al bar",
		CustodianID:    "bar-x",
		RiskTier:       itemdomain.RiskHigh,
		Sensitive:      false,
		CreatedAt:      createdAt,
		UpdatedAt:      acceptedAt,
		PrivateProfile: `{"case_color":"blu","has_crack":true,"sticker":"gatto","notes":"custodia blu con crepa e adesivo a forma di gatto"}`,
	})
	item.RightsAssignedAt = &createdAt
	item.Photos = "[]"

	custody := record.CustodyRecord{
		ID:                "custody-phone",
		ItemID:            "item-phone",
		CustodianType:     itemdomain.CustodyModeIntermediary,
		CustodianID:       "bar-x",
		LegalFinderID:     "bar-x",
		RightsAssignedAt:  &createdAt,
		ApprovalAuthority: itemdomain.CustodyModeIntermediary,
		MaxCustodyDays:    7,
		AcceptedAt:        &acceptedAt,
		DueAt:             &dueAt,
	}

	return seededItem{
		item:    item,
		custody: []record.CustodyRecord{custody},
		events: []record.CustodyEventRecord{
			event("event-phone-rights", "item-phone", itemdomain.EventRightsAssigned, "finder-demo", createdAt, "legal finder assigned to bar-x"),
			event("event-phone-accepted", "item-phone", "accepted", "bar-x", acceptedAt, "custody accepted; due at "+dueAt.Format(time.RFC3339)),
		},
	}
}

func seededWallet(createdAt, foundAt, acceptedAt, closedAt time.Time) seededItem {
	item := baseItem(record.ItemRecord{
		ID:             "item-wallet",
		PublicCode:     "LF-00000002",
		Status:         "transferred",
		LegalFinderID:  "bar-x",
		Category:       "wallet",
		Brand:          "Furla",
		Color:          "marrone",
		Condition:      "worn",
		FoundAt:        foundAt,
		FoundLat:       41.9007,
		FoundLng:       12.5149,
		FoundZone:      "San Lorenzo",
		FoundNotes:     "portafoglio trovato sul marciapiede",
		CustodianID:    "institution-demo",
		RiskTier:       itemdomain.RiskHigh,
		CreatedAt:      createdAt,
		UpdatedAt:      closedAt,
		TransferredAt:  &closedAt,
		LegalTermStart: &closedAt,
		PrivateProfile: `{"contents":["carta di identità","bancomat"],"detail":"portafoglio Furla marrone con due scomparti"}`,
	})
	item.RightsAssignedAt = &createdAt
	item.CustodyMode = itemdomain.CustodyModeInstitution
	item.Photos = "[]"

	intermediaryDue := acceptedAt.Add(7 * 24 * time.Hour)
	intermediaryCustody := record.CustodyRecord{
		ID:                "custody-wallet-intermediary",
		ItemID:            "item-wallet",
		CustodianType:     itemdomain.CustodyModeIntermediary,
		CustodianID:       "bar-x",
		LegalFinderID:     "bar-x",
		RightsAssignedAt:  &createdAt,
		ApprovalAuthority: itemdomain.CustodyModeIntermediary,
		MaxCustodyDays:    7,
		AcceptedAt:        &acceptedAt,
		DueAt:             &intermediaryDue,
		ClosedAt:          &closedAt,
	}
	institutionCustody := record.CustodyRecord{
		ID:                "custody-wallet-institution",
		ItemID:            "item-wallet",
		CustodianType:     itemdomain.CustodyModeInstitution,
		CustodianID:       "institution-demo",
		LegalFinderID:     "bar-x",
		RightsAssignedAt:  &createdAt,
		ApprovalAuthority: itemdomain.CustodyModeInstitution,
		MaxCustodyDays:    365,
		AcceptedAt:        &closedAt,
	}

	return seededItem{
		item:    item,
		custody: []record.CustodyRecord{intermediaryCustody, institutionCustody},
		events: []record.CustodyEventRecord{
			event("event-wallet-rights", "item-wallet", itemdomain.EventRightsAssigned, "finder-demo", createdAt, "legal finder assigned to bar-x"),
			event("event-wallet-accepted", "item-wallet", "accepted", "bar-x", acceptedAt, "custody accepted"),
			event("event-wallet-transferred", "item-wallet", "transferred", "institution-demo", closedAt, "transferred to the public administration"),
		},
	}
}

func seededUmbrella(createdAt, foundAt time.Time) seededItem {
	item := baseItem(record.ItemRecord{
		ID:             "item-umbrella",
		PublicCode:     "LF-00000003",
		Status:         itemdomain.StatusPendingCustody,
		LegalFinderID:  "bar-x",
		Category:       "umbrella",
		Color:          "blu",
		Condition:      "good",
		FoundAt:        foundAt,
		FoundLat:       41.9022,
		FoundLng:       12.5160,
		FoundZone:      "San Lorenzo",
		FoundNotes:     "ombrello dimenticato al tavolo",
		CustodianID:    "bar-x",
		RiskTier:       itemdomain.RiskLow,
		CreatedAt:      createdAt,
		UpdatedAt:      createdAt,
		PrivateProfile: `{"handle":"legno chiaro"}`,
	})
	item.RightsAssignedAt = &createdAt
	item.Photos = "[]"

	custody := record.CustodyRecord{
		ID:                "custody-umbrella",
		ItemID:            "item-umbrella",
		CustodianType:     itemdomain.CustodyModeIntermediary,
		CustodianID:       "bar-x",
		LegalFinderID:     "bar-x",
		RightsAssignedAt:  &createdAt,
		ApprovalAuthority: itemdomain.CustodyModeIntermediary,
		MaxCustodyDays:    7,
	}

	return seededItem{
		item:    item,
		custody: []record.CustodyRecord{custody},
		events: []record.CustodyEventRecord{
			event("event-umbrella-rights", "item-umbrella", itemdomain.EventRightsAssigned, "finder-demo", createdAt, "legal finder assigned to bar-x"),
		},
	}
}

func seededDocument(createdAt, foundAt time.Time) seededItem {
	item := baseItem(record.ItemRecord{
		ID:             "item-document",
		PublicCode:     "LF-00000004",
		Status:         itemdomain.StatusPendingCustody,
		LegalFinderID:  "bar-x",
		Category:       "document",
		Color:          "white",
		Condition:      "good",
		FoundAt:        foundAt,
		FoundLat:       41.9010,
		FoundLng:       12.5150,
		FoundZone:      "San Lorenzo",
		FoundNotes:     "documento trovato vicino alla fermata",
		CustodianID:    "bar-x",
		RiskTier:       itemdomain.RiskSensitive,
		Sensitive:      true,
		CreatedAt:      createdAt,
		UpdatedAt:      createdAt,
		PrivateProfile: `{"document_type":"identity_card","number":"CA00000AA","holder":"Mario Rossi"}`,
	})
	item.RightsAssignedAt = &createdAt
	item.Photos = "[]"

	custody := record.CustodyRecord{
		ID:                "custody-document",
		ItemID:            "item-document",
		CustodianType:     itemdomain.CustodyModeIntermediary,
		CustodianID:       "bar-x",
		LegalFinderID:     "bar-x",
		RightsAssignedAt:  &createdAt,
		ApprovalAuthority: itemdomain.CustodyModeIntermediary,
		MaxCustodyDays:    1,
	}

	return seededItem{
		item:    item,
		custody: []record.CustodyRecord{custody},
		events: []record.CustodyEventRecord{
			event("event-document-rights", "item-document", itemdomain.EventRightsAssigned, "finder-demo", createdAt, "legal finder assigned to bar-x"),
		},
	}
}

// baseItem fills the non-null columns with safe defaults.
func baseItem(item record.ItemRecord) record.ItemRecord {
	item.Source = itemdomain.SourceFinderSelf
	item.CustodyMode = itemdomain.CustodyModeIntermediary
	if item.PrivateProfile == "" {
		item.PrivateProfile = "{}"
	}
	if item.Photos == "" {
		item.Photos = "[]"
	}
	return item
}

func event(id, itemID, eventType, actorID string, at time.Time, notes string) record.CustodyEventRecord {
	return record.CustodyEventRecord{
		ID:        id,
		ItemID:    itemID,
		Type:      eventType,
		ActorID:   actorID,
		Timestamp: at,
		Notes:     notes,
	}
}

func timePtr(value time.Time) *time.Time {
	return &value
}
