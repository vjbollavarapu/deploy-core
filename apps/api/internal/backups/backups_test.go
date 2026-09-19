package backups

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestLocalDestinationAllocate(t *testing.T) {
	org := uuid.New()
	bid := uuid.New()
	uri, err := LocalDestination{}.Allocate(context.Background(), org, bid)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(uri, "local://org/") || !strings.Contains(uri, bid.String()) {
		t.Fatalf("uri=%s", uri)
	}
	if strings.Contains(uri, "..") || strings.HasPrefix(uri, "/") {
		t.Fatalf("unsafe uri=%s", uri)
	}
}

func TestS3DestinationReserved(t *testing.T) {
	d, err := LookupDestination("s3")
	if err != nil {
		t.Fatal(err)
	}
	_, err = d.Allocate(context.Background(), uuid.New(), uuid.New())
	if err == nil {
		t.Fatal("expected s3 not enabled")
	}
}

func TestRestoreConfirmPhrase(t *testing.T) {
	if RestoreConfirmPhrase != "RESTORE" {
		t.Fatalf("confirm=%s", RestoreConfirmPhrase)
	}
}
