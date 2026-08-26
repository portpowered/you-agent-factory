package internal

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testpath"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

func TestLoadReplayArtifactTypedFailuresAndSuccess(t *testing.T) {
	t.Parallel()

	svc := NewService(&unusedLedger{}, NewProjectionService()).(*combinedService)
	if _, err := svc.LoadReplayArtifact(recordings.LoadReplayArtifactRequest{}); !errors.Is(err, recordings.ErrMissingReplayArtifact) {
		t.Fatalf("missing artifact = %v, want ErrMissingReplayArtifact", err)
	}
	if _, err := svc.LoadReplayArtifact(recordings.LoadReplayArtifactRequest{
		ArtifactID: "missing",
	}); !errors.Is(err, recordings.ErrInvalidReplayArtifact) {
		t.Fatalf("unknown artifact = %v, want ErrInvalidReplayArtifact", err)
	}
	svc.lifecycleMu.Lock()
	svc.replayByKey = nil
	svc.lifecycleMu.Unlock()
	if _, err := svc.LoadReplayArtifact(recordings.LoadReplayArtifactRequest{
		ArtifactID: "unavailable",
	}); !errors.Is(err, recordings.ErrInvalidReplayArtifact) {
		t.Fatalf("unavailable artifact store = %v, want ErrInvalidReplayArtifact", err)
	}

	artifact := &recordings.ReplayArtifact{SchemaVersion: "replay.v1"}
	svc.lifecycleMu.Lock()
	svc.replayByKey = map[string]*recordings.ReplayArtifact{
		"artifact:legacy": artifact,
	}
	svc.lifecycleMu.Unlock()

	loaded, err := svc.LoadReplayArtifact(recordings.LoadReplayArtifactRequest{
		Path: "artifact:legacy",
	})
	if err != nil {
		t.Fatalf("LoadReplayArtifact: %v", err)
	}
	if loaded.Artifact == artifact {
		t.Fatal("expected detached replay artifact copy")
	}
	if loaded.Artifact.SchemaVersion != artifact.SchemaVersion {
		t.Fatalf("artifact = %#v", loaded.Artifact)
	}
}

func TestRecordingScopeQueryAndReplayFailuresRemainTyped(t *testing.T) {
	t.Parallel()

	fixture := newFinalizedQueryFixture(t)
	assertProjectionCursorErrors(t, fixture)
	assertReplayPlanErrors(t, fixture)
	assertHistoricalSubscriptionBoundaryErrors(t, fixture)
}

func assertProjectionCursorErrors(t *testing.T, fixture *scopedQueryFixture) {
	t.Helper()
	malformed := fixture.events[0].Cursor
	malformed.StreamGenerationID = ""
	assertProjectionError(t, fixture, malformed, recordings.ErrInvalidReconnectCursor)
	foreign := fixture.events[0].Cursor
	foreign.StreamGenerationID = "other-generation"
	assertProjectionError(t, fixture, foreign, recordings.ErrReconnectCursorUnavailable)
	expired := fixture.events[0].Cursor
	expired.Sequence = 99
	assertProjectionError(t, fixture, expired, recordings.ErrReconnectCursorNotFound)
}

func assertProjectionError(t *testing.T, fixture *scopedQueryFixture, cursor recordings.CanonicalEventCursor, want error) {
	t.Helper()
	if _, err := fixture.root.ReconstructRecordingScope(context.Background(), recordings.ReconstructRecordingScopeRequest{
		Scope: fixture.ref, Through: &cursor,
	}); !errors.Is(err, want) {
		t.Fatalf("projection cursor %v error = %v, want %v", cursor, err, want)
	}
}

func assertReplayPlanErrors(t *testing.T, fixture *scopedQueryFixture) {
	t.Helper()
	if _, err := fixture.root.CreateReplayPlanScope(context.Background(), recordings.CreateReplayPlanScopeRequest{
		Scope: fixture.ref, SchemaVersion: "unsupported", Timing: recordings.ReplayTimingOrderOnly,
	}); !errors.Is(err, recordings.ErrUnsupportedReplayPlan) {
		t.Fatalf("unsupported replay schema error = %v, want ErrUnsupportedReplayPlan", err)
	}
	if _, err := fixture.root.ObserveReplayScope(context.Background(), recordings.ObserveReplayScopeRequest{
		Scope: fixture.ref, Plan: "missing-plan",
	}); !errors.Is(err, recordings.ErrReplayPlanNotFound) {
		t.Fatalf("missing replay plan error = %v, want ErrReplayPlanNotFound", err)
	}
}

