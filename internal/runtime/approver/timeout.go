package approver

import (
	"context"
	"errors"
	"time"
)

// Timeout wraps an approver with a context deadline.
type Timeout struct {
	// Inner is the wrapped approver.
	Inner Approver
	// Timeout is the maximum duration for approval.
	Timeout time.Duration
}

// Name returns the inner approver name.
func (t Timeout) Name() string {
	if t.Inner != nil {
		return t.Inner.Name()
	}
	return "timeout"
}

// Approve executes the inner approver with timeout.
func (t Timeout) Approve(ctx context.Context, req Request) (Decision, error) {
	if t.Inner == nil || t.Timeout <= 0 {
		return Decision{Allowed: false, Reason: "invalid timeout approver", Source: t.Name()}, nil
	}
	ctxTimeout, cancel := context.WithTimeout(ctx, t.Timeout)
	defer cancel()
	decision, err := t.Inner.Approve(ctxTimeout, req)
	if err != nil {
		if errors.Is(ctxTimeout.Err(), context.DeadlineExceeded) {
			return Decision{Allowed: false, Reason: "approval timeout", Source: t.Name()}, nil
		}
		return decision, err
	}
	if errors.Is(ctxTimeout.Err(), context.DeadlineExceeded) {
		return Decision{Allowed: false, Reason: "approval timeout", Source: t.Name()}, nil
	}
	return decision, nil
}
