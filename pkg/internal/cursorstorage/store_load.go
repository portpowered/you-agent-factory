package cursorstorage

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// AgentStorageReader reads session data from cursor-agent CLI store.db files
type AgentStorageReader struct {
	storeDBPaths []string
}

// NewAgentStorageReader creates a new AgentStorageReader with the given store.db paths
func NewAgentStorageReader(storeDBPaths []string) *AgentStorageReader {
	return &AgentStorageReader{
		storeDBPaths: storeDBPaths,
	}
}

// QueryBlobsTable queries the blobs table from a store.db file
func QueryBlobsTable(db *sql.DB) ([]BlobEntry, error) {
	// Check if blobs table exists
	var tableExists bool
	err := db.QueryRow(`
		SELECT EXISTS (
			SELECT name FROM sqlite_master
			WHERE type='table' AND name='blobs'
		)
	`).Scan(&tableExists)
	if err != nil {
		return nil, fmt.Errorf("failed to check for blobs table: %w", err)
	}

	if !tableExists {
		return []BlobEntry{}, nil
	}

	// Query all blobs - we'll need to inspect the schema
	// Common patterns: key-value, id-data, etc.
	// Try to get column names first
	rows, err := db.Query("PRAGMA table_info(blobs)")
	if err != nil {
		return nil, fmt.Errorf("failed to get blobs table info: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var columns []string
	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull int
		var defaultValue sql.NullString
		var pk int

		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &pk); err != nil {
			continue
		}
		columns = append(columns, name)
	}

	if len(columns) == 0 {
		return []BlobEntry{}, nil
	}

	// Build query based on common column patterns
	// Try key-value pattern first (most common for session storage)
	var query string
	if containsString(columns, "key") && containsString(columns, "value") {
		query = "SELECT key, value FROM blobs WHERE value IS NOT NULL"
	} else if containsString(columns, "id") && containsString(columns, "data") {
		// Use ORDER BY rowid to preserve insertion order (chronological order)
		// This ensures messages are in the order they were created
		query = "SELECT id, data FROM blobs WHERE data IS NOT NULL ORDER BY rowid"
	} else if len(columns) >= 2 {
		// Use first two columns
		query = fmt.Sprintf("SELECT %s, %s FROM blobs WHERE %s IS NOT NULL", columns[0], columns[1], columns[1])
	} else {
		return []BlobEntry{}, nil
	}

	rows, err = db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query blobs table: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var entries []BlobEntry
	rowCount := 0
	for rows.Next() {
		rowCount++
		var entry BlobEntry
		var value sql.NullString
		if err := rows.Scan(&entry.Key, &value); err != nil {
			LogWarn("Failed to scan blob row %d: %v", rowCount, err)
			continue
		}
		if value.Valid {
			entry.Value = value.String
			entries = append(entries, entry)
			// Log first few entries for diagnostics
			if rowCount <= 3 {
				valuePreview := entry.Value
				if len(valuePreview) > 200 {
					valuePreview = valuePreview[:200] + "..."
				}
				LogInfo("Blob entry %d: key='%s', value_preview='%s'", rowCount, entry.Key, valuePreview)
			}
		} else {
			LogWarn("Blob row %d has NULL value: key='%s'", rowCount, entry.Key)
		}
	}

	LogInfo("QueryBlobsTable: queried %d rows, returned %d valid entries", rowCount, len(entries))

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}

	return entries, nil
}

// QueryMetaTable queries the meta table from a store.db file
func QueryMetaTable(db *sql.DB) ([]MetaEntry, error) {
	// Check if meta table exists
	var tableExists bool
	err := db.QueryRow(`
		SELECT EXISTS (
			SELECT name FROM sqlite_master
			WHERE type='table' AND name='meta'
		)
	`).Scan(&tableExists)
	if err != nil {
		return nil, fmt.Errorf("failed to check for meta table: %w", err)
	}

	if !tableExists {
		return []MetaEntry{}, nil
	}

	// Query meta table - similar flexible approach
	rows, err := db.Query("PRAGMA table_info(meta)")
	if err != nil {
		return nil, fmt.Errorf("failed to get meta table info: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var columns []string
	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull int
		var defaultValue sql.NullString
		var pk int

		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &pk); err != nil {
			continue
		}
		columns = append(columns, name)
	}

	if len(columns) == 0 {
		return []MetaEntry{}, nil
	}

	var query string
	if containsString(columns, "key") && containsString(columns, "value") {
		query = "SELECT key, value FROM meta WHERE value IS NOT NULL"
	} else if containsString(columns, "id") && containsString(columns, "data") {
		query = "SELECT id, data FROM meta WHERE data IS NOT NULL"
	} else if len(columns) >= 2 {
		query = fmt.Sprintf("SELECT %s, %s FROM meta WHERE %s IS NOT NULL", columns[0], columns[1], columns[1])
	} else {
		return []MetaEntry{}, nil
	}

	rows, err = db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query meta table: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var entries []MetaEntry
	rowCount := 0
	for rows.Next() {
		rowCount++
		var entry MetaEntry
		var value sql.NullString
		if err := rows.Scan(&entry.Key, &value); err != nil {
			LogWarn("Failed to scan meta row %d: %v", rowCount, err)
			continue
		}
		if value.Valid {
			entry.Value = value.String
			entries = append(entries, entry)
			// Log first few entries for diagnostics
			if rowCount <= 3 {
				valuePreview := entry.Value
				if len(valuePreview) > 200 {
					valuePreview = valuePreview[:200] + "..."
				}
				LogInfo("Meta entry %d: key='%s', value_preview='%s'", rowCount, entry.Key, valuePreview)
			}
		} else {
			LogWarn("Meta row %d has NULL value: key='%s'", rowCount, entry.Key)
		}
	}

	LogInfo("QueryMetaTable: queried %d rows, returned %d valid entries", rowCount, len(entries))

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}

	return entries, nil
}