func assertHistoricalSubscriptionBoundaryErrors(t *testing.T, fixture *scopedQueryFixture) {
	t.Helper()
	expired := fixture.events[0].Cursor
	expired.Sequence = 99
	if _, err := fixture.root.SubscribeRecordingScope(context.Background(), recordings.SubscribeRecordingScopeRequest{
		Scope: fixture.ref, Cursor: &expired,
	}); !errors.Is(err, recordings.ErrReconnectCursorExpired) {
		t.Fatalf("historical subscription expired cursor error = %v, want ErrReconnectCursorExpired", err)
	}
	subscription, err := fixture.root.SubscribeRecordingScope(context.Background(), recordings.SubscribeRecordingScopeRequest{
		Scope: fixture.ref,
	})
	if err != nil {
		t.Fatalf("SubscribeRecordingScope canceled delivery setup: %v", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if outcome := subscription.Subscription(canceled); outcome.Kind != recordings.SubscriptionClosed {
		t.Fatalf("historical subscription canceled outcome = %#v, want closed", outcome)
	}
}

func TestClosedRecordingScopeRejectsEveryReadOperation(t *testing.T) {
	t.Parallel()

	fixture := newFinalizedQueryFixture(t)
	closed, err := fixture.root.CloseRecordingScope(context.Background(), recordings.CloseRecordingScopeRequest{
		Scope: fixture.ref, FinishedAt: time.Unix(1_700_000_200, 0).UTC(),
	})
	if err != nil || !closed.Closed {
		t.Fatalf("CloseRecordingScope = %#v, error %v", closed, err)
	}
	repeated, err := fixture.root.CloseRecordingScope(context.Background(), recordings.CloseRecordingScopeRequest{
		Scope: fixture.ref, FinishedAt: time.Unix(1_700_000_201, 0).UTC(),
	})
	if err != nil || !repeated.Closed || !reflect.DeepEqual(repeated.Status, closed.Status) {
		t.Fatalf("repeated CloseRecordingScope = %#v, error %v", repeated, err)
	}
	calls := closedScopeCalls(fixture)
	for name, call := range calls {
		if err := call(); !errors.Is(err, recordings.ErrRecordingScopeClosed) {
			t.Errorf("closed scope %s error = %v, want ErrRecordingScopeClosed", name, err)
		}
	}
}

func TestRecordingScopeOperationsEmitStructuredLifecycleLogs(t *testing.T) {
	t.Parallel()

	logger := &recordingOperationLogger{}
	root := newScopedQueryRootWithLogger(t, logger)
	started, err := root.BeginRecordingScope(context.Background(), recordings.BeginRecordingScopeRequest{
		Enabled: true,
		Scope:   recordings.CanonicalEventScope{FactorySessionID: "logged-scope"},
		Target:  recordings.RecordingTargetRequest{Artifact: "recording://logged"},
	})
	if err != nil {
		t.Fatalf("BeginRecordingScope: %v", err)
	}
	if _, err := root.QueryRecordingScope(context.Background(), recordings.QueryRecordingScopeRequest{Scope: started.Scope}); err != nil {
		t.Fatalf("QueryRecordingScope: %v", err)
	}
	if _, err := root.CloseRecordingScope(context.Background(), recordings.CloseRecordingScopeRequest{
		Scope: started.Scope, FinishedAt: time.Unix(1_700_000_300, 0).UTC(),
	}); err != nil {
		t.Fatalf("CloseRecordingScope: %v", err)
	}

	if len(logger.infos) < 6 {
		t.Fatalf("recording operation logs = %#v, want start and finish for three operations", logger.infos)
	}
	for _, entry := range logger.infos {
		if entry.message != "recordings operation started" && entry.message != "recordings operation finished" {
			t.Fatalf("unexpected operation log = %#v", entry)
		}
		if entry.fields["operation"] == "" {
			t.Fatalf("operation log missing operation name: %#v", entry)
		}
		if entry.fields["scope_ref"] == started.Scope.String() && entry.fields["factory_session_id"] != nil {
			t.Fatalf("scope log duplicated session identity unexpectedly: %#v", entry)
		}
	}
}

func closedScopeCalls(fixture *scopedQueryFixture) map[string]func() error {
	root, ref := fixture.root, fixture.ref
	return map[string]func() error{
		"append": func() error {
			_, err := root.AppendRecordingScopeEvent(context.Background(), recordings.AppendRecordingScopeEventRequest{Scope: ref, Event: fixture.events[0]})
			return err
		},
		"subscribe": func() error {
			_, err := root.SubscribeRecordingScope(context.Background(), recordings.SubscribeRecordingScopeRequest{Scope: ref})
			return err
		},
		"flush": func() error {
			_, err := root.FlushRecordingScope(context.Background(), recordings.FlushRecordingScopeRequest{Scope: ref})
			return err
		},
		"finalize": func() error {
			_, err := root.FinalizeRecordingScope(context.Background(), recordings.FinalizeRecordingScopeRequest{Scope: ref})
			return err
		},
		"query": func() error {
			_, err := root.QueryRecordingScope(context.Background(), recordings.QueryRecordingScopeRequest{Scope: ref})
			return err
		},
		"replay": func() error {
			_, err := root.LoadReplayRecordingScope(context.Background(), recordings.LoadReplayRecordingScopeRequest{Scope: ref})
			return err
		},
		"plan": func() error {
			_, err := root.CreateReplayPlanScope(context.Background(), recordings.CreateReplayPlanScopeRequest{Scope: ref})
			return err
		},
		"observe": func() error {
			_, err := root.ObserveReplayScope(context.Background(), recordings.ObserveReplayScopeRequest{Scope: ref})
			return err
		},
		"projection": func() error {
			_, err := root.ReconstructRecordingScope(context.Background(), recordings.ReconstructRecordingScopeRequest{Scope: ref})
			return err
		},
		"dashboard": func() error {
			_, err := root.QuerySimpleDashboardScope(context.Background(), recordings.QuerySimpleDashboardScopeRequest{Scope: ref})
			return err
		},
		"workstation": func() error {
			_, err := root.QueryWorkstationRequestsScope(context.Background(), recordings.QueryWorkstationRequestsScopeRequest{Scope: ref})
			return err
		},
		"artifact": func() error {
			_, err := root.BuildPortableArtifactScope(context.Background(), recordings.BuildPortableArtifactScopeRequest{Scope: ref})
			return err
		},
		"export": func() error {
			_, err := root.ExportPortableArtifactScope(context.Background(), recordings.ExportPortableArtifactScopeRequest{Scope: ref})
			return err
		},
		"read": func() error {
			_, err := root.ReadPortableArtifactScope(context.Background(), recordings.ReadPortableArtifactScopeRequest{Scope: ref, Reference: fixture.exported.Reference})
			return err
		},
	}
}

func TestRecordingScopeQueriesRemainIsolatedAcrossConcurrentScopes(t *testing.T) {
	t.Parallel()

	root := NewService(&stubLedger{}, NewProjectionService())
	opened := make([]openedScopedQuery, 2)
	for index, sessionID := range []string{"query-scope-a", "query-scope-b"} {
		eventScope := recordings.CanonicalEventScope{FactorySessionID: sessionID}
		started, err := root.BeginRecordingScope(context.Background(), recordings.BeginRecordingScopeRequest{
			Enabled: true, Scope: eventScope,
			Target: recordings.RecordingTargetRequest{Artifact: recordings.RecordingArtifactReference("recording://" + sessionID)},
		})
		if err != nil {
			t.Fatalf("BeginRecordingScope(%s): %v", sessionID, err)
		}
		if _, err := root.AppendRecordingScopeEvent(context.Background(), recordings.AppendRecordingScopeEventRequest{
			Scope: started.Scope, Event: scopedScopeEvent(sessionID+"-event", 0, eventScope),
		}); err != nil {
			t.Fatalf("AppendRecordingScopeEvent(%s): %v", sessionID, err)
		}
		opened[index] = openedScopedQuery{ref: started.Scope, scope: eventScope}
	}
	assertConcurrentScopeProjections(t, root, opened)
	if _, err := root.QueryRecordingScope(context.Background(), recordings.QueryRecordingScopeRequest{Scope: malformedScope(opened[0].ref)}); !errors.Is(err, recordings.ErrRecordingScopeInvalid) {
		t.Fatalf("malformed scope query = %v, want invalid scope", err)
	}
	if _, err := root.QuerySimpleDashboardScope(context.Background(), recordings.QuerySimpleDashboardScopeRequest{Scope: opened[0].ref, SelectedTick: -1}); !errors.Is(err, recordings.ErrInvalidProjectionInput) {
		t.Fatalf("invalid scoped projection = %v, want ErrInvalidProjectionInput", err)
	}
}

func assertConcurrentScopeProjections(t *testing.T, root recordings.Service, opened []openedScopedQuery) {
	t.Helper()
	var wait sync.WaitGroup
	errs := make(chan error, 20)
	for _, selected := range opened {
		selected := selected
		for range 10 {
			wait.Add(1)
			go func() {
				defer wait.Done()
				result, err := root.ReconstructRecordingScope(context.Background(), recordings.ReconstructRecordingScopeRequest{
					Scope: selected.ref, SelectedTick: 1,
				})
				if err != nil {
					errs <- err
					return
				}
				if result.WorldState.Scope != selected.scope || result.Status.EventScope != selected.scope || result.Status.AcceptedEvents != 1 {
					errs <- errors.New("concurrent scoped projection crossed recording ownership")
				}
			}()
		}
	}
	wait.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}

func TestBindReplayExecutionPublishedSuccessShape(t *testing.T) {
	t.Parallel()

	svc := NewService(&unusedLedger{}, NewProjectionService()).(*combinedService)
	if _, err := svc.BindReplayExecution(recordings.BindReplayExecutionRequest{}); !errors.Is(err, recordings.ErrUnsupportedReplayBinding) {
		t.Fatalf("missing artifact = %v, want ErrUnsupportedReplayBinding", err)
	}
	result, err := svc.BindReplayExecution(recordings.BindReplayExecutionRequest{
		Artifact: &recordings.ReplayArtifact{SchemaVersion: "replay.v1"},
	})
	if err != nil {
		t.Fatalf("BindReplayExecution: %v", err)
	}
	if result.Hooks == nil {
		t.Fatalf("hooks = %#v, want empty published slice", result.Hooks)
	}
}

type unusedLedger struct {
	recordings.Ledger
}

func TestReplayInputLoaderReturnsPortableAndLegacyDomainOutcomes(t *testing.T) {
	t.Parallel()

	portablePath := testpath.MustRepoPathFromCaller(
		t,
		0,
		"pkg", "services", "recordings", "internal", "artifacts", "testdata", "valid-v2.json",
	)
	portablePayload, err := os.ReadFile(portablePath)
	if err != nil {
		t.Fatalf("read portable replay fixture: %v", err)
	}

	wantLegacy := &recordings.ReplayArtifact{SchemaVersion: "legacy-v1"}
	cases := []struct {
		name         string
		readFile     recordings.RecordingReadFile
		loadLegacy   recordings.ReplayArtifactLoader
		wantPortable bool
		wantLegacy   *recordings.ReplayArtifact
	}{
		{
			name: "portable recording",
			readFile: func(string) ([]byte, error) {
				return portablePayload, nil
			},
			loadLegacy: func(string) (*recordings.ReplayArtifact, error) {
				t.Fatal("legacy loader called for portable recording")
				return nil, nil
			},
			wantPortable: true,
		},
		{
			name: "legacy artifact",
			readFile: func(string) ([]byte, error) {
				return []byte(`{"schemaVersion":"legacy"}`), nil
			},
			loadLegacy: func(path string) (*recordings.ReplayArtifact, error) {
				if path != "recording.json" {
					t.Fatalf("legacy path = %q, want recording.json", path)
				}
				return wantLegacy, nil
			},
			wantLegacy: wantLegacy,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			loader := NewReplayInputLoader(testCase.readFile, nil, testCase.loadLegacy, nil, logging.NoopLogger{})
			result, err := loader.LoadReplayInput(recordings.LoadReplayInputRequest{Path: "recording.json"})
			if err != nil {
				t.Fatalf("LoadReplayInput() error = %v", err)
			}
			if (result.Portable != nil) != testCase.wantPortable {
				t.Fatalf("Portable = %#v, want present=%t", result.Portable, testCase.wantPortable)
			}
			if !reflect.DeepEqual(result.Legacy, testCase.wantLegacy) {
				t.Fatalf("Legacy = %#v, want %#v", result.Legacy, testCase.wantLegacy)
			}
			if testCase.wantPortable && (result.Portable == nil || result.Portable.Session.ID != "session-js-001") {
				t.Fatalf("Portable session = %#v, want session-js-001", result.Portable)
			}
		})
	}
}

func TestReplayInputLoaderClassifiesDependencyAndValidationFailures(t *testing.T) {
	t.Parallel()

	legacyUnavailable := errors.New("legacy source unavailable")
	tests := []struct {
		name       string
		readFile   recordings.RecordingReadFile
		loadLegacy recordings.ReplayArtifactLoader
		wantCause  error
		wantFamily recordings.ReplayInputFamily
		wantCode   recordings.ReplayArtifactDiagnosticCode
	}{
		{
			name:       "reader unavailable",
			wantFamily: recordings.ReplayInputFamilyPortable,
			wantCode:   recordings.ReplayArtifactDiagnosticDependencyFailure,
		},
		{
			name: "reader canceled",
			readFile: func(string) ([]byte, error) {
				return nil, context.Canceled
			},
			wantCause:  context.Canceled,
			wantFamily: recordings.ReplayInputFamilyPortable,
			wantCode:   recordings.ReplayArtifactDiagnosticCancelled,
		},
		{
			name: "legacy loader unavailable",
			readFile: func(string) ([]byte, error) {
				return []byte(`{"schemaVersion":"legacy"}`), nil
			},
			wantFamily: recordings.ReplayInputFamilyPortable,
			wantCode:   recordings.ReplayArtifactDiagnosticDependencyFailure,
		},
		{
			name: "legacy loader failure",
			readFile: func(string) ([]byte, error) {
				return []byte(`{"schemaVersion":"legacy"}`), nil
			},
			loadLegacy: func(string) (*recordings.ReplayArtifact, error) {
				return nil, legacyUnavailable
			},
			wantCause:  legacyUnavailable,
			wantFamily: recordings.ReplayInputFamilyLegacy,
			wantCode:   recordings.ReplayArtifactDiagnosticDependencyFailure,
		},
		{
			name: "malformed portable recording",
			readFile: func(string) ([]byte, error) {
				return []byte(`{"recordingKind":"` + recordings.KindJavaScriptFactorySession + `","schemaVersion":"not-supported"}`), nil
			},
			loadLegacy: func(string) (*recordings.ReplayArtifact, error) {
				t.Fatal("legacy loader called for malformed portable recording")
				return nil, nil
			},
			wantFamily: recordings.ReplayInputFamilyPortable,
			wantCode:   recordings.ReplayArtifactDiagnosticMalformed,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			loader := NewReplayInputLoader(testCase.readFile, nil, testCase.loadLegacy, nil, logging.NoopLogger{})
			result, err := loader.LoadReplayInput(recordings.LoadReplayInputRequest{Path: "recording.json"})
			if result.Portable != nil || result.Legacy != nil {
				t.Fatalf("LoadReplayInput() result = %#v, want zero result", result)
			}
			if err == nil {
				t.Fatal("LoadReplayInput() error = nil, want classified failure")
			}
			var inputErr *recordings.ReplayInputError
			if !errors.As(err, &inputErr) {
				t.Fatalf("error = %v, want ReplayInputError", err)
			}
			if inputErr.Family != testCase.wantFamily || inputErr.Diagnostic.Code != testCase.wantCode {
				t.Fatalf("error = %#v, want family=%q code=%q", inputErr, testCase.wantFamily, testCase.wantCode)
			}
			if testCase.wantCause != nil && !errors.Is(err, testCase.wantCause) {
				t.Fatalf("error = %v, want cause %v", err, testCase.wantCause)
			}
		})
	}
}

