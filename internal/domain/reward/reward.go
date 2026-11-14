/*
Package reward implements the art. 930 reward notice: the legal finder asks the
searcher for a reward, the system records it and shows it before pickup, and the
settlement happens outside the platform.

The reward is never charged or paid through the system. `payment_status` is
therefore always `outside_system`.
*/
package reward

import "time"

// PaymentStatusOutsideSystem is the only payment status this package records.
const PaymentStatusOutsideSystem = "outside_system"

// Custody event type and audit action appended when a reward is requested.
const (
	EventRewardRequested       = "reward_requested"
	AuditActionRewardRequested = "reward_requested"
	auditEntityRewardNotice    = "reward_notice"
)

// RoleSearcher mirrors auth.RoleSearcher without importing the auth package.
const RoleSearcher = "searcher"

// RewardNotice is the reward read model. It deliberately contains no private
// profile data and no coordinates.
type RewardNotice struct {
	ID                string
	ItemID            string
	LegalFinderID     string
	Requested         bool
	AmountCents       int64
	ShownToSearcherAt *time.Time
	PaymentStatus     string
}

// Viewer identifies who is reading the notice. A searcher read marks the notice
// as shown before pickup.
type Viewer struct {
	ID   string
	Role string
}
