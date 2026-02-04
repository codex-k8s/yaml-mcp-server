package runtime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/codex-k8s/yaml-mcp-server/internal/approver/http"
	"github.com/codex-k8s/yaml-mcp-server/internal/approver/limits"
	"github.com/codex-k8s/yaml-mcp-server/internal/approver/shell"
	"github.com/codex-k8s/yaml-mcp-server/internal/audit"
	"github.com/codex-k8s/yaml-mcp-server/internal/constants"
	"github.com/codex-k8s/yaml-mcp-server/internal/dsl"
	"github.com/codex-k8s/yaml-mcp-server/internal/idempotency"
	"github.com/codex-k8s/yaml-mcp-server/internal/protocol"
	"github.com/codex-k8s/yaml-mcp-server/internal/runtime/approver"
	"github.com/codex-k8s/yaml-mcp-server/internal/runtime/executor"
	"github.com/codex-k8s/yaml-mcp-server/internal/security"
	"github.com/codex-k8s/yaml-mcp-server/internal/templates"
)

// Builder constructs an MCP server from the DSL config.
type Builder struct {
	// Logger is used for structured logging.
	Logger *slog.Logger
	// Audit records approval and tool events.
	Audit audit.Logger
	// Templates provides localized messages.
	Templates templates.Renderer
	// Cache stores idempotent responses.
	Cache *idempotency.Cache
	// CacheKeyStrategy selects how cache keys are computed.
	CacheKeyStrategy string
}

// Build creates an MCP server with tools and resources.
func (b Builder) Build(cfg *dsl.Config) (*mcp.Server, error) {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    cfg.Server.Name,
		Version: cfg.Server.Version,
	}, nil)

	for _, res := range cfg.Resources {
		resource := res
		server.AddResource(&mcp.Resource{
			Name:        resource.Name,
			URI:         resource.URI,
			Description: resource.Description,
			MIMEType:    resource.MIMEType,
		}, func(_ context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			return &mcp.ReadResourceResult{
				Contents: []*mcp.ResourceContents{
					{Text: resource.Text},
				},
			}, nil
		})
	}

	for _, tool := range cfg.Tools {
		if err := b.addTool(server, tool); err != nil {
			return nil, err
		}
	}

	return server, nil
}

