package cursorstorage

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

func containsString(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func extractSessionIDFromPath(path string) string {
	// Extract session ID from path: ~/.cursor/chats/{hash}/{session-id}/store.db
	dir := filepath.Dir(path)
	sessionID := filepath.Base(dir)
	return sessionID
}

// normalizeTimestamp converts a timestamp to milliseconds
// If the timestamp is less than 1e12 (1 trillion), it's assumed to be in seconds and converted to milliseconds
// Otherwise, it's assumed to already be in milliseconds
func normalizeTimestamp(ts int64) int64 {
	// Threshold: timestamps in milliseconds since epoch (2024) are > 1e12
	// Timestamps in seconds since epoch are < 1e12
	if ts < 1e12 {
		// Likely in seconds, convert to milliseconds
		return ts * 1000
	}
	// Already in milliseconds
	return ts
}
// tryBase64Decode attempts to decode a base64 string, returns decoded bytes or error
func tryBase64Decode(s string) ([]byte, error) {
	// Try standard base64
	decoded, err := base64.StdEncoding.DecodeString(s)
	if err == nil {
		return decoded, nil
	}
	// Try URL-safe base64
	decoded, err = base64.URLEncoding.DecodeString(s)
	if err == nil {
		return decoded, nil
	}
	// Try with padding
	if len(s)%4 != 0 {
		padded := s + strings.Repeat("=", 4-len(s)%4)
		decoded, err = base64.StdEncoding.DecodeString(padded)
		if err == nil {
			return decoded, nil
		}
	}
	return nil, fmt.Errorf("not base64 encoded")
}

// tryHexDecode attempts to decode a hex-encoded string, returns decoded bytes or error
func tryHexDecode(s string) ([]byte, error) {
	// Remove whitespace, newlines, and tabs
	cleaned := strings.ReplaceAll(s, " ", "")
	cleaned = strings.ReplaceAll(cleaned, "\n", "")
	cleaned = strings.ReplaceAll(cleaned, "\t", "")
	cleaned = strings.ReplaceAll(cleaned, "\r", "")

	decoded, err := hex.DecodeString(cleaned)
	if err != nil {
		return nil, fmt.Errorf("not hex encoded: %w", err)
	}
	return decoded, nil
}

// extractJSONFromBinary attempts to extract a JSON object from binary data
// Returns the JSON bytes and true if found and valid, or nil and false if not found/invalid
// Validates that the extracted content is actually valid JSON before returning
func extractJSONFromBinary(data []byte) ([]byte, bool) {
	// Look for JSON object start
	startIdx := bytes.Index(data, []byte("{"))
	if startIdx == -1 {
		return nil, false
	}

	// Try to find matching closing brace with proper brace counting
	// Need to handle strings that might contain braces
	depth := 0
	inString := false
	escapeNext := false

	for i := startIdx; i < len(data); i++ {
		if escapeNext {
			escapeNext = false
			continue
		}

		if data[i] == '\\' {
			escapeNext = true
			continue
		}

		if data[i] == '"' && !escapeNext {
			inString = !inString
			continue
		}

		if !inString {
			switch data[i] {
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					// Found complete brace structure - validate it's actually valid JSON
					jsonBytes := data[startIdx : i+1]

					// Quick validation: check if it's valid UTF-8 and has reasonable structure
					if !utf8.Valid(jsonBytes) {
						return nil, false
					}

					// Try to parse as JSON to ensure it's valid
					var testData interface{}
					if err := json.Unmarshal(jsonBytes, &testData); err != nil {
						// Not valid JSON - might be binary data with { } characters
						return nil, false
					}

					// Valid JSON found
					return jsonBytes, true
				}
			}
		}
	}

	return nil, false
}

// isHashLike checks if a string looks like a hash (hex characters, reasonable length)
func isHashLike(s string) bool {
	if len(s) < 16 || len(s) > 128 {
		return false
	}
	for _, r := range s {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') && (r < 'A' || r > 'F') {
			return false
		}
	}
	return true
}
