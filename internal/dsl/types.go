package dsl

// Config is the top-level YAML configuration.
type Config struct {
	// Server describes the MCP server settings.
	Server ServerConfig `yaml:"server"`
	// Tools lists all tool declarations.
	Tools []ToolConfig `yaml:"tools"`
	// Resources lists static resources.
	Resources []ResourceConfig `yaml:"resources"`
}

// ServerConfig defines MCP server settings.
type ServerConfig struct {
	// Name is the MCP server name.
	Name string `yaml:"name"`
	// Version is the MCP server version.
	Version string `yaml:"version"`
	// Transport selects the server transport ("http" or "stdio").
	Transport string `yaml:"transport"`
	// ShutdownTimeout overrides graceful shutdown duration.
	ShutdownTimeout string `yaml:"shutdown_timeout"`
	// Idempotency configures optional response caching.
	Idempotency IdempotencyConfig `yaml:"idempotency_cache"`
	// StartupHooks defines one-time commands executed on start.
	StartupHooks []HookConfig `yaml:"startup_hooks"`
	// HTTP configures HTTP transport.
	HTTP HTTPConfig `yaml:"http"`
	// ApprovalWebhookURL defines the callback URL for async approvers.
	ApprovalWebhookURL string `yaml:"approval_webhook_url"`
}

// HTTPConfig configures the HTTP transport.
type HTTPConfig struct {
	// Listen is the HTTP listen address.
	Listen string `yaml:"listen"`
	// Path is the MCP HTTP endpoint path.
	Path string `yaml:"path"`
	// ReadTimeout limits request read time.
	ReadTimeout string `yaml:"read_timeout"`
	// WriteTimeout limits response write time.
	WriteTimeout string `yaml:"write_timeout"`
	// IdleTimeout controls idle connections.
	IdleTimeout string `yaml:"idle_timeout"`
	// SessionTimeout is reserved for future session retention.
	SessionTimeout string `yaml:"session_timeout"`
	// Stateless disables session tracking.
	Stateless bool `yaml:"stateless"`
}

// ToolConfig declares a tool exposed by the MCP server.
type ToolConfig struct {
	// Name is the tool name.
	Name string `yaml:"name"`
	// Title is the human-friendly tool title.
	Title string `yaml:"title"`
	// Description explains the tool for the agent.
	Description string `yaml:"description"`
	// Annotations provides optional tool hints.
	Annotations *ToolAnnotationsConfig `yaml:"annotations,omitempty"`
	// RequiresApproval forces approval flow even if approvers are empty.
	RequiresApproval bool `yaml:"requires_approval"`
	// Timeout is the tool execution timeout.
	Timeout string `yaml:"timeout"`
	// TimeoutMessage is returned on timeout.
	TimeoutMessage string `yaml:"timeout_message"`
	// InputSchema defines JSON Schema for tool input.
	InputSchema map[string]any `yaml:"input_schema"`
	// OutputSchema defines JSON Schema for tool output.
	OutputSchema map[string]any `yaml:"output_schema"`
	// Executor describes how the tool is executed.
	Executor ExecutorConfig `yaml:"executor"`
	// Approvers lists approval steps to run.
	Approvers []ApproverConfig `yaml:"approvers"`
	// Metadata is an optional opaque map.
	Metadata map[string]any `yaml:"metadata"`
	// Tags is an optional list of tags.
	Tags []string `yaml:"tags"`
}

// ExecutorConfig defines how to execute a tool.
type ExecutorConfig struct {
	// Type selects executor implementation.
	Type string `yaml:"type"`
	// Command is the executable or shell command.
	Command string `yaml:"command"`
	// Args contains command arguments.
	Args []string `yaml:"args"`
	// Env adds environment variables for execution.
	Env map[string]string `yaml:"env"`
	// Timeout is the executor timeout.
	Timeout string `yaml:"timeout"`
}