func (b Builder) addTool(server *mcp.Server, tool dsl.ToolConfig) error {
	exec, err := buildExecutor(tool.Executor)
	if err != nil {
		return fmt.Errorf("tool %s: %w", tool.Name, err)
	}

	chain, err := buildApprovers(tool.Approvers, b.Templates)
	if err != nil {
		return fmt.Errorf("tool %s: %w", tool.Name, err)
	}

	timeout := parseDuration(tool.Timeout, 0)
	if timeout == 0 {
		timeout = parseDuration(tool.Executor.Timeout, 0)
	}

	mcpTool := &mcp.Tool{
		Name:        tool.Name,
		Title:       tool.Title,
		Description: tool.Description,
		InputSchema: tool.InputSchema,
		OutputSchema: func() any {
			if len(tool.OutputSchema) == 0 {
				return nil
			}
			return tool.OutputSchema
		}(),
		Annotations: buildAnnotations(tool.Annotations),
	}

	mcp.AddTool(server, mcpTool, func(ctx context.Context, _ *mcp.CallToolRequest, input map[string]any) (*mcp.CallToolResult, protocol.ToolResponse, error) {
		correlationID, providedID := correlationID(input)
		args := input
		format := responseFormat(args)
		redacted := security.RedactArguments(args)

		if b.Logger != nil {
			b.Logger.Info("tool call", "tool", tool.Name, "correlation_id", correlationID, "args", redacted)
		}
		if b.Audit != nil {
			b.Audit.Record(ctx, audit.Event{Type: "tool_call", Tool: tool.Name, CorrelationID: correlationID})
		}

		cacheKey := ""
		if b.Cache != nil {
			key, err := buildCacheKey(tool.Name, correlationID, providedID, args, b.CacheKeyStrategy)
			if err != nil {
				if b.Logger != nil {
					b.Logger.Warn("cache key build failed", "tool", tool.Name, "error", err)
				}
			} else {
				cacheKey = key
			}
		}
		if b.Cache != nil && cacheKey != "" {
			if cached, ok := b.Cache.Get(cacheKey); ok {
				cached.CorrelationID = correlationID
				if b.Logger != nil {
					b.Logger.Info("tool cache hit", "tool", tool.Name, "correlation_id", correlationID)
				}
				if b.Audit != nil {
					b.Audit.Record(ctx, audit.Event{Type: "cache_hit", Tool: tool.Name, CorrelationID: correlationID, Decision: cached.Decision, Reason: cached.Reason})
				}
				return nil, cached, nil
			}
		}

		ctxTool := ctx
		var cancel context.CancelFunc
		if timeout > 0 {
			ctxTool, cancel = context.WithTimeout(ctx, timeout)
			defer cancel()
		}

		resp := protocol.ToolResponse{
			Status:        protocol.StatusSuccess,
			Decision:      protocol.DecisionApprove,
			Reason:        "",
			CorrelationID: correlationID,
		}

		if tool.RequiresApproval || len(chain.Approvers) > 0 {
			if len(chain.Approvers) == 0 {
				resp.Status = protocol.StatusDenied
				resp.Decision = protocol.DecisionDeny
				resp.Reason = "approval required but no approvers configured"
				applyResponseFormat(format, &resp)
				return nil, resp, nil
			}
			decision, err := chain.Approve(ctxTool, approver.Request{
				ToolName:      tool.Name,
				Arguments:     args,
				CorrelationID: correlationID,
			})
			if err != nil {
				if errors.Is(ctxTool.Err(), context.DeadlineExceeded) {
					resp.Status = protocol.StatusError
					resp.Decision = protocol.DecisionError
					resp.Reason = timeoutMessage(tool.TimeoutMessage)
					applyResponseFormat(format, &resp)
					return nil, resp, nil
				}
				resp.Status = protocol.StatusError
				resp.Decision = protocol.DecisionError
				resp.Reason = err.Error()
				if b.Audit != nil {
					b.Audit.Record(ctx, audit.Event{Type: "approval_error", Tool: tool.Name, CorrelationID: correlationID, Decision: protocol.DecisionError, Reason: err.Error()})
				}
				applyResponseFormat(format, &resp)
				return nil, resp, nil
			}
			if errors.Is(ctxTool.Err(), context.DeadlineExceeded) {
				resp.Status = protocol.StatusError
				resp.Decision = protocol.DecisionError
				resp.Reason = timeoutMessage(tool.TimeoutMessage)
				applyResponseFormat(format, &resp)
				return nil, resp, nil
			}
			if !decision.Allowed {
				resp.Status = protocol.StatusDenied
				resp.Decision = protocol.DecisionDeny
				resp.Reason = decision.Reason
				if b.Audit != nil {
					b.Audit.Record(ctx, audit.Event{Type: "approval_denied", Tool: tool.Name, CorrelationID: correlationID, Decision: protocol.DecisionDeny, Reason: decision.Reason})
				}
				applyResponseFormat(format, &resp)
				return nil, resp, nil
			}
			if b.Audit != nil {
				b.Audit.Record(ctx, audit.Event{Type: "approval_ok", Tool: tool.Name, CorrelationID: correlationID, Decision: protocol.DecisionApprove, Reason: decision.Reason})
			}
		}

		output, err := exec.Execute(ctxTool, executor.Request{
			ToolName:      tool.Name,
			Arguments:     args,
			CorrelationID: correlationID,
		})
		if err != nil {
			if errors.Is(ctxTool.Err(), context.DeadlineExceeded) {
				resp.Status = protocol.StatusError
				resp.Decision = protocol.DecisionError
				resp.Reason = timeoutMessage(tool.TimeoutMessage)
				applyResponseFormat(format, &resp)
				return nil, resp, nil
			}
			resp.Status = protocol.StatusError
			resp.Decision = protocol.DecisionError
			resp.Reason = err.Error()
			if output != "" {
				resp.Reason = fmt.Sprintf("%s: %s", resp.Reason, output)
			}
			if b.Audit != nil {
				b.Audit.Record(ctx, audit.Event{Type: "tool_error", Tool: tool.Name, CorrelationID: correlationID, Decision: protocol.DecisionError, Reason: resp.Reason})
			}
			applyResponseFormat(format, &resp)
			return nil, resp, nil
		}

		if errors.Is(ctxTool.Err(), context.DeadlineExceeded) {
			resp.Status = protocol.StatusError
			resp.Decision = protocol.DecisionError
			resp.Reason = timeoutMessage(tool.TimeoutMessage)
			applyResponseFormat(format, &resp)
			return nil, resp, nil
		}

		resp.Reason = output
		applyResponseFormat(format, &resp)
		if b.Audit != nil {
			b.Audit.Record(ctx, audit.Event{Type: "tool_ok", Tool: tool.Name, CorrelationID: correlationID, Decision: protocol.DecisionApprove, Reason: output})
		}
		if b.Cache != nil && cacheKey != "" && resp.Status != protocol.StatusError {
			b.Cache.Set(cacheKey, resp)
			if b.Logger != nil {
				b.Logger.Info("tool response cached", "tool", tool.Name, "correlation_id", correlationID)
			}
			if b.Audit != nil {
				b.Audit.Record(ctx, audit.Event{Type: "cache_store", Tool: tool.Name, CorrelationID: correlationID, Decision: resp.Decision, Reason: resp.Reason})
			}
		}
		return nil, resp, nil
	})

	return nil
}

