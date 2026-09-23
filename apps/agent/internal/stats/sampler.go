package stats

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Sampler periodically polls container stats and retains a sliding window of historical samples.
type Sampler struct {
	collector *Collector
	cfg       SamplerConfig
	log       *slog.Logger

	mu      sync.RWMutex
	samples map[string][]ContainerStats // containerID -> sliding window
	latest  map[string]ContainerStats   // containerID -> latest snapshot

	stopCh chan struct{}
	doneCh chan struct{}
}

// NewSampler constructs a Sampler.
func NewSampler(collector *Collector, cfg SamplerConfig, log *slog.Logger) *Sampler {
	if cfg.Interval <= 0 {
		cfg.Interval = 30 * time.Second
	}
	if cfg.MaxSamplesPerEntry <= 0 {
		cfg.MaxSamplesPerEntry = 10
	}
	if log == nil {
		log = slog.Default()
	}

	return &Sampler{
		collector: collector,
		cfg:       cfg,
		log:       log,
		samples:   make(map[string][]ContainerStats),
		latest:    make(map[string]ContainerStats),
		stopCh:    make(chan struct{}),
		doneCh:    make(chan struct{}),
	}
}

// Start begins periodic background sampling until ctx is cancelled or Stop is called.
func (s *Sampler) Start(ctx context.Context) {
	go func() {
		defer close(s.doneCh)
		ticker := time.NewTicker(s.cfg.Interval)
		defer ticker.Stop()

		// Run an initial collection immediately
		s.poll(ctx)

		for {
			select {
			case <-ctx.Done():
				return
			case <-s.stopCh:
				return
			case <-ticker.C:
				s.poll(ctx)
			}
		}
	}()
}

// Stop cleanly terminates background sampling.
func (s *Sampler) Stop() {
	select {
	case <-s.stopCh:
		// already stopped
	default:
		close(s.stopCh)
		<-s.doneCh
	}
}

func (s *Sampler) poll(ctx context.Context) {
	stats, err := s.collector.CollectAllManaged(ctx)
	if err != nil {
		if ctx.Err() == nil {
			s.log.Warn("sampler failed to collect managed container stats", slog.String("error", err.Error()))
		}
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	activeIDs := make(map[string]struct{}, len(stats))
	for _, st := range stats {
		activeIDs[st.ContainerID] = struct{}{}
		s.latest[st.ContainerID] = st

		window := s.samples[st.ContainerID]
		window = append(window, st)
		if len(window) > s.cfg.MaxSamplesPerEntry {
			window = window[len(window)-s.cfg.MaxSamplesPerEntry:]
		}
		s.samples[st.ContainerID] = window
	}

	// Clean up stale containers that are no longer reported
	for id := range s.latest {
		if _, ok := activeIDs[id]; !ok {
			delete(s.latest, id)
			delete(s.samples, id)
		}
	}
}

// GetLatest returns the most recent stats snapshot for a container.
func (s *Sampler) GetLatest(containerID string) (*ContainerStats, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	st, ok := s.latest[containerID]
	if !ok {
		return nil, false
	}
	return &st, true
}

// GetAllLatest returns the latest snapshot for all managed containers.
func (s *Sampler) GetAllLatest() []ContainerStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	res := make([]ContainerStats, 0, len(s.latest))
	for _, st := range s.latest {
		res = append(res, st)
	}
	return res
}

// GetSamples returns the rolling history of samples for a container.
func (s *Sampler) GetSamples(containerID string) []ContainerStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	window, ok := s.samples[containerID]
	if !ok {
		return nil
	}
	out := make([]ContainerStats, len(window))
	copy(out, window)
	return out
}