// BlobEntry represents an entry from the blobs table
type BlobEntry struct {
	Key   string
	Value string
}

// MetaEntry represents an entry from the meta table
type MetaEntry struct {
	Key   string
	Value string
}

// LoadSessionFromStoreDB loads session data from a single store.db file
func LoadSessionFromStoreDB(dbPath string) (map[string]*RawBubble, []*RawComposer, map[string][]*MessageContext, SessionTokenUsage, error) {
	db, err := OpenDatabase(dbPath)
	if err != nil {
		return nil, nil, nil, SessionTokenUsage{}, fmt.Errorf("failed to open store.db: %w", err)
	}
	defer func() { _ = db.Close() }()

	// Query both tables
	blobs, err := QueryBlobsTable(db)
	if err != nil {
		return nil, nil, nil, SessionTokenUsage{}, fmt.Errorf("failed to query blobs table: %w", err)
	}

	meta, err := QueryMetaTable(db)
	if err != nil {
		return nil, nil, nil, SessionTokenUsage{}, fmt.Errorf("failed to query meta table: %w", err)
	}

	// Extract session ID from path: ~/.cursor/chats/{hash}/{session-id}/store.db
	// Use this to help identify the session
	sessionID := extractSessionIDFromPath(dbPath)

	// Parse blobs and meta entries to extract bubbles, composers, and contexts
	bubbles := make(map[string]*RawBubble)
	var composers []*RawComposer
	contexts := make(map[string][]*MessageContext)
	var sessionTokenUsage SessionTokenUsage

	// Process blobs - they may contain bubble data
	jsonParseFailures := 0
	for i, blob := range blobs {
		// Try to parse as JSON and identify the type
		var data map[string]interface{}
		valueBytes := []byte(blob.Value)

		// Try JSON first
		if err := json.Unmarshal(valueBytes, &data); err != nil {
			// Not JSON - try base64 decode in case it's encoded
			decoded, decodeErr := tryBase64Decode(blob.Value)
			if decodeErr == nil {
				if jsonErr := json.Unmarshal(decoded, &data); jsonErr == nil {
					// Successfully decoded and parsed
					LogInfo("Blob %d (key='%s') was base64 encoded, decoded successfully", i+1, blob.Key)
				} else {
					// Base64 decoded but not JSON - try extracting JSON from binary
					jsonBytes, found := extractJSONFromBinary(decoded)
					if found {
						// extractJSONFromBinary already validated it's valid JSON
						if jsonErr := json.Unmarshal(jsonBytes, &data); jsonErr == nil {
							LogInfo("Blob %d (key='%s') had JSON embedded in binary data, extracted successfully", i+1, blob.Key)
						} else {
							// This shouldn't happen since extractJSONFromBinary validates, but handle it anyway
							jsonParseFailures++
							if i < 5 {
								valuePreview := blob.Value
								if len(valuePreview) > 100 {
									valuePreview = valuePreview[:100] + "..."
								}
								LogWarn("Blob %d (key='%s') failed JSON parse after extraction: %v. Value preview: %s", i+1, blob.Key, jsonErr, valuePreview)
							}
							continue
						}
					} else {
						// Decoded but still not JSON - log and skip
						jsonParseFailures++
						if i < 5 {
							valuePreview := blob.Value
							if len(valuePreview) > 100 {
								valuePreview = valuePreview[:100] + "..."
							}
							LogWarn("Blob %d (key='%s') failed JSON parse (tried base64 too): %v. Value preview: %s", i+1, blob.Key, jsonErr, valuePreview)
						}
						continue
					}
				}
			} else {
				// Not base64 - try hex decode (like we do for meta entries)
				hexDecoded, hexErr := tryHexDecode(blob.Value)
				if hexErr == nil {
					// Try JSON parse on hex-decoded data
					if jsonErr := json.Unmarshal(hexDecoded, &data); jsonErr == nil {
						LogInfo("Blob %d (key='%s') was hex encoded, decoded successfully", i+1, blob.Key)
					} else {
						// Hex decoded but not JSON - try extracting JSON from binary
						jsonBytes, found := extractJSONFromBinary(hexDecoded)
						if found {
							// extractJSONFromBinary already validated it's valid JSON
							if jsonErr := json.Unmarshal(jsonBytes, &data); jsonErr == nil {
								LogInfo("Blob %d (key='%s') had JSON embedded in hex-decoded binary data, extracted successfully", i+1, blob.Key)
							} else {
								// This shouldn't happen since extractJSONFromBinary validates, but handle it anyway
								jsonParseFailures++
								if i < 5 {
									LogWarn("Blob %d (key='%s') hex decoded but JSON parse failed after extraction: %v", i+1, blob.Key, jsonErr)
								}
								continue
							}
						} else {
							// Hex decoded but no JSON found - skip
							jsonParseFailures++
							if i < 5 {
								LogWarn("Blob %d (key='%s') was hex encoded but contains no JSON", i+1, blob.Key)
							}
							continue
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
							LogInfo("Blob %d (key='%s'): Found valid JSON in binary (len=%d): %s", i+1, blob.Key, len(jsonBytes), jsonPreview)
							LogInfo("Blob %d (key='%s') had JSON embedded in binary data, extracted successfully", i+1, blob.Key)
							// Log fields to understand structure
							keys := make([]string, 0, len(data))
							for k := range data {
								keys = append(keys, k)
							}
							LogInfo("Blob %d extracted JSON fields: %v", i+1, keys)
						} else {
							// This shouldn't happen since extractJSONFromBinary validates, but handle it anyway
							jsonParseFailures++
							if i < 10 {
								LogWarn("Blob %d (key='%s', key_len=%d) failed JSON parse after extraction: %v", i+1, blob.Key, len(blob.Key), jsonErr)
							}
							continue
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
											LogInfo("Blob %d (key='%s'): Extracted JSON from protobuf field %s", i+1, blob.Key, key)
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
								LogInfo("Blob %d (key='%s'): Decoded protobuf, extracted %d string(s): %v", i+1, blob.Key, len(extractedStrings), extractedStrings[:previewCount])
								// If we extracted JSON data, continue processing
								if len(data) > 0 {
									// Continue to bubble parsing below
								} else {
									// No JSON found in protobuf - try text message format
									if bubble := parseTextMessageFormat(blob.Key, blob.Value, sessionID); bubble != nil {
										bubbles[bubble.BubbleID] = bubble
										LogInfo("Blob %d parsed as text message format (user message): bubbleId='%s', text='%s', chatId='%s'", i+1, bubble.BubbleID, bubble.Text, bubble.ChatID)
										continue
									}
									jsonParseFailures++
									continue
								}
							} else {
								// Protobuf decoded but no readable strings found
								jsonParseFailures++
								if i < 5 {
									LogWarn("Blob %d (key='%s'): Decoded as protobuf but no readable strings extracted", i+1, blob.Key)
								}
								continue
							}
						} else {
							// Not protobuf - try parsing as text message format (text$uuid)
							// This handles cursor-agent's user message format: "hello$027f8b2f-d09c-4a69-98b0-b53f0118605d"
							if bubble := parseTextMessageFormat(blob.Key, blob.Value, sessionID); bubble != nil {
								bubbles[bubble.BubbleID] = bubble
								LogInfo("Blob %d parsed as text message format (user message): bubbleId='%s', text='%s', chatId='%s'", i+1, bubble.BubbleID, bubble.Text, bubble.ChatID)
								continue
							} else {
								// Log that we tried but failed to parse as text format
								// Only log if value appears to be readable (not binary garbage)
								if i < 5 && isReadableText(blob.Value) {
									valuePreview := blob.Value
									if len(valuePreview) > 100 {
										valuePreview = valuePreview[:100] + "..."
									}
									LogInfo("Blob %d: tried text message format but didn't match pattern. Value preview: %s", i+1, valuePreview)
								}
								// Not a text message format - the value might be a reference or in a different format
								// Log detailed info for first few failures to understand the format
								jsonParseFailures++
								if i < 10 {
									valuePreview := blob.Value
									fullValue := blob.Value
									if len(valuePreview) > 200 {
										valuePreview = valuePreview[:200] + "..."
									}
									LogWarn("Blob %d (key='%s', key_len=%d) failed JSON parse: %v", i+1, blob.Key, len(blob.Key), err)
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
								continue
							}
						}
					}
				}
			}
		}

		// Log available fields for debugging (only for first few blobs to reduce noise)
		if i < 5 {
			keys := make([]string, 0, len(data))
			for k := range data {
				keys = append(keys, k)
			}
			LogInfo("Blob %d (key='%s') parsed successfully. Available fields: %v", i+1, blob.Key, keys)
		}
		mergeSessionTokenUsage(&sessionTokenUsage, tokenUsageFromData(data))

		// Check if it's a bubble (has bubbleId)
		if _, ok := data["bubbleId"].(string); ok {
			bubble, err := parseBubbleFromData(blob.Key, data, sessionID)
			if err == nil {
				bubbles[bubble.BubbleID] = bubble
			}
		} else if id, ok := data["id"].(string); ok {
			// Check if it's a message format (has id, role, content) - cursor-agent format
			if role, hasRole := data["role"].(string); hasRole {
				bubble, err := parseMessageToBubble(blob.Key, id, role, data, sessionID)
				if err == nil {
					bubbles[bubble.BubbleID] = bubble
					LogInfo("Blob %d converted message (id='%s', role='%s') to bubble (bubbleId='%s')", i+1, id, role, bubble.BubbleID)
				} else {
					LogWarn("Blob %d failed to convert message to bubble: %v", i+1, err)
				}
			}
		} else if role, hasRole := data["role"].(string); hasRole {
			// Handle messages with role and content but no id field
			// Generate a unique ID from the blob key
			generatedID := blob.Key
			if len(blob.Key) > 16 {
				generatedID = blob.Key[:16]
			}
			bubble, err := parseMessageToBubble(blob.Key, generatedID, role, data, sessionID)
			if err == nil {
				bubbles[bubble.BubbleID] = bubble
				LogInfo("Blob %d converted message (no id, role='%s') to bubble (bubbleId='%s')", i+1, role, bubble.BubbleID)
			} else {
				LogWarn("Blob %d failed to convert message to bubble: %v", i+1, err)
			}
		}

		// Check if it's a composer (has composerId)
		if composerID, ok := data["composerId"].(string); ok {
			composer, err := parseComposerFromData(blob.Key, data)
			if err != nil {
				LogWarn("Failed to parse composer from blob key %s: %v", blob.Key, err)
				continue
			}
			if composer.ComposerID == "" {
				LogWarn("Composer parsed but missing composerId. Blob key: %s", blob.Key)
				continue
			}
			composer.ComposerID = composerID
			headerCount := len(composer.FullConversationHeadersOnly)
			LogInfo("Parsed composer %s: %d headers, name='%s'", composer.ComposerID, headerCount, composer.Name)
			composers = append(composers, composer)
		}
	}

	if jsonParseFailures > 0 {
		LogWarn("Failed to parse %d/%d blobs as JSON", jsonParseFailures, len(blobs))
	}

	// Extract session-level metadata from meta table (key="0" contains session metadata)
	var sessionCreatedAt int64 = 0
	var sessionAgentID string
	var sessionName string

	// Process meta - may contain context or additional metadata
	metaJsonParseFailures := 0
	for i, entry := range meta {
		var data map[string]interface{}
		valueBytes := []byte(entry.Value)

		// Try JSON first
		if err := json.Unmarshal(valueBytes, &data); err != nil {
			// Not JSON - try base64 decode
			decoded, decodeErr := tryBase64Decode(entry.Value)
			if decodeErr == nil {
				if jsonErr := json.Unmarshal(decoded, &data); jsonErr == nil {
					LogInfo("Meta %d (key='%s') was base64 encoded, decoded successfully", i+1, entry.Key)
				} else {
					// Base64 decoded but not JSON - try hex decode
					hexDecoded, hexErr := tryHexDecode(entry.Value)
					if hexErr == nil {
						if jsonErr := json.Unmarshal(hexDecoded, &data); jsonErr == nil {
							LogInfo("Meta %d (key='%s') was hex encoded, decoded successfully", i+1, entry.Key)
						} else {
							metaJsonParseFailures++
							if i < 5 {
								valuePreview := entry.Value
								if len(valuePreview) > 100 {
									valuePreview = valuePreview[:100] + "..."
								}
								LogWarn("Meta %d (key='%s') failed JSON parse (tried base64 and hex): %v. Value preview: %s", i+1, entry.Key, jsonErr, valuePreview)
							}
							continue
						}
					} else {
						metaJsonParseFailures++
						if i < 5 {
							valuePreview := entry.Value
							if len(valuePreview) > 100 {
								valuePreview = valuePreview[:100] + "..."
							}
							LogWarn("Meta %d (key='%s') failed JSON parse (tried base64 too): %v. Value preview: %s", i+1, entry.Key, jsonErr, valuePreview)
						}
						continue
					}
				}
			} else {
				// Not base64 - try hex decode
				hexDecoded, hexErr := tryHexDecode(entry.Value)
				if hexErr == nil {
					if jsonErr := json.Unmarshal(hexDecoded, &data); jsonErr == nil {
						LogInfo("Meta %d (key='%s') was hex encoded, decoded successfully", i+1, entry.Key)
					} else {
						metaJsonParseFailures++
						if i < 10 {
							valuePreview := entry.Value
							fullValue := entry.Value
							if len(valuePreview) > 200 {
								valuePreview = valuePreview[:200] + "..."
							}
							LogWarn("Meta %d (key='%s', key_len=%d) failed JSON parse (tried hex): %v", i+1, entry.Key, len(entry.Key), jsonErr)
							LogInfo("  Value (len=%d): %s", len(fullValue), valuePreview)
						}
						continue
					}
				} else {
					metaJsonParseFailures++
					if i < 10 {
						valuePreview := entry.Value
						fullValue := entry.Value
						if len(valuePreview) > 200 {
							valuePreview = valuePreview[:200] + "..."
						}
						LogWarn("Meta %d (key='%s', key_len=%d) failed JSON parse: %v", i+1, entry.Key, len(entry.Key), err)
						LogInfo("  Value (len=%d): %s", len(fullValue), valuePreview)
						if strings.HasPrefix(fullValue, "/") || strings.Contains(fullValue, "$") {
							LogInfo("  Value appears to be a path/reference, not JSON data")
						}
					}
					continue
				}
			}
		}

		// Log available fields for first few entries
		if i < 3 {
			keys := make([]string, 0, len(data))
			for k := range data {
				keys = append(keys, k)
			}
			LogInfo("Meta %d (key='%s') parsed successfully. Available fields: %v", i+1, entry.Key, keys)
		}
		mergeSessionTokenUsage(&sessionTokenUsage, tokenUsageFromData(data))

		// Extract session-level metadata from meta entry with key "0"
		if entry.Key == "0" {
			// Extract createdAt (session creation timestamp)
			if ts, ok := data["createdAt"].(float64); ok {
				sessionCreatedAt = int64(ts)
				LogInfo("Meta: Extracted session createdAt: %d (from meta key='0')", sessionCreatedAt)
			} else if ts, ok := data["createdAt"].(int64); ok {
				sessionCreatedAt = ts
				LogInfo("Meta: Extracted session createdAt: %d (from meta key='0')", sessionCreatedAt)
			}

			// Extract agentId (session ID)
			if agentID, ok := data["agentId"].(string); ok {
				sessionAgentID = agentID
				LogInfo("Meta: Extracted session agentId: %s (from meta key='0')", sessionAgentID)
			}

			// Extract name (session name)
			if name, ok := data["name"].(string); ok {
				sessionName = name
				LogInfo("Meta: Extracted session name: %s (from meta key='0')", sessionName)
			}
		}

		// Check if it's a message context
		if _, ok := data["contextId"].(string); ok {
			context, err := parseContextFromData(entry.Key, data)
			if err == nil {
				composerID := context.ComposerID
				if composerID == "" {
					// Try to extract from key or data
					if cid, ok := data["composerId"].(string); ok {
						composerID = cid
						context.ComposerID = composerID
					}
				}
				if composerID != "" {
					contexts[composerID] = append(contexts[composerID], context)
				}
			}
		}
	}

	// Apply session createdAt to bubbles that don't have timestamps
	if sessionCreatedAt > 0 {
		for bubbleID, bubble := range bubbles {
			if bubble.Timestamp == 0 {
				bubble.Timestamp = sessionCreatedAt
				bubbles[bubbleID] = bubble
				LogInfo("Applied session createdAt (%d) to bubble %s (was missing timestamp)", sessionCreatedAt, bubbleID)
			}
		}
	}

	// Apply session metadata to composers
	if sessionCreatedAt > 0 || sessionName != "" {
		for i := range composers {
			if sessionCreatedAt > 0 && composers[i].CreatedAt == 0 {
				composers[i].CreatedAt = sessionCreatedAt
				LogInfo("Applied session createdAt (%d) to composer %s", sessionCreatedAt, composers[i].ComposerID)
			}
			if sessionName != "" && composers[i].Name == "" {
				composers[i].Name = sessionName
				LogInfo("Applied session name (%s) to composer %s", sessionName, composers[i].ComposerID)
			}
		}
	}

	if metaJsonParseFailures > 0 {
		LogWarn("Failed to parse %d/%d meta entries as JSON", metaJsonParseFailures, len(meta))
	}

	LogInfo("LoadSessionFromStoreDB summary: %d blobs queried, %d meta queried, %d bubbles extracted, %d composers extracted, %d contexts extracted",
		len(blobs), len(meta), len(bubbles), len(composers), len(contexts))

	return bubbles, composers, contexts, sessionTokenUsage, nil
}

