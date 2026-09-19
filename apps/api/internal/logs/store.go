package logs

import (
	"context"
)

// Store is the pluggable log backend. Default is an in-process ring buffer.
// Future adapters: Loki, OpenSearch, ClickHouse.
type Store interface {
	Append(ctx context.Context, entries []Entry) error
	Query(ctx context.Context, q Query) (entries []Entry, nextCursor string, err error)
	// Subscribe delivers new entries after the query watermark. Caller must
	// cancel ctx / call unsubscribe on client disconnect. Slow consumers are
	// dropped (backpressure) rather than blocking writers indefinitely.
	Subscribe(ctx context.Context, q Query) (ch <-chan Entry, unsubscribe func(), err error)
}

// Config controls bounded buffer behavior for the memory store.
type Config struct {
	MaxEntriesPerStream int
	SubscriberBuffer    int
	MaxQueryLimit       int
}

func (c Config) withDefaults() Config {
	if c.MaxEntriesPerStream <= 0 {
		c.MaxEntriesPerStream = 5000
	}
	if c.SubscriberBuffer <= 0 {
		c.SubscriberBuffer = 256
	}
	if c.MaxQueryLimit <= 0 {
		c.MaxQueryLimit = 500
	}
	return c
}
