package dto

import (
	"time"

	feedomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/fee"
	rewarddomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/reward"
)

// FeeResponse is the safe fee read model. It contains no item profile, exact
// location or payment secret.
type FeeResponse struct {
	ID          string           `json:"id"`
	ItemID      string           `json:"itemId"`
	Applies     string           `json:"applies"`
	CustodyDays int              `json:"custodyDays"`
	AmountCents int64            `json:"amountCents"`
	Currency    string           `json:"currency"`
	Status      string           `json:"status"`
	Paid        bool             `json:"paid"`
	Payment     *PaymentResponse `json:"payment,omitempty"`
}

// PaymentResponse is the mock-payment receipt returned to the searcher.
type PaymentResponse struct {
	ID          string     `json:"id"`
	FeeID       string     `json:"feeId"`
	Provider    string     `json:"provider"`
	ProviderRef string     `json:"providerRef"`
	Status      string     `json:"status"`
	PaidAt      *time.Time `json:"paidAt,omitempty"`
	ReceiptURL  string     `json:"receiptUrl"`
}

// CreateRewardNoticePayload is the optional JSON body of
// POST /api/items/{id}/reward-notice. The amount is requested, never charged.
type CreateRewardNoticePayload struct {
	AmountCents int64 `json:"amountCents"`
}

// RewardNoticeResponse is the reward notice shown to the searcher before
// pickup. Settlements always happen outside the platform.
type RewardNoticeResponse struct {
	ID                string     `json:"id"`
	ItemID            string     `json:"itemId"`
	LegalFinderID     string     `json:"legalFinderId"`
	Requested         bool       `json:"requested"`
	AmountCents       int64      `json:"amountCents"`
	ShownToSearcherAt *time.Time `json:"shownToSearcherAt,omitempty"`
	PaymentStatus     string     `json:"paymentStatus"`
}

// ToFeeResponse maps the fee read model.
func ToFeeResponse(f feedomain.Fee) FeeResponse {
	response := FeeResponse{
		ID:          f.ID,
		ItemID:      f.ItemID,
		Applies:     f.Applies,
		CustodyDays: f.CustodyDays,
		AmountCents: f.AmountCents,
		Currency:    f.Currency,
		Status:      f.Status,
		Paid:        f.Paid,
	}
	if f.Payment != nil {
		payment := toPaymentResponse(*f.Payment)
		response.Payment = &payment
	}
	return response
}

// ToPaymentResponse maps a mock payment.
func ToPaymentResponse(p feedomain.Payment) PaymentResponse {
	return toPaymentResponse(p)
}

func toPaymentResponse(p feedomain.Payment) PaymentResponse {
	return PaymentResponse{
		ID:          p.ID,
		FeeID:       p.FeeID,
		Provider:    p.Provider,
		ProviderRef: p.ProviderRef,
		Status:      p.Status,
		PaidAt:      p.PaidAt,
		ReceiptURL:  p.ReceiptURL,
	}
}

// ToRewardNoticeResponse maps the reward notice.
func ToRewardNoticeResponse(n rewarddomain.RewardNotice) RewardNoticeResponse {
	return RewardNoticeResponse{
		ID:                n.ID,
		ItemID:            n.ItemID,
		LegalFinderID:     n.LegalFinderID,
		Requested:         n.Requested,
		AmountCents:       n.AmountCents,
		ShownToSearcherAt: n.ShownToSearcherAt,
		PaymentStatus:     n.PaymentStatus,
	}
}
