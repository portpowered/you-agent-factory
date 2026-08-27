package wire_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testpath"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingswire "github.com/portpowered/infinite-you/pkg/services/recordings/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

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
		logging.NoopLogger{},
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
}

func TestReplayInputLoaderMetadataModeDoesNotMaterializePortableHistory(t *testing.T) {
	t.Parallel()

	const sessionID = "session-metadata-only-001"
	var payload strings.Builder
	fmt.Fprintf(
		&payload,
		`{"recordingKind":%q,"schemaVersion":"2","replayCompatibilityVersion":"1","session":{"id":%q},"events":[`,
		recordings.KindJavaScriptFactorySession,
		sessionID,
	)
	for index := range 256 {
		if index > 0 {
			payload.WriteByte(',')
		}
		fmt.Fprintf(&payload, `{"id":"event-%d","payload":%q}`, index, strings.Repeat("x", 4096))
	}
	payload.WriteString(`]}`)

	readCalls := 0
	openCalls := 0
	loader := recordingswire.NewReplayInputLoader(
		func(string) ([]byte, error) {
			readCalls++
			return nil, errors.New("metadata mode must not use the full replay reader")
		},
		func(string) (*recordings.ReplayArtifact, error) {
			return nil, errors.New("metadata mode must not use the full legacy loader")
		},
		logging.NoopLogger{},
		func(string) (io.ReadCloser, error) {
			openCalls++
			return io.NopCloser(strings.NewReader(payload.String())), nil
		},
	)

	result, err := loader.LoadReplayInput(recordings.LoadReplayInputRequest{
		Path: "recording.json", MetadataOnly: true,
	})
	if err != nil {
		t.Fatalf("LoadReplayInput(metadata) error = %v", err)
	}
	if result.Portable != nil || result.Legacy != nil {
		t.Fatalf("result = %#v, want metadata without replay values", result)
	}
	if result.Metadata == nil || result.Metadata.FactorySessionID != sessionID {
		t.Fatalf("metadata = %#v, want session %q", result.Metadata, sessionID)
	}
	if readCalls != 0 {
		t.Fatalf("full replay reader calls = %d, want zero", readCalls)
	}
	if openCalls != 2 {
		t.Fatalf("streaming opener calls = %d, want classification and metadata reads", openCalls)
	}
}

func TestReplayInputLoaderMetadataModeReadsOnlyV2Header(t *testing.T) {
	t.Parallel()

	const sessionID = "00000000-0000-4000-8000-000000000099"
	header := []byte(`{"recordType":"header","schemaVersion":"agent-factory.replay.v2","recordedAt":"2026-08-24T00:00:00Z","sessionId":"` + sessionID + `","factoryIdentity":{"id":"factory","name":"factory","factoryDirectory":"factory","sourceDirectory":"factory"},"hashes":{"factory_hash":"sha256:factory","workers_hash":"sha256:workers","workstations_hash":"sha256:workstations","runtime_config_hash":"sha256:runtime"}}` + "\n")
	openCalls := 0
	loader := recordingswire.NewReplayInputLoader(
		func(string) ([]byte, error) {
			return nil, errors.New("metadata mode must not use the full replay reader")
		},
		func(string) (*recordings.ReplayArtifact, error) {
			return nil, errors.New("metadata mode must not use the full legacy loader")
		},
		logging.NoopLogger{},
		func(string) (io.ReadCloser, error) {
			openCalls++
			return &headerOnlyReadCloser{data: header}, nil
		},
	)

	result, err := loader.LoadReplayInput(recordings.LoadReplayInputRequest{
		Path: "2026/08/24/" + sessionID + ".jsonl", MetadataOnly: true,
	})
	if err != nil {
		t.Fatalf("LoadReplayInput(metadata) error = %v", err)
	}
	if result.Metadata == nil || result.Metadata.FactorySessionID != sessionID {
		t.Fatalf("metadata = %#v, want session %q", result.Metadata, sessionID)
	}
	if openCalls != 2 {
		t.Fatalf("streaming opener calls = %d, want classification and metadata reads", openCalls)
	}
}

