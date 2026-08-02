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