// LoadAllSessionsFromAgentStorage loads all sessions from all store.db files
func (r *AgentStorageReader) LoadAllSessionsFromAgentStorage() (map[string]*RawBubble, []*RawComposer, map[string][]*MessageContext, error) {
	allBubbles := make(map[string]*RawBubble)
	var allComposers []*RawComposer
	allContexts := make(map[string][]*MessageContext)

	for _, dbPath := range r.storeDBPaths {
		bubbles, composers, contexts, _, err := LoadSessionFromStoreDB(dbPath)
		if err != nil {
			// Log error but continue with other files
			LogWarn("Failed to load session from %s: %v", dbPath, err)
			continue
		}

		// Merge bubbles (use bubbleID as key, so duplicates are overwritten)
		for id, bubble := range bubbles {
			allBubbles[id] = bubble
		}

		// Append composers
		allComposers = append(allComposers, composers...)
		LogInfo("Loaded from %s: %d bubbles, %d composers, %d context entries", dbPath, len(bubbles), len(composers), len(contexts))

		// Merge contexts
		for composerID, ctxList := range contexts {
			allContexts[composerID] = append(allContexts[composerID], ctxList...)
		}
	}

	LogInfo("Total loaded from agent storage: %d bubbles, %d composers, %d context groups", len(allBubbles), len(allComposers), len(allContexts))
	return allBubbles, allComposers, allContexts, nil
}

