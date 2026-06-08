package cache

import "LEPG/internal/model"

type UploadStatus int

const (
	UploadNotSent UploadStatus = 0
	UploadSending UploadStatus = 1
	UploadSent    UploadStatus = 2
	UploadFailed  UploadStatus = 3
)

// CachedReading wraps a model.Reading with client-side upload tracking.
type CachedReading struct {
	model.Reading
	Status UploadStatus
}

func (CachedReading) TableName() string { return "readings" }
