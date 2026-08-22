package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"

	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/envelope"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/session"
)

func TestClassify(t *testing.T) {
	tests := []struct {
		name  string
		cause error
		want  RejectionKind
	}{
		{
			name:  "unsupported content",
			cause: fmt.Errorf("acp: prompt block 0: %w: image", session.ErrUnsupportedContent),
			want:  RejectionUnsupportedContent,
		},
		{
			name:  "unsupported update",
			cause: fmt.Errorf("%w: tool_call", session.ErrUnsupportedUpdate),
			want:  RejectionUnsupportedUpdate,
		},
		{
			name:  "missing required field falls back to malformed request",
			cause: errors.New("acp: cwd is required"),
			want:  RejectionMalformedRequest,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Classify(tt.cause); got != tt.want {
				t.Errorf("Classify() = %q, want %q", got, tt.want)
			}
			if got2 := Classify(tt.cause); got2 != tt.want {
				t.Errorf("Classify() is not stable across repeated evaluation: %q then %q", tt.want, got2)
			}
		})
	}
}

func TestMethodNotFound_NeverCarriesTheClientSuppliedMethodName(t *testing.T) {
	reqErr := MethodNotFound("session/experimental_fork")
	if reqErr.Code != -32601 {
		t.Errorf("MethodNotFound() code = %d, want -32601", reqErr.Code)
	}

	encoded, err := json.Marshal(reqErr)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if strings.Contains(string(encoded), "session/experimental_fork") {
		t.Error("MethodNotFound() echoed the client-supplied method name into its serialized output")
	}
	if !strings.Contains(string(encoded), redactedMethodPlaceholder) {
		t.Errorf("MethodNotFound() encoded output = %s, want it to contain %q", encoded, redactedMethodPlaceholder)
	}
}

// TestMethodNotFound_RedactsAdversarialMethodValues seeds a credential, a raw
// provider command, an absolute filesystem path, a tool payload fragment,
// an internal topology sentinel, and values crafted to look like a
// plausible JSON-RPC method name (so a shape-matching allowlist would admit
// them) as the client-controlled "method" value, then proves none of those
// sentinels survive into the serialized method-not-found error:
// MethodNotFound never echoes back any client-supplied value.
func TestMethodNotFound_RedactsAdversarialMethodValues(t *testing.T) {
	sentinels := []string{
		"sk-live-credential-ABC123XYZ",
		"sk_live_credential_ABC123XYZ",
		"internal_topology_node_7",
		"/usr/local/bin/agent --token=sk-live-credential-ABC123XYZ",
		"/home/operator/.ssh/id_rsa",
		"tool_call raw_output: {\"secret\":\"do-not-leak\"}",
		"internal-dispatch-node-7.factory.internal",
	}

	for _, sentinel := range sentinels {
		reqErr := MethodNotFound(sentinel)
		encoded, err := json.Marshal(reqErr)
		if err != nil {
			t.Fatalf("json.Marshal() error = %v", err)
		}
		if strings.Contains(string(encoded), sentinel) {
			t.Errorf("MethodNotFound(%q) leaked the sentinel into serialized error %s", sentinel, encoded)
		}
	}
}

// TestSafeReject_RedactsSensitiveInternalCauses seeds a credential, a raw
// provider command, an absolute filesystem path, a prompt/tool payload
// fragment, and an internal topology sentinel into internal causes (both a
// rejected-input style error and a wrapped internal cause), then proves
// none of those sentinels survive into the serialized protocol-facing
// error. This is the redaction proof the AC requires for story 3: the
// bounded RejectionKind classification can never carry payload data because
// Classify never reads a cause's message text.
func TestSafeReject_RedactsSensitiveInternalCauses(t *testing.T) {
	sentinels := []string{
		"sk-live-credential-ABC123XYZ",
		"/usr/local/bin/agent --token=sk-live-credential-ABC123XYZ",
		"/home/operator/.ssh/id_rsa",
		"tool_call raw_output: {\"secret\":\"do-not-leak\"}",
		"internal-dispatch-node-7.factory.internal",
	}

	for _, sentinel := range sentinels {
		cause := fmt.Errorf("acp: request rejected: %s", sentinel)
		reqErr := SafeReject(cause)

		encoded, err := json.Marshal(reqErr)
		if err != nil {
			t.Fatalf("json.Marshal() error = %v", err)
		}
		if strings.Contains(string(encoded), sentinel) {
			t.Errorf("SafeReject() leaked sentinel %q into serialized error %s", sentinel, encoded)
		}
		if strings.Contains(string(encoded), cause.Error()) {
			t.Errorf("SafeReject() leaked the raw internal cause into serialized error %s", encoded)
		}
	}
}

