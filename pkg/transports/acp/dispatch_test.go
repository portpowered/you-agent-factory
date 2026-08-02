package acp_test

import (
	"encoding/json"
	"reflect"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"

	"github.com/portpowered/infinite-you/pkg/transports/acp"
)

func TestClassifyMethod(t *testing.T) {
	cases := []struct {
		method string
		want   acp.MethodDisposition
	}{
		{"session/new", acp.MethodDispositionDeferred},
		{"session/prompt", acp.MethodDispositionDeferred},
		{"session/request_permission", acp.MethodDispositionDeferred},
		{"initialize", acp.MethodDispositionDeferred},
		{"totally/unknown", acp.MethodDispositionUnknown},
		{"", acp.MethodDispositionUnknown},
	}
	for _, tc := range cases {
		got := acp.ClassifyMethod(tc.method)
		if got != tc.want {
			t.Errorf("ClassifyMethod(%q) = %q, want %q", tc.method, got, tc.want)
		}
	}
}

func TestUnsupportedMethodResponseWithWireID(t *testing.T) {
	cases := []struct {
		name   string
		method string
		wireID acpsdk.RequestId
	}{
		{"unknown method, string id", "totally/unknown", stringWireID("abc")},
		{"deferred method, numeric id", "session/new", numberWireID(42)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			outcome := acp.UnsupportedMethodResponse(tc.method, &tc.wireID)
			if outcome == nil {
				t.Fatalf("UnsupportedMethodResponse(%q) = nil, want non-nil outcome", tc.method)
			}
			if outcome.Error == nil {
				t.Fatalf("UnsupportedMethodResponse(%q).Error = nil, want method-not-found error", tc.method)
			}
			if outcome.Error.Code != -32601 {
				t.Fatalf("UnsupportedMethodResponse(%q).Error.Code = %d, want -32601", tc.method, outcome.Error.Code)
			}
			if outcome.ResponseID != tc.wireID {
				t.Fatalf("UnsupportedMethodResponse(%q).ResponseID = %+v, want original id %+v", tc.method, outcome.ResponseID, tc.wireID)
			}
		})
	}
}

func TestUnsupportedMethodResponseForNotificationEmitsNoResponse(t *testing.T) {
	outcome := acp.UnsupportedMethodResponse("totally/unknown", nil)
	if outcome != nil {
		t.Fatalf("UnsupportedMethodResponse(notification) = %+v, want nil (no response emitted)", outcome)
	}
}

type decodeParamsFixture struct {
	Foo string `json:"foo"`
}

func TestDecodeMethodParamsSuccess(t *testing.T) {
	got, decodeErr := acp.DecodeMethodParams[decodeParamsFixture](json.RawMessage(`{"foo":"bar"}`))
	if decodeErr != nil {
		t.Fatalf("DecodeMethodParams() unexpected error: %v", decodeErr)
	}
	if got.Foo != "bar" {
		t.Fatalf("DecodeMethodParams() = %+v, want Foo=bar", got)
	}
}

func TestDecodeMethodParamsRejectsMissingParams(t *testing.T) {
	cases := []json.RawMessage{nil, json.RawMessage(""), json.RawMessage("   ")}
	for _, raw := range cases {
		_, decodeErr := acp.DecodeMethodParams[decodeParamsFixture](raw)
		if decodeErr == nil {
			t.Fatalf("DecodeMethodParams(%q) expected invalid-params error, got nil", string(raw))
		}
		if decodeErr.Code != -32602 {
			t.Fatalf("DecodeMethodParams(%q).Code = %d, want -32602", string(raw), decodeErr.Code)
		}
	}
}

func TestDecodeMethodParamsRejectsMalformedJSON(t *testing.T) {
	_, decodeErr := acp.DecodeMethodParams[decodeParamsFixture](json.RawMessage(`{not json`))
	if decodeErr == nil {
		t.Fatal("DecodeMethodParams(malformed) expected invalid-params error, got nil")
	}
	if decodeErr.Code != -32602 {
		t.Fatalf("DecodeMethodParams(malformed).Code = %d, want -32602", decodeErr.Code)
	}
}

