package security

import "strings"

var sensitiveSubstrings = []string{
	"token",
	"password",
	"authorization",
	"apikey",
	"api_key",
	"access_key",
	"private_key",
	"credentials",
	"auth",
	"passwd",
	"key",
	"sig",
	"signature",
	"cookie",
	"session",
	"jwt",
	"bearer",
	"credential",
	"pwd",
	"passphrase",
	"secret_value",
}

var allowList = map[string]struct{}{
	"secret_name": {},
}

// RedactArguments returns a copy of arguments with sensitive values replaced.
func RedactArguments(values map[string]any) map[string]any {
	if values == nil {
		return nil
	}
	redacted := make(map[string]any, len(values))
	for key, value := range values {
		if isSensitiveKey(key) {
			redacted[key] = "***"
			continue
		}
		redacted[key] = value
	}
	return redacted
}

func isSensitiveKey(key string) bool {
	lower := strings.ToLower(strings.TrimSpace(key))
	if _, ok := allowList[lower]; ok {
		return false
	}
	if strings.Contains(lower, "secret") && strings.Contains(lower, "name") {
		return false
	}
	for _, part := range sensitiveSubstrings {
		if strings.Contains(lower, part) {
			return true
		}
	}
	return false
}