func TestReplayInputLoaderMetadataModeReadsLegacySessionIdentityWithoutEvents(t *testing.T) {
	t.Parallel()

	const sessionID = "session-legacy-metadata-001"
	payload := `{"schemaVersion":"replay.v1","events":[{"context":{"sessionId":"` + sessionID + `"},"id":"event-1","payload":{"private":"history"}},{"context":{"sessionId":"` + sessionID + `"},"id":"event-2","payload":{"private":"history"}}]}`
	readCalls := 0
	openCalls := 0
	loader := recordingswire.NewReplayInputLoader(
		func(string) ([]byte, error) {
			readCalls++
			return nil, errors.New("metadata mode must not use the full replay reader")
		},
		func(string) (*recordings.ReplayArtifact, error) {
			return nil, errors.New("metadata mode must not use the full legacy loader")
		},
		logging.NoopLogger{},
		func(string) (io.ReadCloser, error) {
			openCalls++
			return io.NopCloser(strings.NewReader(payload)), nil
		},
	)

	result, err := loader.LoadReplayInput(recordings.LoadReplayInputRequest{
		Path: "2026/08/24/legacy.json", MetadataOnly: true,
	})
	if err != nil {
		t.Fatalf("LoadReplayInput(metadata) error = %v", err)
	}
	if result.Portable != nil || result.Legacy != nil {
		t.Fatalf("result = %#v, want metadata without replay values", result)
	}
	if result.Metadata == nil || result.Metadata.FactorySessionID != sessionID {
		t.Fatalf("metadata = %#v, want session %q", result.Metadata, sessionID)
	}
	if readCalls != 0 {
		t.Fatalf("full replay reader calls = %d, want zero", readCalls)
	}
	if openCalls != 3 {
		t.Fatalf("streaming opener calls = %d, want classification and two legacy metadata reads", openCalls)
	}
}

type headerOnlyReadCloser struct {
	data []byte
	read bool
}

func (reader *headerOnlyReadCloser) Read(buffer []byte) (int, error) {
	if reader.read {
		return 0, errors.New("metadata reader consumed bytes after the v2 header")
	}
	reader.read = true
	return copy(buffer, reader.data), nil
}

func (reader *headerOnlyReadCloser) Close() error { return nil }

func TestReplayInputLoaderReturnsPortableDecodeDiagnostics(t *testing.T) {
	t.Parallel()

	path := testpath.MustRepoPathFromCaller(
		t,
		0,
		"pkg", "services", "recordings", "internal", "artifacts", "testdata", "valid-v2.json",
	)
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(payload, &document); err != nil {
		t.Fatal(err)
	}
	document["futureReplayField"] = json.RawMessage(`true`)
	payload, err = json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	loader := recordingswire.NewReplayInputLoader(
		func(string) ([]byte, error) { return payload, nil },
		nil,
		logging.NoopLogger{},
	)
	result, err := loader.LoadReplayInput(recordings.LoadReplayInputRequest{Path: "recording.json"})
	if err != nil {
		t.Fatalf("LoadReplayInput() error = %v", err)
	}
	if result.Diagnostics == nil {
		t.Fatal("replay diagnostics = nil, want ignored future-field path")
	}
	if !reflect.DeepEqual(result.Diagnostics.IgnoredJSONPaths, []string{"$.futureReplayField"}) {
		t.Fatalf("ignored paths = %#v, want future replay path", result.Diagnostics.IgnoredJSONPaths)
	}
}

