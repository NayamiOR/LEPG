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
	ID         int64    `bun:",pk,autoincrement"`
	Device     string   // device hash (16-char hex), for indexing
	DeviceName string   // readable name
	Point      string   // point hash (16-char hex), for indexing
	PointName  string   // readable name
	DataType   DataType
	Value      string  // unified serialized value
	Quality    Quality // data quality indicator
	Unit       string
	Timestamp  int64   `bun:",notnull"` // In milliseconds
}

func HashDevice(name string) string {
	h := sha256.Sum256([]byte(name))
	return hex.EncodeToString(h[:8])
}

func HashPoint(deviceName, pointName string) string {
	h := sha256.Sum256([]byte(deviceName + ":" + pointName))
	return hex.EncodeToString(h[:8])
}

func SerializeValue(dt DataType, numVal float64, boolVal bool) string {
	if dt == DataTypeBool {
		return strconv.FormatBool(boolVal)
	}
	return strconv.FormatFloat(numVal, 'g', -1, 64)
}

func ParseValue(dt DataType, s string) (float64, bool, error) {
	if dt == DataTypeBool {
		b, err := strconv.ParseBool(s)
		return 0, b, err
	}
	f, err := strconv.ParseFloat(s, 64)
	return f, false, err
}
