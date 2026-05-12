package job

import "errors"

var (
	ErrInvalidJob       = errors.New("invalid job")
	ErrJobNotFound      = errors.New("job not found")
	ErrQueueUnavailable = errors.New("queue unavailable")
	ErrStoreConflict    = errors.New("store conflict")
)
