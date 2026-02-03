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
}
