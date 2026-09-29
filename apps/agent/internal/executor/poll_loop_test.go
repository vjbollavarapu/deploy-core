package executor_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deploycore/deploy-core/apps/agent/internal/executor"
	"github.com/deploycore/deploy-core/packages/protocol-go"
	"github.com/google/uuid"
)

// immediatePollTransport returns as soon as the control plane would: an empty
// 200 is a successful poll, not a long-poll hold. The production HTTP client
// behaves this way; the blocking mock used by dispatch tests does not.
type immediatePollTransport struct {
	mockTransport
	mu    sync.Mutex
	poll  func() ([]protocol.CommandEnvelope, error)
	times []time.Time
}

func (t *immediatePollTransport) PollCommands(ctx context.Context) ([]protocol.CommandEnvelope, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	t.mu.Lock()
	t.times = append(t.times, time.Now())
	t.mu.Unlock()
	if t.poll == nil {
		return nil, nil
	}
	return t.poll()
}

func (t *immediatePollTransport) pollTimes() []time.Time {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]time.Time, len(t.times))
	copy(out, t.times)
	return out
}

func TestRunLoop_EmptyPollIsThrottled(t *testing.T) {
	t.Parallel()
	tr := &immediatePollTransport{}
	exec := newImmediateExecutor(t, uuid.New(), tr)
	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		cancel()
		exec.Stop()
	}()

	exec.Start(ctx)
	times := waitForPolls(t, tr, 2, executor.IdlePollInterval+time.Second)
	gap := times[1].Sub(times[0])
	if gap < executor.IdlePollInterval-250*time.Millisecond {
		t.Fatalf("empty polls ran back-to-back: gap %s, idle interval %s", gap, executor.IdlePollInterval)
	}
	if extra := len(tr.pollTimes()); extra > 3 {
		t.Fatalf("idle polling issued %d requests before the second interval elapsed", extra)
	}
}

func TestRunLoop_CommandBatchDispatchesWithoutIdleDelay(t *testing.T) {
	t.Parallel()
	serverID := uuid.New()
	cmd := makeEnvelope(serverID, protocol.OpStopContainer)
	var delivered atomic.Bool
	tr := &immediatePollTransport{
		poll: func() ([]protocol.CommandEnvelope, error) {
			if delivered.CompareAndSwap(false, true) {
				return []protocol.CommandEnvelope{cmd}, nil
			}
			return nil, nil
		},
	}
	exec := newImmediateExecutor(t, serverID, tr)
	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		cancel()
		exec.Stop()
	}()

	exec.Start(ctx)
	waitForStatus(t, &tr.mockTransport, cmd.ID, string(executor.StateAccepted), 500*time.Millisecond)
	times := waitForPolls(t, tr, 2, 500*time.Millisecond)
	gap := times[1].Sub(times[0])
	if gap >= executor.IdlePollInterval/2 {
		t.Fatalf("next poll waited %s after a non-empty batch; dispatch must not sit behind the idle interval", gap)
	}
}

func TestRunLoop_CancelInterruptsIdleWait(t *testing.T) {
	t.Parallel()
	tr := &immediatePollTransport{}
	exec := newImmediateExecutor(t, uuid.New(), tr)
	ctx, cancel := context.WithCancel(context.Background())

	exec.Start(ctx)
	waitForPolls(t, tr, 1, time.Second)

	cancel()
	stopped := make(chan struct{})
	go func() {
		exec.Stop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("shutdown stayed blocked in the idle poll wait")
	}
}

func TestRunLoop_ErrorBackoffRemainsIntact(t *testing.T) {
	t.Parallel()
	var n atomic.Int32
	tr := &immediatePollTransport{
		poll: func() ([]protocol.CommandEnvelope, error) {
			if n.Add(1) <= 2 {
				return nil, errors.New("control plane unavailable")
			}
			return nil, nil
		},
	}
	exec := newImmediateExecutor(t, uuid.New(), tr)
	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		cancel()
		exec.Stop()
	}()

	exec.Start(ctx)
	// Two failures use 1s then 2s. The following empty success uses the idle
	// interval, which only matches the error schedule when backoff was reset.
	times := waitForPolls(t, tr, 4, 8*time.Second)
	gap1 := times[1].Sub(times[0])
	gap2 := times[2].Sub(times[1])
	gap3 := times[3].Sub(times[2])

	if gap1 < 800*time.Millisecond || gap1 > 1700*time.Millisecond {
		t.Fatalf("first poll error backoff = %s, want about 1s", gap1)
	}
	if gap2 < 1600*time.Millisecond || gap2 > 3500*time.Millisecond {
		t.Fatalf("second poll error backoff = %s, want about 2s", gap2)
	}
	if gap3 < executor.IdlePollInterval-250*time.Millisecond || gap3 > executor.IdlePollInterval+1500*time.Millisecond {
		t.Fatalf("backoff after a successful empty poll = %s, want the idle interval %s", gap3, executor.IdlePollInterval)
	}
}

func newImmediateExecutor(t *testing.T, serverID uuid.UUID, tr *immediatePollTransport) *executor.Executor {
	t.Helper()
	return executor.NewWithTransport(executor.Config{
		ServerID:       serverID,
		MaxConcurrency: 4,
		DataDir:        t.TempDir(),
	}, tr)
}

func waitForPolls(t *testing.T, tr *immediatePollTransport, n int, timeout time.Duration) []time.Time {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		times := tr.pollTimes()
		if len(times) >= n {
			return times[:n]
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %d polls, saw %d", n, len(times))
		}
		time.Sleep(10 * time.Millisecond)
	}
}
