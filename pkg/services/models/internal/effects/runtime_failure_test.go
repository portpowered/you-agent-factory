package effects

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestRuntimeStageErrorClassifiesEveryMaterialStageAndUnwrapsCause(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		stage RuntimeStage
		class RuntimeFailureClass
	}{
		{name: "artifact resolve", stage: RuntimeStageArtifactResolve, class: RuntimeFailureUnavailable},
		{name: "artifact download", stage: RuntimeStageArtifactDownload, class: RuntimeFailureUnavailable},
		{name: "artifact digest", stage: RuntimeStageArtifactDigest, class: RuntimeFailureIntegrityMismatch},
		{name: "backend extract", stage: RuntimeStageBackendExtract, class: RuntimeFailureExtractionFailed},
		{name: "backend start", stage: RuntimeStageBackendStart, class: RuntimeFailureProcessStartFailed},
		{name: "protocol load", stage: RuntimeStageProtocolLoad, class: RuntimeFailureProtocolIncompatible},
		{name: "invoke", stage: RuntimeStageInvoke, class: RuntimeFailureInvocationFailed},
	}

	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			cause := errors.New("typed cause")
			wrapped := NewRuntimeStageError(test.stage, test.class, cause)
			outer := fmt.Errorf("outer context: %w", wrapped)

			stage, class, ok := ClassifyRuntimeFailure(outer)
			if !ok || stage != test.stage || class != test.class {
				t.Fatalf("classification = (%q, %q, %t), want (%q, %q, true)", stage, class, ok, test.stage, test.class)
			}
			if !errors.Is(outer, cause) {
				t.Fatal("errors.Is did not reach the original typed cause")
			}
			var got *RuntimeStageError
			if !errors.As(outer, &got) || got != wrapped {
				t.Fatalf("errors.As = %#v, want wrapped runtime failure", got)
			}
			if strings.Contains(wrapped.Error(), cause.Error()) {
				t.Fatalf("runtime wrapper copied raw cause: %q", wrapped.Error())
			}
		})
	}
}

func TestProjectRuntimeFailureRedactsSensitiveCauseToDigest(t *testing.T) {
	t.Parallel()

	markers := []string{
		"token=secret-token-marker",
		"https://signed.example.test/model?signature=signed-url-marker",
		"grpc://127.0.0.1:49321",
		"C:\\isolated\\cache\\model.bin",
		"prompt=private prompt marker",
		"RIFF raw-media-marker",
	}
	causeText := strings.Join(markers, " ")
	cause := errors.New(causeText)
	failure := NewRuntimeStageError(
		RuntimeStageProtocolLoad,
		RuntimeFailureProtocolIncompatible,
		cause,
	)
	diagnostic := ProjectRuntimeFailure(failure, 1234*time.Millisecond)
	body, err := json.Marshal(diagnostic)
	if err != nil {
		t.Fatalf("marshal safe diagnostic: %v", err)
	}
	serialized := string(body)
	for _, marker := range markers {
		if strings.Contains(serialized, marker) {
			t.Fatalf("safe diagnostic leaked marker %q: %s", marker, serialized)
		}
	}
	if diagnostic.Stage != RuntimeStageProtocolLoad ||
		diagnostic.Class != RuntimeFailureProtocolIncompatible ||
		diagnostic.Outcome != "FAILED" ||
		diagnostic.DurationMillis != 1234 {
		t.Fatalf("diagnostic = %#v, want bounded protocol-load failure", diagnostic)
	}
	expected := sha256.Sum256([]byte(causeText))
	if diagnostic.CauseSHA256 != hex.EncodeToString(expected[:]) {
		t.Fatalf("cause digest = %q, want %q", diagnostic.CauseSHA256, hex.EncodeToString(expected[:]))
	}
	for key, value := range diagnostic.DiagnosticFields() {
		if strings.Contains(serialized, value) && strings.Contains(value, "marker") {
			t.Fatalf("diagnostic field %q copied marker value %q", key, value)
		}
	}
}

func TestClassifyRuntimeFailureFailsClosedForUnknownClassifier(t *testing.T) {
	t.Parallel()

	err := fmt.Errorf("wrapped: %w", unknownRuntimeClassifier{})
	if stage, class, ok := ClassifyRuntimeFailure(err); ok || stage != "" || class != "" {
		t.Fatalf("unknown classification = (%q, %q, %t), want empty false", stage, class, ok)
	}
	wrapped := WrapRuntimeFailure(RuntimeStageInvoke, errors.New("invoke cause"))
	if stage, class, ok := ClassifyRuntimeFailure(wrapped); !ok || stage != RuntimeStageInvoke || class != RuntimeFailureInvocationFailed {
		t.Fatalf("wrapped default classification = (%q, %q, %t), want INVOKE/INVOCATION_FAILED/true", stage, class, ok)
	}
}

type unknownRuntimeClassifier struct{}

func (unknownRuntimeClassifier) Error() string { return "unknown" }

func (unknownRuntimeClassifier) ModelRuntimeStage() string { return "UNKNOWN" }

func (unknownRuntimeClassifier) ModelRuntimeFailureClass() string { return "UNKNOWN" }