func TestReplayInputLoaderLoadsCurrentWorkerHistoryExport(t *testing.T) {
	t.Parallel()

	path := testpath.MustRepoPathFromCaller(
		t,
		0,
		"pkg", "services", "recordings", "internal", "artifacts", "testdata", "valid-v3-worker-history.json",
	)
	loader := recordingswire.NewReplayInputLoader(
		recordings.RecordingReadFile(os.ReadFile),
		func(string) (*recordings.ReplayArtifact, error) {
			t.Fatal("legacy loader must not be called for a current portable recording")
			return nil, nil
		},
		logging.NoopLogger{},
	)
	result, err := loader.LoadReplayInput(recordings.LoadReplayInputRequest{Path: path})
	if err != nil {
		t.Fatalf("LoadReplayInput() error = %v", err)
	}
	if result.Portable == nil || result.Portable.SchemaVersion != recordings.PortableRecordingSchemaV3 {
		t.Fatalf("Portable = %#v, want current schema", result.Portable)
	}
	history := result.Portable.WorkerHistory
	if history == nil || history.Availability != recordings.PortableRecordingWorkerHistoryAvailable ||
		history.WorkerPortableRecording == nil || len(history.Records) != 3 {
		t.Fatalf("Worker history = %#v, want available ordered history", history)
	}
	if history.Correlation.FactorySessionID != result.Portable.Session.ID ||
		history.Records[1].Provenance.Fidelity != workers.FidelityNormalized {
		t.Fatalf("Worker correlation/fidelity = %#v", history)
	}
}

func TestReplayInputLoaderDelegatesLegacyArtifact(t *testing.T) {
	t.Parallel()

	requestedPath := ""
	tempFile := writeTempReplayInputFile(t, `{"schemaVersion":"legacy"}`)
	want := &recordings.ReplayArtifact{SchemaVersion: "legacy"}
	loader := recordingswire.NewReplayInputLoader(
		recordings.RecordingReadFile(os.ReadFile),
		func(path string) (*recordings.ReplayArtifact, error) {
			requestedPath = path
			return want, nil
		},
		logging.NoopLogger{},
	)
	result, err := loader.LoadReplayInput(recordings.LoadReplayInputRequest{Path: tempFile})
	if err != nil {
		t.Fatalf("LoadReplayInput() error = %v", err)
	}
	if result.Portable != nil {
		t.Fatal("Portable = non-nil, want nil for a legacy artifact")
	}
	if result.Legacy != want {
		t.Fatalf("Legacy = %v, want %v", result.Legacy, want)
	}
	if requestedPath != tempFile {
		t.Fatalf("legacy loader path = %q, want %q", requestedPath, tempFile)
	}
}

func TestReplayInputLoaderRejectsMissingReader(t *testing.T) {
	t.Parallel()

	loader := recordingswire.NewReplayInputLoader(nil, nil, logging.NoopLogger{})
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
		logging.NoopLogger{},
	)
	_, err := loader.LoadReplayInput(recordings.LoadReplayInputRequest{Path: "recording.json"})
	if !errors.Is(err, want) {
		t.Fatalf("LoadReplayInput() error = %v, want %v", err, want)
	}
}

func TestReplayInputLoaderRejectsMissingLegacyLoader(t *testing.T) {
	t.Parallel()

	tempFile := writeTempReplayInputFile(t, `{"schemaVersion":"legacy"}`)
	loader := recordingswire.NewReplayInputLoader(recordings.RecordingReadFile(os.ReadFile), nil, logging.NoopLogger{})
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
		logging.NoopLogger{},
	)
	result, err := loader.LoadReplayInput(recordings.LoadReplayInputRequest{Path: tempFile})
	if err == nil {
		t.Fatal("malformed portable recording error = nil")
	}
	if result.Portable != nil || result.Legacy != nil {
		t.Fatalf("result = %+v, want zero-value result on failure", result)
	}
}

func TestReplayInputLoaderRejectsTrailingPortableDocumentWithoutLegacyFallback(t *testing.T) {
	t.Parallel()

	path := testpath.MustRepoPathFromCaller(
		t,
		0,
		"pkg", "services", "recordings", "internal", "artifacts", "testdata", "valid-v2.json",
	)
	valid, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read valid portable recording: %v", err)
	}
	loader := recordingswire.NewReplayInputLoader(
		func(string) ([]byte, error) { return append(valid, []byte("\n{}")...), nil },
		func(string) (*recordings.ReplayArtifact, error) {
			t.Fatal("legacy loader must not be called for a trailing portable document")
			return nil, nil
		},
		logging.NoopLogger{},
	)

	result, err := loader.LoadReplayInput(recordings.LoadReplayInputRequest{Path: "recording.json"})
	if err == nil {
		t.Fatal("trailing portable recording error = nil")
	}
	if result.Portable != nil || result.Legacy != nil {
		t.Fatalf("result = %#v, want zero result", result)
	}
	var inputErr *recordings.ReplayInputError
	if !errors.As(err, &inputErr) || inputErr.Family != recordings.ReplayInputFamilyPortable {
		t.Fatalf("error = %v, want portable ReplayInputError", err)
	}
	if inputErr.Diagnostic.Code != recordings.ReplayArtifactDiagnosticMalformed {
		t.Fatalf("diagnostic = %#v, want malformed portable diagnostic", inputErr.Diagnostic)
	}
}

