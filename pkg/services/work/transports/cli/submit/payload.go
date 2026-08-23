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

// maxSubmitPayloadStdinBytes is the inclusive source-byte cap for a payload
// deliberately supplied through stdin. The same limit is also applied below
// to the compact JSON representation that Work admission measures.
const maxSubmitPayloadStdinBytes = workdomain.MaxWorkPayloadBytes

func readSubmitPayload(
	read workdomain.PayloadFileReader,
	payloadPath string,
	stdin io.Reader,
) (payload json.RawMessage, raw []byte, payloadType string, err error) {
	if read == nil {
		return nil, nil, "", fmt.Errorf("Work payload file reader is required")
	}
	stdinPayload := strings.TrimSpace(payloadPath) == "-"
	if stdinPayload {
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
		if stdinPayload {
			if err := validateSubmitStdinPayloadSize(raw); err != nil {
				return nil, nil, "", err
			}
		}
		return raw, raw, payloadType, nil
	}

	encoded, err := json.Marshal(string(raw))
	if err != nil {
		return nil, nil, "", fmt.Errorf("encode payload: %w", err)
	}
	if stdinPayload {
		if err := validateSubmitStdinPayloadSize(encoded); err != nil {
			return nil, nil, "", err
		}
	}
	return encoded, raw, payloadType, nil
}

func readSubmitStdinPayload(stdin io.Reader) ([]byte, error) {
	data, err := readBoundedStdin(
		stdin,
		maxSubmitPayloadStdinBytes,
		"payload stdin",
		"use a payload file for larger input",
	)
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, fmt.Errorf("payload stdin input is empty")
	}
	return data, nil
}

// validateSubmitStdinPayloadSize applies the Work admission boundary to the
// compact JSON bytes that the CLI will send. Text stdin is JSON-encoded after
// it is read, so checking only the source bytes would let escaping expand the
// request past Work's limit.
func validateSubmitStdinPayloadSize(payload []byte) error {
	var compact bytes.Buffer
	if err := json.Compact(&compact, payload); err != nil {
		return fmt.Errorf("validate payload stdin size: %w", err)
	}
	if compact.Len() <= maxSubmitPayloadStdinBytes {
		return nil
	}
	return fmt.Errorf(
		"payload stdin compact JSON exceeds the %d-byte Work payload limit (encoded size %d); use a payload file for larger input",
		maxSubmitPayloadStdinBytes,
		compact.Len(),
	)
}

func classifySubmitPayloadBytes(raw []byte) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) > 0 && trimmed[0] == '{' && json.Valid(trimmed) {
		return "json"
	}
	return "markdown"
}
