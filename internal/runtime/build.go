package runtime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	approverhttp "github.com/codex-k8s/yaml-mcp-server/internal/approver/http"
	"github.com/codex-k8s/yaml-mcp-server/internal/approver/limits"
	"github.com/codex-k8s/yaml-mcp-server/internal/approver/shell"
	"github.com/codex-k8s/yaml-mcp-server/internal/audit"
	"github.com/codex-k8s/yaml-mcp-server/internal/constants"
	"github.com/codex-k8s/yaml-mcp-server/internal/dsl"
	"github.com/codex-k8s/yaml-mcp-server/internal/idempotency"
	"github.com/codex-k8s/yaml-mcp-server/internal/protocol"
	"github.com/codex-k8s/yaml-mcp-server/internal/runtime/approver"
	"github.com/codex-k8s/yaml-mcp-server/internal/runtime/executor"
	"github.com/codex-k8s/yaml-mcp-server/internal/templates"
	"github.com/codex-k8s/yaml-mcp-server/internal/timeutil"
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
	// Lang selects approver language.
	Lang string
	// ApprovalWebhookURL is the callback URL for async approvers.
	ApprovalWebhookURL string
	// ExecutorWebhookURL is the callback URL for async executors.
	ExecutorWebhookURL string
	// HTTPApprovals stores pending async approvals.
	HTTPApprovals *approverhttp.PendingStore
	// HTTPExecutions stores pending async executions.
	HTTPExecutions *executor.PendingStore
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
	exec, err := buildExecutor(tool, b)
	if err != nil {
		return fmt.Errorf("tool %s: %w", tool.Name, err)
	}

	chain, err := buildApprovers(tool.Approvers, b.Templates, b)
	if err != nil {
		return fmt.Errorf("tool %s: %w", tool.Name, err)
	}

	timeout := timeutil.ParseDurationOrDefault(tool.Timeout, 0)
	if timeout == 0 {
		timeout = timeutil.ParseDurationOrDefault(tool.Executor.Timeout, 0)
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
		if b.Logger != nil {
			b.Logger.Info("tool call", "tool", tool.Name, "correlation_id", correlationID, "args", args)
		}
		b.recordAudit(ctx, "tool_call", tool.Name, correlationID, "", "")

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
				b.recordAudit(ctx, "cache_hit", tool.Name, correlationID, cached.Decision, cached.Reason)
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
				if applyTimeoutResponse(ctxTool, &resp, tool.TimeoutMessage, format) {
					return nil, resp, nil
				}
				resp.Status = protocol.StatusError
				resp.Decision = protocol.DecisionError
				resp.Reason = err.Error()
				b.recordAudit(ctx, "approval_error", tool.Name, correlationID, protocol.DecisionError, err.Error())
				applyResponseFormat(format, &resp)
				return nil, resp, nil
			}
			if applyTimeoutResponse(ctxTool, &resp, tool.TimeoutMessage, format) {
				return nil, resp, nil
			}
			if !decision.Allowed {
				resp.Status = protocol.StatusDenied
				resp.Decision = protocol.DecisionDeny
				resp.Reason = decision.Reason
				b.recordAudit(ctx, "approval_denied", tool.Name, correlationID, protocol.DecisionDeny, decision.Reason)
				applyResponseFormat(format, &resp)
				return nil, resp, nil
			}
			b.recordAudit(ctx, "approval_ok", tool.Name, correlationID, protocol.DecisionApprove, decision.Reason)
		}

		output, err := exec.Execute(ctxTool, executor.Request{
			ToolName:      tool.Name,
			Arguments:     args,
			CorrelationID: correlationID,
		})
		if err != nil {
			if applyTimeoutResponse(ctxTool, &resp, tool.TimeoutMessage, format) {
				return nil, resp, nil
			}
			resp.Status = protocol.StatusError
			resp.Decision = protocol.DecisionError
			resp.Reason = err.Error()
			if output != "" {
				resp.Reason = fmt.Sprintf("%s: %s", resp.Reason, output)
			}
			b.recordAudit(ctx, "tool_error", tool.Name, correlationID, protocol.DecisionError, resp.Reason)
			applyResponseFormat(format, &resp)
			return nil, resp, nil
		}

		if applyTimeoutResponse(ctxTool, &resp, tool.TimeoutMessage, format) {
			return nil, resp, nil
		}

		resp.Reason = output
		applyResponseFormat(format, &resp)
		b.recordAudit(ctx, "tool_ok", tool.Name, correlationID, protocol.DecisionApprove, output)
		if b.Cache != nil && cacheKey != "" && resp.Status != protocol.StatusError {
			b.Cache.Set(cacheKey, resp)
			if b.Logger != nil {
				b.Logger.Info("tool response cached", "tool", tool.Name, "correlation_id", correlationID)
			}
			b.recordAudit(ctx, "cache_store", tool.Name, correlationID, resp.Decision, resp.Reason)
		}
		return nil, resp, nil
	})

	return nil
}

