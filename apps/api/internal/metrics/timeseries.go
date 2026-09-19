package metrics

import (
	"context"
	"sync"
	"time"
)

// TimeSeriesStore is the abstraction for Prometheus-compatible storage later.
// Current/summary metrics live in PostgreSQL snapshots; high-frequency series
// must not be written indefinitely into transactional Postgres.
type TimeSeriesStore interface {
	Write(ctx context.Context, samples []Sample) error
	Query(ctx context.Context, q RangeQuery) ([]Series, error)
}

type Sample struct {
	Metric    string
	Labels    map[string]string
	Timestamp time.Time
	Value     float64
}

type RangeQuery struct {
	Metric    string
	Labels    map[string]string
	Start     time.Time
	End       time.Time
	MaxPoints int
}

type Series struct {
	Metric string
	Labels map[string]string
	Points []Point
}

type Point struct {
	Timestamp time.Time
	Value     float64
}

// MemoryTimeSeries is a bounded in-process ring for recent samples (dev/test).
type MemoryTimeSeries struct {
	mu           sync.Mutex
	maxPerSeries int
	// keyed by metric|sorted labels
	series map[string]*ringSeries
}

type ringSeries struct {
	metric string
	labels map[string]string
	points []Point
}

func NewMemoryTimeSeries(maxPerSeries int) *MemoryTimeSeries {
	if maxPerSeries <= 0 {
		maxPerSeries = 256
	}
	return &MemoryTimeSeries{maxPerSeries: maxPerSeries, series: map[string]*ringSeries{}}
}

func (m *MemoryTimeSeries) Write(_ context.Context, samples []Sample) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range samples {
		key := seriesKey(s.Metric, s.Labels)
		rs, ok := m.series[key]
		if !ok {
			rs = &ringSeries{metric: s.Metric, labels: cloneLabels(s.Labels)}
			m.series[key] = rs
		}
		rs.points = append(rs.points, Point{Timestamp: s.Timestamp.UTC(), Value: s.Value})
		if len(rs.points) > m.maxPerSeries {
			rs.points = append([]Point(nil), rs.points[len(rs.points)-m.maxPerSeries:]...)
		}
	}
	return nil
}

func (m *MemoryTimeSeries) Query(_ context.Context, q RangeQuery) ([]Series, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Series, 0)
	for _, rs := range m.series {
		if q.Metric != "" && rs.metric != q.Metric {
			continue
		}
		if !labelsMatch(rs.labels, q.Labels) {
			continue
		}
		pts := make([]Point, 0, len(rs.points))
		for _, p := range rs.points {
			if !q.Start.IsZero() && p.Timestamp.Before(q.Start) {
				continue
			}
			if !q.End.IsZero() && p.Timestamp.After(q.End) {
				continue
			}
			pts = append(pts, p)
		}
		if q.MaxPoints > 0 && len(pts) > q.MaxPoints {
			pts = pts[len(pts)-q.MaxPoints:]
		}
		if len(pts) == 0 {
			continue
		}
		out = append(out, Series{Metric: rs.metric, Labels: cloneLabels(rs.labels), Points: pts})
	}
	return out, nil
}

// NopTimeSeries discards writes (production default until Prometheus is wired).
type NopTimeSeries struct{}

func (NopTimeSeries) Write(context.Context, []Sample) error { return nil }
func (NopTimeSeries) Query(context.Context, RangeQuery) ([]Series, error) {
	return nil, nil
}

func seriesKey(metric string, labels map[string]string) string {
	key := metric
	// Stable enough for in-memory; Prometheus adapter will use proper label sets.
	for k, v := range labels {
		key += "|" + k + "=" + v
	}
	return key
}

func cloneLabels(in map[string]string) map[string]string {
	if in == nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func labelsMatch(have, want map[string]string) bool {
	if len(want) == 0 {
		return true
	}
	for k, v := range want {
		if have[k] != v {
			return false
		}
	}
	return true
}
