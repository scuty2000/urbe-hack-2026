package repository

import (
	"context"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository/record"
)

func (r *repo) CreateLostReport(ctx context.Context, report record.LostReportRecord) error {
	if report.Embedding == "" {
		report.Embedding = "[]"
	}
	if err := r.gormdb.WithContext(ctx).Create(&report).Error; err != nil {
		return wrapErr("CreateLostReport", err)
	}
	return nil
}

// ListLostReports returns reports newest first; an empty status means "all".
func (r *repo) ListLostReports(ctx context.Context, status string) ([]record.LostReportRecord, error) {
	query := r.gormdb.WithContext(ctx).Model(&record.LostReportRecord{})
	if status != "" {
		query = query.Where("status = ?", status)
	}
	var reports []record.LostReportRecord
	if err := query.Order("rowid DESC").Find(&reports).Error; err != nil {
		return nil, wrapErr("ListLostReports", err)
	}
	return reports, nil
}
