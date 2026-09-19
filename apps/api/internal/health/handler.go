package health

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/deploycore/deploy-core/apps/api/pkg/apierror"
	"github.com/deploycore/deploy-core/apps/api/pkg/requestid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Handler serves liveness and readiness probes.
type Handler struct {
	pool *pgxpool.Pool
}

func NewHandler(pool *pgxpool.Pool) *Handler {
	return &Handler{pool: pool}
}

type statusResponse struct {
	Status string `json:"status"`
}

// Live handles GET /health — process is up.
func (h *Handler) Live(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, statusResponse{Status: "ok"})
}

// Ready handles GET /ready — dependencies are reachable.
func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if h.pool == nil {
		apierror.WriteJSON(w, requestid.FromContext(r.Context()), apierror.Unavailable("database not configured"))
		return
	}
	if err := h.pool.Ping(ctx); err != nil {
		apierror.WriteJSON(w, requestid.FromContext(r.Context()), apierror.Unavailable("database unavailable"))
		return
	}
	writeJSON(w, http.StatusOK, statusResponse{Status: "ready"})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
