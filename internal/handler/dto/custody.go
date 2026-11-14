package dto

import (
	"time"

	custodydomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/custody"
	itemdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/item"
)

// RefuseCustodyPayload is the JSON body of POST /api/items/:id/custody/refuse.
type RefuseCustodyPayload struct {
	Reason string `json:"reason"`
}

// CustodyResponse summarises an accepted handover and its legal deadline.
//
// It deliberately contains no private profile and no coordinates: the
// intermediary already holds the item, but the read model stays role-safe.
type CustodyResponse struct {
	ItemID         string     `json:"itemId"`
	Status         string     `json:"status"`
	CustodianType  string     `json:"custodianType"`
	CustodianID    string     `json:"custodianId"`
	LegalFinderID  string     `json:"legalFinderId"`
	MaxCustodyDays int        `json:"maxCustodyDays"`
	AcceptedAt     *time.Time `json:"acceptedAt"`
	DueAt          *time.Time `json:"dueAt"`
	DaysLeft       int        `json:"daysLeft"`
}

// ToCustodyResponse maps an accepted custody and its remaining days.
func ToCustodyResponse(c custodydomain.Custody, daysLeft int) CustodyResponse {
	return CustodyResponse{
		ItemID:         c.ItemID,
		Status:         itemdomain.StatusInCustody,
		CustodianType:  c.CustodianType,
		CustodianID:    c.CustodianID,
		LegalFinderID:  c.LegalFinderID,
		MaxCustodyDays: c.MaxCustodyDays,
		AcceptedAt:     c.AcceptedAt,
		DueAt:          c.DueAt,
		DaysLeft:       daysLeft,
	}
}

// TickResponse reports the items moved to a pickup-due status by one tick.
type TickResponse struct {
	Updated int            `json:"updated"`
	Items   []ItemResponse `json:"items"`
}

// ToTickResponse maps the items transitioned by MarkPickupDue.
func ToTickResponse(items []itemdomain.Item) TickResponse {
	return TickResponse{
		Updated: len(items),
		Items:   ToItemListResponse(items).Items,
	}
}
