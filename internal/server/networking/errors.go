package networking

import "errors"

var (
	ErrConnectionNotFound = errors.New("networking: connection not found")
	ErrSendQueueFull      = errors.New("networking: connection send queue full")
	ErrInvalidPayload     = errors.New("networking: invalid payload")
)