func TestReplayInputLoaderClassifiesLegacyLoaderFailure(t *testing.T) {
	t.Parallel()

	path := writeTempReplayInputFile(t, `{"schemaVersion":"legacy"}`)
	want := errors.New("legacy replay unavailable")
	loader := recordingswire.NewReplayInputLoader(
		recordings.RecordingReadFile(os.ReadFile),
		func(string) (*recordings.ReplayArtifact, error) { return nil, want },
		logging.NoopLogger{},
	)
	result, err := loader.LoadReplayInput(recordings.LoadReplayInputRequest{Path: path})
	if result.Portable != nil || result.Legacy != nil {
		t.Fatalf("result = %#v, want zero result on legacy load failure", result)
	}
	if !errors.Is(err, want) {
		t.Fatalf("LoadReplayInput() error = %v, want wrapped %v", err, want)
	}
	var inputErr *recordings.ReplayInputError
	if !errors.As(err, &inputErr) || inputErr.Family != recordings.ReplayInputFamilyLegacy {
		t.Fatalf("LoadReplayInput() error = %v, want legacy ReplayInputError", err)
	}
}

func TestReplayInputLoaderPublishesDetachedSafeDiagnostic(t *testing.T) {
	t.Parallel()

	loader := recordingswire.NewReplayInputLoader(
		func(string) ([]byte, error) {
			return []byte(`{"recordingKind":"` + recordings.KindJavaScriptFactorySession + `","schemaVersion":"2","replayCompatibilityVersion":"99"}`), nil
		},
		nil,
		logging.NoopLogger{},
	)

	_, err := loader.LoadReplayInput(recordings.LoadReplayInputRequest{Path: `C:\\private\\customer-recording.json`})
	first := requireReplayInputDiagnostic(t, err)
	if first.Code != recordings.ReplayArtifactDiagnosticUnsupportedVersion ||
		first.Area != "compatibility" ||
		first.Path != "replayCompatibilityVersion" ||
		len(first.SupportedVersions) == 0 {
		t.Fatalf("diagnostic = %#v, want detached supported-version facts", first)
	}
	first.Path = "mutated"
	first.SupportedVersions[0] = "mutated"

	_, err = loader.LoadReplayInput(recordings.LoadReplayInputRequest{Path: `C:\\private\\customer-recording.json`})
	second := requireReplayInputDiagnostic(t, err)
	if second.Path == "mutated" || second.SupportedVersions[0] == "mutated" {
		t.Fatalf("later diagnostic observed caller mutation: %#v", second)
	}
}

func TestReplayInputLoaderClassifiesUnsupportedSchemaSeparately(t *testing.T) {
	t.Parallel()

	loader := recordingswire.NewReplayInputLoader(
		func(string) ([]byte, error) {
			return []byte(`{"recordingKind":"` + recordings.KindJavaScriptFactorySession + `","schemaVersion":"99","replayCompatibilityVersion":"1","secret":"provider output"}`), nil
		},
		nil,
		logging.NoopLogger{},
	)

	_, err := loader.LoadReplayInput(recordings.LoadReplayInputRequest{Path: "recording.json"})
	diagnostic := requireReplayInputDiagnostic(t, err)
	if diagnostic.Code != recordings.ReplayArtifactDiagnosticUnsupportedSchema ||
		diagnostic.Area != "compatibility" || diagnostic.Path != "schemaVersion" ||
		diagnostic.EncounteredVersion != "99" || diagnostic.Action != recordings.PortableRecordingCompatibilityAction {
		t.Fatalf("diagnostic = %#v, want unsupported-schema compatibility facts", diagnostic)
	}
	if len(diagnostic.SupportedVersions) == 0 || strings.Contains(diagnostic.Message, "provider output") {
		t.Fatalf("diagnostic = %#v, want supported versions without payload content", diagnostic)
	}
}

