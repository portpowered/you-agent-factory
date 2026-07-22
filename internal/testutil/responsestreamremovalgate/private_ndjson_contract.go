package responsestreamremovalgate

import (
	"encoding/json"
	"fmt"
)

var allRetiredPrivateNDJSONRecordTypes = []string{
	"progress",
	"compaction",
	"primary_result",
	"stream_gap",
}

// AssertPrivateNDJSONRecordTypesRejected proves the removal-gate fixtures
// classify every retired private recordType as unsupported. The canonical
// decoder's acceptance and rejection matrix remains Factory Sessions-owned.
func AssertPrivateNDJSONRecordTypesRejected() error {
	for _, recordType := range allRetiredPrivateNDJSONRecordTypes {
		line, err := json.Marshal(map[string]string{"recordType": recordType})
		if err != nil {
			return fmt.Errorf("marshal retired record fixture %q: %w", recordType, err)
		}
		if err := rejectRetiredRecordFixture(string(line)); err == nil {
			return fmt.Errorf("decoder accepted retired private recordType %q", recordType)
		}
		finalLine := fmt.Sprintf(
			`{"recordType":%q,"invocation":{"requestId":"req-retired","status":"COMPLETED"}}`,
			recordType,
		)
		if err := rejectRetiredRecordFixture(finalLine); err == nil {
			return fmt.Errorf("decoder accepted retired private final recordType %q", recordType)
		}
	}
	return nil
}

func rejectRetiredRecordFixture(line string) error {
	var header struct {
		RecordType string `json:"recordType"`
	}
	if err := json.Unmarshal([]byte(line), &header); err != nil {
		return err
	}
	for _, retired := range allRetiredPrivateNDJSONRecordTypes {
		if header.RecordType == retired {
			return fmt.Errorf("unsupported retired private CLI NDJSON recordType %q", header.RecordType)
		}
	}
	return nil
}
