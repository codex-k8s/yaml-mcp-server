package audit

import (
	"context"
	"log/slog"
)

// Event represents an audit entry for tool execution and approvals.
type Event struct {
	// Type describes the event kind.
	Type string
	// Tool is the tool name.
	Tool string
	// CorrelationID links related events.
	CorrelationID string
	// Decision is the approval decision.
	Decision string
	// Reason provides additional context.
	Reason string
}

// Logger records audit events.
type Logger interface {
	// Record stores an audit event.
	Record(ctx context.Context, event Event)
}

// StdLogger writes audit events to slog.
type StdLogger struct {
	logger *slog.Logger
}

// New returns a StdLogger.
func New(logger *slog.Logger) *StdLogger {
	return &StdLogger{logger: logger}
}

// Record logs an audit event.
func (l *StdLogger) Record(_ context.Context, event Event) {
	if l == nil || l.logger == nil {
		return
	}
	l.logger.Info("audit",
		"type", event.Type,
		"tool", event.Tool,
		"correlation_id", event.CorrelationID,
		"decision", event.Decision,
		"reason", event.Reason,
	)
}
