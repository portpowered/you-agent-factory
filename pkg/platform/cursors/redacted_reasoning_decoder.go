package cursors

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

// DecodeRedactedReasoning attempts to decode a redacted reasoning string
// It tries multiple decoding strategies:
// 1. Base64URL decode + protobuf decode
// 2. Base64URL decode + extract readable strings
// 3. Return original if decoding fails
// pkgmaintcheck:ignore-cyclomatic-complexity MIT-ported cursor-session redacted reasoning decoding stays grouped until extraction refactor.
// decodeRedactedReasoning is the internal implementation
func decodeRedactedReasoning(encoded string) (string, bool) {
	if encoded == "" {
		return "", false
	}

	// Try base64url decode first
	decoded, err := decodeBase64URL(encoded)
	if err != nil {
		// Not base64url - return original
		return encoded, false
	}

	// Try protobuf decode
	if fields, ok := tryProtobufDecode(decoded); ok {
		// Extract readable strings from protobuf fields
		var textParts []string
		for key, value := range fields {
			if str, ok := value.(string); ok {
				// Try to extract JSON from the string
				if jsonBytes, found := extractJSONFromBinary([]byte(str)); found {
					// Found JSON - try to extract readable text from it
					var jsonData map[string]interface{}
					if err := json.Unmarshal(jsonBytes, &jsonData); err == nil {
						// Extract all string values from JSON
						for _, v := range jsonData {
							if s, ok := v.(string); ok && isReadableText(s) && len(s) > 10 {
								textParts = append(textParts, s)
							}
						}
					}
				}
				// Also check if the string itself is readable
				if isReadableText(str) && len(str) > 10 {
					textParts = append(textParts, str)
				}
			} else if nestedMap, ok := value.(map[string]interface{}); ok {
				// Recursively extract strings from nested maps
				extracted := extractStringsFromMap(nestedMap)
				textParts = append(textParts, extracted...)
			}
			// Log for debugging (only first few fields to avoid spam)
			if len(fields) <= 5 {
				valueStr := fmt.Sprintf("%v", value)
				previewLen := 100
				if len(valueStr) < previewLen {
					previewLen = len(valueStr)
				}
				LogDebug("Protobuf field %s: type=%T, value_preview=%v", key, value, valueStr[:previewLen])
			}
		}
		if len(textParts) > 0 {
			return strings.Join(textParts, "\n"), true
		}
	}

	// Try to extract readable strings directly from decoded bytes
	if extractedStrings, err := decodeProtobufStrings(decoded); err == nil && len(extractedStrings) > 0 {
		var readableStrings []string
		for _, s := range extractedStrings {
			// Try to extract JSON from the string
			if jsonBytes, found := extractJSONFromBinary([]byte(s)); found {
				var jsonData map[string]interface{}
				if err := json.Unmarshal(jsonBytes, &jsonData); err == nil {
					// Extract all string values from JSON
					for _, v := range jsonData {
						if str, ok := v.(string); ok && isReadableText(str) && len(str) > 10 {
							readableStrings = append(readableStrings, str)
						}
					}
				}
			}
			// Also check if the string itself is readable
			if isReadableText(s) && len(s) > 10 {
				readableStrings = append(readableStrings, s)
			}
		}
		if len(readableStrings) > 0 {
			return strings.Join(readableStrings, "\n"), true
		}
	}

	// If we decoded but couldn't extract readable text, check if it looks encrypted
	// High entropy (close to 8 bits/byte) suggests encryption
	entropy := calculateEntropy(decoded)
	if entropy > 6.0 {
		return fmt.Sprintf("[Encrypted: %d bytes, entropy=%.2f bits/byte - content is encrypted and cannot be decoded without the key]", len(decoded), entropy), false
	}

	// Low entropy but still not readable - might be compressed or encoded differently
	return fmt.Sprintf("[Encoded: %d bytes, entropy=%.2f bits/byte - could not decode]", len(decoded), entropy), false
}

// decodeBase64URL decodes a base64url-encoded string
func decodeBase64URL(s string) ([]byte, error) {
	// Add padding if needed
	padLen := (4 - len(s)%4) % 4
	s += strings.Repeat("=", padLen)

	// Try URL-safe base64 first
	decoded, err := base64.URLEncoding.DecodeString(s)
	if err != nil {
		// Try standard base64 as fallback
		decoded, err = base64.StdEncoding.DecodeString(s)
	}
	return decoded, err
}

// extractStringsFromMap recursively extracts readable strings from a map
func extractStringsFromMap(m map[string]interface{}) []string {
	var strings []string
	for _, value := range m {
		if str, ok := value.(string); ok && isReadableText(str) && len(str) > 10 {
			strings = append(strings, str)
		} else if nestedMap, ok := value.(map[string]interface{}); ok {
			strings = append(strings, extractStringsFromMap(nestedMap)...)
		}
	}
	return strings
}

// calculateEntropy calculates the Shannon entropy of the data
// High entropy (close to 8.0) suggests encryption or compression
func calculateEntropy(data []byte) float64 {
	if len(data) == 0 {
		return 0
	}

	byteCounts := make(map[byte]int)
	for _, b := range data {
		byteCounts[b]++
	}

	entropy := 0.0
	dataLen := float64(len(data))
	for _, count := range byteCounts {
		if count > 0 {
			p := float64(count) / dataLen
			entropy -= p * math.Log2(p)
		}
	}

	return entropy
}