func TestRejectEnvelope_ClassifiesInvalidJSONAsUncorrelatedParseError(t *testing.T) {
	_, err := envelope.Decode("conn-1", 1, json.RawMessage(`{not json`))
	if err == nil {
		t.Fatal("envelope.Decode() error = nil, want a decode failure")
	}

	reqErr, id, hasID := RejectEnvelope(err)
	if reqErr.Code != -32700 {
		t.Errorf("RejectEnvelope() code = %d, want -32700 (parse error)", reqErr.Code)
	}
	if hasID {
		t.Errorf("RejectEnvelope() recovered id %+v for unparseable JSON, want no recoverable id", id)
	}
}

func TestRejectEnvelope_ClassifiesInvalidShapeAsCorrelatedInvalidRequest(t *testing.T) {
	_, err := envelope.Decode("conn-1", 1, json.RawMessage(`{"jsonrpc":"1.0","id":7,"method":"session/prompt"}`))
	if err == nil {
		t.Fatal("envelope.Decode() error = nil, want a decode failure")
	}

	reqErr, id, hasID := RejectEnvelope(err)
	if reqErr.Code != -32600 {
		t.Errorf("RejectEnvelope() code = %d, want -32600 (invalid request)", reqErr.Code)
	}
	if !hasID {
		t.Fatal("RejectEnvelope() recovered no id, want the request's valid numeric id")
	}
	gotID, err := id.MarshalJSON()
	if err != nil {
		t.Fatalf("id.MarshalJSON() error = %v", err)
	}
	if string(gotID) != "7" {
		t.Errorf("RejectEnvelope() recovered id = %s, want 7", gotID)
	}
}

func TestRejectEnvelope_InvalidShapeWithUnrecoverableIDIsUncorrelated(t *testing.T) {
	_, err := envelope.Decode("conn-1", 1, json.RawMessage(`{"jsonrpc":"2.0","id":{},"method":"session/prompt"}`))
	if err == nil {
		t.Fatal("envelope.Decode() error = nil, want a decode failure")
	}

	reqErr, _, hasID := RejectEnvelope(err)
	if reqErr.Code != -32600 {
		t.Errorf("RejectEnvelope() code = %d, want -32600 (invalid request)", reqErr.Code)
	}
	if hasID {
		t.Error("RejectEnvelope() recovered an id from a malformed id token, want no recoverable id")
	}
}

func TestRejectEnvelope_FallsBackToSafeRejectForNonDecodeErrors(t *testing.T) {
	cause := errors.New("acp: cwd is required")
	reqErr, _, hasID := RejectEnvelope(cause)
	if reqErr.Code != -32602 {
		t.Errorf("RejectEnvelope() code = %d, want -32602 (invalid params, matching SafeReject)", reqErr.Code)
	}
	if hasID {
		t.Error("RejectEnvelope() recovered an id for a non-DecodeError cause, want no recoverable id")
	}
}

// TestParseErrorAndInvalidRequest_NeverCarrySensitiveData proves both
// bounded constructors serialize only their fixed static reason label,
// mirroring the redaction proofs already required of MethodNotFound and
// SafeReject.
func TestParseErrorAndInvalidRequest_NeverCarrySensitiveData(t *testing.T) {
	for _, reqErr := range []*acpsdk.RequestError{ParseError(), InvalidRequest()} {
		encoded, err := json.Marshal(reqErr)
		if err != nil {
			t.Fatalf("json.Marshal() error = %v", err)
		}
		var decoded map[string]any
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}
		data, ok := decoded["data"].(map[string]any)
		if !ok {
			t.Fatalf("encoded error %s has no object data field", encoded)
		}
		if len(data) != 1 {
			t.Fatalf("encoded error data = %v, want exactly one static reason field", data)
		}
		if _, ok := data["reason"]; !ok {
			t.Fatalf("encoded error data = %v, want a reason field", data)
		}
	}
}

func TestSafeReject_IsDeterministic(t *testing.T) {
	cause := errors.New("acp: cwd must be an absolute path")

	first, err := json.Marshal(SafeReject(cause))
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	second, err := json.Marshal(SafeReject(cause))
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if string(first) != string(second) {
		t.Fatalf("SafeReject() is not deterministic: %s vs %s", first, second)
	}
}