func TestReplayInputLoaderMapsPortableValidationAreasToOwnedDiagnostics(t *testing.T) {
	t.Parallel()

	path := testpath.MustRepoPathFromCaller(
		t,
		0,
		"pkg", "services", "recordings", "internal", "artifacts", "testdata", "valid-v2.json",
	)
	validPayload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read valid portable recording: %v", err)
	}
	valid, err := recordings.DecodePortableRecording(bytes.NewReader(validPayload))
	if err != nil {
		t.Fatalf("decode valid portable recording: %v", err)
	}

	testCases := []struct {
		name     string
		mutate   func(*recordings.PortableRecording)
		wantCode recordings.ReplayArtifactDiagnosticCode
	}{
		{
			name:     "identity",
			mutate:   func(recording *recordings.PortableRecording) { recording.Session.ID = "" },
			wantCode: recordings.ReplayArtifactDiagnosticInvalidIdentity,
		},
		{
			name:     "summary",
			mutate:   func(recording *recordings.PortableRecording) { recording.Session.Status = "UNSUPPORTED" },
			wantCode: recordings.ReplayArtifactDiagnosticInvalidSummary,
		},
		{
			name:     "integrity",
			mutate:   func(recording *recordings.PortableRecording) { recording.Source.Hash = "invalid" },
			wantCode: recordings.ReplayArtifactDiagnosticInvalidIntegrity,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			recording := valid
			testCase.mutate(&recording)
			payload, err := json.Marshal(recording)
			if err != nil {
				t.Fatalf("marshal portable recording: %v", err)
			}
			loader := recordingswire.NewReplayInputLoader(
				func(string) ([]byte, error) { return payload, nil },
				nil,
				logging.NoopLogger{},
			)
			result, err := loader.LoadReplayInput(recordings.LoadReplayInputRequest{Path: "recording.json"})
			if result.Portable != nil || result.Legacy != nil {
				t.Fatalf("LoadReplayInput() result = %#v, want zero result", result)
			}
			if diagnostic := requireReplayInputDiagnostic(t, err); diagnostic.Code != testCase.wantCode {
				t.Fatalf("diagnostic = %#v, want code %q", diagnostic, testCase.wantCode)
			}
		})
	}
}

func TestReplayInputLoaderLogsSafeIntentAndTerminalOutcomes(t *testing.T) {
	t.Parallel()

	const privatePath = `C:\\customer\\private\\recording.json`
	const privatePayload = "customer-token-should-not-appear"
	const privateError = "reader failed with customer-token-should-not-appear"
	validPortable := testpath.MustRepoPathFromCaller(
		t,
		0,
		"pkg", "services", "recordings", "internal", "artifacts", "testdata", "valid-v2.json",
	)

	testCases := []struct {
		name       string
		readFile   recordings.RecordingReadFile
		loadLegacy recordings.ReplayArtifactLoader
		wantResult string
		wantClass  string
		wantFamily string
	}{
		{
			name: "portable success",
			readFile: func(string) ([]byte, error) {
				return os.ReadFile(validPortable)
			},
			wantResult: "success",
			wantFamily: string(recordings.ReplayInputFamilyPortable),
		},
		{
			name: "validation failure",
			readFile: func(string) ([]byte, error) {
				return []byte(`{"recordingKind":"` + recordings.KindJavaScriptFactorySession + `","unknown":"` + privatePayload + `"}`), nil
			},
			wantResult: "validation_failure",
			wantClass:  string(recordings.ReplayArtifactDiagnosticMalformed),
		},
		{
			name: "dependency failure",
			readFile: func(string) ([]byte, error) {
				return nil, errors.New(privateError)
			},
			wantResult: "dependency_failure",
			wantClass:  "read_failure",
		},
		{
			name: "cancellation",
			readFile: func(string) ([]byte, error) {
				return nil, context.Canceled
			},
			wantResult: "canceled",
			wantClass:  "read_failure",
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			logger := &capturingReplayInputLogger{}
			loader := recordingswire.NewReplayInputLoader(
				testCase.readFile,
				testCase.loadLegacy,
				logger,
			)
			_, err := loader.LoadReplayInput(recordings.LoadReplayInputRequest{Path: privatePath})
			if err != nil {
				diagnostic := requireReplayInputDiagnostic(t, err)
				serialized := fmt.Sprint(diagnostic)
				for _, unsafeValue := range []string{privatePath, privatePayload, privateError} {
					if strings.Contains(serialized, unsafeValue) {
						t.Fatalf("safe replay-input diagnostic leaked %q: %#v", unsafeValue, diagnostic)
					}
				}
			}
			assertReplayInputLogs(
				t,
				logger.entries,
				testCase.wantResult,
				testCase.wantClass,
				testCase.wantFamily,
				privatePath,
				privatePayload,
				privateError,
			)
		})
	}
}

