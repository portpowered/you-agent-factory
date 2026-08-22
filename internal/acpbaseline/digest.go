package acpbaseline

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// MaxDigestStringLength is the point past which a preserved-key string is
// replaced by a length-and-hash marker rather than kept.
const MaxDigestStringLength = 120

// protocolKeys are the only keys whose string values survive digesting.
//
// Everything else becomes a length marker. This is what makes a committed
// digest both reviewable and structurally incapable of carrying a prompt, a
// file body, or a credential: the structure that a comparison needs is
// preserved, and the content that must never be committed is not.
var protocolKeys = map[string]bool{
	"jsonrpc": true, "method": true, "sessionUpdate": true, "type": true,
	"kind": true, "status": true, "stopReason": true, "category": true,
	"role": true, "outcome": true, "protocolVersion": true, "priority": true,
	"name": true, "id": true, "configId": true, "optionId": true, "modeId": true,
}

// DigestValue reduces one decoded JSON value to its structural digest.
//
// Object keys, array lengths, booleans, numbers, and nulls are preserved
// exactly, because those are the signal a capability comparison reads.
func DigestValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		digested := make(map[string]any, len(typed))
		for key, nested := range typed {
			if text, ok := nested.(string); ok {
				// A protocol-significant value is the signal a comparison
				// reads, so it survives verbatim. Everything else becomes a
				// length marker. Recursing here instead would digest it too,
				// because the generic string case cannot see its key.
				if protocolKeys[key] {
					digested[key] = text
					continue
				}
				digested[key] = digestString(text)
				continue
			}
			digested[key] = DigestValue(nested)
		}
		return digested
	case []any:
		digested := make([]any, 0, len(typed))
		for _, nested := range typed {
			digested = append(digested, DigestValue(nested))
		}
		return digested
	case string:
		return digestString(typed)
	default:
		return value
	}
}

func digestString(text string) string {
	if len(text) > MaxDigestStringLength {
		sum := sha256.Sum256([]byte(text))
		return fmt.Sprintf("<text:len=%d,sha256=%s>", len(text), hex.EncodeToString(sum[:4]))
	}
	return fmt.Sprintf("<str:len=%d>", len(text))
}

// DigestRecordLine reduces one raw transcript record to its committable form.
//
// Timestamps become an ordinal so a re-capture with unchanged behavior diffs
// to nothing, and the frame is replaced by its structural digest.
func DigestRecordLine(raw []byte, ordinal int) ([]byte, error) {
	var record map[string]any
	if err := json.Unmarshal(raw, &record); err != nil {
		return nil, err
	}
	delete(record, "t")
	record["seq"] = ordinal

	if frame, ok := record["frame"]; ok {
		record["frame"] = DigestValue(frame)
	}
	if text, ok := record["text"].(string); ok {
		record["text"] = digestString(text)
	}
	return json.Marshal(record)
}

// ContainsRawContent reports whether a digested line still carries a raw
// string value, which would mean an undigested transcript was about to be
// committed. It is the enforcement behind the commit policy: prose alone would
// not survive.
func ContainsRawContent(raw []byte) (string, bool) {
	var record map[string]any
	if err := json.Unmarshal(raw, &record); err != nil {
		return "", false
	}
	return findRawString(record, "")
}

func findRawString(value any, path string) (string, bool) {
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if text, ok := typed[key].(string); ok {
				if protocolKeys[key] || isDigestMarker(text) || isEnvelopeKey(key) {
					continue
				}
				return path + "." + key, true
			}
			if found, ok := findRawString(typed[key], path+"."+key); ok {
				return found, true
			}
		}
	case []any:
		for index, nested := range typed {
			if found, ok := findRawString(nested, fmt.Sprintf("%s[%d]", path, index)); ok {
				return found, true
			}
		}
	}
	return "", false
}

func isDigestMarker(text string) bool {
	return strings.HasPrefix(text, "<str:len=") ||
		strings.HasPrefix(text, "<text:len=") ||
		text == "<redacted>"
}

// isEnvelopeKey names the record's own transport fields, which are bounded
// enums and identities rather than payload.
func isEnvelopeKey(key string) bool {
	switch key {
	case "conn", "peer", "dir", "stream", "err", "v":
		return true
	}
	return false
}
