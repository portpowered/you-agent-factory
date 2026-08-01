package common_test

import (
	"io"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/factory_visualization/transports/http/common"
)

const wantObjectValidationMessage = "request payload must contain one JSON object"

func TestDecodeStrictJSONRejectsNonObjectPayloads(t *testing.T) {
	t.Parallel()

	for _, payload := range []string{"null", "[]", `"text"`, "true"} {
		payload := payload
		t.Run(payload, func(t *testing.T) {
			t.Parallel()

			_, err := common.DecodeStrictJSON[struct{}](strings.NewReader(payload))
			if err == nil {
				t.Fatalf("DecodeStrictJSON(%q) = nil, want object validation error", payload)
			}
			if !common.IsDecodeError(err) {
				t.Fatalf("DecodeStrictJSON(%q) error = %T, want DecodeError", payload, err)
			}
			if message, ok := common.RequestFieldValidationMessage(err); !ok || message != wantObjectValidationMessage {
				t.Fatalf("validation message = %q, ok = %v, want stable object validation message", message, ok)
			}
		})
	}
}

func TestDecodeStrictJSONRejectsNilBody(t *testing.T) {
	t.Parallel()

	_, err := common.DecodeStrictJSON[struct{}](nil)
	if err == nil {
		t.Fatal("DecodeStrictJSON(nil) = nil, want error")
	}
	if !common.IsDecodeError(err) {
		t.Fatalf("DecodeStrictJSON(nil) error = %T, want DecodeError", err)
	}
}

func TestDecodeOptionalJSONRequestAllowsNilAndEmptyBodyButRejectsNull(t *testing.T) {
	t.Parallel()

	for name, body := range map[string]io.Reader{
		"nil":   nil,
		"empty": strings.NewReader("  \n"),
	} {
		name, body := name, body
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			value, err := common.DecodeOptionalJSONRequest(body, func() struct{} { return struct{}{} })
			if err != nil {
				t.Fatalf("DecodeOptionalJSONRequest: %v", err)
			}
			if value != (struct{}{}) {
				t.Fatalf("value = %#v, want zero value", value)
			}
		})
	}

	_, err := common.DecodeOptionalJSONRequest(strings.NewReader("null"), func() struct{} { return struct{}{} })
	if err == nil {
		t.Fatal("DecodeOptionalJSONRequest(null) = nil, want object validation error")
	}
	if message, ok := common.RequestFieldValidationMessage(err); !ok || message != wantObjectValidationMessage {
		t.Fatalf("validation message = %q, ok = %v, want stable object validation message", message, ok)
	}
}
