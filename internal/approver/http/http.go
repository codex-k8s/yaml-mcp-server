package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/codex-k8s/yaml-mcp-server/internal/protocol"
	"github.com/codex-k8s/yaml-mcp-server/internal/runtime/approver"
	"github.com/codex-k8s/yaml-mcp-server/internal/security"
)

// Client calls external HTTP approvers.
type Client struct {
	// Label is a human-friendly name.
	Label string
	// URL is the approver endpoint.
	URL string
	// Method overrides HTTP method.
	Method string
	// Headers adds HTTP headers.
	Headers map[string]string
	// Timeout is the HTTP timeout.
	Timeout time.Duration
}

// Request is the payload sent to HTTP approvers.
type Request struct {
	// CorrelationID links related requests.
	CorrelationID string `json:"correlation_id"`
	// Tool is the tool name.
	Tool string `json:"tool"`
	// Arguments are sanitized tool arguments.
	Arguments map[string]any `json:"arguments"`
}

// Name returns approver name for audit and logging.
func (c Client) Name() string {
	if c.Label != "" {
		return c.Label
	}
	return "http"
}

// Approve sends a request to the HTTP approver and parses the decision.
func (c Client) Approve(ctx context.Context, req approver.Request) (approver.Decision, error) {
	if c.URL == "" {
		return approver.Decision{Allowed: false, Reason: "approver url is empty", Source: c.Name()}, nil
	}

	payload := Request{
		CorrelationID: req.CorrelationID,
		Tool:          req.ToolName,
		Arguments:     security.RedactArguments(req.Arguments),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return approver.Decision{Allowed: false, Reason: "failed to encode request", Source: c.Name()}, err
	}

	method := c.Method
	if method == "" {
		method = http.MethodPost
	}

	request, err := http.NewRequestWithContext(ctx, method, c.URL, bytes.NewReader(body))
	if err != nil {
		return approver.Decision{Allowed: false, Reason: "failed to build request", Source: c.Name()}, err
	}
	request.Header.Set("Content-Type", "application/json")
	for key, value := range c.Headers {
		request.Header.Set(key, value)
	}

	client := &http.Client{Timeout: c.Timeout}
	resp, err := client.Do(request)
	if err != nil {
		return approver.Decision{Allowed: false, Reason: "approver request failed", Source: c.Name()}, err
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return approver.Decision{
			Allowed: false,
			Reason:  fmt.Sprintf("approver status %d: %s", resp.StatusCode, strings.TrimSpace(string(data))),
			Source:  c.Name(),
		}, nil
	}

	var parsed protocol.ApproverResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return approver.Decision{Allowed: false, Reason: "invalid approver response", Source: c.Name()}, err
	}

	decision := strings.ToLower(strings.TrimSpace(parsed.Decision))
	switch decision {
	case protocol.DecisionApprove:
		return approver.Decision{Allowed: true, Reason: fallbackReason(parsed.Reason, "approved"), Source: c.Name()}, nil
	case protocol.DecisionDeny:
		return approver.Decision{Allowed: false, Reason: fallbackReason(parsed.Reason, "denied"), Source: c.Name()}, nil
	case protocol.DecisionError:
		return approver.Decision{Allowed: false, Reason: fallbackReason(parsed.Reason, "approver error"), Source: c.Name()}, nil
	default:
		return approver.Decision{Allowed: false, Reason: "unknown approver decision", Source: c.Name()}, fmt.Errorf("unknown approver decision: %s", decision)
	}
}

func fallbackReason(reason, fallback string) string {
	if strings.TrimSpace(reason) == "" {
		return fallback
	}
	return reason
}
