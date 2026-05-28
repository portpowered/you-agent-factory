package apitypes

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// Int64String serializes an int64 as a JSON decimal string while accepting
// numeric JSON values for compatibility with older clients.
type Int64String int64

func (value Int64String) MarshalJSON() ([]byte, error) {
	return json.Marshal(strconv.FormatInt(int64(value), 10))
}

func (value *Int64String) UnmarshalJSON(data []byte) error {
	if value == nil {
		return fmt.Errorf("unmarshal Int64String: nil receiver")
	}

	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		parsed, parseErr := strconv.ParseInt(text, 10, 64)
		if parseErr != nil {
			return fmt.Errorf("parse quoted int64 %q: %w", text, parseErr)
		}
		if parsed < 0 {
			return fmt.Errorf("parse quoted int64 %q: value must be non-negative", text)
		}
		*value = Int64String(parsed)
		return nil
	}

	var number json.Number
	if err := json.Unmarshal(data, &number); err != nil {
		return fmt.Errorf("unmarshal int64 string: %w", err)
	}
	parsed, err := strconv.ParseInt(number.String(), 10, 64)
	if err != nil {
		return fmt.Errorf("parse numeric int64 %q: %w", number.String(), err)
	}
	if parsed < 0 {
		return fmt.Errorf("parse numeric int64 %q: value must be non-negative", number.String())
	}
	*value = Int64String(parsed)
	return nil
}

func (value Int64String) Int64() int64 {
	return int64(value)
}
