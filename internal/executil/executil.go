package executil

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"text/template"
)

// TemplateData defines the available fields in command templates.
type TemplateData struct {
	// Args are tool arguments.
	Args map[string]any
	// ToolName is the tool name.
	ToolName string
	// CorrelationID links related operations.
	CorrelationID string
}

// RenderTemplate renders a string template with TemplateData.
func RenderTemplate(value string, data TemplateData) (string, error) {
	tmpl, err := template.New("value").Funcs(template.FuncMap{
		"arg": func(name string) any {
			if data.Args == nil {
				return nil
			}
			return data.Args[name]
		},
	}).Parse(value)
	if err != nil {
		return "", fmt.Errorf("template parse: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("template render: %w", err)
	}
	return buf.String(), nil
}

// BuildCommand builds an exec.Cmd with rendered command, args and env.
func BuildCommand(ctx context.Context, command string, args []string, env map[string]string, data TemplateData) (*exec.Cmd, error) {
	renderedCommand, err := RenderTemplate(command, data)
	if err != nil {
		return nil, err
	}

	renderedArgs := make([]string, 0, len(args))
	for _, arg := range args {
		rendered, err := RenderTemplate(arg, data)
		if err != nil {
			return nil, err
		}
		renderedArgs = append(renderedArgs, rendered)
	}

	var cmd *exec.Cmd
	if len(renderedArgs) == 0 {
		cmd = exec.CommandContext(ctx, "bash", "-c", renderedCommand)
	} else {
		cmd = exec.CommandContext(ctx, renderedCommand, renderedArgs...)
	}

	cmd.Env = os.Environ()
	for key, value := range env {
		rendered, err := RenderTemplate(value, data)
		if err != nil {
			return nil, err
		}
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", key, rendered))
	}

	return cmd, nil
}

// RunCommand executes a command and returns output, exit code, and error.
func RunCommand(ctx context.Context, command string, args []string, env map[string]string, data TemplateData) (string, int, error) {
	cmd, err := BuildCommand(ctx, command, args, env, data)
	if err != nil {
		return "", -1, err
	}

	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	err = cmd.Run()
	exitCode := -1
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}
	return output.String(), exitCode, err
}
