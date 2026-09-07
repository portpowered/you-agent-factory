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

func TestRuntimeFailureSubcauseProjectsAndRequiresBoundedArchiveEvidence(t *testing.T) {
	t.Parallel()

	cases := []RuntimeFailureSubcause{
		RuntimeSubcauseArchiveSelection,
		RuntimeSubcauseArchiveOpen,
		RuntimeSubcauseEntryValidation,
		RuntimeSubcauseEntryCopy,
		RuntimeSubcauseExecutableDiscovery,
		RuntimeSubcauseEndpointReservation,
		RuntimeSubcauseCleanup,
	}
	for _, subcause := range cases {
		subcause := subcause
		t.Run(string(subcause), func(t *testing.T) {
			t.Parallel()

			cause := errors.New("token=private path=C:\\cache\\backend.zip")
			failure := NewRuntimeStageErrorWithSubcause(
				RuntimeStageBackendExtract,
				RuntimeFailureExtractionFailed,
				subcause,
				cause,
			)
			diagnostic := ProjectRuntimeFailure(failure, time.Second)
			if diagnostic.Subcause != subcause {
				t.Fatalf("diagnostic subcause = %q, want %q", diagnostic.Subcause, subcause)
			}
			if diagnostic.DiagnosticFields()["failure_subcause"] != string(subcause) {
				t.Fatalf("diagnostic fields = %#v, want bounded subcause", diagnostic.DiagnosticFields())
			}
			if len(diagnostic.CauseSHA256) != sha256.Size*2 || diagnostic.CauseSHA256 != strings.ToLower(diagnostic.CauseSHA256) {
				t.Fatalf("cause digest = %q, want lowercase 64-hex digest", diagnostic.CauseSHA256)
			}

			sink := &runtimeEvidenceRecords{}
			recorder := NewOrderedRuntimeEvidenceRecorder(sink)
			RecordRuntimeEvidenceTerminal(recorder, RuntimeStageBackendExtract, failure, time.Second)
			records := sink.snapshot()
			if len(records) != 1 || records[0].Subcause != subcause {
				t.Fatalf("archive evidence = %#v, want one %s record", records, subcause)
			}
		})
	}

	// A caller cannot omit the operation from an extraction failure or use an
	// unbounded value in its place.
	sink := &runtimeEvidenceRecords{}
	recorder := NewOrderedRuntimeEvidenceRecorder(sink)
	recorder.RecordRuntimeEvidence(RuntimeEvidenceRecord{
		Kind: RuntimeEvidenceKindStage, Stage: RuntimeStageBackendExtract,
		Outcome: RuntimeEvidenceOutcomeFailed, Class: RuntimeFailureExtractionFailed,
		CauseSHA256: strings.Repeat("a", sha256.Size*2),
	})
	if len(sink.snapshot()) != 0 {
		t.Fatal("archive evidence without a subcause was accepted")
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

func TestRuntimeCauseSHA256IncludesJoinedCleanupCause(t *testing.T) {
	t.Parallel()

	startCause := errors.New("process start token=start-secret path=C:\\private\\backend")
	cleanupCause := errors.New("cleanup token=cleanup-secret path=C:\\private\\workspace")
	failure := NewRuntimeStageErrorWithSubcause(
		RuntimeStageBackendStart,
		RuntimeFailureProcessStartFailed,
		RuntimeSubcauseCleanup,
		errors.Join(startCause, cleanupCause),
	)
	diagnostic := ProjectRuntimeFailure(failure, time.Second)
	expected := sha256.Sum256([]byte(startCause.Error() + "\n" + cleanupCause.Error()))
	if diagnostic.CauseSHA256 != hex.EncodeToString(expected[:]) {
		t.Fatalf("joined cause digest = %q, want %q", diagnostic.CauseSHA256, hex.EncodeToString(expected[:]))
	}
	if diagnostic.Subcause != RuntimeSubcauseCleanup ||
		diagnostic.Stage != RuntimeStageBackendStart ||
		diagnostic.Class != RuntimeFailureProcessStartFailed {
		t.Fatalf("joined cleanup diagnostic = %#v, want bounded start cleanup failure", diagnostic)
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

func TestRuntimeEvidenceRecorderOrdersEveryMaterialFailureAndOneTerminal(t *testing.T) {
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

	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			sink := &runtimeEvidenceRecords{}
			recorder := NewOrderedRuntimeEvidenceRecorder(sink)
			cause := runtimeEvidenceFailureCause(testCase.stage, testCase.class)
			RecordRuntimeEvidenceStage(recorder, testCase.stage, cause, 11*time.Millisecond)
			RecordRuntimeEvidenceTerminal(recorder, testCase.stage, cause, 12*time.Millisecond)
			RecordRuntimeEvidenceTerminal(recorder, testCase.stage, cause, 13*time.Millisecond)

			assertOrderedRuntimeFailureEvidence(t, sink.snapshot(), testCase.stage, testCase.class)
		})
	}
}

func runtimeEvidenceFailureCause(stage RuntimeStage, class RuntimeFailureClass) error {
	const message = "controlled private cause"
	if stage == RuntimeStageBackendExtract {
		return NewRuntimeStageErrorWithSubcause(stage, class, RuntimeSubcauseArchiveOpen, errors.New(message))
	}
	return NewRuntimeStageError(stage, class, errors.New(message))
}

func assertOrderedRuntimeFailureEvidence(
	t *testing.T,
	records []RuntimeEvidenceRecord,
	wantStage RuntimeStage,
	wantClass RuntimeFailureClass,
) {
	t.Helper()
	if len(records) != 2 {
		t.Fatalf("evidence records = %d, want one stage and one terminal: %#v", len(records), records)
	}
	if records[0].Sequence != 1 || records[1].Sequence != 2 {
		t.Fatalf("evidence sequence = (%d, %d), want (1, 2)", records[0].Sequence, records[1].Sequence)
	}
	if records[0].Kind != RuntimeEvidenceKindStage || records[1].Kind != RuntimeEvidenceKindTerminal {
		t.Fatalf("evidence kinds = (%q, %q), want (STAGE, TERMINAL)", records[0].Kind, records[1].Kind)
	}
	for _, record := range records {
		if record.Stage != wantStage || record.Outcome != RuntimeEvidenceOutcomeFailed ||
			record.Class != wantClass || len(record.CauseSHA256) != sha256.Size*2 {
			t.Fatalf("bounded record = %#v, want stage=%s class=%s failed digest", record, wantStage, wantClass)
		}
		if record.CauseSHA256 != strings.ToLower(record.CauseSHA256) {
			t.Fatalf("cause digest is not lowercase: %q", record.CauseSHA256)
		}
		if wantStage == RuntimeStageBackendExtract && record.Subcause != RuntimeSubcauseArchiveOpen {
			t.Fatalf("archive subcause = %q, want %q", record.Subcause, RuntimeSubcauseArchiveOpen)
		}
	}
}

func TestRuntimeEvidenceRecorderRedactsCauseAndRejectsMalformedRecords(t *testing.T) {
	t.Parallel()

	sink := &runtimeEvidenceRecords{}
	recorder := NewOrderedRuntimeEvidenceRecorder(sink)
	markers := []string{
		"token=runtime-evidence-token-marker",
		"https://signed.example.test/runtime?signature=url-marker",
		"grpc://127.0.0.1:49321",
		"C:\\isolated\\cache\\model.bin",
		"prompt=private prompt marker",
		"RIFF raw-media-marker",
	}
	cause := errors.New(strings.Join(markers, " "))
	RecordRuntimeEvidenceStage(
		recorder, RuntimeStageProtocolLoad,
		NewRuntimeStageError(RuntimeStageProtocolLoad, RuntimeFailureProtocolIncompatible, cause),
		1*time.Millisecond,
	)
	RecordRuntimeEvidenceTerminal(
		recorder, RuntimeStageProtocolLoad,
		NewRuntimeStageError(RuntimeStageProtocolLoad, RuntimeFailureProtocolIncompatible, cause),
		2*time.Millisecond,
	)
	// A caller cannot smuggle unbounded text through the ordered boundary.
	recorder.RecordRuntimeEvidence(RuntimeEvidenceRecord{
		Kind: RuntimeEvidenceKindStage, Stage: RuntimeStage("raw-path-marker"),
		Outcome: RuntimeEvidenceOutcomeFailed, Class: RuntimeFailureProtocolIncompatible,
		CauseSHA256: strings.Repeat("a", sha256.Size*2),
	})

	body, err := json.Marshal(sink.snapshot())
	if err != nil {
		t.Fatalf("marshal evidence records: %v", err)
	}
	serialized := string(body)
	for _, marker := range markers {
		if strings.Contains(serialized, marker) {
			t.Fatalf("evidence leaked marker %q: %s", marker, serialized)
		}
	}
	if len(sink.snapshot()) != 2 {
		t.Fatalf("evidence records after malformed input = %d, want 2", len(sink.snapshot()))
	}
}

func TestRuntimeEvidenceRecorderNilAndNilErrorAreSafe(t *testing.T) {
	t.Parallel()

	RecordRuntimeEvidenceStage(nil, RuntimeStageInvoke, errors.New("ignored"), time.Second)
	RecordRuntimeEvidenceTerminal(nil, RuntimeStageInvoke, errors.New("ignored"), time.Second)
	if recorder := NewOrderedRuntimeEvidenceRecorder(nil); recorder != nil {
		t.Fatal("nil recorder was not preserved as nil")
	}

	sink := &runtimeEvidenceRecords{}
	recorder := NewOrderedRuntimeEvidenceRecorder(sink)
	RecordRuntimeEvidenceStage(recorder, RuntimeStageInvoke, nil, -time.Second)
	RecordRuntimeEvidenceTerminal(recorder, RuntimeStageInvoke, nil, -time.Second)
	records := sink.snapshot()
	if len(records) != 2 {
		t.Fatalf("nil-error evidence records = %d, want 2", len(records))
	}
	for _, record := range records {
		if record.Outcome != RuntimeEvidenceOutcomeCompleted || record.Class != "" ||
			record.CauseSHA256 != "" || record.DurationMillis != 0 {
			t.Fatalf("nil-error record = %#v, want bounded completed observation", record)
		}
	}
}

type runtimeEvidenceRecords struct {
	records []RuntimeEvidenceRecord
}

func (sink *runtimeEvidenceRecords) RecordRuntimeEvidence(record RuntimeEvidenceRecord) {
	if sink == nil {
		return
	}
	sink.records = append(sink.records, record)
}

func (sink *runtimeEvidenceRecords) snapshot() []RuntimeEvidenceRecord {
	if sink == nil {
		return nil
	}
	return append([]RuntimeEvidenceRecord(nil), sink.records...)
}

type unknownRuntimeClassifier struct{}

func (unknownRuntimeClassifier) Error() string { return "unknown" }

func (unknownRuntimeClassifier) ModelRuntimeStage() string { return "UNKNOWN" }

func (unknownRuntimeClassifier) ModelRuntimeFailureClass() string { return "UNKNOWN" }
