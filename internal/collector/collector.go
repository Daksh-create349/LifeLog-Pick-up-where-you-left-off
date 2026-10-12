package collector

import (
	"context"
	"errors"
)

var (
	ErrInvalidRoot     = errors.New("invalid monitored root path")
	ErrRestrictedPath  = errors.New("cannot monitor restricted path")
	ErrAlreadyWatching = errors.New("watcher is already active for this path")
	ErrRootUnavailable = errors.New("monitored root is inaccessible")
)

// Collector defines the lifecycle of a filesystem event collector.
type Collector interface {
	// Start starts collecting events until ctx is cancelled.
	Start(ctx context.Context) error
	// Strategy returns the name of the monitoring implementation.
	Strategy() string
	// MonitoredRoot returns the resolved absolute directory being watched.
	MonitoredRoot() string
}
