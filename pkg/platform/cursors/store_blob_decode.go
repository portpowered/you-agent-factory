package cursors

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

// backendsizecheck:ignore-function MIT-ported cursor-session blob decode fallback chain stays grouped until behavior-preserving extraction refactor.
// pkgmaintcheck:ignore-function-lines MIT-ported cursor-session blob decode fallback chain stays grouped until behavior-preserving extraction refactor.
// pkgmaintcheck:ignore-cyclomatic-complexity MIT-ported cursor-session blob decode fallback chain stays grouped until behavior-preserving extraction refactor.
func decodeBlobEntryValue(blob BlobEntry, index int, sessionID string, jsonParseFailures *int) (map[string]interface{}, *RawBubble, bool) {
	var data map[string]interface{}
	valueBytes := []byte(blob.Value)

	// Try JSON first
	if err := json.Unmarshal(valueBytes, &data); err != nil {
		// Not JSON - try base64 decode in case it's encoded
		decoded, decodeErr := tryBase64Decode(blob.Value)
		if decodeErr == nil {
			if jsonErr := json.Unmarshal(decoded, &data); jsonErr == nil {
				// Successfully decoded and parsed
				LogInfo("Blob %d (key='%s') was base64 encoded, decoded successfully", index+1, blob.Key)
			} else {
				// Base64 decoded but not JSON - try extracting JSON from binary
				jsonBytes, found := extractJSONFromBinary(decoded)
				if found {
					// extractJSONFromBinary already validated it's valid JSON
					if jsonErr := json.Unmarshal(jsonBytes, &data); jsonErr == nil {
						LogInfo("Blob %d (key='%s') had JSON embedded in binary data, extracted successfully", index+1, blob.Key)
					} else {
						// This shouldn't happen since extractJSONFromBinary validates, but handle it anyway
						(*jsonParseFailures)++
						if index < 5 {
							valuePreview := blob.Value
							if len(valuePreview) > 100 {
								valuePreview = valuePreview[:100] + "..."
							}
							LogWarn("Blob %d (key='%s') failed JSON parse after extraction: %v. Value preview: %s", index+1, blob.Key, jsonErr, valuePreview)
						}
						return nil, nil, false
					}
				} else {
					// Decoded but still not JSON - log and skip
					(*jsonParseFailures)++
					if index < 5 {
						valuePreview := blob.Value
						if len(valuePreview) > 100 {
							valuePreview = valuePreview[:100] + "..."
						}
						LogWarn("Blob %d (key='%s') failed JSON parse (tried base64 too): %v. Value preview: %s", index+1, blob.Key, jsonErr, valuePreview)
					}
					return nil, nil, false
				}
			}
		} else {
			// Not base64 - try hex decode (like we do for meta entries)
			hexDecoded, hexErr := tryHexDecode(blob.Value)
			if hexErr == nil {
				// Try JSON parse on hex-decoded data
				if jsonErr := json.Unmarshal(hexDecoded, &data); jsonErr == nil {
					LogInfo("Blob %d (key='%s') was hex encoded, decoded successfully", index+1, blob.Key)
				} else {
					// Hex decoded but not JSON - try extracting JSON from binary
					jsonBytes, found := extractJSONFromBinary(hexDecoded)
					if found {
						// extractJSONFromBinary already validated it's valid JSON
						if jsonErr := json.Unmarshal(jsonBytes, &data); jsonErr == nil {
							LogInfo("Blob %d (key='%s') had JSON embedded in hex-decoded binary data, extracted successfully", index+1, blob.Key)
						} else {
							// This shouldn't happen since extractJSONFromBinary validates, but handle it anyway
							(*jsonParseFailures)++
							if index < 5 {
								LogWarn("Blob %d (key='%s') hex decoded but JSON parse failed after extraction: %v", index+1, blob.Key, jsonErr)
							}
							return nil, nil, false
						}
					} else {
						// Hex decoded but no JSON found - skip
						(*jsonParseFailures)++
						if index < 5 {
							LogWarn("Blob %d (key='%s') was hex encoded but contains no JSON", index+1, blob.Key)
						}
						return nil, nil, false
					}
				}
			} else {
				// Not hex - try extracting JSON from binary data
				jsonBytes, found := extractJSONFromBinary(valueBytes)
				if found {
					// extractJSONFromBinary already validated it's valid JSON, so we can parse it directly
					if jsonErr := json.Unmarshal(jsonBytes, &data); jsonErr == nil {
						jsonPreview := string(jsonBytes)
						if len(jsonPreview) > 200 {
							jsonPreview = jsonPreview[:200] + "..."
						}
						LogInfo("Blob %d (key='%s'): Found valid JSON in binary (len=%d): %s", index+1, blob.Key, len(jsonBytes), jsonPreview)
						LogInfo("Blob %d (key='%s') had JSON embedded in binary data, extracted successfully", index+1, blob.Key)
						// Log fields to understand structure
						keys := make([]string, 0, len(data))
						for k := range data {
							keys = append(keys, k)
						}
						LogInfo("Blob %d extracted JSON fields: %v", index+1, keys)
					} else {
						// This shouldn't happen since extractJSONFromBinary validates, but handle it anyway
						(*jsonParseFailures)++
						if index < 10 {
							LogWarn("Blob %d (key='%s', key_len=%d) failed JSON parse after extraction: %v", index+1, blob.Key, len(blob.Key), jsonErr)
						}
						return nil, nil, false
					}
				} else {
					// Not base64 and no JSON in binary - try protobuf decode
					if protobufFields, found := tryProtobufDecode(valueBytes); found {
						// Initialize data map if it's nil
						if data == nil {
							data = make(map[string]interface{})
						}
						// Extract strings from protobuf fields
						var extractedStrings []string
						for key, value := range protobufFields {
							if str, ok := value.(string); ok && isReadableText(str) {
								extractedStrings = append(extractedStrings, str)
								// Try to parse as JSON if it looks like JSON
								if strings.HasPrefix(str, "{") || strings.HasPrefix(str, "[") {
									var jsonData map[string]interface{}
									if jsonErr := json.Unmarshal([]byte(str), &jsonData); jsonErr == nil {
										// Merge JSON data into our data map
										for k, v := range jsonData {
											data[k] = v
										}
										LogInfo("Blob %d (key='%s'): Extracted JSON from protobuf field %s", index+1, blob.Key, key)
									}
								}
							} else if nestedMap, ok := value.(map[string]interface{}); ok {
								// Nested protobuf - extract strings from it
								for nestedKey, nestedValue := range nestedMap {
									if nestedStr, ok := nestedValue.(string); ok && isReadableText(nestedStr) {
										extractedStrings = append(extractedStrings, nestedStr)
										// Try to parse as JSON
										if strings.HasPrefix(nestedStr, "{") || strings.HasPrefix(nestedStr, "[") {
											var jsonData map[string]interface{}
											if jsonErr := json.Unmarshal([]byte(nestedStr), &jsonData); jsonErr == nil {
												// Merge into data map with nested key prefix
												for k, v := range jsonData {
													data[fmt.Sprintf("%s_%s", nestedKey, k)] = v
												}
											}
										}
									}
								}
							}
						}
						if len(extractedStrings) > 0 {
							previewCount := 3
							if len(extractedStrings) < previewCount {
								previewCount = len(extractedStrings)
							}
							LogInfo("Blob %d (key='%s'): Decoded protobuf, extracted %d string(s): %v", index+1, blob.Key, len(extractedStrings), extractedStrings[:previewCount])
							// If we extracted JSON data, continue processing
							if len(data) > 0 {
								// Continue to bubble parsing below
							} else {
								// No JSON found in protobuf - try text message format
								if bubble := parseTextMessageFormat(blob.Key, blob.Value, sessionID); bubble != nil {
									LogInfo("Blob %d parsed as text message format (user message): bubbleId='%s', text='%s', chatId='%s'", index+1, bubble.BubbleID, bubble.Text, bubble.ChatID)
									return nil, nil, false
								}
								(*jsonParseFailures)++
								return nil, nil, false
							}
						} else {
							// Protobuf decoded but no readable strings found
							(*jsonParseFailures)++
							if index < 5 {
								LogWarn("Blob %d (key='%s'): Decoded as protobuf but no readable strings extracted", index+1, blob.Key)
							}
							return nil, nil, false
						}
					} else {
						// Not protobuf - try parsing as text message format (text$uuid)
						// This handles cursor-agent's user message format: "hello$027f8b2f-d09c-4a69-98b0-b53f0118605d"
						if bubble := parseTextMessageFormat(blob.Key, blob.Value, sessionID); bubble != nil {
							LogInfo("Blob %d parsed as text message format (user message): bubbleId='%s', text='%s', chatId='%s'", index+1, bubble.BubbleID, bubble.Text, bubble.ChatID)
							return nil, nil, false
						} else {
							// Log that we tried but failed to parse as text format
							// Only log if value appears to be readable (not binary garbage)
							if index < 5 && isReadableText(blob.Value) {
								valuePreview := blob.Value
								if len(valuePreview) > 100 {
									valuePreview = valuePreview[:100] + "..."
								}
								LogInfo("Blob %d: tried text message format but didn't match pattern. Value preview: %s", index+1, valuePreview)
							}
							// Not a text message format - the value might be a reference or in a different format
							// Log detailed info for first few failures to understand the format
							(*jsonParseFailures)++
							if index < 10 {
								valuePreview := blob.Value
								fullValue := blob.Value
								if len(valuePreview) > 200 {
									valuePreview = valuePreview[:200] + "..."
								}
								LogWarn("Blob %d (key='%s', key_len=%d) failed JSON parse: %v", index+1, blob.Key, len(blob.Key), err)
								LogInfo("  Value (len=%d): %s", len(fullValue), valuePreview)
								LogInfo("  Key looks like hash: %v", isHashLike(blob.Key))
								// Check if value looks like a path or reference
								if strings.HasPrefix(fullValue, "/") || strings.Contains(fullValue, "$") {
									LogInfo("  Value appears to be a path/reference, not JSON data")
								}
								// Check if there's a { in the value that might indicate JSON
								if bytes.Contains(valueBytes, []byte("{")) {
									LogInfo("  Value contains '{' but extraction failed - JSON might be incomplete or malformed")
								}
							}
							return nil, nil, false
						}
					}
				}
			}
		}
	}

	return data, nil, true
}

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
