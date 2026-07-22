package cursor

import (
	"encoding/json"
	"fmt"
	"strings"

	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
)

// LoadSessionFromStoreDB loads session data from a single store.db file.
func LoadSessionFromStoreDB(files providersessions.FileSystem, openSQLDatabase providersessions.CursorOpenSQLDatabase, dbPath string) (map[string]*RawBubble, []*RawComposer, map[string][]*MessageContext, SessionTokenUsage, error) {
	db, err := OpenDatabase(files, openSQLDatabase, dbPath)
	if err != nil {
		return nil, nil, nil, SessionTokenUsage{}, fmt.Errorf("failed to open store.db: %w", err)
	}
	defer func() { _ = db.Close() }()

	blobs, err := QueryBlobsTable(db)
	if err != nil {
		return nil, nil, nil, SessionTokenUsage{}, fmt.Errorf("failed to query blobs table: %w", err)
	}

	meta, err := QueryMetaTable(db)
	if err != nil {
		return nil, nil, nil, SessionTokenUsage{}, fmt.Errorf("failed to query meta table: %w", err)
	}

	sessionID := extractSessionIDFromPath(dbPath)
	bubbles := make(map[string]*RawBubble)
	var composers []*RawComposer
	contexts := make(map[string][]*MessageContext)
	var sessionTokenUsage SessionTokenUsage

	jsonParseFailures := loadSessionBlobs(blobs, sessionID, bubbles, &composers, &sessionTokenUsage)
	if jsonParseFailures > 0 {
		LogWarn("Failed to parse %d/%d blobs as JSON", jsonParseFailures, len(blobs))
	}

	var sessionMeta sessionMetaFields
	metaParseFailures := loadSessionMeta(meta, contexts, &sessionTokenUsage, &sessionMeta)
	if metaParseFailures > 0 {
		LogWarn("Failed to parse %d/%d meta entries as JSON", metaParseFailures, len(meta))
	}

	applySessionMetadata(bubbles, composers, sessionMeta)

	LogInfo("LoadSessionFromStoreDB summary: %d blobs queried, %d meta queried, %d bubbles extracted, %d composers extracted, %d contexts extracted",
		len(blobs), len(meta), len(bubbles), len(composers), len(contexts))

	return bubbles, composers, contexts, sessionTokenUsage, nil
}

func loadSessionBlobs(
	blobs []BlobEntry,
	sessionID string,
	bubbles map[string]*RawBubble,
	composers *[]*RawComposer,
	tokenUsage *SessionTokenUsage,
) int {
	jsonParseFailures := 0
	for i, blob := range blobs {
		data, earlyBubble, ok := decodeBlobEntryValue(blob, i, sessionID, &jsonParseFailures)
		if !ok {
			continue
		}
		if earlyBubble != nil {
			bubbles[earlyBubble.BubbleID] = earlyBubble
			continue
		}
		ingestDecodedBlobData(blob, i, data, sessionID, bubbles, composers, tokenUsage)
	}
	return jsonParseFailures
}

