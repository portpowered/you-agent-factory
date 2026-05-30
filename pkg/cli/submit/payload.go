package submit

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/portpowered/infinite-you/pkg/cli/clidiag"
)

func readSubmitPayload(payloadPath string) (payload json.RawMessage, raw []byte, payloadType string, err error) {
	raw, err = os.ReadFile(payloadPath)
	if err != nil {
		return nil, nil, "", fmt.Errorf("read payload file: %w", err)
	}

	payloadType = clidiag.PayloadType(payloadPath)
	if payloadType == "json" {
		if !json.Valid(raw) {
			return nil, nil, "", fmt.Errorf("payload file is not valid JSON: %s", payloadPath)
		}
		return raw, raw, payloadType, nil
	}

	encoded, err := json.Marshal(string(raw))
	if err != nil {
		return nil, nil, "", fmt.Errorf("encode payload: %w", err)
	}
	return encoded, raw, payloadType, nil
}
