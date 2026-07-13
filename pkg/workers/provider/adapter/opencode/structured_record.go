package opencode

import (
	"encoding/json"
	"strings"
	"unicode/utf8"
)

const (
	maxCorrelationLength  = 256
	maxPublishedTextBytes = 256 * 1024
)

type structuredRecord struct {
	Type      string          `json:"type"`
	SessionID string          `json:"sessionID"`
	Part      structuredPart  `json:"part"`
	Error     structuredError `json:"error"`
}

type structuredPart struct {
	ID        string          `json:"id"`
	MessageID string          `json:"messageID"`
	SessionID string          `json:"sessionID"`
	CallID    string          `json:"callID"`
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	Tool      string          `json:"tool"`
	Time      structuredTime  `json:"time"`
	State     structuredState `json:"state"`
	Tokens    structuredUsage `json:"tokens"`
}

type structuredTime struct {
	Start *int64 `json:"start"`
	End   *int64 `json:"end"`
}

type structuredState struct {
	Status string `json:"status"`
}

type structuredUsage struct {
	Input     int64 `json:"input"`
	Output    int64 `json:"output"`
	Reasoning int64 `json:"reasoning"`
}

type structuredError struct {
	Name string          `json:"name"`
	Data json.RawMessage `json:"data"`
}

func decodeStructuredRecord(raw []byte) (structuredRecord, error) {
	var record structuredRecord
	err := json.Unmarshal(raw, &record)
	record.Type = strings.TrimSpace(record.Type)
	return record, err
}

func recordSessionID(record structuredRecord) string {
	if validCorrelation(record.SessionID) {
		return record.SessionID
	}
	if validCorrelation(record.Part.SessionID) {
		return record.Part.SessionID
	}
	return ""
}

func validCorrelation(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > maxCorrelationLength || strings.Contains(value, "..") {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '-' || char == '_' || char == '.' {
			continue
		}
		return false
	}
	return true
}

func boundedText(value string) string {
	if len(value) <= maxPublishedTextBytes {
		return value
	}
	end := maxPublishedTextBytes
	for end > 0 && !utf8.ValidString(value[:end]) {
		end--
	}
	return value[:end]
}
