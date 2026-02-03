package executor

import "context"

// Request contains tool execution inputs.
type Request struct {
	// ToolName is the tool being executed.
	ToolName string
	// Arguments are tool arguments.
	Arguments map[string]any
	// CorrelationID links related executions.
	CorrelationID string
	// TimeoutMessage is an optional timeout message.
	TimeoutMessage string
}

// Executor executes a tool command.
type Executor interface {
	// Execute runs the tool logic and returns a message.
	Execute(ctx context.Context, req Request) (string, error)
}
