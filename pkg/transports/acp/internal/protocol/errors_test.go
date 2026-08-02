package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

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

func TestMethodNotFound_CarriesOnlyTheMethodName(t *testing.T) {
	reqErr := MethodNotFound("session/experimental_fork")
	if reqErr.Code != -32601 {
		t.Errorf("MethodNotFound() code = %d, want -32601", reqErr.Code)
	}

	encoded, err := json.Marshal(reqErr)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if !strings.Contains(string(encoded), "session/experimental_fork") {
		t.Error("MethodNotFound() encoded output does not contain the method name")
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
