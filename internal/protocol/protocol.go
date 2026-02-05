package protocol

// Tool execution statuses.
const (
	StatusSuccess = "success"
	StatusDenied  = "denied"
	StatusError   = "error"
	StatusPending = "pending"
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
	// RiskAssessment describes potential risks (10-500 chars).
	RiskAssessment string `json:"risk_assessment,omitempty"`
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

// ExecutorCallback contains webhook configuration for async executors.
type ExecutorCallback struct {
	// URL is the webhook URL for result callbacks.
	URL string `json:"url"`
}

// ExecutorTool describes tool metadata for external executors.
type ExecutorTool struct {
	// Name is the tool name.
	Name string `json:"name"`
	// Title is an optional human-friendly title.
	Title string `json:"title,omitempty"`
	// Description explains the tool behavior.
	Description string `json:"description,omitempty"`
	// InputSchema defines expected tool arguments.
	InputSchema map[string]any `json:"input_schema,omitempty"`
	// OutputSchema defines tool response schema.
	OutputSchema map[string]any `json:"output_schema,omitempty"`
	// Metadata is an optional opaque map.
	Metadata map[string]any `json:"metadata,omitempty"`
	// Tags is an optional tool tags list.
	Tags []string `json:"tags,omitempty"`
}

// ExecutorRequest is the payload sent to HTTP executors.
type ExecutorRequest struct {
	// CorrelationID links related requests.
	CorrelationID string `json:"correlation_id"`
	// Tool describes the requested tool.
	Tool ExecutorTool `json:"tool"`
	// Arguments are tool arguments.
	Arguments map[string]any `json:"arguments"`
	// Spec contains declarative executor settings from YAML.
	Spec map[string]any `json:"spec,omitempty"`
	// Lang selects message language (ru/en).
	Lang string `json:"lang,omitempty"`
	// Markup selects message formatting (markdown/html).
	Markup string `json:"markup,omitempty"`
	// TimeoutSec defines execution timeout in seconds.
	TimeoutSec int `json:"timeout_sec,omitempty"`
	// Callback defines webhook settings for async executors.
	Callback *ExecutorCallback `json:"callback,omitempty"`
}

// ExecutorResponse is the fixed JSON response expected from HTTP executors.
type ExecutorResponse struct {
	// Status is one of success/error/pending.
	Status string `json:"status"`
	// Result provides execution output.
	Result any `json:"result,omitempty"`
	// CorrelationID links the response to the request.
	CorrelationID string `json:"correlation_id,omitempty"`
	// RequestID is an optional external identifier.
	RequestID string `json:"request_id,omitempty"`
}

// ExecutorDecision is the payload sent back to yaml-mcp-server via webhook.
type ExecutorDecision struct {
	// CorrelationID links related requests.
	CorrelationID string `json:"correlation_id"`
	// Status is one of success/error.
	Status string `json:"status"`
	// Result provides execution output or error details.
	Result any `json:"result,omitempty"`
	// Tool is an optional tool name for observability.
	Tool string `json:"tool,omitempty"`
	// Metadata is an optional opaque payload.
	Metadata map[string]any `json:"metadata,omitempty"`
	// RequestID is an optional external identifier.
	RequestID string `json:"request_id,omitempty"`
}
