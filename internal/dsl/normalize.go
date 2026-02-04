package dsl

import "fmt"

func normalizeConfig(cfg *Config) error {
	for i := range cfg.Tools {
		input, err := normalizeSchema(cfg.Tools[i].InputSchema)
		if err != nil {
			return fmt.Errorf("tools[%d].input_schema: %w", i, err)
		}
		cfg.Tools[i].InputSchema = input
		output, err := normalizeSchema(cfg.Tools[i].OutputSchema)
		if err != nil {
			return fmt.Errorf("tools[%d].output_schema: %w", i, err)
		}
		cfg.Tools[i].OutputSchema = output
	}
	return nil
}

func normalizeSchema(schema map[string]any) (map[string]any, error) {
	if schema == nil {
		return nil, nil
	}
	normalized, err := normalizeValue(schema)
	if err != nil {
		return nil, err
	}
	if normalized == nil {
		return nil, nil
	}
	result, ok := normalized.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("schema must be an object")
	}
	return result, nil
}

func normalizeValue(value any) (any, error) {
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, val := range v {
			normalized, err := normalizeValue(val)
			if err != nil {
				return nil, err
			}
			out[key] = normalized
		}
		return out, nil
	case map[any]any:
		out := make(map[string]any, len(v))
		for key, val := range v {
			keyStr, ok := key.(string)
			if !ok {
				return nil, fmt.Errorf("schema key must be string, got %T", key)
			}
			normalized, err := normalizeValue(val)
			if err != nil {
				return nil, err
			}
			out[keyStr] = normalized
		}
		return out, nil
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			normalized, err := normalizeValue(item)
			if err != nil {
				return nil, err
			}
			out[i] = normalized
		}
		return out, nil
	default:
		return value, nil
	}
}