func requireReplayInputDiagnostic(t *testing.T, err error) *recordings.ReplayArtifactDiagnostic {
	t.Helper()
	var inputErr *recordings.ReplayInputError
	if !errors.As(err, &inputErr) {
		t.Fatalf("error = %v, want ReplayInputError", err)
	}
	diagnostic := inputErr.Diagnostic
	return &diagnostic
}

type replayInputLogEntry struct {
	message string
	fields  map[string]any
}

type capturingReplayInputLogger struct {
	entries []replayInputLogEntry
}

func (logger *capturingReplayInputLogger) Debug(string, ...any)   {}
func (logger *capturingReplayInputLogger) Warn(string, ...any)    {}
func (logger *capturingReplayInputLogger) Error(string, ...any)   {}
func (logger *capturingReplayInputLogger) Verbose(string, ...any) {}

func (logger *capturingReplayInputLogger) Info(message string, fields ...any) {
	entry := replayInputLogEntry{message: message, fields: make(map[string]any, len(fields)/2)}
	for index := 0; index+1 < len(fields); index += 2 {
		entry.fields[fields[index].(string)] = fields[index+1]
	}
	logger.entries = append(logger.entries, entry)
}

// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
func assertReplayInputLogs(
	t *testing.T,
	entries []replayInputLogEntry,
	wantOutcome string,
	wantClass string,
	wantFamily string,
	forbidden ...string,
) {
	t.Helper()
	if len(entries) != 2 {
		t.Fatalf("log entries = %#v, want accepted intent and one terminal outcome", entries)
	}
	if entries[0].message != "recordings replay input accepted" ||
		entries[0].fields["operation"] != "load_replay_input" ||
		entries[0].fields["input_source"] != "filesystem_path" {
		t.Fatalf("intent log = %#v, want safe replay-input intent", entries[0])
	}
	if entries[1].message != "recordings replay input outcome" ||
		entries[1].fields["operation"] != "load_replay_input" ||
		entries[1].fields["outcome"] != wantOutcome {
		t.Fatalf("outcome log = %#v, want outcome=%q class=%q family=%q", entries[1], wantOutcome, wantClass, wantFamily)
	}
	if wantClass == "" && entries[1].fields["error_class"] != nil ||
		wantClass != "" && entries[1].fields["error_class"] != wantClass {
		t.Fatalf("outcome error_class = %#v, want %q", entries[1].fields["error_class"], wantClass)
	}
	if wantFamily == "" && entries[1].fields["replay_family"] != nil ||
		wantFamily != "" && entries[1].fields["replay_family"] != wantFamily {
		t.Fatalf("outcome replay_family = %#v, want %q", entries[1].fields["replay_family"], wantFamily)
	}
	for _, entry := range entries {
		serialized := entry.message + " " + fmt.Sprint(entry.fields)
		for _, value := range forbidden {
			if strings.Contains(serialized, value) {
				t.Fatalf("safe replay-input log leaked %q: %s", value, serialized)
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