// Helper functions

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

func parseBubbleFromData(key string, data map[string]interface{}, sessionID string) (*RawBubble, error) {
	bubble := &RawBubble{}

	// Extract bubbleId
	if id, ok := data["bubbleId"].(string); ok {
		bubble.BubbleID = id
	} else {
		return nil, fmt.Errorf("missing bubbleId in data")
	}

	// Extract chatId (use sessionID if not present)
	if chatID, ok := data["chatId"].(string); ok {
		bubble.ChatID = chatID
	} else {
		bubble.ChatID = sessionID
	}

	// Extract text
	if text, ok := data["text"].(string); ok {
		bubble.Text = text
	}

	// Extract richText
	if richText, ok := data["richText"].(string); ok {
		bubble.RichText = richText
	}

	// Extract codeBlocks
	if codeBlocks, ok := data["codeBlocks"].([]interface{}); ok {
		for _, cb := range codeBlocks {
			if cbMap, ok := cb.(map[string]interface{}); ok {
				codeBlock := CodeBlock{}
				if lang, ok := cbMap["language"].(string); ok {
					codeBlock.Language = lang
				}
				if content, ok := cbMap["content"].(string); ok {
					codeBlock.Content = content
				}
				bubble.CodeBlocks = append(bubble.CodeBlocks, codeBlock)
			}
		}
	}

	// Extract timestamp
	// NOTE: cursor-agent does NOT store timestamps in individual bubble blobs.
	// Timestamps are only available at the session level in the meta table (key="0", field="createdAt").
	// Individual bubbles inherit the session createdAt timestamp.
	// Normalize to milliseconds (formatTimestamp expects milliseconds)
	if ts, ok := data["timestamp"].(float64); ok {
		bubble.Timestamp = normalizeTimestamp(int64(ts))
	} else if ts, ok := data["timestamp"].(int64); ok {
		bubble.Timestamp = normalizeTimestamp(ts)
	} else {
		bubble.Timestamp = 0
	}

	// Extract type
	if t, ok := data["type"].(float64); ok {
		bubble.Type = int(t)
	} else if t, ok := data["type"].(int); ok {
		bubble.Type = t
	}

	return bubble, nil
}

