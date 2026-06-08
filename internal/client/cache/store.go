package cache

import (
	"context"
)

type Store interface {
	SaveReadings(ctx context.Context, readings []*CachedReading) error
	LoadReadings(ctx context.Context, limit int) ([]*CachedReading, error)
	LoadPendingReadings(ctx context.Context, limit int) ([]*CachedReading, error)
	UpdateReadingsStatus(ctx context.Context, ids []int64, status UploadStatus) error
	DeleteReadings(ctx context.Context, ids []int64) error
	Close() error
}
