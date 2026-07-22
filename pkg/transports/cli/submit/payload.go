package submit

import (
	"encoding/json"
	"fmt"

	workdomain "github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
)

func readSubmitPayload(read workdomain.PayloadFileReader, payloadPath string) (payload json.RawMessage, raw []byte, payloadType string, err error) {
	if read == nil {
		return nil, nil, "", fmt.Errorf("Work payload file reader is required")
	}
	raw, err = read(payloadPath)
	if err != nil {
		return nil, nil, "", err
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
