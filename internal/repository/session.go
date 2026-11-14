package repository

import (
	"context"
	"time"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository/record"
)

func (r *repo) CreateSession(ctx context.Context, session record.SessionRecord) error {
	if session.CreatedAt.IsZero() {
		session.CreatedAt = time.Now().UTC()
	}
	session.CreatedAt = utc(session.CreatedAt)
	session.ExpiresAt = utc(session.ExpiresAt)
	if err := r.gormdb.WithContext(ctx).Create(&session).Error; err != nil {
		return wrapErr("CreateSession", err)
	}
	return nil
}

func (r *repo) GetSessionByTokenHash(ctx context.Context, tokenHash string) (record.SessionRecord, error) {
	var session record.SessionRecord
	if err := r.gormdb.WithContext(ctx).
		Where("token_hash = ?", tokenHash).
		First(&session).Error; err != nil {
		return session, wrapErr("GetSessionByTokenHash", err)
	}
	return session, nil
}

// DeleteSession is idempotent: logging out twice is not an error.
func (r *repo) DeleteSession(ctx context.Context, tokenHash string) error {
	if err := r.gormdb.WithContext(ctx).
		Where("token_hash = ?", tokenHash).
		Delete(&record.SessionRecord{}).Error; err != nil {
		return wrapErr("DeleteSession", err)
	}
	return nil
}

func (r *repo) DeleteExpiredSessions(ctx context.Context, now time.Time) (int64, error) {
	res := r.gormdb.WithContext(ctx).
		Where("expires_at IS NOT NULL AND expires_at <= ?", now.UTC()).
		Delete(&record.SessionRecord{})
	if res.Error != nil {
		return 0, wrapErr("DeleteExpiredSessions", res.Error)
	}
	return res.RowsAffected, nil
}
