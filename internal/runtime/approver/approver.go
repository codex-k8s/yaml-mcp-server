package approver

import "context"

// Request defines the input sent to approvers.
type Request struct {
	// ToolName is the tool being approved.
	ToolName string
	// Arguments are tool arguments.
	Arguments map[string]any
	// CorrelationID links related approvals.
	CorrelationID string
}

// Decision represents the approver decision.
type Decision struct {
	// Allowed indicates approval result.
	Allowed bool
	// Reason explains the decision.
	Reason string
	// Source identifies the approver.
	Source string
}

// Approver checks whether an action is allowed.
type Approver interface {
	// Name returns the approver identifier.
	Name() string
	// Approve returns a decision for the given request.
	Approve(ctx context.Context, req Request) (Decision, error)
}

// Chain runs approvers sequentially until a decision is made.
type Chain struct {
	// Approvers is the ordered list to execute.
	Approvers []Approver
}

// Approve executes all approvers in order.
func (c Chain) Approve(ctx context.Context, req Request) (Decision, error) {
	for _, item := range c.Approvers {
		decision, err := item.Approve(ctx, req)
		if err != nil {
			return Decision{Allowed: false, Reason: err.Error(), Source: item.Name()}, err
		}
		if !decision.Allowed {
			if decision.Source == "" {
				decision.Source = item.Name()
			}
			return decision, nil
		}
	}
	return Decision{Allowed: true, Reason: "approved"}, nil
}
