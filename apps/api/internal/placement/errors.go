package placement

import "errors"

// ErrInsufficientResources is returned when no server can host the workload.
var ErrInsufficientResources = errors.New("insufficient resources")