// HookConfig defines a startup hook command.
type HookConfig struct {
	// Command is the startup command to run.
	Command string `yaml:"command"`
	// Args are optional arguments.
	Args []string `yaml:"args"`
	// Env adds environment variables for the hook.
	Env map[string]string `yaml:"env"`
	// Timeout controls hook execution duration.
	Timeout string `yaml:"timeout"`
}

// ApproverConfig defines a single approver configuration.
type ApproverConfig struct {
	// Type selects approver implementation.
	Type string `yaml:"type"`
	// Name is a human-friendly approver name.
	Name string `yaml:"name"`
	// Timeout limits approver execution time.
	Timeout string `yaml:"timeout"`
	// URL defines HTTP approver endpoint.
	URL string `yaml:"url"`
	// Method overrides HTTP method.
	Method string `yaml:"method"`
	// Headers adds HTTP headers.
	Headers map[string]string `yaml:"headers"`
	// Async enables webhook-based approvals.
	Async bool `yaml:"async"`
	// Markup selects approver message formatting (markdown/html).
	Markup string `yaml:"markup"`
	// WebhookURL overrides the server approval webhook URL.
	WebhookURL string `yaml:"webhook_url"`
	// Command is a shell approver command.
	Command string `yaml:"command"`
	// Args are shell approver arguments.
	Args []string `yaml:"args"`
	// Env adds environment variables for the approver.
	Env map[string]string `yaml:"env"`
	// MaxTotal limits total tool calls.
	MaxTotal int `yaml:"max_total"`
	// RatePerMinute limits requests per minute.
	RatePerMinute int `yaml:"rate_per_minute"`
	// FieldPolicies validates input fields.
	FieldPolicies map[string]FieldPolicy `yaml:"fields"`
	// AllowExitCodes defines allowed shell exit codes.
	AllowExitCodes []int `yaml:"allow_exit_codes"`
	// Payload is reserved for custom approvers.
	Payload map[string]any `yaml:"payload"`
}

// FieldPolicy defines validation rules for tool input fields.
type FieldPolicy struct {
	// Regex validates string value format.
	Regex string `yaml:"regex"`
	// Min sets numeric minimum.
	Min *float64 `yaml:"min"`
	// Max sets numeric maximum.
	Max *float64 `yaml:"max"`
	// MinLength sets string minimum length.
	MinLength *int `yaml:"min_length"`
	// MaxLength sets string maximum length.
	MaxLength *int `yaml:"max_length"`
}

// ResourceConfig declares a static MCP resource.
type ResourceConfig struct {
	// Name is a human-friendly resource name.
	Name string `yaml:"name"`
	// URI is the resource identifier.
	URI string `yaml:"uri"`
	// Description explains the resource.
	Description string `yaml:"description"`
	// MIMEType sets the content type.
	MIMEType string `yaml:"mime_type"`
	// Text is the static resource content.
	Text string `yaml:"text"`
}

// IdempotencyConfig configures response caching for repeated tool calls.
type IdempotencyConfig struct {
	// Enabled toggles idempotency caching.
	Enabled bool `yaml:"enabled"`
	// TTL controls how long cached responses are kept.
	TTL string `yaml:"ttl"`
	// MaxEntries limits the cache size.
	MaxEntries int `yaml:"max_entries"`
	// KeyStrategy selects cache key strategy (correlation_id, arguments_hash, auto).
	KeyStrategy string `yaml:"key_strategy"`
}

// ToolAnnotationsConfig defines tool behavior hints.
type ToolAnnotationsConfig struct {
	// ReadOnlyHint indicates a read-only tool.
	ReadOnlyHint bool `yaml:"read_only_hint,omitempty"`
	// DestructiveHint indicates the tool may be destructive.
	DestructiveHint *bool `yaml:"destructive_hint,omitempty"`
	// IdempotentHint indicates repeated calls have no additional effect.
	IdempotentHint bool `yaml:"idempotent_hint,omitempty"`
	// OpenWorldHint indicates interaction with external entities.
	OpenWorldHint *bool `yaml:"open_world_hint,omitempty"`
	// Title is an optional tool display title.
	Title string `yaml:"title,omitempty"`
}
