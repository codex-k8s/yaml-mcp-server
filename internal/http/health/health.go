package health

import (
	"net/http"
	"sync/atomic"
)

type Handler struct {
	ready atomic.Bool
}

// New returns a health handler instance.
func New() *Handler {
	return &Handler{}
}

// SetReady marks the handler as ready.
func (h *Handler) SetReady() {
	h.ready.Store(true)
}

// SetNotReady marks the handler as not ready.
func (h *Handler) SetNotReady() {
	h.ready.Store(false)
}

// Healthz handles liveness probes.
func (h *Handler) Healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// Readyz handles readiness probes.
func (h *Handler) Readyz(w http.ResponseWriter, _ *http.Request) {
	if h.ready.Load() {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready"))
		return
	}
	w.WriteHeader(http.StatusServiceUnavailable)
	_, _ = w.Write([]byte("not ready"))
}
