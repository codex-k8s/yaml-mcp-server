package executor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/codex-k8s/yaml-mcp-server/internal/protocol"
)

// HTTP calls an external HTTP executor.
type HTTP struct {
	// URL is the executor endpoint.
	URL string
	// Method overrides HTTP method.
	Method string
	// Headers adds HTTP headers.
	Headers map[string]string
	// Timeout is the HTTP client timeout.
	Timeout time.Duration
	// Async enables webhook-based execution flow.
	Async bool
	// WebhookURL is the yaml-mcp-server callback URL.
	WebhookURL string
	// Pending stores async execution requests.
	Pending *PendingStore
	// Spec contains declarative executor settings.
	Spec map[string]any
	// Tool describes the tool metadata sent to external executor.
	Tool protocol.ExecutorTool
	// Lang defines the preferred language for messages.
	Lang string
	// Markup selects message formatting (markdown/html).
	Markup string
}

// Execute sends execution request to external HTTP executor and parses result.
func (h HTTP) Execute(ctx context.Context, req Request) (string, error) {
	if strings.TrimSpace(h.URL) == "" {
		return "", errors.New("executor url is empty")
	}
	if h.Async {
		if strings.TrimSpace(h.WebhookURL) == "" {
			return "", errors.New("executor webhook url is empty")
		}
		if h.Pending == nil {
			return "", errors.New("executor async store is not configured")
		}
	}

	timeoutSec := 0
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining > 0 {
			timeoutSec = int(remaining.Seconds())
			if timeoutSec < 1 {
				timeoutSec = 1
			}
		}
	}

	payload := protocol.ExecutorRequest{
		CorrelationID: req.CorrelationID,
		Tool:          h.Tool,
		Arguments:     req.Arguments,
		Spec:          h.Spec,
		Lang:          normalizeLang(h.Lang, "en"),
		Markup:        h.Markup,
		TimeoutSec:    timeoutSec,
	}
	if h.Async {
		payload.Callback = &protocol.ExecutorCallback{URL: h.WebhookURL}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to encode request: %w", err)
	}

	method := strings.ToUpper(strings.TrimSpace(h.Method))
	if method == "" {
		method = http.MethodPost
	}
	request, err := http.NewRequestWithContext(ctx, method, h.URL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to build request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	for key, value := range h.Headers {
		request.Header.Set(key, value)
	}

	clientTimeout := h.Timeout
	if clientTimeout <= 0 {
		clientTimeout = 10 * time.Second
	}
	client := &http.Client{Timeout: clientTimeout}

	var pendingCh <-chan asyncResult
	if h.Async {
		ch, err := h.Pending.Register(req.CorrelationID)
		if err != nil {
			return "", err
		}
		pendingCh = ch
		defer h.Pending.Cancel(req.CorrelationID)
	}

	resp, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("executor request failed: %w", err)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	dataTrimmed := strings.TrimSpace(string(data))

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if h.Async && resp.StatusCode == http.StatusAccepted {
			return h.awaitResult(ctx, pendingCh)
		}
		return "", fmt.Errorf("executor status %d: %s", resp.StatusCode, dataTrimmed)
	}

	if h.Async && resp.StatusCode == http.StatusAccepted && len(bytes.TrimSpace(data)) == 0 {
		return h.awaitResult(ctx, pendingCh)
	}

	var parsed protocol.ExecutorResponse
	if err := json.Unmarshal(data, &parsed); err == nil && strings.TrimSpace(parsed.Status) != "" {
		status := strings.ToLower(strings.TrimSpace(parsed.Status))
		result := stringifyResult(parsed.Result)
		switch status {
		case protocol.StatusSuccess:
			if result == "" {
				return "ok", nil
			}
			return result, nil
		case protocol.StatusError:
			if result == "" {
				result = "executor error"
			}
			return result, errors.New(result)
		case protocol.StatusPending:
			if h.Async {
				return h.awaitResult(ctx, pendingCh)
			}
			return "", errors.New("executor returned pending status")
		default:
			return "", fmt.Errorf("unknown executor status: %s", status)
		}
	}

	if h.Async && resp.StatusCode == http.StatusAccepted {
		return h.awaitResult(ctx, pendingCh)
	}
	return dataTrimmed, nil
}

func (h HTTP) awaitResult(ctx context.Context, pendingCh <-chan asyncResult) (string, error) {
	if pendingCh == nil {
		return "", errors.New("missing pending execution channel")
	}
	select {
	case result, ok := <-pendingCh:
		if !ok {
			return "", errors.New("execution webhook channel closed")
		}
		if result.status == protocol.StatusSuccess {
			return result.result, nil
		}
		if strings.TrimSpace(result.result) == "" {
			return "", errors.New("executor error")
		}
		return result.result, errors.New(result.result)
	case <-ctx.Done():
		return "execution timeout", ctx.Err()
	}
}

func stringifyResult(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	default:
		data, err := json.Marshal(typed)
		if err != nil {
			return fmt.Sprintf("%v", typed)
		}
		return strings.TrimSpace(string(data))
	}
}

func normalizeLang(value, fallback string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	switch value {
	case "ru", "en":
		return value
	}
	fallback = strings.TrimSpace(strings.ToLower(fallback))
	switch fallback {
	case "ru", "en":
		return fallback
	default:
		return "en"
	}
}
