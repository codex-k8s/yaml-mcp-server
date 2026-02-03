package executor

import (
	"context"
	"strings"

	"github.com/codex-k8s/yaml-mcp-server/internal/executil"
)

// Shell executes a command as a tool.
type Shell struct {
	// Command is the shell command to execute.
	Command string
	// Args are command arguments.
	Args []string
	// Env adds environment variables.
	Env map[string]string
}

// Execute runs the configured shell command.
func (s Shell) Execute(ctx context.Context, req Request) (string, error) {
	output, _, err := executil.RunCommand(ctx, s.Command, s.Args, s.Env, executil.TemplateData{
		Args:          req.Arguments,
		ToolName:      req.ToolName,
		CorrelationID: req.CorrelationID,
	})
	if err != nil {
		return strings.TrimSpace(output), err
	}
	return strings.TrimSpace(output), nil
}
