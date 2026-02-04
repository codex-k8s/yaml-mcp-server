package runtime

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

func buildCacheKey(toolName, correlationID string, providedID bool, args map[string]any, strategy string) (string, error) {
	keyStrategy := strings.ToLower(strings.TrimSpace(strategy))
	if keyStrategy == "" {
		keyStrategy = "auto"
	}

	var key string
	switch keyStrategy {
	case "correlation_id":
		key = correlationID
	case "arguments_hash":
		hash, err := hashArguments(args)
		if err != nil {
			return "", err
		}
		key = hash
	case "auto":
		if providedID && correlationID != "" {
			key = correlationID
		} else {
			hash, err := hashArguments(args)
			if err != nil {
				return "", err
			}
			key = hash
		}
	default:
		return "", fmt.Errorf("unsupported cache key strategy: %s", strategy)
	}
	if strings.TrimSpace(key) == "" {
		return "", nil
	}
	return fmt.Sprintf("%s:%s", toolName, key), nil
}

func hashArguments(args map[string]any) (string, error) {
	filtered := make(map[string]any)
	for k, v := range args {
		switch k {
		case "correlation_id", "request_id":
			continue
		default:
			filtered[k] = v
		}
	}
	data, err := canonicalJSON(filtered)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func canonicalJSON(value any) ([]byte, error) {
	switch v := value.(type) {
	case nil:
		return []byte("null"), nil
	case string:
		return []byte(strconv.Quote(v)), nil
	case json.Number:
		return []byte(v.String()), nil
	case bool, float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return json.Marshal(v)
	case []any:
		var buf bytes.Buffer
		buf.WriteByte('[')
		for i, item := range v {
			if i > 0 {
				buf.WriteByte(',')
			}
			data, err := canonicalJSON(item)
			if err != nil {
				return nil, err
			}
			buf.Write(data)
		}
		buf.WriteByte(']')
		return buf.Bytes(), nil
	case map[string]any:
		return canonicalMapJSON(v)
	case map[any]any:
		converted := make(map[string]any, len(v))
		for key, item := range v {
			converted[fmt.Sprint(key)] = item
		}
		return canonicalMapJSON(converted)
	default:
		return json.Marshal(v)
	}
}

func canonicalMapJSON(value map[string]any) ([]byte, error) {
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, key := range keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		buf.WriteString(strconv.Quote(key))
		buf.WriteByte(':')
		data, err := canonicalJSON(value[key])
		if err != nil {
			return nil, err
		}
		buf.Write(data)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}
