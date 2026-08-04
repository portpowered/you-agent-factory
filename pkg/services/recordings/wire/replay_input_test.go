package wire_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testpath"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingswire "github.com/portpowered/infinite-you/pkg/services/recordings/wire"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

type portableReplayInputExpectation struct {
	name, fixture, sessionID, schemaVersion, sourceRef, resultStatus string
	eventIDs                                                         []string
	secretsRedacted                                                  int64
	artifactCount                                                    int
	resultPresent                                                    bool
}

func TestReplayInputLoaderClassifiesPortableRecording(t *testing.T) {
	t.Parallel()

	path := testpath.MustRepoPathFromCaller(
		t,
		0,
		"pkg", "services", "recordings", "internal", "artifacts", "testdata", "valid-v2.json",
	)
	loader := recordingswire.NewReplayInputLoader(
		recordings.RecordingReadFile(os.ReadFile),
		func(string) (*recordings.ReplayArtifact, error) {
			t.Fatal("legacy loader must not be called for a portable recording")
			return nil, nil
		},
		zap.NewNop(),
	)
	result, err := loader.LoadReplayInput(recordings.LoadReplayInputRequest{Path: path})
	if err != nil {
		t.Fatalf("LoadReplayInput() error = %v", err)
	}
	if result.Legacy != nil {
		t.Fatal("Legacy = non-nil, want nil for a portable recording")
	}
	if result.Portable == nil {
		t.Fatal("Portable = nil, want decoded portable recording")
	}
	if got := result.Portable.Session.ID; got != "session-js-001" {
		t.Fatalf("Portable.Session.ID = %q, want session-js-001", got)
	}
	if got := result.Portable.Redaction.SecretsRedacted; got != 2 {
		t.Fatalf("Portable.Redaction.SecretsRedacted = %d, want 2", got)
	}
	if got := result.Portable.Events[1].ArtifactIDs[0]; got != "artifact-1" {
		t.Fatalf("Portable.Events[1].ArtifactIDs[0] = %q, want artifact-1", got)
	}

	result.Portable.Events[1].ArtifactIDs[0] = "mutated"
	second, err := loader.LoadReplayInput(recordings.LoadReplayInputRequest{Path: path})
	if err != nil {
		t.Fatalf("LoadReplayInput() second read = %v", err)
	}
	if second.Portable == nil || second.Portable.Events[1].ArtifactIDs[0] != "artifact-1" {
		t.Fatalf("LoadReplayInput() second read = %#v, want detached portable event values", second.Portable)
	}
}

// TestReplayInputLoaderPreservesSupportedPortableRecordingSchemas proves the
// narrow capability preserves the compatibility, identity, canonical event,
// result, and redaction facts of every previously supported portable
// recording schema without exposing the compatibility document type.
func TestReplayInputLoaderPreservesSupportedPortableRecordingSchemas(t *testing.T) {
	t.Parallel()

	cases := []portableReplayInputExpectation{
		{
			name: "version one", fixture: "valid-v1.json", sessionID: "session-historical-001", schemaVersion: "1",
			sourceRef: "workflow/historical.js", eventIDs: []string{"event-historical-1"}, secretsRedacted: 0,
		},
		{
			name: "version two", fixture: "valid-v2.json", sessionID: "session-js-001", schemaVersion: "2",
			sourceRef: "workflow/example.js", resultStatus: "FINAL", eventIDs: []string{"event-1", "event-2"},
			secretsRedacted: 2, artifactCount: 1, resultPresent: true,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			path := portableRecordingFixturePath(t, testCase.fixture)
			loader := recordingswire.NewReplayInputLoader(recordings.RecordingReadFile(os.ReadFile), nil, zap.NewNop())
			result, err := loader.LoadReplayInput(recordings.LoadReplayInputRequest{Path: path})
			if err != nil {
				t.Fatalf("LoadReplayInput: %v", err)
			}
			assertPortableReplayInputFacts(t, result, testCase)
		})
	}
}

