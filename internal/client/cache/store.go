package cache

import (
	"LEPG/internal/model"
	"context"
)

type Reading struct {
	ID         int64 `bun:",pk,autoincrement"`
	DeviceName string
	PointName  string
	DataType   model.DataType
	BoolVal    bool
	NumVal     float64
	Unit       string
	Timestamp  int64 `bun:",notnull"` // In milliseconds
	DeviceHash string
}

type Store interface {
	SaveReadings(ctx context.Context, readings []*Reading) error
	LoadReadings(ctx context.Context, limit int) ([]*Reading, error)
	DeleteReadings(ctx context.Context, ids []int64) error
	Close() error
}
