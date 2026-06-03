package cache

import (
	"LEPG/internal/model"
	"context"
	"time"
)

type StoredReading struct {
	ID         int64  `bun:",pk,autoincrement"`
	Sn         string // 来源网关
	UploadTime int64  `bun:",notnull"` // 服务端收到时间(ms)
	DeviceName string
	PointName  string
	DataType   model.DataType
	BoolVal    bool
	NumVal     float64
	Unit       string
	Timestamp  int64 `bun:",notnull"` // 采集时间(ms)
	DeviceHash string
}

type QueryFilter struct {
	Sn         string
	DeviceName string
	StartTime  int64 // ms, 0 = no filter
	EndTime    int64 // ms, 0 = no filter
	Limit      int   // 0 = default 1000
}

type Store interface {
	SaveReadings(ctx context.Context, sn string, readings []*model.Reading) error
	QueryReadings(ctx context.Context, filter QueryFilter) ([]*StoredReading, error)
	Close() error
}

func nowMs() int64 {
	return time.Now().UnixMilli()
}
