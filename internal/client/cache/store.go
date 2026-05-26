package cache

import "LEPG/internal/model"

type Reading struct {
	ID          int64          `bun:",pk,autoincrement"`
	DeviceName  string
	PointName   string
	DataType    model.DataType
	BoolVal     bool
	NumVal      float64
	Unit        string
	Timestamp   int64 `bun:",notnull"` // In milliseconds
	DeviceHash  string
}
