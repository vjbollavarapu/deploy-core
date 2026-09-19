package pagination_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deploycore/deploy-core/apps/api/pkg/pagination"
)

func TestFromRequestDefaultsAndCaps(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/items?limit=500&offset=-1", nil)
	p := pagination.FromRequest(req)
	if p.Limit != pagination.MaxLimit {
		t.Fatalf("limit = %d, want %d", p.Limit, pagination.MaxLimit)
	}
	if p.Offset != 0 {
		t.Fatalf("offset = %d, want 0", p.Offset)
	}

	req = httptest.NewRequest(http.MethodGet, "/items?limit=10&offset=30", nil)
	p = pagination.FromRequest(req)
	if p.Limit != 10 || p.Offset != 30 {
		t.Fatalf("got %+v", p)
	}
}
