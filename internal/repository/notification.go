package repository

import (
	"context"
	"time"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository/record"
)

func (r *repo) CreateNotification(ctx context.Context, notification record.NotificationRecord) error {
	if notification.CreatedAt.IsZero() {
		notification.CreatedAt = time.Now().UTC()
	}
	if notification.Payload == "" {
		notification.Payload = "{}"
	}
	notification.CreatedAt = utc(notification.CreatedAt)
	if err := r.gormdb.WithContext(ctx).Create(&notification).Error; err != nil {
		return wrapErr("CreateNotification", err)
	}
	return nil
}

func (r *repo) ListNotificationsByUser(ctx context.Context, userID string) ([]record.NotificationRecord, error) {
	var notifications []record.NotificationRecord
	if err := r.gormdb.WithContext(ctx).
		Where("recipient_id = ?", userID).
		Order("created_at DESC, rowid DESC").
		Find(&notifications).Error; err != nil {
		return nil, wrapErr("ListNotificationsByUser", err)
	}
	return notifications, nil
}

func (r *repo) GetNotificationByID(ctx context.Context, id string) (record.NotificationRecord, error) {
	var notification record.NotificationRecord
	if err := r.gormdb.WithContext(ctx).Where("id = ?", id).First(&notification).Error; err != nil {
		return notification, wrapErr("GetNotificationByID", err)
	}
	return notification, nil
}

func (r *repo) MarkNotificationRead(ctx context.Context, id string, readAt time.Time) error {
	res := r.gormdb.WithContext(ctx).
		Model(&record.NotificationRecord{}).
		Where("id = ?", id).
		Update("read_at", readAt.UTC())
	if res.Error != nil {
		return wrapErr("MarkNotificationRead", res.Error)
	}
	if res.RowsAffected == 0 {
		return notFound("MarkNotificationRead", id)
	}
	return nil
}