// isValidUUID checks if a string is a valid UUID format
var uuidRegex = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func isValidUUID(s string) bool {
	return uuidRegex.MatchString(strings.ToLower(s))
}

// isReadableText checks if text is mostly readable (not garbled binary data)
// Returns true if text is valid UTF-8 and has a reasonable ratio of printable characters
func isReadableText(text string) bool {
	// Must be valid UTF-8
	if !utf8.ValidString(text) {
		return false
	}

	// Empty text is not readable
	if len(text) == 0 {
		return false
	}

	// Count printable vs non-printable characters
	printableCount := 0
	totalRunes := 0
	for _, r := range text {
		totalRunes++
		if unicode.IsPrint(r) || r == '\n' || r == '\r' || r == '\t' {
			printableCount++
		}
	}

	// If we have very few runes, require all to be printable
	if totalRunes < 5 {
		return printableCount == totalRunes
	}

	// For longer text, require at least 70% printable characters
	// This filters out binary data that happens to contain a $ character
	printableRatio := float64(printableCount) / float64(totalRunes)
	return printableRatio >= 0.70
}

// parseTextMessageFormat parses cursor-agent's text message format: "text$uuid"
// Returns a RawBubble if the format matches and data is valid, nil otherwise
// Handles format like: "hello$027f8b2f-d09c-4a69-98b0-b53f0118605d" (may have control chars)
func parseTextMessageFormat(key, value, sessionID string) *RawBubble {
	// First, check if value is valid UTF-8 - if not, it's likely binary data
	if !utf8.ValidString(value) {
		return nil
	}

	// First, aggressively remove all control characters except newlines/tabs/carriage returns
	// This handles cases where the value starts with control chars like \x05, \n, etc.
	cleaned := strings.Map(func(r rune) rune {
		// Keep printable characters, newlines, tabs, carriage returns, and space
		if r >= 32 || r == '\n' || r == '\r' || r == '\t' {
			return r
		}
		// Remove all other control characters
		return -1
	}, value)

	// Trim whitespace from both ends
	cleaned = strings.TrimSpace(cleaned)

	// Check if value matches pattern: text$uuid
	// Example: "hello$027f8b2f-d09c-4a69-98b0-b53f0118605d"
	dollarIdx := strings.Index(cleaned, "$")
	if dollarIdx == -1 || dollarIdx == 0 {
		return nil // No $ found or $ is at start
	}

	// Extract text before $ and clean it
	text := strings.TrimSpace(cleaned[:dollarIdx])
	// Remove any remaining control characters (shouldn't be any after first pass, but be safe)
	text = strings.Map(func(r rune) rune {
		if r < 32 && r != '\n' && r != '\r' && r != '\t' {
			return -1 // Remove control characters except newlines/tabs
		}
		return r
	}, text)
	text = strings.TrimSpace(text)

	// Trim quotes from beginning and end (both single and double quotes)
	// This handles cases like: "text$uuid or 'text$uuid
	text = strings.Trim(text, `"'`)

	if text == "" {
		return nil // No text before $
	}

	// Validate that the text is actually readable (not garbled binary data)
	if !isReadableText(text) {
		return nil // Text is not readable, likely binary data
	}

	// Extract UUID after $ (optional, but useful for bubble ID)
	uuidPart := ""
	if dollarIdx+1 < len(cleaned) {
		uuidPart = strings.TrimSpace(cleaned[dollarIdx+1:])
		// Remove control characters from UUID
		uuidPart = strings.Map(func(r rune) rune {
			if r < 32 {
				return -1
			}
			return r
		}, uuidPart)
	}

	// If UUID part exists, validate it's a proper UUID format
	if uuidPart != "" && !isValidUUID(uuidPart) {
		// If it looks like it should be a UUID but isn't valid, this might be garbled data
		// Only accept if the text part is clearly readable
		if len(text) < 10 {
			return nil // Short text with invalid UUID is suspicious
		}
		// For longer readable text, we'll accept it but use a generated ID
		uuidPart = ""
	}

	// Use UUID as bubble ID if available, otherwise use a hash of the text
	bubbleID := uuidPart
	if bubbleID == "" {
		// Generate a simple ID from the blob key (first 8 chars)
		if len(key) >= 8 {
			bubbleID = key[:8]
		} else {
			bubbleID = key
		}
	}

	// Create user bubble (type=1 for user messages)
	bubble := &RawBubble{
		BubbleID:  bubbleID,
		ChatID:    sessionID,
		Type:      1, // User message
		Text:      text,
		Timestamp: time.Now().UnixMilli(), // Use current time if not available
	}

	return bubble
}

