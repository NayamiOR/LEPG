package model

type Reading struct {
	ID         int64 `bun:",pk,autoincrement"`
	DeviceName string
	PointName  string
	DataType   DataType
	BoolVal    bool
	NumVal     float64
	Unit       string
	Timestamp  int64 `bun:",notnull"` // In milliseconds
	DeviceHash string
}
