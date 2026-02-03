package shell

import (
	"context"
	"strings"

	"github.com/codex-k8s/yaml-mcp-server/internal/executil"
	"github.com/codex-k8s/yaml-mcp-server/internal/runtime/approver"
)

// Approver runs a shell command and decides based on exit code.
type Approver struct {
	// Label is a human-friendly name.
	Label string
	// Command is the shell command to execute.
	Command string
	// Args are optional command arguments.
	Args []string
	// Env adds environment variables for the command.
	Env map[string]string
	// AllowExitCodes declares additional success exit codes.
	AllowExitCodes []int
}

// Name returns approver name for audit and logging.
func (a Approver) Name() string {
	if a.Label != "" {
		return a.Label
	}
	return "shell"
}

// Approve executes the shell command and returns an approval decision.
func (a Approver) Approve(ctx context.Context, req approver.Request) (approver.Decision, error) {
	output, exitCode, err := executil.RunCommand(ctx, a.Command, a.Args, a.Env, executil.TemplateData{
		Args:          req.Arguments,
		ToolName:      req.ToolName,
		CorrelationID: req.CorrelationID,
	})

	allowed := err == nil
	if !allowed && len(a.AllowExitCodes) > 0 {
		for _, code := range a.AllowExitCodes {
			if code == exitCode {
				allowed = true
				break
			}
		}
	}

	reason := strings.TrimSpace(output)
	if reason == "" {
		if allowed {
			reason = "approved"
		} else {
			reason = "denied"
		}
	}

	return approver.Decision{Allowed: allowed, Reason: reason, Source: a.Name()}, nil
}