func buildExecutor(tool dsl.ToolConfig, builder Builder) (executor.Executor, error) {
	cfg := tool.Executor
	switch strings.ToLower(strings.TrimSpace(cfg.Type)) {
	case constants.ExecutorShell:
		return executor.Shell{
			Command: cfg.Command,
			Args:    cfg.Args,
			Env:     cfg.Env,
		}, nil
	case constants.ExecutorHTTP:
		webhookURL := strings.TrimSpace(cfg.WebhookURL)
		if webhookURL == "" {
			webhookURL = strings.TrimSpace(builder.ExecutorWebhookURL)
		}
		return executor.HTTP{
			URL:        cfg.URL,
			Method:     cfg.Method,
			Headers:    cfg.Headers,
			Timeout:    timeutil.ParseDurationOrDefault(cfg.Timeout, 10*time.Second),
			Async:      cfg.Async,
			WebhookURL: webhookURL,
			Pending:    builder.HTTPExecutions,
			Spec:       cfg.Spec,
			Tool: protocol.ExecutorTool{
				Name:        tool.Name,
				Title:       tool.Title,
				Description: tool.Description,
				InputSchema: tool.InputSchema,
				OutputSchema: func() map[string]any {
					if len(tool.OutputSchema) == 0 {
						return nil
					}
					return tool.OutputSchema
				}(),
				Metadata: tool.Metadata,
				Tags:     tool.Tags,
			},
			Lang:   builder.Lang,
			Markup: "markdown",
		}, nil
	default:
		return nil, fmt.Errorf("unknown executor type: %s", cfg.Type)
	}
}

func buildApprovers(configs []dsl.ApproverConfig, renderer templates.Renderer, builder Builder) (approver.Chain, error) {
	if len(configs) == 0 {
		return approver.Chain{}, nil
	}

	var items []approver.Approver
	for _, cfg := range configs {
		timeout := timeutil.ParseDurationOrDefault(cfg.Timeout, 0)
		switch cfg.Type {
		case constants.ApproverHTTP:
			webhookURL := strings.TrimSpace(cfg.WebhookURL)
			if webhookURL == "" {
				webhookURL = strings.TrimSpace(builder.ApprovalWebhookURL)
			}
			markup := strings.TrimSpace(cfg.Markup)
			if markup == "" {
				markup = "markdown"
			}
			client := approverhttp.Client{
				Label:      cfg.Name,
				URL:        cfg.URL,
				Method:     cfg.Method,
				Headers:    cfg.Headers,
				Timeout:    timeutil.ParseDurationOrDefault(cfg.Timeout, 10*time.Second),
				Async:      cfg.Async,
				Lang:       builder.Lang,
				Markup:     markup,
				Pending:    builder.HTTPApprovals,
				WebhookURL: webhookURL,
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

func timeoutMessage(value string) string {
	if strings.TrimSpace(value) == "" {
		return "timeout"
	}
	return value
}

func (b Builder) recordAudit(ctx context.Context, eventType, tool, correlationID, decision, reason string) {
	if b.Audit == nil {
		return
	}
	b.Audit.Record(ctx, audit.Event{
		Type:          eventType,
		Tool:          tool,
		CorrelationID: correlationID,
		Decision:      decision,
		Reason:        reason,
	})
}

func applyTimeoutResponse(ctx context.Context, resp *protocol.ToolResponse, timeoutMsg, format string) bool {
	if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return false
	}
	resp.Status = protocol.StatusError
	resp.Decision = protocol.DecisionError
	resp.Reason = timeoutMessage(timeoutMsg)
	applyResponseFormat(format, resp)
	return true
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
