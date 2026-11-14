/*
Package fee implements the public-administration custody fee and its mock
payment.

A direct return to the owner before the item is transferred is free. After the
item has been transferred to the institution, the fee is the active PA tariff:
`ceil(custody days) * daily_rate_cents + fixed_fee_cents`. Fees can be
recalculated until they are paid; once paid they are final.

Payment is deliberately provider-agnostic. The mock provider records a `paid`
payment with a local receipt URL so the demo can exercise the whole pickup flow
without real payment keys.
*/
package fee

import (
	"time"
)

// Fee appliers. A direct return is free; only a transferred item owes the PA
// tariff.
const (
	AppliesNoneDirectReturn = "none_direct_return"
	AppliesPATariff         = "pa_tariff"
)

// Fee statuses. `estimated` fees can be recalculated; `paid` and `waived` are
// final.
const (
	StatusEstimated = "estimated"
	StatusFinal     = "final"
	StatusPaid      = "paid"
	StatusWaived    = "waived"
)

// ProviderMock is the only payment provider implemented by this task.
const ProviderMock = "mock"

// Custody event and audit action appended when the PA fee is settled.
const (
	EventFeePaid        = "fee_paid"
	AuditActionFeePaid  = "fee_paid"
	auditEntityFee      = "fee"
	reasonFreeFee       = "no amount is due for a direct return"
	reasonAlreadyPaid   = "fee is already settled"
	defaultCurrencyCode = "EUR"
)

// itemStatusTransferred is the item state in which the PA tariff applies.
const itemStatusTransferred = "transferred"

// Fee is the fee read model returned to the HTTP layer.
type Fee struct {
	ID          string
	ItemID      string
	Applies     string
	PATariffID  string
	CustodyDays int
	AmountCents int64
	Currency    string
	Status      string
	// Paid is true for a free/direct return and for a settled PA fee.
	Paid bool
	// Payment is the latest payment, when one exists.
	Payment *Payment
}

// Payment is a provider-agnostic payment read model. ReceiptURL points at the
// local receipt representation; no provider secret is involved.
type Payment struct {
	ID          string
	FeeID       string
	Provider    string
	ProviderRef string
	Status      string
	PaidAt      *time.Time
	ReceiptURL  string
}

// Tariff is the fallback PA tariff used when the database has no active
// `tariffs` row. The seeded demo always has one; this keeps the service usable
// in a fresh environment configured only through the environment variables.
type Tariff struct {
	DailyRateCents int64
	FixedFeeCents  int64
	Currency       string
}
