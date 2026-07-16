package responsestreamremovalgate

import (
	"encoding/json"
	"fmt"

	parityfixtures "github.com/portpowered/infinite-you/internal/testutil/providerparity"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/responsestream/ndjsoncontract"
)

// AssertPrivateNDJSONRecordTypesRejected proves the canonical CLI NDJSON decoder
// rejects every retired private recordType on the public contract surface.
func AssertPrivateNDJSONRecordTypesRejected() error {
	for _, recordType := range ndjsoncontract.RetiredPrivateRecordTypes {
		line, err := json.Marshal(map[string]string{"recordType": recordType})
		if err != nil {
			return fmt.Errorf("marshal retired record fixture %q: %w", recordType, err)
		}
		if _, _, err := parityfixtures.DecodeTransportCLINDJSON([]string{string(line)}); err == nil {
			return fmt.Errorf("decoder accepted retired private recordType %q", recordType)
		}
		finalLine := fmt.Sprintf(
			`{"recordType":%q,"invocation":{"requestId":"req-retired","status":"COMPLETED"}}`,
			recordType,
		)
		if _, _, err := parityfixtures.DecodeTransportCLINDJSON([]string{
			`{"recordType":"response_event","event":{}}`,
			finalLine,
		}); err == nil {
			return fmt.Errorf("decoder accepted retired private final recordType %q", recordType)
		}
	}
	return nil
}
