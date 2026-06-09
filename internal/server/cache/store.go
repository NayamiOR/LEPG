package cache

import (
	"LEPG/internal/model"
	"context"
	"time"
)

type StoredReading struct {
	ID         int64          `bun:",pk,autoincrement"`
	Sn         string         // 来源网关
	UploadTime int64          `bun:",notnull"` // 服务端收到时间(ms)
	Device     string         `bun:",notnull"`
	DeviceName string
	Point      string         `bun:",notnull"`
	PointName  string
	DataType   model.DataType `bun:",notnull"`
	Value      string         `bun:",notnull"`
	Quality    model.Quality  `bun:",notnull"`
	Unit       string
	Timestamp  int64          `bun:",notnull"` // 采集时间(ms)
}

type QueryFilter struct {
	Sn        string
	Device    string
	StartTime int64 // ms, 0 = no filter
	EndTime   int64 // ms, 0 = no filter
	Limit     int   // 0 = default 1000
}

type Store interface {
	SaveReadings(ctx context.Context, sn string, readings []*model.Reading) error
	QueryReadings(ctx context.Context, filter QueryFilter) ([]*StoredReading, error)
	Close() error
}

func nowMs() int64 {
	return time.Now().UnixMilli()
}