// TestReplayInputLoaderMapsPortableValidationDiagnostics proves portable
// recording validation failures remain directly owned, matchable capability
// outcomes with no partial replay facts.
func TestReplayInputLoaderMapsPortableValidationDiagnostics(t *testing.T) {
	t.Parallel()
	fixture, err := os.ReadFile(portableRecordingFixturePath(t, "valid-v2.json"))
	if err != nil {
		t.Fatalf("ReadFile fixture: %v", err)
	}

	cases := []struct {
		name, from, to, area, path string
		code                       recordings.ReplayInputDiagnosticCode
		versions                   []string
	}{
		{
			name: "unknown field", from: "\n}", to: ",\"unexpected\":true\n}",
			code: recordings.ReplayInputDiagnosticMalformed, area: "document", path: "",
		},
		{
			name: "unsupported compatibility", from: `"replayCompatibilityVersion": "1"`, to: `"replayCompatibilityVersion": "999"`,
			code: recordings.ReplayInputDiagnosticUnsupportedVersion, area: "compatibility", path: "replayCompatibilityVersion", versions: []string{"1"},
		},
		{
			name: "invalid identity", from: `"id": "session-js-001"`, to: `"id": ""`,
			code: recordings.ReplayInputDiagnosticInvalidIdentity, area: "session", path: "session.id",
		},
		{
			name: "invalid summary", from: `"status": "FINAL"`, to: `"status": "UNKNOWN"`,
			code: recordings.ReplayInputDiagnosticInvalidSummary, area: "result", path: "result.status",
		},
		{
			name: "invalid digest", from: "sha256:1111111111111111111111111111111111111111111111111111111111111111", to: "not-a-digest",
			code: recordings.ReplayInputDiagnosticInvalidDigest, area: "digest", path: "source.hash",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			payload := strings.Replace(string(fixture), testCase.from, testCase.to, 1)
			if payload == string(fixture) {
				t.Fatalf("fixture did not contain replacement %q", testCase.from)
			}
			loader := recordingswire.NewReplayInputLoader(
				func(string) ([]byte, error) { return []byte(payload), nil },
				nil,
				zap.NewNop(),
			)
			result, err := loader.LoadReplayInput(recordings.LoadReplayInputRequest{Path: "portable-recording.json"})
			assertReplayInputDiagnostic(t, err, testCase.code, testCase.area, testCase.path, testCase.versions)
			if result.Portable != nil || result.Legacy != nil {
				t.Fatalf("LoadReplayInput() result = %#v, want no partial replay input", result)
			}
		})
	}
}

func TestRecordingReplayArtifactsRuntimeConstructionIsInert(t *testing.T) {
	t.Parallel()

	built := false
	factory := recordingswire.NewRecordingReplayArtifactsFactory(
		func(string) ([]byte, error) {
			t.Fatal("replay input reader called during construction")
			return nil, nil
		},
		func(string) (*recordings.ReplayArtifact, error) {
			t.Fatal("legacy replay loader called during construction")
			return nil, nil
		},
		zap.NewNop(),
		func(
			recordings.Ledger,
			recordings.ProjectionService,
		) (recordings.RecordingReplayArtifacts, recordings.RecordingLifecycle, error) {
			built = true
			return nil, nil, nil
		},
	)
	capability := factory()
	if capability == nil {
		t.Fatal("factory() = nil, want phase-aware capability")
	}
	if built {
		t.Fatal("runtime artifact builder called during construction")
	}
	_, err := capability.LoadReplay(recordings.LoadReplayRequest{RecordingID: "not-bound"})
	var typed *recordings.ReplayArtifactError
	if !errors.As(err, &typed) || typed.Kind != recordings.ReplayArtifactErrorUnavailable {
		t.Fatalf("LoadReplay() error = %v, want unavailable typed error before binding", err)
	}
	if built {
		t.Fatal("runtime artifact builder called by an unbound artifact operation")
	}
}

