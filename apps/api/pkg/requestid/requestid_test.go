package requestid_test

import (
	"context"
	"testing"

	"github.com/deploycore/deploy-core/apps/api/pkg/requestid"
)

func TestNewAndContextRoundTrip(t *testing.T) {
	t.Parallel()

	id := requestid.New()
	if len(id) != 32 {
		t.Fatalf("id length = %d", len(id))
	}
	ctx := requestid.WithContext(context.Background(), id)
	if got := requestid.FromContext(ctx); got != id {
		t.Fatalf("from context = %s", got)
	}
	if got := requestid.FromContext(context.Background()); got != "" {
		t.Fatalf("empty context should return blank, got %q", got)
	}
}
