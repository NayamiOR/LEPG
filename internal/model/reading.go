package model

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
)

type Quality uint8

const (
	QualityGood    Quality = 0
	QualityBad     Quality = 1
	QualityUnknown Quality = 2
)

type Reading struct {
	ID         int64  `bun:",pk,autoincrement"`
	Device     string // device hash (16-char hex), for indexing
	DeviceName string // readable name
	Point      string // point hash (16-char hex), for indexing
	PointName  string // readable name
	DataType   DataType
	Value      string  // unified serialized value
	Quality    Quality // data quality indicator
	Unit       string
	Timestamp  int64 `bun:",notnull"` // In milliseconds
}

func HashDevice(name string) string {
	h := sha256.Sum256([]byte(name))
	return hex.EncodeToString(h[:8])
}

func HashPoint(deviceName, pointName string) string {
	h := sha256.Sum256([]byte(deviceName + ":" + pointName))
	return hex.EncodeToString(h[:8])
}

func SerializeValue(dt DataType, value any) string {
	switch dt {
	case DataTypeBool:
		if b, ok := value.(bool); ok {
			return strconv.FormatBool(b)
		}
	case DataTypeInt16, DataTypeUint16, DataTypeInt32, DataTypeUint32, DataTypeFloat32, DataTypeFloat64:
		if f, ok := value.(float64); ok {
			return strconv.FormatFloat(f, 'g', -1, 64)
		}
	case DataTypeJSON:
		if s, ok := value.(string); ok {
			return s
		}
	}
	return ""
}

func ParseValue(dt DataType, s string) (float64, bool, error) {
	if dt == DataTypeBool {
		b, err := strconv.ParseBool(s)
		return 0, b, err
	}
	f, err := strconv.ParseFloat(s, 64)
	return f, false, err
}