// parseMessageToBubble converts a message format (id, role, content) to a RawBubble
// This handles cursor-agent's message format where messages have id, role, and content fields
func parseMessageToBubble(key, id, role string, data map[string]interface{}, sessionID string) (*RawBubble, error) {
	// Create unique bubbleID by combining message id with blob key
	// This prevents multiple messages with the same id from overwriting each other
	// Use first 8 chars of blob key to keep it readable
	var bubbleID string
	if len(key) >= 8 {
		bubbleID = id + "-" + key[:8]
	} else {
		bubbleID = id + "-" + key
	}

	bubble := &RawBubble{
		BubbleID: bubbleID,
		ChatID:   sessionID,
	}

	// Map role to type: "user" = 1, "assistant" = 2
	switch role {
	case "user":
		bubble.Type = 1
	case "assistant":
		bubble.Type = 2
	default:
		// Default to assistant if unknown
		bubble.Type = 2
	}

	// Extract text from content array
	if content, ok := data["content"].([]interface{}); ok {
		var textParts []string
		for _, item := range content {
			if itemMap, ok := item.(map[string]interface{}); ok {
				itemType, _ := itemMap["type"].(string)

				// Handle tool calls
				if itemType == "tool_call" || itemType == "function_call" {
					toolCallParts := []string{"[Tool Call]"}
					if name, ok := itemMap["name"].(string); ok {
						toolCallParts = append(toolCallParts, fmt.Sprintf("Tool: %s", name))
					}
					if toolCallID, ok := itemMap["tool_call_id"].(string); ok {
						toolCallParts = append(toolCallParts, fmt.Sprintf("ID: %s", toolCallID))
					}
					if args, ok := itemMap["arguments"].(string); ok {
						toolCallParts = append(toolCallParts, fmt.Sprintf("Arguments: %s", args))
					} else if argsMap, ok := itemMap["arguments"].(map[string]interface{}); ok {
						argsJSON, err := json.MarshalIndent(argsMap, "  ", "  ")
						if err == nil {
							toolCallParts = append(toolCallParts, fmt.Sprintf("Arguments:\n%s", string(argsJSON)))
						}
					}
					textParts = append(textParts, strings.Join(toolCallParts, "\n"))
				} else if itemType == "tool" {
					// Tool response
					toolParts := []string{"[Tool Response]"}
					if name, ok := itemMap["name"].(string); ok {
						toolParts = append(toolParts, fmt.Sprintf("Tool: %s", name))
					}
					if toolCallID, ok := itemMap["tool_call_id"].(string); ok {
						toolParts = append(toolParts, fmt.Sprintf("Call ID: %s", toolCallID))
					}
					if content, ok := itemMap["content"].(string); ok {
						toolParts = append(toolParts, fmt.Sprintf("Content: %s", content))
					}
					textParts = append(textParts, strings.Join(toolParts, "\n"))
				} else if text, ok := itemMap["text"].(string); ok {
					// Regular text content
					textParts = append(textParts, text)
				} else if data, ok := itemMap["data"].(string); ok {
					// Some content items have "data" field (like redacted-reasoning)
					if itemType == "redacted-reasoning" {
						// Try to decode the redacted reasoning (may be base64url + protobuf encoded)
						decoded, wasDecoded := decodeRedactedReasoning(data)
						if wasDecoded {
							// Successfully decoded - show the actual reasoning content
							textParts = append(textParts, fmt.Sprintf("```\n[Redacted Reasoning - Decoded]\n%s\n```", decoded))
						} else if strings.Contains(decoded, "[Encrypted:") {
							// Encrypted content - show encryption message
							textParts = append(textParts, fmt.Sprintf("```\n%s\n```", decoded))
						} else {
							// Could not decode - show as-is in code block
							textParts = append(textParts, fmt.Sprintf("```\n[Redacted Reasoning]\n%s\n```", data))
						}
					} else {
						textParts = append(textParts, data)
					}
				} else {
					// Unknown content type - try to extract any readable fields
					var unknownParts []string
					if itemType != "" {
						unknownParts = append(unknownParts, fmt.Sprintf("[%s]", itemType))
					}
					// Try to extract all common fields
					if name, ok := itemMap["name"].(string); ok && name != "" {
						unknownParts = append(unknownParts, fmt.Sprintf("Name: %s", name))
					}
					if id, ok := itemMap["id"].(string); ok && id != "" {
						unknownParts = append(unknownParts, fmt.Sprintf("ID: %s", id))
					}
					if toolCallID, ok := itemMap["tool_call_id"].(string); ok && toolCallID != "" {
						unknownParts = append(unknownParts, fmt.Sprintf("Tool Call ID: %s", toolCallID))
					}
					if content, ok := itemMap["content"].(string); ok && content != "" {
						// Limit content length for readability
						contentPreview := content
						if len(contentPreview) > 500 {
							contentPreview = contentPreview[:500] + "..."
						}
						unknownParts = append(unknownParts, fmt.Sprintf("Content: %s", contentPreview))
					}
					if args, ok := itemMap["arguments"].(string); ok && args != "" {
						unknownParts = append(unknownParts, fmt.Sprintf("Arguments: %s", args))
					} else if argsMap, ok := itemMap["arguments"].(map[string]interface{}); ok {
						argsJSON, err := json.MarshalIndent(argsMap, "  ", "  ")
						if err == nil {
							argsStr := string(argsJSON)
							if len(argsStr) > 500 {
								argsStr = argsStr[:500] + "..."
							}
							unknownParts = append(unknownParts, fmt.Sprintf("Arguments:\n%s", argsStr))
						}
					}
					// If we extracted any fields, use them; otherwise just note the type
					if len(unknownParts) > 1 || (len(unknownParts) == 1 && !strings.Contains(unknownParts[0], "content]")) {
						textParts = append(textParts, strings.Join(unknownParts, "\n"))
					} else if len(unknownParts) == 1 {
						textParts = append(textParts, unknownParts[0])
					}
				}
			}
		}
		if len(textParts) > 0 {
			bubble.Text = strings.Join(textParts, "\n\n")
		}
	}

	// Extract timestamp if available
	// NOTE: cursor-agent does NOT store timestamps in individual message blobs.
	// Timestamps are only available at the session level in the meta table (key="0", field="createdAt").
	// Individual messages inherit the session createdAt timestamp.
	// Normalize to milliseconds (formatTimestamp expects milliseconds)
	if ts, ok := data["timestamp"].(float64); ok {
		bubble.Timestamp = normalizeTimestamp(int64(ts))
	} else if ts, ok := data["timestamp"].(int64); ok {
		bubble.Timestamp = normalizeTimestamp(ts)
	} else {
		// Leave timestamp as 0 - will be filled from session createdAt in meta table
		bubble.Timestamp = 0
	}

	return bubble, nil
}

