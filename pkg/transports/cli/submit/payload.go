package submit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	workdomain "github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
)

func readSubmitPayload(
	read workdomain.PayloadFileReader,
	payloadPath string,
	stdin io.Reader,
) (payload json.RawMessage, raw []byte, payloadType string, err error) {
	if read == nil {
		return nil, nil, "", fmt.Errorf("Work payload file reader is required")
	}
	if strings.TrimSpace(payloadPath) == "-" {
		raw, err = readSubmitStdinPayload(stdin)
		if err != nil {
			return nil, nil, "", err
		}
		payloadType = classifySubmitPayloadBytes(raw)
	} else {
		raw, err = read(payloadPath)
		if err != nil {
			return nil, nil, "", err
		}
		payloadType = clidiag.PayloadType(payloadPath)
	}

	if payloadType == "json" {
		if !json.Valid(raw) {
			label := payloadPath
			if label == "-" {
				label = "stdin"
			}
			return nil, nil, "", fmt.Errorf("payload file is not valid JSON: %s", label)
		}
		return raw, raw, payloadType, nil
	}

	encoded, err := json.Marshal(string(raw))
	if err != nil {
		return nil, nil, "", fmt.Errorf("encode payload: %w", err)
	}
	return encoded, raw, payloadType, nil
}

func readSubmitStdinPayload(stdin io.Reader) ([]byte, error) {
	if stdin == nil {
		return nil, fmt.Errorf("read payload stdin: process stdin reader is required")
	}
	data, err := io.ReadAll(stdin)
	if err != nil {
		return nil, fmt.Errorf("read payload stdin: %w", err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, fmt.Errorf("payload stdin input is empty")
	}
	return data, nil
}

func classifySubmitPayloadBytes(raw []byte) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) > 0 && trimmed[0] == '{' && json.Valid(trimmed) {
		return "json"
	}
	return "markdown"
}
