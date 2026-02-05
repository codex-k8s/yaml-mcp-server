package http

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"github.com/codex-k8s/yaml-mcp-server/internal/maputil"
	"github.com/codex-k8s/yaml-mcp-server/internal/protocol"
	"github.com/codex-k8s/yaml-mcp-server/internal/runtime/approver"
)

// PendingStore keeps async approval results.
type PendingStore struct {
	mu      sync.Mutex
	pending map[string]*pendingApproval
}

type pendingApproval struct {
	ch     chan approver.Decision
	source string
}

// NewPendingStore creates a new async approval store.
func NewPendingStore() *PendingStore {
	return &PendingStore{pending: make(map[string]*pendingApproval)}
}

// Register allocates a pending slot for correlationID.
func (s *PendingStore) Register(correlationID, source string) (<-chan approver.Decision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.pending[correlationID]; exists {
		return nil, errAlreadyPending
	}
	ch := make(chan approver.Decision, 1)
	s.pending[correlationID] = &pendingApproval{ch: ch, source: source}
	return ch, nil
}

// Resolve delivers a decision for correlationID.
func (s *PendingStore) Resolve(correlationID string, decision approver.Decision) bool {
	entry, ok := maputil.Pop(&s.mu, s.pending, correlationID)
	if !ok {
		return false
	}
	if decision.Source == "" {
		decision.Source = entry.source
	}
	select {
	case entry.ch <- decision:
	default:
	}
	close(entry.ch)
	return true
}

// Cancel removes a pending approval without a decision.
func (s *PendingStore) Cancel(correlationID string) {
	entry, ok := maputil.Pop(&s.mu, s.pending, correlationID)
	if ok {
		close(entry.ch)
	}
}

// WebhookHandler handles async approval callbacks.
type WebhookHandler struct {
	Store  *PendingStore
	Logger *slog.Logger
}

// ServeHTTP processes webhook callbacks from async approvers.
func (h *WebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if h.Store == nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	var payload protocol.ApproverDecision
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&payload); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	correlationID := strings.TrimSpace(payload.CorrelationID)
	decision := strings.ToLower(strings.TrimSpace(payload.Decision))
	if correlationID == "" || decision == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	switch decision {
	case protocol.DecisionApprove, protocol.DecisionDeny, protocol.DecisionError:
	default:
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var resolved bool
	switch decision {
	case protocol.DecisionApprove:
		resolved = h.Store.Resolve(correlationID, approver.Decision{Allowed: true, Reason: payload.Reason})
	case protocol.DecisionDeny:
		resolved = h.Store.Resolve(correlationID, approver.Decision{Allowed: false, Reason: fallbackReason(payload.Reason, "denied")})
	case protocol.DecisionError:
		resolved = h.Store.Resolve(correlationID, approver.Decision{Allowed: false, Reason: fallbackReason(payload.Reason, "approver error")})
	}
	if !resolved {
		if h.Logger != nil {
			h.Logger.Warn("approval webhook not found", "correlation_id", correlationID)
		}
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusOK)
}

var errAlreadyPending = errors.New("approval already pending")