func TestDecodeMethodParamsRejectsNullParams(t *testing.T) {
	_, decodeErr := acp.DecodeMethodParams[decodeParamsFixture](json.RawMessage(`null`))
	if decodeErr == nil {
		t.Fatal("DecodeMethodParams(null) expected invalid-params error, got nil")
	}
	if decodeErr.Code != -32602 {
		t.Fatalf("DecodeMethodParams(null).Code = %d, want -32602", decodeErr.Code)
	}
}

// TestDecodeMethodParamsRejectsSemanticallyInvalidSupportedRequest proves a
// syntactically valid but semantically incomplete supported request (an
// object that unmarshals cleanly but fails the real acp-go-sdk request
// type's own Validate(), e.g. session/prompt with no prompt content) is
// rejected as invalid params rather than silently accepted with a zero
// required field.
func TestDecodeMethodParamsRejectsSemanticallyInvalidSupportedRequest(t *testing.T) {
	cases := []struct {
		name string
		raw  json.RawMessage
	}{
		{"empty object missing required prompt", json.RawMessage(`{}`)},
		{"sessionId present but prompt still missing", json.RawMessage(`{"sessionId":"session-1"}`)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, decodeErr := acp.DecodeMethodParams[acpsdk.PromptRequest](tc.raw)
			if decodeErr == nil {
				t.Fatalf("DecodeMethodParams(%s) expected invalid-params error, got value %+v", tc.raw, got)
			}
			if decodeErr.Code != -32602 {
				t.Fatalf("DecodeMethodParams(%s).Code = %d, want -32602", tc.raw, decodeErr.Code)
			}
			if !reflect.DeepEqual(got, acpsdk.PromptRequest{}) {
				t.Fatalf("DecodeMethodParams(%s) returned a non-zero value %+v on error, want the zero value", tc.raw, got)
			}
		})
	}
}

// TestDecodeMethodParamsRejectsMissingRequiredScalarField proves the
// regression a prior review found: acpsdk.PromptRequest.Validate() only
// checks that Prompt is non-nil, so a request that omits the required
// sessionId scalar field (rather than the required prompt slice field)
// previously decoded successfully with SessionId left at its Go zero value
// ("") indistinguishable from an explicit empty sessionId. Required-field
// presence must be checked against the raw JSON object's keys, not just the
// SDK's own Validate().
func TestDecodeMethodParamsRejectsMissingRequiredScalarField(t *testing.T) {
	raw := json.RawMessage(`{"prompt":[{"type":"text","text":"hi"}]}`)
	got, decodeErr := acp.DecodeMethodParams[acpsdk.PromptRequest](raw)
	if decodeErr == nil {
		t.Fatalf("DecodeMethodParams(%s) expected invalid-params error for omitted sessionId, got value %+v", raw, got)
	}
	if decodeErr.Code != -32602 {
		t.Fatalf("DecodeMethodParams(%s).Code = %d, want -32602", raw, decodeErr.Code)
	}
	if !reflect.DeepEqual(got, acpsdk.PromptRequest{}) {
		t.Fatalf("DecodeMethodParams(%s) returned a non-zero value %+v on error, want the zero value", raw, got)
	}
}

type requiredAndOptionalFieldsFixture struct {
	Required string  `json:"required"`
	Optional *string `json:"optional,omitempty"`
}

