package repository

import (
	"context"
	"time"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository/record"
)

func (r *repo) CreateUser(ctx context.Context, user record.UserRecord) error {
	if user.CreatedAt.IsZero() {
		user.CreatedAt = time.Now().UTC()
	}
	user.CreatedAt = utc(user.CreatedAt)
	if err := r.gormdb.WithContext(ctx).Create(&user).Error; err != nil {
		return wrapErr("CreateUser", err)
	}
	return nil
}

func (r *repo) GetUserByID(ctx context.Context, id string) (record.UserRecord, error) {
	var user record.UserRecord
	if err := r.gormdb.WithContext(ctx).Where("id = ?", id).First(&user).Error; err != nil {
		return user, wrapErr("GetUserByID", err)
	}
	return user, nil
}

func (r *repo) ListUsers(ctx context.Context) ([]record.UserRecord, error) {
	var users []record.UserRecord
	if err := r.gormdb.WithContext(ctx).Order("id ASC").Find(&users).Error; err != nil {
		return nil, wrapErr("ListUsers", err)
	}
	return users, nil
}
