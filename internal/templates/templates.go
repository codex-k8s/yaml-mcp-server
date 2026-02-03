package templates

import (
	"embed"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"
)

//go:embed data/*.json
var files embed.FS

// Renderer renders localized messages by key.
type Renderer interface {
	// Render returns a localized message by key.
	Render(key string, data any) (string, error)
}

// Bundle holds parsed templates for a selected language.
type Bundle struct {
	lang      string
	templates map[string]*template.Template
}

// Load loads localized templates for the specified language (default: en).
func Load(lang string) (*Bundle, error) {
	if strings.TrimSpace(lang) == "" {
		lang = "en"
	}
	lang = strings.ToLower(lang)

	if lang != "ru" && lang != "en" {
		lang = "en"
	}

	path := fmt.Sprintf("data/%s.json", lang)
	raw, err := files.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read templates: %w", err)
	}

	var messages map[string]string
	if err := json.Unmarshal(raw, &messages); err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}

	parsed := make(map[string]*template.Template, len(messages))
	for key, value := range messages {
		tmpl, err := template.New(key).Parse(value)
		if err != nil {
			return nil, fmt.Errorf("parse template %s: %w", key, err)
		}
		parsed[key] = tmpl
	}

	return &Bundle{lang: lang, templates: parsed}, nil
}

// Render renders a message by key with the supplied data.
func (b *Bundle) Render(key string, data any) (string, error) {
	if b == nil {
		return "", fmt.Errorf("templates bundle is nil")
	}
	tmpl, ok := b.templates[key]
	if !ok {
		return "", fmt.Errorf("template not found: %s", key)
	}
	var out strings.Builder
	if err := tmpl.Execute(&out, data); err != nil {
		return "", fmt.Errorf("render template %s: %w", key, err)
	}
	return out.String(), nil
}