// TestRecordingReplayArtifactsRuntimeBindsAndForwardsOperations proves that
// the same phase-aware capability used for runtime opening becomes the
// canonical finalized-recording capability after binding. Each public
// operation is observed through its returned result rather than implementation
// topology.
func TestRecordingReplayArtifactsRuntimeBindsAndForwardsOperations(t *testing.T) {
	t.Parallel()

	artifacts := &phaseReplayArtifactsStub{}
	lifecycle := &phaseRecordingLifecycleStub{}
	builderCalls := 0
	capability := recordingswire.NewRecordingReplayArtifactsFactory(
		nil,
		nil,
		nil,
		func(
			recordings.Ledger,
			recordings.ProjectionService,
		) (recordings.RecordingReplayArtifacts, recordings.RecordingLifecycle, error) {
			builderCalls++
			return artifacts, lifecycle, nil
		},
	)()

	bound, err := capability.BindRecordingLifecycle(nil, nil)
	if err != nil || bound != lifecycle || builderCalls != 1 {
		t.Fatalf("BindRecordingLifecycle() = (%#v, %v), builder calls %d; want lifecycle once", bound, err, builderCalls)
	}
	if _, err := capability.LoadReplay(recordings.LoadReplayRequest{RecordingID: "recording-1"}); err != nil {
		t.Fatalf("LoadReplay() error = %v", err)
	}
	if _, err := capability.BuildArtifact(recordings.BuildArtifactRequest{RecordingID: "recording-1"}); err != nil {
		t.Fatalf("BuildArtifact() error = %v", err)
	}
	if _, err := capability.ValidateArtifact(recordings.ValidateArtifactRequest{}); err != nil {
		t.Fatalf("ValidateArtifact() error = %v", err)
	}
	if _, err := capability.EncodeArtifact(recordings.EncodeArtifactRequest{}); err != nil {
		t.Fatalf("EncodeArtifact() error = %v", err)
	}
	if _, err := capability.DecodeArtifact(recordings.DecodeArtifactRequest{Payload: []byte(`{}`)}); err != nil {
		t.Fatalf("DecodeArtifact() error = %v", err)
	}
	if _, err := capability.SummarizeArtifact(recordings.SummarizeArtifactRequest{}); err != nil {
		t.Fatalf("SummarizeArtifact() error = %v", err)
	}
	if _, err := capability.ExportArtifact(context.Background(), recordings.ExportArtifactRequest{RecordingID: "recording-1"}); err != nil {
		t.Fatalf("ExportArtifact() error = %v", err)
	}
	if _, err := capability.ReadArtifact(context.Background(), recordings.ReadArtifactRequest{RecordingID: "recording-1"}); err != nil {
		t.Fatalf("ReadArtifact() error = %v", err)
	}
	if got := strings.Join(artifacts.calls, ","); got != "load,build,validate,encode,decode,summarize,export,read" {
		t.Fatalf("forwarded operations = %q, want every finalized-recording operation once", got)
	}
	if _, err := capability.BindRecordingLifecycle(nil, nil); err == nil || builderCalls != 1 {
		t.Fatalf("second BindRecordingLifecycle() = %v, builder calls %d; want one-time binding", err, builderCalls)
	}
}

func TestReplayInputLoaderDelegatesLegacyArtifact(t *testing.T) {
	t.Parallel()

	requestedPath := ""
	tempFile := writeTempReplayInputFile(t, `{"schemaVersion":"legacy"}`)
	want := &recordings.ReplayArtifact{
		SchemaVersion: "legacy",
		Events:        []recordings.FactoryEvent{{Id: "legacy-event"}},
	}
	loader := recordingswire.NewReplayInputLoader(
		recordings.RecordingReadFile(os.ReadFile),
		func(path string) (*recordings.ReplayArtifact, error) {
			requestedPath = path
			return want, nil
		},
		zap.NewNop(),
	)
	result, err := loader.LoadReplayInput(recordings.LoadReplayInputRequest{Path: tempFile})
	if err != nil {
		t.Fatalf("LoadReplayInput() error = %v", err)
	}
	if result.Portable != nil {
		t.Fatal("Portable = non-nil, want nil for a legacy artifact")
	}
	if result.Legacy == nil || result.Legacy.SchemaVersion != want.SchemaVersion {
		t.Fatalf("Legacy = %#v, want detached legacy artifact with schema %q", result.Legacy, want.SchemaVersion)
	}
	if got := string(result.Legacy.Events[0].EventJSON); got == "" {
		t.Fatal("Legacy.Events[0].EventJSON = empty, want complete detached event envelope")
	}
	result.Legacy.Events[0].EventJSON[0] = 'x'
	second, err := loader.LoadReplayInput(recordings.LoadReplayInputRequest{Path: tempFile})
	if err != nil {
		t.Fatalf("LoadReplayInput() second legacy read = %v", err)
	}
	if second.Legacy == nil || second.Legacy.Events[0].EventJSON[0] != '{' {
		t.Fatalf("LoadReplayInput() second legacy read = %#v, want detached event bytes", second.Legacy)
	}
	if requestedPath != tempFile {
		t.Fatalf("legacy loader path = %q, want %q", requestedPath, tempFile)
	}
}

