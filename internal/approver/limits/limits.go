package limits

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/codex-k8s/yaml-mcp-server/internal/runtime/approver"
	"github.com/codex-k8s/yaml-mcp-server/internal/templates"
)

// Approver limits tool usage by count, rate, and field policies.
type Approver struct {
	// Name is a human-friendly name.
	Name string
	// MaxTotal limits total tool calls.
	MaxTotal int
	// RatePerMinute limits calls per minute.
	RatePerMinute int
	// FieldPolicies validates input fields.
	FieldPolicies map[string]FieldPolicy
}

// FieldPolicy describes validation rules for a single field.
type FieldPolicy struct {
	// Regex validates string value format.
	Regex string
	// Min sets numeric minimum.
	Min *float64
	// Max sets numeric maximum.
	Max *float64
	// MinLength sets string minimum length.
	MinLength *int
	// MaxLength sets string maximum length.
	MaxLength *int
}

type limiterState struct {
	count   int
	limiter *rate.Limiter
}

// Store keeps per-tool counters and compiled policies.
type Store struct {
	mu       sync.Mutex
	byTool   map[string]*limiterState
	policy   Approver
	compiled map[string]*regexp.Regexp
	renderer templates.Renderer
}

// NewApprover creates a limits approver and validates regex rules.
func NewApprover(name string, maxTotal, ratePerMinute int, policies map[string]FieldPolicy, renderer templates.Renderer) (*Store, error) {
	compiled := make(map[string]*regexp.Regexp, len(policies))
	for field, policy := range policies {
		if policy.Regex == "" {
			continue
		}
		re, err := regexp.Compile(policy.Regex)
		if err != nil {
			return nil, fmt.Errorf("invalid regex for field %s: %w", field, err)
		}
		compiled[field] = re
	}
	return &Store{
		byTool:   make(map[string]*limiterState),
		policy:   Approver{Name: name, MaxTotal: maxTotal, RatePerMinute: ratePerMinute, FieldPolicies: policies},
		compiled: compiled,
		renderer: renderer,
	}, nil
}

// Name returns approver name for audit and logging.
func (s *Store) Name() string {
	if s.policy.Name != "" {
		return s.policy.Name
	}
	return "limits"
}

// Approve validates fields and rate limits the tool usage.
func (s *Store) Approve(_ context.Context, req approver.Request) (approver.Decision, error) {
	if err := s.checkFields(req.Arguments); err != nil {
		return approver.Decision{Allowed: false, Reason: err.Error(), Source: s.Name()}, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	state := s.byTool[req.ToolName]
	if state == nil {
		state = &limiterState{}
		if s.policy.RatePerMinute > 0 {
			state.limiter = rate.NewLimiter(rate.Every(time.Minute/time.Duration(s.policy.RatePerMinute)), s.policy.RatePerMinute)
		}
		s.byTool[req.ToolName] = state
	}

	if s.policy.MaxTotal > 0 && state.count >= s.policy.MaxTotal {
		return approver.Decision{Allowed: false, Reason: s.render("limits.max_total", nil, "Maximum number of calls exceeded"), Source: s.Name()}, nil
	}
	if state.limiter != nil && !state.limiter.Allow() {
		return approver.Decision{Allowed: false, Reason: s.render("limits.rate_limit", nil, "Rate limit exceeded"), Source: s.Name()}, nil
	}

	state.count++
	return approver.Decision{Allowed: true, Reason: "approved", Source: s.Name()}, nil
}

func (s *Store) checkFields(args map[string]any) error {
	for field, policy := range s.policy.FieldPolicies {
		value, ok := args[field]
		if !ok {
			continue
		}

		switch v := value.(type) {
		case string:
			if err := s.checkStringLength(field, v, policy); err != nil {
				return err
			}
			re := s.compiled[field]
			if re != nil && !re.MatchString(v) {
				return errors.New(s.render("limits.field_regex", map[string]any{"Field": field}, "Field "+field+" does not match required format"))
			}
		case float64:
			if err := s.checkNumericRange(field, v, policy); err != nil {
				return err
			}
		case int:
			if err := s.checkNumericRange(field, float64(v), policy); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Store) checkStringLength(field, value string, policy FieldPolicy) error {
	return s.checkMinMax(
		field,
		float64(len(value)),
		toFloat64(policy.MinLength),
		toFloat64(policy.MaxLength),
		boundarySpec{Key: "limits.field_min_length", DataKey: "MinLength", FallbackTemplate: "Field %s is too short"},
		boundarySpec{Key: "limits.field_max_length", DataKey: "MaxLength", FallbackTemplate: "Field %s is too long"},
	)
}

func (s *Store) checkNumericRange(field string, value float64, policy FieldPolicy) error {
	return s.checkMinMax(
		field,
		value,
		policy.Min,
		policy.Max,
		boundarySpec{Key: "limits.field_min", DataKey: "Min", FallbackTemplate: "Field %s is below minimum value"},
		boundarySpec{Key: "limits.field_max", DataKey: "Max", FallbackTemplate: "Field %s is above maximum value"},
	)
}

type boundarySpec struct {
	Key              string
	DataKey          string
	FallbackTemplate string
}

func (s *Store) checkMinMax(field string, value float64, min, max *float64, minSpec, maxSpec boundarySpec) error {
	checks := []struct {
		limit    *float64
		violated bool
		spec     boundarySpec
	}{
		{limit: min, violated: min != nil && value < *min, spec: minSpec},
		{limit: max, violated: max != nil && value > *max, spec: maxSpec},
	}
	for _, check := range checks {
		if check.limit == nil {
			continue
		}
		if err := s.renderBoundary(
			check.violated,
			check.spec.Key,
			map[string]any{"Field": field, check.spec.DataKey: *check.limit},
			fmt.Sprintf(check.spec.FallbackTemplate, field),
		); err != nil {
			return err
		}
	}
	return nil
}

func toFloat64(value *int) *float64 {
	if value == nil {
		return nil
	}
	out := float64(*value)
	return &out
}

func (s *Store) renderBoundary(violated bool, key string, data map[string]any, fallback string) error {
	if !violated {
		return nil
	}
	return errors.New(s.render(key, data, fallback))
}

func (s *Store) render(key string, data map[string]any, fallback string) string {
	if s.renderer == nil {
		return fallback
	}
	rendered, err := s.renderer.Render(key, data)
	if err != nil {
		return fallback
	}
	return rendered
}
