package events

import (
	"encoding/json"
	"strings"
	"testing"
)

const factoryEventRecordType = "factory_event"

type ndjsonRecord struct {
	RecordType string
	Payload    json.RawMessage
	Raw        string
}

func decodeNDJSONRecords(t *testing.T, stdout string) []ndjsonRecord {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	records := make([]ndjsonRecord, 0, len(lines))
	for index, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &fields); err != nil {
			t.Fatalf("decode NDJSON record %d: %v\nline: %s", index, err, line)
		}
		var recordType string
		if err := json.Unmarshal(fields["recordType"], &recordType); err != nil {
			t.Fatalf("decode recordType for record %d: %v\nline: %s", index, err, line)
		}
		payloadKey := "event"
		if recordType == "invocation_result" {
			payloadKey = "response"
		}
		if len(fields) != 2 || len(fields[payloadKey]) == 0 {
			t.Fatalf("record %d fields = %v, want only recordType and %s", index, mapKeys(fields), payloadKey)
		}
		records = append(records, ndjsonRecord{RecordType: recordType, Payload: fields[payloadKey], Raw: line})
	}
	return records
}

func mapKeys(values map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}
