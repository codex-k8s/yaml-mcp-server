package executor

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"github.com/codex-k8s/yaml-mcp-server/internal/maputil"
	"github.com/codex-k8s/yaml-mcp-server/internal/protocol"
)

type asyncResult struct {
	status string
	result string
}

type pendingExecution struct {
	ch chan asyncResult
}

// PendingStore keeps async execution results.
type PendingStore struct {
	mu      sync.Mutex
	pending map[string]*pendingExecution
}

// NewPendingStore creates a new async execution store.
func NewPendingStore() *PendingStore {
	return &PendingStore{pending: make(map[string]*pendingExecution)}
}

// Register allocates a pending slot for correlationID.
func (s *PendingStore) Register(correlationID string) (<-chan asyncResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.pending[correlationID]; exists {
		return nil, errExecutionAlreadyPending
	}
	ch := make(chan asyncResult, 1)
	s.pending[correlationID] = &pendingExecution{ch: ch}
	return ch, nil
}

// Resolve delivers an async execution result for correlationID.
func (s *PendingStore) Resolve(correlationID, status, result string) bool {
	entry, ok := maputil.Pop(&s.mu, s.pending, correlationID)
	if !ok {
		return false
	}

	select {
	case entry.ch <- asyncResult{status: status, result: result}:
	default:
	}
	close(entry.ch)
	return true
}

// Cancel removes a pending execution without a result.
func (s *PendingStore) Cancel(correlationID string) {
	entry, ok := maputil.Pop(&s.mu, s.pending, correlationID)
	if ok {
		close(entry.ch)
	}
}

// WebhookHandler handles async executor callbacks.
type WebhookHandler struct {
	Store  *PendingStore
	Logger *slog.Logger
}

// ServeHTTP processes webhook callbacks from async executors.
func (h *WebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if h.Store == nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var payload protocol.ExecutorDecision
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&payload); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	correlationID := strings.TrimSpace(payload.CorrelationID)
	status := strings.ToLower(strings.TrimSpace(payload.Status))
	if correlationID == "" || status == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	switch status {
	case protocol.StatusSuccess, protocol.StatusError:
	default:
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	result := stringifyResult(payload.Result)
	if status == protocol.StatusSuccess && strings.TrimSpace(result) == "" {
		result = "ok"
	}
	if status == protocol.StatusError && strings.TrimSpace(result) == "" {
		result = "executor error"
	}

	if !h.Store.Resolve(correlationID, status, result) {
		if h.Logger != nil {
			h.Logger.Warn("executor webhook not found", "correlation_id", correlationID)
		}
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusOK)
}

var errExecutionAlreadyPending = errors.New("execution already pending")
