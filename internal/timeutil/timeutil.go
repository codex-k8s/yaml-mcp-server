package timeutil

import (
	"strings"
	"time"
)

// ParseDurationOrDefault parses duration and returns def on empty or invalid value.
func ParseDurationOrDefault(value string, def time.Duration) time.Duration {
	if strings.TrimSpace(value) == "" {
		return def
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return def
	}
	return parsed
}
