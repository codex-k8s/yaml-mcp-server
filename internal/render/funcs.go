package render

import (
	"os"
	"strings"
	"text/template"
)

// FuncMap returns template helpers for YAML rendering.
func FuncMap(tracker *EnvTracker) template.FuncMap {
	return template.FuncMap{
		"env": func(key string) (string, error) {
			if tracker != nil {
				tracker.markUsed(key)
			}
			value, ok := os.LookupEnv(key)
			if !ok {
				if tracker != nil {
					tracker.markMissing(key)
				}
				return "", nil
			}
			return value, nil
		},
		"envOr": func(key, def string) string {
			if tracker != nil {
				tracker.markUsed(key)
			}
			if value, ok := os.LookupEnv(key); ok {
				return value
			}
			return def
		},
		"default": func(def, value string) string {
			if value == "" {
				return def
			}
			return value
		},
		"ternary": func(cond bool, a, b string) string {
			if cond {
				return a
			}
			return b
		},
		"join": func(sep string, items []string) string {
			return strings.Join(items, sep)
		},
		"lower":      strings.ToLower,
		"upper":      strings.ToUpper,
		"trimPrefix": strings.TrimPrefix,
		"trimSuffix": strings.TrimSuffix,
		"replace":    strings.ReplaceAll,
	}
}
