package http

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
	"github.com/codex-k8s/yaml-mcp-server/internal/runtime/approver"
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
	// Async enables webhook-based async approvals.
	Async bool
	// WebhookURL is the yaml-mcp-server callback URL.
	WebhookURL string
	// Lang defines the preferred language for approver messages.
	Lang string
	// Markup selects approval message markup (markdown/html).
	Markup string
	// Pending stores async approvals.
	Pending *PendingStore
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
	if c.Async {
		if c.WebhookURL == "" {
			return approver.Decision{Allowed: false, Reason: "approver webhook url is empty", Source: c.Name()}, nil
		}
		if c.Pending == nil {
			return approver.Decision{Allowed: false, Reason: "approver async store is not configured", Source: c.Name()}, nil
		}
	}

	payload := protocol.ApproverRequest{
		CorrelationID: req.CorrelationID,
		Tool:          req.ToolName,
		Arguments:     req.Arguments,
		Lang:          c.Lang,
		Markup:        c.Markup,
	}
	if c.Async {
		payload.Callback = &protocol.ApproverCallback{URL: c.WebhookURL}
		if c.Timeout > 0 {
			payload.TimeoutSec = int(c.Timeout.Seconds())
		}
	}

	if justification, ok := extractString(req.Arguments, "justification"); ok {
		if err := validateReasonLength("justification", justification); err != nil {
			return approver.Decision{Allowed: false, Reason: err.Error(), Source: c.Name()}, nil
		}
		payload.Justification = justification
	}
	if approvalRequest, ok := extractString(req.Arguments, "approval_request"); ok {
		if err := validateReasonLength("approval_request", approvalRequest); err != nil {
			return approver.Decision{Allowed: false, Reason: err.Error(), Source: c.Name()}, nil
		}
		payload.ApprovalRequest = approvalRequest
	}
	if links, ok := extractLinks(req.Arguments); ok {
		payload.LinksToCode = links
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

	var pendingCh <-chan approver.Decision
	if c.Async {
		ch, err := c.Pending.Register(req.CorrelationID, c.Name())
		if err != nil {
			return approver.Decision{Allowed: false, Reason: "approval already pending", Source: c.Name()}, err
		}
		pendingCh = ch
		defer c.Pending.Cancel(req.CorrelationID)
	}

	resp, err := client.Do(request)
	if err != nil {
		return approver.Decision{Allowed: false, Reason: "approver request failed", Source: c.Name()}, err
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if c.Async && resp.StatusCode == http.StatusAccepted {
			return c.awaitDecision(ctx, pendingCh)
		}
		return approver.Decision{
			Allowed: false,
			Reason:  fmt.Sprintf("approver status %d: %s", resp.StatusCode, strings.TrimSpace(string(data))),
			Source:  c.Name(),
		}, nil
	}

	if c.Async && resp.StatusCode == http.StatusAccepted && len(bytes.TrimSpace(data)) == 0 {
		return c.awaitDecision(ctx, pendingCh)
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
	case protocol.DecisionPending:
		if c.Async {
			return c.awaitDecision(ctx, pendingCh)
		}
		return approver.Decision{Allowed: false, Reason: "approver returned pending decision", Source: c.Name()}, nil
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

func (c Client) awaitDecision(ctx context.Context, pendingCh <-chan approver.Decision) (approver.Decision, error) {
	if pendingCh == nil {
		return approver.Decision{Allowed: false, Reason: "missing pending approval channel", Source: c.Name()}, errors.New("pending channel is nil")
	}
	select {
	case decision, ok := <-pendingCh:
		if !ok {
			return approver.Decision{Allowed: false, Reason: "approval webhook channel closed", Source: c.Name()}, errors.New("pending channel closed")
		}
		if decision.Source == "" {
			decision.Source = c.Name()
		}
		return decision, nil
	case <-ctx.Done():
		return approver.Decision{Allowed: false, Reason: "approval timeout", Source: c.Name()}, ctx.Err()
	}
}

func extractString(args map[string]any, key string) (string, bool) {
	if args == nil {
		return "", false
	}
	raw, ok := args[key]
	if !ok {
		return "", false
	}
	value, ok := raw.(string)
	if !ok {
		return "", false
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	return value, true
}

func extractLinks(args map[string]any) ([]protocol.ApproverLink, bool) {
	if args == nil {
		return nil, false
	}
	raw, ok := args["links_to_code"]
	if !ok || raw == nil {
		return nil, false
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, false
	}
	if len(items) == 0 {
		return nil, false
	}
	if len(items) > 5 {
		items = items[:5]
	}
	links := make([]protocol.ApproverLink, 0, len(items))
	for _, item := range items {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		text, _ := obj["text"].(string)
		url, _ := obj["url"].(string)
		text = strings.TrimSpace(text)
		url = strings.TrimSpace(url)
		if text == "" || url == "" {
			continue
		}
		links = append(links, protocol.ApproverLink{Text: text, URL: url})
	}
	if len(links) == 0 {
		return nil, false
	}
	return links, true
}

func validateReasonLength(field, value string) error {
	length := len([]rune(strings.TrimSpace(value)))
	if length == 0 {
		return fmt.Errorf("%s is empty", field)
	}
	if length < 10 || length > 500 {
		return fmt.Errorf("%s must be 10-500 characters", field)
	}
	return nil
}
