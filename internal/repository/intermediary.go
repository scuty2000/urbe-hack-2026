package repository

import (
	"context"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository/record"
)

func (r *repo) CreateIntermediary(ctx context.Context, intermediary record.IntermediaryRecord) error {
	if err := r.gormdb.WithContext(ctx).Create(&intermediary).Error; err != nil {
		return wrapErr("CreateIntermediary", err)
	}
	return nil
}

func (r *repo) GetIntermediaryByID(ctx context.Context, id string) (record.IntermediaryRecord, error) {
	var intermediary record.IntermediaryRecord
	if err := r.gormdb.WithContext(ctx).Where("id = ?", id).First(&intermediary).Error; err != nil {
		return intermediary, wrapErr("GetIntermediaryByID", err)
	}
	return intermediary, nil
}

func (r *repo) ListIntermediaries(ctx context.Context) ([]record.IntermediaryRecord, error) {
	var intermediaries []record.IntermediaryRecord
	if err := r.gormdb.WithContext(ctx).Order("name ASC").Find(&intermediaries).Error; err != nil {
		return nil, wrapErr("ListIntermediaries", err)
	}
	return intermediaries, nil
}
