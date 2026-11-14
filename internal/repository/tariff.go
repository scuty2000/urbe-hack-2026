package repository

import (
	"context"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository/record"
)

func (r *repo) CreateTariff(ctx context.Context, tariff record.TariffRecord) error {
	if tariff.Currency == "" {
		tariff.Currency = "EUR"
	}
	if err := r.gormdb.WithContext(ctx).Create(&tariff).Error; err != nil {
		return wrapErr("CreateTariff", err)
	}
	return nil
}

// GetActiveTariff returns the most recently created active tariff.
func (r *repo) GetActiveTariff(ctx context.Context) (record.TariffRecord, error) {
	var tariff record.TariffRecord
	if err := r.gormdb.WithContext(ctx).
		Where("active = ?", true).
		Order("rowid DESC").
		First(&tariff).Error; err != nil {
		return tariff, wrapErr("GetActiveTariff", err)
	}
	return tariff, nil
}

func (r *repo) ListTariffs(ctx context.Context) ([]record.TariffRecord, error) {
	var tariffs []record.TariffRecord
	if err := r.gormdb.WithContext(ctx).Order("rowid DESC").Find(&tariffs).Error; err != nil {
		return nil, wrapErr("ListTariffs", err)
	}
	return tariffs, nil
}