func TestReplayInputLoaderRejectsMissingReader(t *testing.T) {
	t.Parallel()

	loader := recordingswire.NewReplayInputLoader(nil, nil, zap.NewNop())
	if _, err := loader.LoadReplayInput(recordings.LoadReplayInputRequest{Path: "recording.json"}); err == nil {
		t.Fatal("missing reader error = nil")
	}
}

func TestReplayInputLoaderWrapsReadFailure(t *testing.T) {
	t.Parallel()

	want := errors.New("recording read unavailable")
	loader := recordingswire.NewReplayInputLoader(
		func(path string) ([]byte, error) {
			if path != "recording.json" {
				t.Fatalf("path = %q, want recording.json", path)
			}
			return nil, want
		},
		nil,
		zap.NewNop(),
	)
	_, err := loader.LoadReplayInput(recordings.LoadReplayInputRequest{Path: "recording.json"})
	if !errors.Is(err, want) {
		t.Fatalf("LoadReplayInput() error = %v, want %v", err, want)
	}
}

func TestReplayInputLoaderRejectsMissingLegacyLoader(t *testing.T) {
	t.Parallel()

	tempFile := writeTempReplayInputFile(t, `{"schemaVersion":"legacy"}`)
	loader := recordingswire.NewReplayInputLoader(recordings.RecordingReadFile(os.ReadFile), nil, zap.NewNop())
	if _, err := loader.LoadReplayInput(recordings.LoadReplayInputRequest{Path: tempFile}); err == nil {
		t.Fatal("missing legacy loader error = nil")
	}
}

func TestReplayInputLoaderPropagatesMalformedPortableRecording(t *testing.T) {
	t.Parallel()

	tempFile := writeTempReplayInputFile(
		t,
		`{"recordingKind":"`+recordings.KindJavaScriptFactorySession+`","schemaVersion":"not-a-real-version"}`,
	)
	loader := recordingswire.NewReplayInputLoader(
		recordings.RecordingReadFile(os.ReadFile),
		func(string) (*recordings.ReplayArtifact, error) {
			t.Fatal("legacy loader must not be called for a portable recording payload")
			return nil, nil
		},
		zap.NewNop(),
	)
	result, err := loader.LoadReplayInput(recordings.LoadReplayInputRequest{Path: tempFile})
	if err == nil {
		t.Fatal("malformed portable recording error = nil")
	}
	if result.Portable != nil || result.Legacy != nil {
		t.Fatalf("result = %+v, want zero-value result on failure", result)
	}
	var typed *recordings.ReplayInputError
	if !errors.As(err, &typed) {
		t.Fatalf("LoadReplayInput() error = %T, want *recordings.ReplayInputError", err)
	}
	if typed.Kind != recordings.ReplayInputErrorPortable {
		t.Fatalf("ReplayInputError.Kind = %q, want %q", typed.Kind, recordings.ReplayInputErrorPortable)
	}
	if typed.Diagnostic == nil {
		t.Fatal("ReplayInputError.Diagnostic = nil, want unsupported-version diagnostic")
	}
	if typed.Diagnostic.Code != recordings.ReplayInputDiagnosticUnsupportedVersion {
		t.Fatalf(
			"ReplayInputError.Diagnostic.Code = %q, want %q",
			typed.Diagnostic.Code,
			recordings.ReplayInputDiagnosticUnsupportedVersion,
		)
	}
	if typed.Diagnostic.Area == "" || typed.Diagnostic.Message == "" || len(typed.Diagnostic.SupportedVersions) == 0 {
		t.Fatalf("ReplayInputError.Diagnostic = %#v, want structured supported-version facts", typed.Diagnostic)
	}
}

func TestReplayInputLoaderLogsIntentAndFailureWithoutInputPathOrPayload(t *testing.T) {
	t.Parallel()

	core, observed := observer.New(zap.InfoLevel)
	loader := recordingswire.NewReplayInputLoader(nil, nil, zap.New(core))
	if _, err := loader.LoadReplayInput(recordings.LoadReplayInputRequest{Path: "private/replay.json"}); err == nil {
		t.Fatal("LoadReplayInput() error = nil, want reader configuration error")
	}

	entries := observed.All()
	if len(entries) != 2 {
		t.Fatalf("log entry count = %d, want 2", len(entries))
	}
	if entries[0].Message != "loading replay input" || entries[1].Message != "replay input reader is not configured" {
		t.Fatalf("log messages = %q, %q", entries[0].Message, entries[1].Message)
	}
	for _, entry := range entries {
		for _, field := range entry.Context {
			if field.Key == "path" || field.Key == "payload" || field.String == "private/replay.json" {
				t.Fatalf("log field = %#v, must not expose replay input path or payload", field)
			}
		}
	}
}

