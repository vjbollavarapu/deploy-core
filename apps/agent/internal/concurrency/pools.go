package concurrency

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"sync"
)

var (
	// ErrPoolSaturated is returned when the concurrency pool limit is reached and acquire times out.
	ErrPoolSaturated = errors.New("concurrency limit exceeded: worker pool saturated")
)

// Category identifies operational concurrency queues.
type Category string

const (
	CategoryBuild        Category = "BUILD"
	CategoryDeployment   Category = "DEPLOYMENT"
	CategoryBackup       Category = "BACKUP"
	CategoryContainerOps Category = "CONTAINER_OPS"
)

// Limits configures maximum concurrent executions per category.
type Limits struct {
	MaxBuilds       int
	MaxDeployments  int
	MaxBackups      int
	MaxContainerOps int
}

// DefaultLimits loads defaults with environment variable overrides.
func DefaultLimits() Limits {
	l := Limits{
		MaxBuilds:       2,
		MaxDeployments:  3,
		MaxBackups:      2,
		MaxContainerOps: 10,
	}

	if v := os.Getenv("MAX_CONCURRENT_BUILDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			l.MaxBuilds = n
		}
	}
	if v := os.Getenv("MAX_CONCURRENT_DEPLOYMENTS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			l.MaxDeployments = n
		}
	}
	if v := os.Getenv("MAX_CONCURRENT_BACKUPS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			l.MaxBackups = n
		}
	}

	return l
}

// PoolManager controls bounded concurrency across critical operational categories.
type PoolManager struct {
	mu    sync.RWMutex
	pools map[Category]chan struct{}
}

// NewPoolManager constructs a PoolManager with specified limits.
func NewPoolManager(limits Limits) *PoolManager {
	if limits.MaxBuilds <= 0 {
		limits.MaxBuilds = 2
	}
	if limits.MaxDeployments <= 0 {
		limits.MaxDeployments = 3
	}
	if limits.MaxBackups <= 0 {
		limits.MaxBackups = 2
	}
	if limits.MaxContainerOps <= 0 {
		limits.MaxContainerOps = 10
	}

	pm := &PoolManager{
		pools: make(map[Category]chan struct{}),
	}

	pm.pools[CategoryBuild] = make(chan struct{}, limits.MaxBuilds)
	pm.pools[CategoryDeployment] = make(chan struct{}, limits.MaxDeployments)
	pm.pools[CategoryBackup] = make(chan struct{}, limits.MaxBackups)
	pm.pools[CategoryContainerOps] = make(chan struct{}, limits.MaxContainerOps)

	return pm
}

// Acquire acquires a slot in the designated category pool.
// Returns a release function that must be called when the task finishes.
func (pm *PoolManager) Acquire(ctx context.Context, category Category) (func(), error) {
	pm.mu.RLock()
	ch, ok := pm.pools[category]
	pm.mu.RUnlock()

	if !ok {
		// Category not explicitly pooled; permit with no-op release
		return func() {}, nil
	}

	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("%w: %v", ErrPoolSaturated, ctx.Err())
	case ch <- struct{}{}:
		var once sync.Once
		release := func() {
			once.Do(func() {
				<-ch
			})
		}
		return release, nil
	}
}

// ActiveCount returns current active workers in the given category.
func (pm *PoolManager) ActiveCount(category Category) int {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	ch, ok := pm.pools[category]
	if !ok {
		return 0
	}
	return len(ch)
}
