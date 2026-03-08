package utils

import "time"

const (
	CUSTOM_EPOCH = 1577836800000 // 2020-1-1 00:00:00 in milliseconds
)

type Timestamp uint32

func NewTimestamp() Timestamp {
	now := time.Now().UnixMilli()
	return Timestamp((now - CUSTOM_EPOCH))
}

// func ToUnixMilli(timestamp uint32) int64 {
func (t Timestamp) ToUnixMilli() int64 {
	return int64(t) + CUSTOM_EPOCH
}