func writeTempReplayInputFile(t *testing.T, contents string) string {
	t.Helper()
	file, err := os.CreateTemp(t.TempDir(), "replay-input-*.json")
	if err != nil {
		t.Fatalf("create temp replay input file: %v", err)
	}
	defer file.Close()
	if _, err := file.WriteString(contents); err != nil {
		t.Fatalf("write temp replay input file: %v", err)
	}
	return file.Name()
}

func portableRecordingFixturePath(t *testing.T, fixture string) string {
	t.Helper()
	return testpath.MustRepoPathFromCaller(
		t,
		0,
		"pkg", "services", "recordings", "internal", "artifacts", "testdata", fixture,
	)
}

func assertPortableReplayInputFacts(
	t *testing.T,
	result recordings.LoadReplayInputResult,
	want portableReplayInputExpectation,
) {
	t.Helper()
	if result.Legacy != nil || result.Portable == nil {
		t.Fatalf("LoadReplayInput() = %#v, want only a portable result", result)
	}
	portable := result.Portable
	if portable.RecordingKind != recordings.KindJavaScriptFactorySession ||
		portable.SchemaVersion != want.schemaVersion ||
		portable.ReplayCompatibilityVersion != "1" ||
		portable.Session.ID != want.sessionID ||
		portable.Source.Ref != want.sourceRef {
		t.Fatalf("LoadReplayInput() portable facts = %#v, want supported schema facts", portable)
	}
	if len(portable.Artifacts) != want.artifactCount || len(portable.Events) != len(want.eventIDs) {
		t.Fatalf(
			"LoadReplayInput() artifacts/events = (%d, %d), want (%d, %d)",
			len(portable.Artifacts),
			len(portable.Events),
			want.artifactCount,
			len(want.eventIDs),
		)
	}
	for index, eventID := range want.eventIDs {
		if portable.Events[index].ID != eventID || portable.Events[index].Sequence != int64(index) {
			t.Fatalf("LoadReplayInput() Events[%d] = %#v, want ID %q in canonical order", index, portable.Events[index], eventID)
		}
	}
	if portable.Redaction.SecretsRedacted != want.secretsRedacted ||
		!portable.Redaction.RuntimeStateOmitted ||
		!portable.Redaction.CheckpointBodiesOmitted ||
		!portable.Redaction.ProviderTranscriptsOmitted ||
		!portable.Redaction.ChildDispatchesOmitted {
		t.Fatalf("LoadReplayInput() Redaction = %#v, want preserved privacy facts", portable.Redaction)
	}
	if !want.resultPresent {
		if portable.Result != nil {
			t.Fatalf("LoadReplayInput() Result = %#v, want no result for historical schema", portable.Result)
		}
		return
	}
	if portable.Result == nil || portable.Result.Status != want.resultStatus || portable.Result.Mode != "final" {
		t.Fatalf("LoadReplayInput() Result = %#v, want status %q", portable.Result, want.resultStatus)
	}
}

func assertReplayInputDiagnostic(
	t *testing.T,
	err error,
	wantCode recordings.ReplayInputDiagnosticCode,
	wantArea, wantPath string,
	wantVersions []string,
) {
	t.Helper()
	var typed *recordings.ReplayInputError
	if !errors.As(err, &typed) || typed.Kind != recordings.ReplayInputErrorPortable || typed.Diagnostic == nil {
		t.Fatalf("LoadReplayInput() error = %v, want a portable validation error with diagnostic", err)
	}
	diagnostic := typed.Diagnostic
	if diagnostic.Code != wantCode || diagnostic.Area != wantArea || diagnostic.Path != wantPath || diagnostic.Message == "" {
		t.Fatalf(
			"diagnostic = %#v, want code %q, area %q, path %q, and safe message",
			diagnostic,
			wantCode,
			wantArea,
			wantPath,
		)
	}
	if len(diagnostic.SupportedVersions) != len(wantVersions) {
		t.Fatalf("diagnostic supported versions = %#v, want %#v", diagnostic.SupportedVersions, wantVersions)
	}
	for index, version := range wantVersions {
		if diagnostic.SupportedVersions[index] != version {
			t.Fatalf("diagnostic supported versions = %#v, want %#v", diagnostic.SupportedVersions, wantVersions)
		}
	}
}