// TestDecodeMethodParamsRequiredFieldPresenceIsGeneric proves the
// required-field-presence check is driven by the json tag's omitempty
// option, not by a per-type allowlist: a struct with a required field
// omitted is rejected regardless of the field's zero value, an explicit
// zero value for a required field is accepted (presence, not truthiness,
// is what is checked), and an omitted optional field is accepted.
func TestDecodeMethodParamsRequiredFieldPresenceIsGeneric(t *testing.T) {
	t.Run("omitted required field is rejected", func(t *testing.T) {
		_, decodeErr := acp.DecodeMethodParams[requiredAndOptionalFieldsFixture](json.RawMessage(`{"optional":"x"}`))
		if decodeErr == nil {
			t.Fatal("DecodeMethodParams(omitted required field) expected invalid-params error, got nil")
		}
	})
	t.Run("explicit zero value for required field is accepted", func(t *testing.T) {
		got, decodeErr := acp.DecodeMethodParams[requiredAndOptionalFieldsFixture](json.RawMessage(`{"required":""}`))
		if decodeErr != nil {
			t.Fatalf("DecodeMethodParams(explicit empty required field) unexpected error: %v", decodeErr)
		}
		if got.Required != "" {
			t.Fatalf("DecodeMethodParams() = %+v, want Required=\"\"", got)
		}
	})
	t.Run("omitted optional field is accepted", func(t *testing.T) {
		got, decodeErr := acp.DecodeMethodParams[requiredAndOptionalFieldsFixture](json.RawMessage(`{"required":"present"}`))
		if decodeErr != nil {
			t.Fatalf("DecodeMethodParams(omitted optional field) unexpected error: %v", decodeErr)
		}
		if got.Optional != nil {
			t.Fatalf("DecodeMethodParams() = %+v, want Optional=nil", got)
		}
	})
	t.Run("non-struct T is unaffected", func(t *testing.T) {
		got, decodeErr := acp.DecodeMethodParams[map[string]any](json.RawMessage(`{}`))
		if decodeErr != nil {
			t.Fatalf("DecodeMethodParams(map[string]any) unexpected error: %v", decodeErr)
		}
		if len(got) != 0 {
			t.Fatalf("DecodeMethodParams() = %+v, want empty map", got)
		}
	})
}

type requiredFieldPresenceEdgeCasesFixture struct {
	unexported string //nolint:unused // exercises the unexported-field skip branch
	NoJSONTag  string
	Dash       string `json:"-"`
	NoName     string `json:",omitempty"`
	Required   string `json:"required"`
}

// TestDecodeMethodParamsRequiredFieldPresenceSkipsNonRequiredTagShapes
// proves the required-field-presence check only constrains fields with an
// explicit json tag naming a required key: an unexported field, a field
// with no json tag at all, a field tagged "-", and an omitempty field with
// no explicit name are all ignored, so a params object providing only the
// one genuinely required field decodes successfully.
func TestDecodeMethodParamsRequiredFieldPresenceSkipsNonRequiredTagShapes(t *testing.T) {
	got, decodeErr := acp.DecodeMethodParams[requiredFieldPresenceEdgeCasesFixture](json.RawMessage(`{"required":"x"}`))
	if decodeErr != nil {
		t.Fatalf("DecodeMethodParams() unexpected error: %v", decodeErr)
	}
	if got.Required != "x" {
		t.Fatalf("DecodeMethodParams() = %+v, want Required=x", got)
	}
}

type stringCustomUnmarshalFixture struct {
	Value string
}

func (f *stringCustomUnmarshalFixture) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	f.Value = s
	return nil
}

// TestDecodeMethodParamsSkipsRequiredFieldCheckForNonObjectPayload proves
// required-field-presence checking is a no-op when the raw JSON-RPC params
// is not itself a JSON object, even though T is a struct type: a struct
// with a custom UnmarshalJSON may legitimately decode from a bare JSON
// string, and presence checking only applies when there is a top-level
// object to check keys against.
func TestDecodeMethodParamsSkipsRequiredFieldCheckForNonObjectPayload(t *testing.T) {
	got, decodeErr := acp.DecodeMethodParams[stringCustomUnmarshalFixture](json.RawMessage(`"hello"`))
	if decodeErr != nil {
		t.Fatalf("DecodeMethodParams() unexpected error: %v", decodeErr)
	}
	if got.Value != "hello" {
		t.Fatalf("DecodeMethodParams() = %+v, want Value=hello", got)
	}
}

func TestDecodeMethodParamsAcceptsSemanticallyValidSupportedRequest(t *testing.T) {
	raw := json.RawMessage(`{"sessionId":"session-1","prompt":[{"type":"text","text":"hi"}]}`)
	got, decodeErr := acp.DecodeMethodParams[acpsdk.PromptRequest](raw)
	if decodeErr != nil {
		t.Fatalf("DecodeMethodParams() unexpected error: %v", decodeErr)
	}
	if len(got.Prompt) != 1 {
		t.Fatalf("DecodeMethodParams() = %+v, want exactly one prompt content block", got)
	}
}
