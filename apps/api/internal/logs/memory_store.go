package logs

import (
	"context"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// MemoryStore is a process-local ring-buffer LogStore. It never writes to PostgreSQL.
type MemoryStore struct {
	cfg  Config
	mu   sync.RWMutex
	bufs map[string]*ringBuffer
	seq  atomic.Uint64
}

func NewMemoryStore(cfg Config) *MemoryStore {
	return &MemoryStore{
		cfg:  cfg.withDefaults(),
		bufs: map[string]*ringBuffer{},
	}
}

type ringBuffer struct {
	mu          sync.Mutex
	entries     []Entry
	max         int
	subscribers map[uint64]chan Entry
	nextSubID   uint64
}

func (s *MemoryStore) Append(_ context.Context, entries []Entry) error {
	if len(entries) == 0 {
		return nil
	}
	grouped := map[string][]Entry{}
	for _, e := range entries {
		if e.Sequence == 0 {
			e.Sequence = s.seq.Add(1)
		}
		if e.Cursor == "" {
			e.Cursor = cursorFromSeq(e.Sequence)
		}
		if e.Timestamp.IsZero() {
			e.Timestamp = time.Now().UTC()
		}
		if e.Stream == "" {
			e.Stream = StreamStdout
		}
		key := StreamKey(e.OrganizationID, e.Kind, e.ApplicationID)
		grouped[key] = append(grouped[key], e)
	}
	for key, batch := range grouped {
		s.buffer(key).append(batch)
	}
	return nil
}

func (s *MemoryStore) Query(_ context.Context, q Query) ([]Entry, string, error) {
	limit := q.Limit
	if limit <= 0 || limit > s.cfg.MaxQueryLimit {
		limit = s.cfg.MaxQueryLimit
	}
	key := StreamKey(q.OrganizationID, q.Kind, q.ApplicationID)
	filtered := filterEntries(s.buffer(key).snapshot(), q)
	if len(filtered) > limit {
		filtered = filtered[len(filtered)-limit:]
	}
	next := ""
	if len(filtered) > 0 {
		next = filtered[len(filtered)-1].Cursor
	}
	return filtered, next, nil
}

func (s *MemoryStore) Subscribe(ctx context.Context, q Query) (<-chan Entry, func(), error) {
	key := StreamKey(q.OrganizationID, q.Kind, q.ApplicationID)
	buf := s.buffer(key)

	subCtx, cancel := context.WithCancel(ctx)
	// Subscribe before snapshot so we don't miss lines written between query and live.
	liveCh, unsubLive := buf.subscribe(s.cfg.SubscriberBuffer)
	out := make(chan Entry, s.cfg.SubscriberBuffer)

	go func() {
		defer close(out)
		defer unsubLive()
		defer cancel()

		entries, _, _ := s.Query(subCtx, Query{
			OrganizationID: q.OrganizationID,
			ApplicationID:  q.ApplicationID,
			DeploymentID:   q.DeploymentID,
			Kind:           q.Kind,
			Since:          q.Since,
			Cursor:         q.Cursor,
			Limit:          s.cfg.MaxQueryLimit,
		})
		var lastSeq uint64
		if q.Cursor != "" {
			if n, ok := ParseCursorSeq(q.Cursor); ok {
				lastSeq = n
			}
		}
		for _, e := range entries {
			lastSeq = e.Sequence
			select {
			case <-subCtx.Done():
				return
			case out <- e:
			default:
				// Slow consumer during replay: drop older historical lines.
			}
		}

		for {
			select {
			case <-subCtx.Done():
				return
			case e, ok := <-liveCh:
				if !ok {
					return
				}
				if e.Sequence <= lastSeq {
					continue
				}
				if !matchEntry(e, q) {
					continue
				}
				if q.Since != nil && !e.Timestamp.After(*q.Since) {
					continue
				}
				lastSeq = e.Sequence
				select {
				case <-subCtx.Done():
					return
				case out <- e:
				default:
					// Backpressure: drop for this slow subscriber; never block ingest.
				}
			}
		}
	}()

	return out, cancel, nil
}

func (s *MemoryStore) buffer(key string) *ringBuffer {
	s.mu.Lock()
	defer s.mu.Unlock()
	if b, ok := s.bufs[key]; ok {
		return b
	}
	b := &ringBuffer{
		max:         s.cfg.MaxEntriesPerStream,
		subscribers: map[uint64]chan Entry{},
	}
	s.bufs[key] = b
	return b
}

func (b *ringBuffer) append(entries []Entry) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.entries = append(b.entries, entries...)
	if len(b.entries) > b.max {
		b.entries = append([]Entry(nil), b.entries[len(b.entries)-b.max:]...)
	}
	for _, e := range entries {
		for _, ch := range b.subscribers {
			select {
			case ch <- e:
			default:
				// Drop for slow subscriber.
			}
		}
	}
}

func (b *ringBuffer) snapshot() []Entry {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]Entry, len(b.entries))
	copy(out, b.entries)
	return out
}

func (b *ringBuffer) subscribe(bufSize int) (chan Entry, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.nextSubID++
	id := b.nextSubID
	ch := make(chan Entry, bufSize)
	b.subscribers[id] = ch
	unsub := func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if c, ok := b.subscribers[id]; ok {
			delete(b.subscribers, id)
			close(c)
		}
	}
	return ch, unsub
}

func filterEntries(entries []Entry, q Query) []Entry {
	out := make([]Entry, 0, len(entries))
	for _, e := range entries {
		if !matchEntry(e, q) {
			continue
		}
		if q.Cursor != "" {
			if n, ok := ParseCursorSeq(q.Cursor); ok {
				if e.Sequence <= n {
					continue
				}
			} else if !afterCursor(e.Cursor, q.Cursor) {
				continue
			}
		}
		if q.Since != nil && !e.Timestamp.After(*q.Since) {
			continue
		}
		out = append(out, e)
	}
	return out
}

func matchEntry(e Entry, q Query) bool {
	if e.OrganizationID != q.OrganizationID || e.Kind != q.Kind {
		return false
	}
	if q.ApplicationID != nil && (e.ApplicationID == nil || *e.ApplicationID != *q.ApplicationID) {
		return false
	}
	if q.DeploymentID != nil && (e.DeploymentID == nil || *e.DeploymentID != *q.DeploymentID) {
		return false
	}
	return true
}

func cursorFromSeq(seq uint64) string {
	return "m" + strconv.FormatUint(seq, 10)
}

func afterCursor(cur, after string) bool {
	if after == "" {
		return true
	}
	a, okA := ParseCursorSeq(cur)
	b, okB := ParseCursorSeq(after)
	if okA && okB {
		return a > b
	}
	return cur > after
}

func ParseCursorSeq(cur string) (uint64, bool) {
	if len(cur) < 2 || cur[0] != 'm' {
		return 0, false
	}
	n, err := strconv.ParseUint(cur[1:], 10, 64)
	return n, err == nil
}
