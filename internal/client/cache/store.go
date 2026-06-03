package cache

import (
	"LEPG/internal/model"
	"context"
)

type Store interface {
	SaveReadings(ctx context.Context, readings []*model.Reading) error
	LoadReadings(ctx context.Context, limit int) ([]*model.Reading, error)
	LoadPendingReadings(ctx context.Context, limit int) ([]*model.Reading, error)
	UpdateReadingsStatus(ctx context.Context, ids []int64, status int) error
	DeleteReadings(ctx context.Context, ids []int64) error
	Close() error
}
