package render

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"text/template"
)

// EnvTracker tracks referenced environment variables during template rendering.
type EnvTracker struct {
	missing map[string]struct{}
	used    map[string]struct{}
}

func (t *EnvTracker) markUsed(key string) {
	if t.used == nil {
		t.used = map[string]struct{}{}
	}
	t.used[key] = struct{}{}
}

func (t *EnvTracker) markMissing(key string) {
	if t.missing == nil {
		t.missing = map[string]struct{}{}
	}
	t.missing[key] = struct{}{}
}

// Missing returns a list of missing environment variables.
func (t *EnvTracker) Missing() []string {
	out := make([]string, 0, len(t.missing))
	for key := range t.missing {
		out = append(out, key)
	}
	return out
}

// RenderFile loads and renders a YAML template file.
func RenderFile(path string) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	return RenderBytes(path, raw)
}

// RenderBytes renders a YAML template from raw bytes.
func RenderBytes(name string, raw []byte) ([]byte, error) {
	tracker := &EnvTracker{}
	templateName := name
	if strings.TrimSpace(templateName) == "" {
		templateName = "config"
	}
	tmpl, err := template.New(templateName).Funcs(FuncMap(tracker)).Option("missingkey=error").Parse(string(raw))
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, map[string]any{}); err != nil {
		if len(tracker.missing) > 0 {
			return nil, fmt.Errorf("missing env vars: %s", strings.Join(tracker.Missing(), ", "))
		}
		return nil, fmt.Errorf("render template: %w", err)
	}

	if len(tracker.missing) > 0 {
		return nil, fmt.Errorf("missing env vars: %s", strings.Join(tracker.Missing(), ", "))
	}

	return buf.Bytes(), nil
}