type phaseReplayArtifactsStub struct {
	calls []string
}

func (stub *phaseReplayArtifactsStub) LoadReplayInput(recordings.LoadReplayInputRequest) (recordings.LoadReplayInputResult, error) {
	return recordings.LoadReplayInputResult{}, nil
}

func (stub *phaseReplayArtifactsStub) LoadReplay(request recordings.LoadReplayRequest) (recordings.LoadReplayResult, error) {
	stub.calls = append(stub.calls, "load")
	return recordings.LoadReplayResult{Replay: recordings.ReplayFacts{RecordingID: request.RecordingID}}, nil
}

func (stub *phaseReplayArtifactsStub) BuildArtifact(recordings.BuildArtifactRequest) (recordings.BuildArtifactResult, error) {
	stub.calls = append(stub.calls, "build")
	return recordings.BuildArtifactResult{}, nil
}

func (stub *phaseReplayArtifactsStub) ValidateArtifact(recordings.ValidateArtifactRequest) (recordings.ValidateArtifactResult, error) {
	stub.calls = append(stub.calls, "validate")
	return recordings.ValidateArtifactResult{}, nil
}

func (stub *phaseReplayArtifactsStub) EncodeArtifact(recordings.EncodeArtifactRequest) (recordings.EncodeArtifactResult, error) {
	stub.calls = append(stub.calls, "encode")
	return recordings.EncodeArtifactResult{}, nil
}

func (stub *phaseReplayArtifactsStub) DecodeArtifact(recordings.DecodeArtifactRequest) (recordings.DecodeArtifactResult, error) {
	stub.calls = append(stub.calls, "decode")
	return recordings.DecodeArtifactResult{}, nil
}

func (stub *phaseReplayArtifactsStub) SummarizeArtifact(recordings.SummarizeArtifactRequest) (recordings.SummarizeArtifactResult, error) {
	stub.calls = append(stub.calls, "summarize")
	return recordings.SummarizeArtifactResult{}, nil
}

func (stub *phaseReplayArtifactsStub) ExportArtifact(context.Context, recordings.ExportArtifactRequest) (recordings.ExportArtifactResult, error) {
	stub.calls = append(stub.calls, "export")
	return recordings.ExportArtifactResult{}, nil
}

func (stub *phaseReplayArtifactsStub) ReadArtifact(context.Context, recordings.ReadArtifactRequest) (recordings.ReadArtifactResult, error) {
	stub.calls = append(stub.calls, "read")
	return recordings.ReadArtifactResult{}, nil
}

type phaseRecordingLifecycleStub struct{}

func (*phaseRecordingLifecycleStub) Begin(recordings.BeginRecordingRequest) (recordings.RecordingLifecycleResult, error) {
	return recordings.RecordingLifecycleResult{}, nil
}

func (*phaseRecordingLifecycleStub) Bind(recordings.BindLifecycleRequest) (recordings.RecordingLifecycleResult, error) {
	return recordings.RecordingLifecycleResult{}, nil
}

func (*phaseRecordingLifecycleStub) AppendEvent(recordings.AppendLifecycleEventRequest) (recordings.RecordingLifecycleResult, error) {
	return recordings.RecordingLifecycleResult{}, nil
}

func (*phaseRecordingLifecycleStub) RecordFailure(recordings.RecordLifecycleFailureRequest) (recordings.RecordingLifecycleResult, error) {
	return recordings.RecordingLifecycleResult{}, nil
}

func (*phaseRecordingLifecycleStub) Flush(recordings.FlushLifecycleRequest) (recordings.RecordingLifecycleResult, error) {
	return recordings.RecordingLifecycleResult{}, nil
}

func (*phaseRecordingLifecycleStub) Stop(recordings.StopLifecycleRequest) error {
	return nil
}

func (*phaseRecordingLifecycleStub) Finish(recordings.FinishLifecycleRequest) (recordings.RecordingLifecycleResult, error) {
	return recordings.RecordingLifecycleResult{}, nil
}

func (*phaseRecordingLifecycleStub) Status(recordings.LifecycleStatusRequest) (recordings.RecordingLifecycleResult, error) {
	return recordings.RecordingLifecycleResult{}, nil
}
