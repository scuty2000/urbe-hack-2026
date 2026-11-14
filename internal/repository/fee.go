package repository

import (
	"context"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository/record"
)

func (r *repo) CreateFee(ctx context.Context, fee record.FeeRecord) error {
	if fee.Currency == "" {
		fee.Currency = "EUR"
	}
	if err := r.gormdb.WithContext(ctx).Create(&fee).Error; err != nil {
		return wrapErr("CreateFee", err)
	}
	return nil
}

func (r *repo) GetFeeByID(ctx context.Context, id string) (record.FeeRecord, error) {
	var fee record.FeeRecord
	if err := r.gormdb.WithContext(ctx).Where("id = ?", id).First(&fee).Error; err != nil {
		return fee, wrapErr("GetFeeByID", err)
	}
	return fee, nil
}

// GetFeeByItemID returns the most recently created fee for the item.
func (r *repo) GetFeeByItemID(ctx context.Context, itemID string) (record.FeeRecord, error) {
	var fee record.FeeRecord
	if err := r.gormdb.WithContext(ctx).
		Where("item_id = ?", itemID).
		Order("rowid DESC").
		First(&fee).Error; err != nil {
		return fee, wrapErr("GetFeeByItemID", err)
	}
	return fee, nil
}

func (r *repo) UpdateFee(ctx context.Context, fee record.FeeRecord) error {
	res := r.gormdb.WithContext(ctx).
		Model(&record.FeeRecord{}).
		Where("id = ?", fee.ID).
		Updates(fee)
	if res.Error != nil {
		return wrapErr("UpdateFee", res.Error)
	}
	if res.RowsAffected == 0 {
		return notFound("UpdateFee", fee.ID)
	}
	return nil
}

func (r *repo) CreatePayment(ctx context.Context, payment record.PaymentRecord) error {
	payment.PaidAt = utcPtr(payment.PaidAt)
	if err := r.gormdb.WithContext(ctx).Create(&payment).Error; err != nil {
		return wrapErr("CreatePayment", err)
	}
	return nil
}

func (r *repo) GetPaymentByID(ctx context.Context, id string) (record.PaymentRecord, error) {
	var payment record.PaymentRecord
	if err := r.gormdb.WithContext(ctx).Where("id = ?", id).First(&payment).Error; err != nil {
		return payment, wrapErr("GetPaymentByID", err)
	}
	return payment, nil
}

// GetPaymentByFeeID returns the most recent payment for the fee.
func (r *repo) GetPaymentByFeeID(ctx context.Context, feeID string) (record.PaymentRecord, error) {
	var payment record.PaymentRecord
	if err := r.gormdb.WithContext(ctx).
		Where("fee_id = ?", feeID).
		Order("rowid DESC").
		First(&payment).Error; err != nil {
		return payment, wrapErr("GetPaymentByFeeID", err)
	}
	return payment, nil
}

func (r *repo) CreateRewardNotice(ctx context.Context, notice record.RewardNoticeRecord) error {
	if notice.PaymentStatus == "" {
		notice.PaymentStatus = "outside_system"
	}
	notice.ShownToSearcherAt = utcPtr(notice.ShownToSearcherAt)
	if err := r.gormdb.WithContext(ctx).Create(&notice).Error; err != nil {
		return wrapErr("CreateRewardNotice", err)
	}
	return nil
}

// GetRewardNoticeByItemID returns the most recently created notice.
func (r *repo) GetRewardNoticeByItemID(ctx context.Context, itemID string) (record.RewardNoticeRecord, error) {
	var notice record.RewardNoticeRecord
	if err := r.gormdb.WithContext(ctx).
		Where("item_id = ?", itemID).
		Order("rowid DESC").
		First(&notice).Error; err != nil {
		return notice, wrapErr("GetRewardNoticeByItemID", err)
	}
	return notice, nil
}

func (r *repo) UpdateRewardNotice(ctx context.Context, notice record.RewardNoticeRecord) error {
	notice.ShownToSearcherAt = utcPtr(notice.ShownToSearcherAt)
	res := r.gormdb.WithContext(ctx).
		Model(&record.RewardNoticeRecord{}).
		Where("id = ?", notice.ID).
		Updates(notice)
	if res.Error != nil {
		return wrapErr("UpdateRewardNotice", res.Error)
	}
	if res.RowsAffected == 0 {
		return notFound("UpdateRewardNotice", notice.ID)
	}
	return nil
}