func TestReplayInputLoaderPreservesDetachedPortableDiagnostics(t *testing.T) {
	t.Parallel()

	loader := NewReplayInputLoader(
		func(string) ([]byte, error) {
			return []byte(`{"recordingKind":"` + recordings.KindJavaScriptFactorySession + `","schemaVersion":"2","replayCompatibilityVersion":"99"}`), nil
		},
		nil,
		nil,
		nil,
		logging.NoopLogger{},
	)

	_, err := loader.LoadReplayInput(recordings.LoadReplayInputRequest{Path: `C:\customer\private\recording.json`})
	var inputErr *recordings.ReplayInputError
	if !errors.As(err, &inputErr) {
		t.Fatalf("error = %v, want ReplayInputError", err)
	}
	if inputErr.Diagnostic.Code != recordings.ReplayArtifactDiagnosticUnsupportedVersion ||
		inputErr.Diagnostic.Area != "compatibility" ||
		inputErr.Diagnostic.Path != "replayCompatibilityVersion" ||
		len(inputErr.Diagnostic.SupportedVersions) == 0 {
		t.Fatalf("diagnostic = %#v, want detached unsupported-version facts", inputErr.Diagnostic)
	}
	inputErr.Diagnostic.Path = "mutated"
	inputErr.Diagnostic.SupportedVersions[0] = "mutated"

	_, err = loader.LoadReplayInput(recordings.LoadReplayInputRequest{Path: "recording.json"})
	var second *recordings.ReplayInputError
	if !errors.As(err, &second) {
		t.Fatalf("second error = %v, want ReplayInputError", err)
	}
	if second.Diagnostic.Path == "mutated" || second.Diagnostic.SupportedVersions[0] == "mutated" {
		t.Fatalf("later diagnostic observed caller mutation: %#v", second.Diagnostic)
	}
}

