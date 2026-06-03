package cursorstorage

import (
	"encoding/json"
	"strings"
)

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
