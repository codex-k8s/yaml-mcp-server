package constants

// Executor type aliases.
const (
	ExecutorShell = "shell"
	ExecutorHTTP  = "http"
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
