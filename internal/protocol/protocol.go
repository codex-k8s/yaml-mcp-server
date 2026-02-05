package protocol

// Tool execution statuses.
const (
	StatusSuccess = "success"
	StatusDenied  = "denied"
	StatusError   = "error"
)

// Approval decisions.
const (
	DecisionApprove = "approve"
	DecisionDeny    = "deny"
	DecisionError   = "error"
	DecisionPending = "pending"
)

// ToolResponse is the fixed JSON response returned to MCP clients.
type ToolResponse struct {
	// Status indicates the execution status.
	Status string `json:"status"`
	// Decision indicates approval decision.
	Decision string `json:"decision"`
	// Reason is a human-readable message.
	Reason string `json:"reason,omitempty"`
	// CorrelationID links related requests.
	CorrelationID string `json:"correlation_id"`
}

// ApproverResponse is the fixed JSON response expected from HTTP approvers.
type ApproverResponse struct {
	// Decision is the approver decision.
	Decision string `json:"decision"`
	// Reason provides additional context.
	Reason string `json:"reason,omitempty"`
	// CorrelationID links the decision to the request.
	CorrelationID string `json:"correlation_id,omitempty"`
	// RequestID is an optional external identifier.
	RequestID string `json:"request_id,omitempty"`
}

// ApproverLink defines a human-friendly link.
type ApproverLink struct {
	// Text is the link label.
	Text string `json:"text"`
	// URL is the link target.
	URL string `json:"url"`
}

// ApproverCallback contains webhook configuration for async approvers.
type ApproverCallback struct {
	// URL is the webhook URL for decision callbacks.
	URL string `json:"url"`
}

// ApproverRequest is the payload sent to HTTP approvers.
type ApproverRequest struct {
	// CorrelationID links related requests.
	CorrelationID string `json:"correlation_id"`
	// Tool is the tool name.
	Tool string `json:"tool"`
	// Arguments are tool arguments.
	Arguments map[string]any `json:"arguments"`
	// Justification is a short reason from the model (10-500 chars).
	Justification string `json:"justification,omitempty"`
	// ApprovalRequest describes the requested action (10-500 chars).
	ApprovalRequest string `json:"approval_request,omitempty"`
	// LinksToCode are optional code references.
	LinksToCode []ApproverLink `json:"links_to_code,omitempty"`
	// Lang selects message language (ru/en).
	Lang string `json:"lang,omitempty"`
	// Markup selects message formatting (markdown/html).
	Markup string `json:"markup,omitempty"`
	// TimeoutSec defines approver timeout in seconds.
	TimeoutSec int `json:"timeout_sec,omitempty"`
	// Callback defines webhook settings for async approvers.
	Callback *ApproverCallback `json:"callback,omitempty"`
}

// ApproverDecision is the payload sent back to yaml-mcp-server via webhook.
type ApproverDecision struct {
	// CorrelationID links related requests.
	CorrelationID string `json:"correlation_id"`
	// Decision is the approver decision.
	Decision string `json:"decision"`
	// Reason provides additional context.
	Reason string `json:"reason,omitempty"`
	// Tool is an optional tool name for observability.
	Tool string `json:"tool,omitempty"`
	// Metadata is an optional opaque payload.
	Metadata map[string]any `json:"metadata,omitempty"`
	// RequestID is an optional external identifier.
	RequestID string `json:"request_id,omitempty"`
}
