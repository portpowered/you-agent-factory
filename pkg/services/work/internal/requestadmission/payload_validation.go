package requestadmission

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// MaxWorkPayloadBytes is the inclusive UTF-8 JSON byte limit for one Work
// payload at admission.
const MaxWorkPayloadBytes = 64 * 1024

// PayloadSizeError is the safe, typed admission failure for one Work payload
// whose compact JSON representation exceeds MaxWorkPayloadBytes.
type PayloadSizeError struct {
	WorkName          string
	WorkID            string
	PayloadBytes      int
	PayloadLimitBytes int
}

func (e *PayloadSizeError) Error() string {
	if e == nil {
		return ""
	}
	identity := strings.TrimSpace(e.WorkName)
	if identity == "" {
		identity = strings.TrimSpace(e.WorkID)
	}
	if identity == "" {
		identity = "<unnamed>"
	}
	return fmt.Sprintf(
		"work_request: Work %q payload exceeds byte limit: payloadBytes=%d payloadLimitBytes=%d",
		identity,
		e.PayloadBytes,
		e.PayloadLimitBytes,
	)
}

func validateWorkPayloadSize(workName, workID string, payload any) error {
	encoded, err := compactWorkPayloadJSON(payload)
	if err != nil {
		return fmt.Errorf("work_request: Work %q payload could not be encoded: %w", workName, err)
	}
	if len(encoded) == 0 || len(encoded) <= MaxWorkPayloadBytes {
		return nil
	}
	return &PayloadSizeError{
		WorkName:          workName,
		WorkID:            workID,
		PayloadBytes:      len(encoded),
		PayloadLimitBytes: MaxWorkPayloadBytes,
	}
}

func workPayloadForAdmissionSize(content []ContentPart, payload any) (any, error) {
	raw, err := rawWorkPayload(payload)
	if err != nil {
		return nil, err
	}
	if len(raw) > 0 {
		return raw, nil
	}

	text, hasText, err := legacyTextPayloadFromCanonicalContent(content)
	if err != nil {
		return nil, err
	}
	if hasText {
		return text, nil
	}
	return nil, nil
}

func compactWorkPayloadJSON(payload any) ([]byte, error) {
	raw, err := rawWorkPayload(payload)
	if err != nil {
		return nil, err
	}
	return compactRawWorkPayload(raw)
}

func compactRawWorkPayload(raw []byte) ([]byte, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, nil
	}
	if !json.Valid(raw) {
		return json.Marshal(string(raw))
	}

	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		return nil, err
	}
	return compact.Bytes(), nil
}
