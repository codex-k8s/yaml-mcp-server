package constants

// Executor type aliases.
const (
	ExecutorShell = "shell"
)

// Approver type aliases.
const (
	ApproverHTTP   = "http"
	ApproverShell  = "shell"
	ApproverLimits = "limits"
)

// Idempotency cache key strategies.
const (
	CacheKeyStrategyAuto          = "auto"
	CacheKeyStrategyCorrelationID = "correlation_id"
	CacheKeyStrategyArgumentsHash = "arguments_hash"
)