func buildExecutor(cfg dsl.ExecutorConfig) (executor.Executor, error) {
	switch cfg.Type {
	case constants.ExecutorShell:
		return executor.Shell{
			Command: cfg.Command,
			Args:    cfg.Args,
			Env:     cfg.Env,
		}, nil
	default:
		return nil, fmt.Errorf("unknown executor type: %s", cfg.Type)
	}
}

func buildApprovers(configs []dsl.ApproverConfig, renderer templates.Renderer) (approver.Chain, error) {
	if len(configs) == 0 {
		return approver.Chain{}, nil
	}

	var items []approver.Approver
	for _, cfg := range configs {
		timeout := parseDuration(cfg.Timeout, 0)
		switch cfg.Type {
		case constants.ApproverHTTP:
			client := http.Client{
				Label:   cfg.Name,
				URL:     cfg.URL,
				Method:  cfg.Method,
				Headers: cfg.Headers,
				Timeout: parseDuration(cfg.Timeout, 10*time.Second),
			}
			items = append(items, wrapTimeout(client, timeout))
		case constants.ApproverShell:
			approverItem := shell.Approver{
				Label:          cfg.Name,
				Command:        cfg.Command,
				Args:           cfg.Args,
				Env:            cfg.Env,
				AllowExitCodes: cfg.AllowExitCodes,
			}
			items = append(items, wrapTimeout(approverItem, timeout))
		case constants.ApproverLimits:
			approverItem, err := limits.NewApprover(cfg.Name, cfg.MaxTotal, cfg.RatePerMinute, toFieldPolicies(cfg.FieldPolicies), renderer)
			if err != nil {
				return approver.Chain{}, err
			}
			items = append(items, wrapTimeout(approverItem, timeout))
		default:
			return approver.Chain{}, fmt.Errorf("unknown approver type: %s", cfg.Type)
		}
	}
	return approver.Chain{Approvers: items}, nil
}

func wrapTimeout(item approver.Approver, timeout time.Duration) approver.Approver {
	if timeout <= 0 {
		return item
	}
	return approver.Timeout{Inner: item, Timeout: timeout}
}

func toFieldPolicies(policies map[string]dsl.FieldPolicy) map[string]limits.FieldPolicy {
	if policies == nil {
		return nil
	}
	out := make(map[string]limits.FieldPolicy, len(policies))
	for key, value := range policies {
		out[key] = limits.FieldPolicy{
			Regex:     value.Regex,
			Min:       value.Min,
			Max:       value.Max,
			MinLength: value.MinLength,
			MaxLength: value.MaxLength,
		}
	}
	return out
}

func parseDuration(value string, def time.Duration) time.Duration {
	if strings.TrimSpace(value) == "" {
		return def
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return def
	}
	return parsed
}

func timeoutMessage(value string) string {
	if strings.TrimSpace(value) == "" {
		return "timeout"
	}
	return value
}

func buildAnnotations(cfg *dsl.ToolAnnotationsConfig) *mcp.ToolAnnotations {
	if cfg == nil {
		return nil
	}
	return &mcp.ToolAnnotations{
		ReadOnlyHint:    cfg.ReadOnlyHint,
		DestructiveHint: cfg.DestructiveHint,
		IdempotentHint:  cfg.IdempotentHint,
		OpenWorldHint:   cfg.OpenWorldHint,
		Title:           cfg.Title,
	}
}

func correlationID(args map[string]any) (string, bool) {
	if args != nil {
		if raw, ok := args["correlation_id"].(string); ok && raw != "" {
			return raw, true
		}
		if raw, ok := args["request_id"].(string); ok && raw != "" {
			return raw, true
		}
	}
	return newCorrelationID(), false
}

func responseFormat(args map[string]any) string {
	if args == nil {
		return ""
	}
	raw, ok := args["response_format"]
	if !ok {
		return ""
	}
	switch v := raw.(type) {
	case string:
		return strings.ToLower(strings.TrimSpace(v))
	default:
		return strings.ToLower(strings.TrimSpace(fmt.Sprint(v)))
	}
}

func applyResponseFormat(format string, resp *protocol.ToolResponse) {
	if resp == nil {
		return
	}
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", "json":
		return
	case "markdown":
		message := strings.TrimSpace(resp.Reason)
		if message == "" {
			message = "no details"
		}
		resp.Reason = fmt.Sprintf("**status**: %s\n**decision**: %s\n\n%s", resp.Status, resp.Decision, message)
	default:
		return
	}
}

func newCorrelationID() string {
	now := time.Now().UTC().UnixNano()
	return fmt.Sprintf("corr-%d", now)
}