func loadSessionMeta(
	meta []MetaEntry,
	contexts map[string][]*MessageContext,
	tokenUsage *SessionTokenUsage,
	sessionMeta *sessionMetaFields,
) int {
	metaJSONParseFailures := 0
	for i, entry := range meta {
		data, ok := decodeMetaEntryValue(entry, i, &metaJSONParseFailures)
		if !ok {
			continue
		}

		if i < 3 {
			keys := make([]string, 0, len(data))
			for k := range data {
				keys = append(keys, k)
			}
			LogInfo("Meta %d (key='%s') parsed successfully. Available fields: %v", i+1, entry.Key, keys)
		}
		mergeSessionTokenUsage(tokenUsage, tokenUsageFromData(data))

		fields := extractSessionMetaFromEntry(entry, data)
		if fields.createdAt > 0 {
			sessionMeta.createdAt = fields.createdAt
		}
		if fields.agentID != "" {
			sessionMeta.agentID = fields.agentID
		}
		if fields.name != "" {
			sessionMeta.name = fields.name
		}

		if _, ok := data["contextId"].(string); ok {
			context, err := parseContextFromData(entry.Key, data)
			if err == nil {
				composerID := context.ComposerID
				if composerID == "" {
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
	return metaJSONParseFailures
}

func applySessionMetadata(bubbles map[string]*RawBubble, composers []*RawComposer, sessionMeta sessionMetaFields) {
	if sessionMeta.createdAt > 0 {
		for bubbleID, bubble := range bubbles {
			if bubble.Timestamp == 0 {
				bubble.Timestamp = sessionMeta.createdAt
				bubbles[bubbleID] = bubble
				LogInfo("Applied session createdAt (%d) to bubble %s (was missing timestamp)", sessionMeta.createdAt, bubbleID)
			}
		}
	}

	if sessionMeta.createdAt == 0 && sessionMeta.name == "" {
		return
	}

	for i := range composers {
		if sessionMeta.createdAt > 0 && composers[i].CreatedAt == 0 {
			composers[i].CreatedAt = sessionMeta.createdAt
			LogInfo("Applied session createdAt (%d) to composer %s", sessionMeta.createdAt, composers[i].ComposerID)
		}
		if sessionMeta.name != "" && composers[i].Name == "" {
			composers[i].Name = sessionMeta.name
			LogInfo("Applied session name (%s) to composer %s", sessionMeta.name, composers[i].ComposerID)
		}
	}
}

// ingestDecodedBlobData maps decoded blob JSON into bubbles, composers, and token usage.
func ingestDecodedBlobData(
	blob BlobEntry,
	index int,
	data map[string]interface{},
	sessionID string,
	bubbles map[string]*RawBubble,
	composers *[]*RawComposer,
	tokenUsage *SessionTokenUsage,
) {
	if index < 5 {
		keys := make([]string, 0, len(data))
		for k := range data {
			keys = append(keys, k)
		}
		LogInfo("Blob %d (key='%s') parsed successfully. Available fields: %v", index+1, blob.Key, keys)
	}
	mergeSessionTokenUsage(tokenUsage, tokenUsageFromData(data))

	if _, ok := data["bubbleId"].(string); ok {
		bubble, err := parseBubbleFromData(blob.Key, data, sessionID)
		if err == nil {
			bubbles[bubble.BubbleID] = bubble
		}
		return
	}

	if id, ok := data["id"].(string); ok {
		if role, hasRole := data["role"].(string); hasRole {
			bubble, err := parseMessageToBubble(blob.Key, id, role, data, sessionID)
			if err == nil {
				bubbles[bubble.BubbleID] = bubble
				LogInfo("Blob %d converted message (id='%s', role='%s') to bubble (bubbleId='%s')", index+1, id, role, bubble.BubbleID)
			} else {
				LogWarn("Blob %d failed to convert message to bubble: %v", index+1, err)
			}
		}
		return
	}

	if role, hasRole := data["role"].(string); hasRole {
		generatedID := blob.Key
		if len(blob.Key) > 16 {
			generatedID = blob.Key[:16]
		}
		bubble, err := parseMessageToBubble(blob.Key, generatedID, role, data, sessionID)
		if err == nil {
			bubbles[bubble.BubbleID] = bubble
			LogInfo("Blob %d converted message (no id, role='%s') to bubble (bubbleId='%s')", index+1, role, bubble.BubbleID)
		} else {
			LogWarn("Blob %d failed to convert message to bubble: %v", index+1, err)
		}
		return
	}

	if composerID, ok := data["composerId"].(string); ok {
		composer, err := parseComposerFromData(blob.Key, data)
		if err != nil {
			LogWarn("Failed to parse composer from blob key %s: %v", blob.Key, err)
			return
		}
		if composer.ComposerID == "" {
			LogWarn("Composer parsed but missing composerId. Blob key: %s", blob.Key)
			return
		}
		composer.ComposerID = composerID
		headerCount := len(composer.FullConversationHeadersOnly)
		LogInfo("Parsed composer %s: %d headers, name='%s'", composer.ComposerID, headerCount, composer.Name)
		*composers = append(*composers, composer)
	}
}

type sessionMetaFields struct {
	createdAt int64
	agentID   string
	name      string
}

func decodeMetaEntryValue(entry MetaEntry, index int, jsonParseFailures *int) (map[string]interface{}, bool) {
	var data map[string]interface{}
	valueBytes := []byte(entry.Value)

	if err := json.Unmarshal(valueBytes, &data); err != nil {
		decoded, decodeErr := tryBase64Decode(entry.Value)
		if decodeErr == nil {
			if jsonErr := json.Unmarshal(decoded, &data); jsonErr == nil {
				LogInfo("Meta %d (key='%s') was base64 encoded, decoded successfully", index+1, entry.Key)
			} else {
				hexDecoded, hexErr := tryHexDecode(entry.Value)
				if hexErr == nil {
					if jsonErr := json.Unmarshal(hexDecoded, &data); jsonErr == nil {
						LogInfo("Meta %d (key='%s') was hex encoded, decoded successfully", index+1, entry.Key)
					} else {
						(*jsonParseFailures)++
						logMetaDecodeFailure(index, entry, jsonErr)
						return nil, false
					}
				} else {
					(*jsonParseFailures)++
					logMetaDecodeFailure(index, entry, jsonErr)
					return nil, false
				}
			}
		} else {
			hexDecoded, hexErr := tryHexDecode(entry.Value)
			if hexErr == nil {
				if jsonErr := json.Unmarshal(hexDecoded, &data); jsonErr == nil {
					LogInfo("Meta %d (key='%s') was hex encoded, decoded successfully", index+1, entry.Key)
				} else {
					(*jsonParseFailures)++
					logMetaDecodeFailureExtended(index, entry, err, jsonErr)
					return nil, false
				}
			} else {
				(*jsonParseFailures)++
				logMetaDecodeFailureExtended(index, entry, err, nil)
				return nil, false
			}
		}
	}

	return data, true
}

func logMetaDecodeFailure(index int, entry MetaEntry, jsonErr error) {
	if index >= 5 {
		return
	}
	valuePreview := entry.Value
	if len(valuePreview) > 100 {
		valuePreview = valuePreview[:100] + "..."
	}
	LogWarn("Meta %d (key='%s') failed JSON parse (tried base64 and hex): %v. Value preview: %s", index+1, entry.Key, jsonErr, valuePreview)
}

func logMetaDecodeFailureExtended(index int, entry MetaEntry, err error, jsonErr error) {
	if index >= 10 {
		return
	}
	valuePreview := entry.Value
	fullValue := entry.Value
	if len(valuePreview) > 200 {
		valuePreview = valuePreview[:200] + "..."
	}
	if jsonErr != nil {
		LogWarn("Meta %d (key='%s', key_len=%d) failed JSON parse (tried hex): %v", index+1, entry.Key, len(entry.Key), jsonErr)
	} else {
		LogWarn("Meta %d (key='%s', key_len=%d) failed JSON parse: %v", index+1, entry.Key, len(entry.Key), err)
	}
	LogInfo("  Value (len=%d): %s", len(fullValue), valuePreview)
	if strings.HasPrefix(fullValue, "/") || strings.Contains(fullValue, "$") {
		LogInfo("  Value appears to be a path/reference, not JSON data")
	}
}

func extractSessionMetaFromEntry(entry MetaEntry, data map[string]interface{}) sessionMetaFields {
	var fields sessionMetaFields
	if entry.Key != "0" {
		return fields
	}
	if ts, ok := data["createdAt"].(float64); ok {
		fields.createdAt = int64(ts)
		LogInfo("Meta: Extracted session createdAt: %d (from meta key='0')", fields.createdAt)
	} else if ts, ok := data["createdAt"].(int64); ok {
		fields.createdAt = ts
		LogInfo("Meta: Extracted session createdAt: %d (from meta key='0')", fields.createdAt)
	}
	if agentID, ok := data["agentId"].(string); ok {
		fields.agentID = agentID
		LogInfo("Meta: Extracted session agentId: %s (from meta key='0')", fields.agentID)
	}
	if name, ok := data["name"].(string); ok {
		fields.name = name
		LogInfo("Meta: Extracted session name: %s (from meta key='0')", fields.name)
	}
	return fields
}
