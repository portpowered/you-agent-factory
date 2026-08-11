package generatedartifacts

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// DecodeJSONRejectingDuplicateKeys decodes JSON while rejecting repeated
// object keys before the standard decoder can silently keep the last value.
func DecodeJSONRejectingDuplicateKeys(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := rejectDuplicateJSONKeys(decoder, "$"); err != nil {
		return err
	}
	if token, err := decoder.Token(); err != io.EOF {
		if err != nil {
			return fmt.Errorf("decode trailing JSON: %w", err)
		}
		return fmt.Errorf("unexpected trailing JSON value %v", token)
	}
	return json.Unmarshal(raw, target)
}

func rejectDuplicateJSONKeys(decoder *json.Decoder, path string) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	if delimiter == '{' {
		seen := map[string]struct{}{}
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key := keyToken.(string)
			if _, exists := seen[key]; exists {
				return fmt.Errorf("duplicate object key %q at %s", key, path)
			}
			seen[key] = struct{}{}
			if err := rejectDuplicateJSONKeys(decoder, path+"."+key); err != nil {
				return err
			}
		}
		return expectJSONDelimiter(decoder, '}', path)
	}
	if delimiter != '[' {
		return nil
	}
	for index := 0; decoder.More(); index++ {
		if err := rejectDuplicateJSONKeys(decoder, fmt.Sprintf("%s[%d]", path, index)); err != nil {
			return err
		}
	}
	return expectJSONDelimiter(decoder, ']', path)
}

func expectJSONDelimiter(decoder *json.Decoder, want json.Delim, path string) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if token != want {
		return fmt.Errorf("expected JSON delimiter %q at %s, got %v", want, path, token)
	}
	return nil
}
