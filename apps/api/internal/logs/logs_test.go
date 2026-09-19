package logs

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestMemoryStoreBoundedAndCursor(t *testing.T) {
	store := NewMemoryStore(Config{MaxEntriesPerStream: 3, MaxQueryLimit: 100, SubscriberBuffer: 8})
	org := uuid.New()
	app := uuid.New()
	ctx := context.Background()

	for i := 1; i <= 5; i++ {
		msg := string(rune('a' + i - 1))
		if err := store.Append(ctx, []Entry{{
			OrganizationID: org,
			ApplicationID:  &app,
			Kind:           KindRuntime,
			Stream:         StreamStdout,
			Message:        msg,
			Timestamp:      time.Now().UTC(),
		}}); err != nil {
			t.Fatal(err)
		}
	}

	entries, _, err := store.Query(ctx, Query{
		OrganizationID: org,
		ApplicationID:  &app,
		Kind:           KindRuntime,
		Limit:          10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("len=%d want 3 (bounded)", len(entries))
	}
	if entries[0].Message != "c" || entries[2].Message != "e" {
		t.Fatalf("entries=%v", []string{entries[0].Message, entries[1].Message, entries[2].Message})
	}

	after := entries[0].Cursor
	more, _, err := store.Query(ctx, Query{
		OrganizationID: org,
		ApplicationID:  &app,
		Kind:           KindRuntime,
		Cursor:         after,
		Limit:          10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(more) != 2 || more[0].Message != "d" {
		t.Fatalf("cursor filter=%v", more)
	}
}

func TestMemoryStoreSubscribeBackpressureAndCleanup(t *testing.T) {
	store := NewMemoryStore(Config{MaxEntriesPerStream: 100, SubscriberBuffer: 1, MaxQueryLimit: 100})
	org := uuid.New()
	app := uuid.New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, unsub, err := store.Subscribe(ctx, Query{
		OrganizationID: org,
		ApplicationID:  &app,
		Kind:           KindBuild,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer unsub()

	// Flood without reading — must not block Append.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 50; i++ {
			_ = store.Append(context.Background(), []Entry{{
				OrganizationID: org,
				ApplicationID:  &app,
				Kind:           KindBuild,
				Stream:         StreamStderr,
				Message:        "line",
				Timestamp:      time.Now().UTC(),
			}})
		}
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Append blocked on slow subscriber")
	}

	unsub()
	cancel()
	// Channel should close after unsubscribe/cancel.
	deadline := time.After(2 * time.Second)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("subscriber channel did not close")
		}
	}
}

func TestMemoryStoreDeploymentFilter(t *testing.T) {
	store := NewMemoryStore(Config{})
	org := uuid.New()
	app := uuid.New()
	dep1 := uuid.New()
	dep2 := uuid.New()
	ctx := context.Background()
	_ = store.Append(ctx, []Entry{
		{OrganizationID: org, ApplicationID: &app, DeploymentID: &dep1, Kind: KindRuntime, Stream: StreamStdout, Message: "one"},
		{OrganizationID: org, ApplicationID: &app, DeploymentID: &dep2, Kind: KindRuntime, Stream: StreamStdout, Message: "two"},
	})
	got, _, err := store.Query(ctx, Query{OrganizationID: org, ApplicationID: &app, DeploymentID: &dep1, Kind: KindRuntime})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Message != "one" {
		t.Fatalf("got=%v", got)
	}
}
