package metrics

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestMemoryTimeSeriesBoundedQuery(t *testing.T) {
	ts := NewMemoryTimeSeries(3)
	ctx := context.Background()
	server := uuid.New().String()
	labels := map[string]string{"server_id": server}
	base := time.Now().UTC()
	for i := 0; i < 5; i++ {
		_ = ts.Write(ctx, []Sample{{
			Metric: "server_cpu_percent", Labels: labels,
			Timestamp: base.Add(time.Duration(i) * time.Second),
			Value:     float64(i),
		}})
	}
	series, err := ts.Query(ctx, RangeQuery{Metric: "server_cpu_percent", Labels: labels})
	if err != nil {
		t.Fatal(err)
	}
	if len(series) != 1 {
		t.Fatalf("series=%d", len(series))
	}
	if len(series[0].Points) != 3 {
		t.Fatalf("points=%d want 3", len(series[0].Points))
	}
	if series[0].Points[0].Value != 2 || series[0].Points[2].Value != 4 {
		t.Fatalf("points=%v", series[0].Points)
	}
}

func TestNopTimeSeries(t *testing.T) {
	var n NopTimeSeries
	if err := n.Write(context.Background(), []Sample{{Metric: "x", Value: 1}}); err != nil {
		t.Fatal(err)
	}
	got, err := n.Query(context.Background(), RangeQuery{Metric: "x"})
	if err != nil || len(got) != 0 {
		t.Fatalf("got=%v err=%v", got, err)
	}
}