func TestReplayInputLoaderReportsIgnoredFutureFields(t *testing.T) {
	t.Parallel()

	path := testpath.MustRepoPathFromCaller(
		t,
		0,
		"pkg", "services", "recordings", "internal", "artifacts", "testdata", "valid-v2.json",
	)
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read portable replay fixture: %v", err)
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(payload, &document); err != nil {
		t.Fatalf("decode portable replay fixture: %v", err)
	}
	document["futureReplayField"] = json.RawMessage(`true`)
	payload, err = json.Marshal(document)
	if err != nil {
		t.Fatalf("encode portable replay fixture: %v", err)
	}

	loader := NewReplayInputLoader(
		func(string) ([]byte, error) { return payload, nil },
		nil,
		nil,
		nil,
		logging.NoopLogger{},
	)
	result, err := loader.LoadReplayInput(recordings.LoadReplayInputRequest{Path: "recording.json"})
	if err != nil {
		t.Fatalf("LoadReplayInput() error = %v", err)
	}
	if result.Diagnostics == nil || !reflect.DeepEqual(result.Diagnostics.IgnoredJSONPaths, []string{"$.futureReplayField"}) {
		t.Fatalf("diagnostics = %#v, want ignored future field", result.Diagnostics)
	}
}

type recordingOperationLogEntry struct {
	message string
	fields  map[string]any
}

type recordingOperationLogger struct {
	infos []recordingOperationLogEntry
}

func (logger *recordingOperationLogger) Debug(string, ...any)   {}
func (logger *recordingOperationLogger) Warn(string, ...any)    {}
func (logger *recordingOperationLogger) Error(string, ...any)   {}
func (logger *recordingOperationLogger) Verbose(string, ...any) {}

func (logger *recordingOperationLogger) Info(message string, fields ...any) {
	values := make(map[string]any, len(fields)/2)
	for index := 0; index+1 < len(fields); index += 2 {
		key, ok := fields[index].(string)
		if ok {
			values[key] = fields[index+1]
		}
	}
	logger.infos = append(logger.infos, recordingOperationLogEntry{message: message, fields: values})
}