func parseComposerFromData(key string, data map[string]interface{}) (*RawComposer, error) {
	composer := &RawComposer{}

	// Extract composerId
	if id, ok := data["composerId"].(string); ok {
		composer.ComposerID = id
	}

	// Extract name
	if name, ok := data["name"].(string); ok {
		composer.Name = name
	}

	// Extract fullConversationHeadersOnly
	if headers, ok := data["fullConversationHeadersOnly"].([]interface{}); ok {
		for _, h := range headers {
			if hMap, ok := h.(map[string]interface{}); ok {
				header := ConversationHeader{}
				if bubbleID, ok := hMap["bubbleId"].(string); ok {
					header.BubbleID = bubbleID
				}
				if t, ok := hMap["type"].(float64); ok {
					header.Type = int(t)
				} else if t, ok := hMap["type"].(int); ok {
					header.Type = t
				}
				composer.FullConversationHeadersOnly = append(composer.FullConversationHeadersOnly, header)
			}
		}
	}

	// Fallback to legacy format: conversation[] array
	if len(composer.FullConversationHeadersOnly) == 0 {
		// Try legacy format: conversation[] array
		if convArray, ok := data["conversation"].([]interface{}); ok && len(convArray) > 0 {
			LogInfo("Composer %s: Using legacy conversation[] format (found %d entries)", composer.ComposerID, len(convArray))
			// Convert legacy format to headers
			for _, entry := range convArray {
				if entryMap, ok := entry.(map[string]interface{}); ok {
					header := ConversationHeader{}
					if bubbleID, ok := entryMap["bubbleId"].(string); ok {
						header.BubbleID = bubbleID
					}
					if t, ok := entryMap["type"].(float64); ok {
						header.Type = int(t)
					} else if t, ok := entryMap["type"].(int); ok {
						header.Type = t
					}
					if header.BubbleID != "" {
						composer.FullConversationHeadersOnly = append(composer.FullConversationHeadersOnly, header)
					}
				}
			}
		} else {
			// Log available fields for debugging
			keys := make([]string, 0, len(data))
			for k := range data {
				keys = append(keys, k)
			}
			LogWarn("Composer %s: No conversation data found. Available fields: %v", composer.ComposerID, keys)
		}
	}

	// Extract timestamps
	if ts, ok := data["createdAt"].(float64); ok {
		composer.CreatedAt = int64(ts)
	} else if ts, ok := data["createdAt"].(int64); ok {
		composer.CreatedAt = ts
	}

	if ts, ok := data["lastUpdatedAt"].(float64); ok {
		composer.LastUpdatedAt = int64(ts)
	} else if ts, ok := data["lastUpdatedAt"].(int64); ok {
		composer.LastUpdatedAt = ts
	}

	return composer, nil
}

func parseContextFromData(key string, data map[string]interface{}) (*MessageContext, error) {
	context := &MessageContext{}

	// Extract contextId
	if id, ok := data["contextId"].(string); ok {
		context.ContextID = id
	}

	// Extract bubbleId
	if id, ok := data["bubbleId"].(string); ok {
		context.BubbleID = id
	}

	// Extract composerId
	if id, ok := data["composerId"].(string); ok {
		context.ComposerID = id
	}

	// Extract other optional fields
	if gitStatus, ok := data["gitStatusRaw"].(string); ok {
		context.GitStatusRaw = gitStatus
	}

	if terminalFiles, ok := data["terminalFiles"].([]interface{}); ok {
		for _, tf := range terminalFiles {
			if str, ok := tf.(string); ok {
				context.TerminalFiles = append(context.TerminalFiles, str)
			}
		}
	}

	if projectLayouts, ok := data["projectLayouts"].([]interface{}); ok {
		for _, pl := range projectLayouts {
			if str, ok := pl.(string); ok {
				context.ProjectLayouts = append(context.ProjectLayouts, str)
			}
		}
	}

	return context, nil
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
