package service

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/factorydefinitionfixtures"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	operatorconfig "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

type boardReferenceSource struct {
	factorydefinitions.MutableLoadedFactorySource
	directory string
}

func (source boardReferenceSource) FactoryDir() string { return source.directory }

type boardReferenceOwner struct {
	durableexecution.Service
	path                        string
	failure                     error
	saveFailure                 error
	durable                     bool
	loads, saves                int
	savedFactory, savedArtifact string
}

func (owner *boardReferenceOwner) LoadCurrentBoard(context.Context, string) (string, error) {
	owner.loads++
	return owner.path, owner.failure
}

func (owner *boardReferenceOwner) SaveCurrentBoard(_ context.Context, factory, artifact string) error {
	owner.saves++
	owner.savedFactory, owner.savedArtifact = factory, artifact
	if owner.saveFailure != nil {
		return owner.saveFailure
	}
	return owner.failure
}

func (owner *boardReferenceOwner) HasDurableState(context.Context, string) (bool, error) {
	return owner.durable, owner.failure
}

func TestCurrentBoardIntentSurvivesRuntimeActivation(t *testing.T) {
	t.Parallel()
	selected := factorysessions.SessionStartRequest{
		RuntimeSelection: &factorysessions.SessionRuntimeSelection{
			Mode:      factorysessions.SessionRuntimeModeService,
			Host:      factorysessions.RuntimeHostRequest{Port: 1234},
			Recording: factorysessions.SessionRecordingSelection{ImplicitCurrentBoard: true},
		},
	}
	inputs := runtimeActivationInputs(factorydefinitions.RuntimeSelection{}, selected, false,
		workers.RuntimeSelection{}, recordings.RuntimeSelection{}, "", operatorconfig.ResolvedDefaults{}, nil)
	restored := sessionRequestFromActivation(factoryruntime.RuntimeActivationRequest{
		Inputs: inputs, Runtime: factoryruntime.RuntimeSelection{Mode: factorydefinitions.RuntimeModeService},
	})
	if !restored.RuntimeSelection.Recording.ImplicitCurrentBoard ||
		restored.RuntimeSelection.Mode != selected.RuntimeSelection.Mode ||
		restored.RuntimeSelection.Host.Port != selected.RuntimeSelection.Host.Port {
		t.Fatalf("activation lost startup selection: %#v", restored.RuntimeSelection)
	}
}

func TestCurrentBoardRecordingRequiresCanonicalRepositoryIdentity(t *testing.T) {
	t.Parallel()
	directory := filepath.Join(t.TempDir(), "factory")
	for _, tc := range []struct {
		name, directory string
		kind            factorydefinitions.FactoryEventType
		valid           bool
	}{
		{"same repository", directory, factorydefinitions.FactoryEventTypeRunRequest, true},
		{"sibling repository", directory + "-sibling", factorydefinitions.FactoryEventTypeRunRequest, false},
		{"missing directory", "", factorydefinitions.FactoryEventTypeRunRequest, false},
		{"relative directory", "factory", factorydefinitions.FactoryEventTypeRunRequest, false},
		{"no canonical request", directory, factorydefinitions.FactoryEventTypeWorkRequest, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			payload, err := json.Marshal(map[string]any{"factory": map[string]string{"factoryDirectory": tc.directory}})
			if err != nil {
				t.Fatal(err)
			}
			err = validateCurrentBoardFactoryDirectory([]factorydefinitions.FactoryEvent{{Type: tc.kind, Payload: payload}}, directory)
			if (err == nil) != tc.valid {
				t.Fatalf("canonical repository validation = %v, want valid=%v", err, tc.valid)
			}
		})
	}
}

func TestCurrentBoardRecordingRejectsForeignContinuation(t *testing.T) {
	t.Parallel()
	directory := filepath.Join(t.TempDir(), "factory")
	request := func(directory string) factorydefinitions.FactoryEvent {
		payload, err := json.Marshal(map[string]any{"factory": map[string]string{"factoryDirectory": directory}})
		if err != nil {
			t.Fatal(err)
		}
		return factorydefinitions.FactoryEvent{Type: factorydefinitions.FactoryEventTypeRunRequest, Payload: payload}
	}
	for _, tc := range []struct {
		name   string
		events []factorydefinitions.FactoryEvent
		valid  bool
	}{
		{"same repository continuation", []factorydefinitions.FactoryEvent{request(directory), request(directory)}, true},
		{"foreign continuation", []factorydefinitions.FactoryEvent{request(directory), request(directory + "-sibling")}, false},
		{"foreign origin", []factorydefinitions.FactoryEvent{request(directory + "-sibling"), request(directory)}, false},
		{"missing continuation identity", []factorydefinitions.FactoryEvent{request(directory), request("")}, false},
		{"malformed continuation", []factorydefinitions.FactoryEvent{request(directory), {Type: factorydefinitions.FactoryEventTypeRunRequest, Payload: json.RawMessage(`{"private":`)}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateCurrentBoardFactoryDirectory(tc.events, directory)
			if (err == nil) != tc.valid {
				t.Fatalf("continuation validation = %v, want valid=%v", err, tc.valid)
			}
			if err != nil && strings.Contains(err.Error(), "private") {
				t.Fatal("repository diagnostic exposed event content")
			}
		})
	}
}

func TestCurrentBoardReferenceSelectionAndPublication(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"selected", "fresh", "legacy requires history", "read failure", "explicit", "batch", "peer", "resume", "replay"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			owner := &boardReferenceOwner{path: "retained.json", durable: true}
			selection := &factorysessions.SessionRuntimeSelection{
				Recording: factorysessions.SessionRecordingSelection{ImplicitCurrentBoard: true},
				Mode:      factorysessions.SessionRuntimeModeService,
				Host:      factorysessions.RuntimeHostRequest{Port: 1234},
			}
			opening := &sessionRuntimeOpening{
				sessionID: "~default", sessionSelection: selection,
				load:             RuntimeLoad{LoadedFactoryCfg: boardReferenceSource{directory: "factory-directory"}},
				durableExecution: DurableExecution{Service: owner},
				configured:       preparedRuntime{Recordings: recordings.RuntimeSelection{RecordPath: "fresh.json"}},
			}
			bypass, wantError := configureBoardReferenceCase(name, owner, opening)
			err := (&Root{}).selectCurrentBoardReference(t.Context(), opening)
			if (err != nil) != wantError {
				t.Fatalf("selection error = %v, want error %v", err, wantError)
			}
			if bypass {
				if name == "resume" {
					if owner.loads != 0 || owner.saves != 0 {
						t.Fatal("resume selected the old reference instead of its explicit source")
					}
					return // Successor publication is covered by the explicit eligibility table.
				}
				if err := opening.publishCurrentBoardReference(t.Context()); err != nil || owner.loads != 0 || owner.saves != 0 {
					t.Fatal("explicit/peer/batch opening touched reference")
				}
				return
			}
			if wantError {
				if opening.configured.Recordings.RecordPath != "fresh.json" || owner.saves != 0 {
					t.Fatal("failed selection changed opening or reference")
				}
				return
			}
			assertCurrentBoardPublication(t, name, owner, opening)
		})
	}
}

type currentBoardTargetPlanner struct {
	recordings.RuntimeScopeService
	target  recordings.LiveRecordingTarget
	err     error
	calls   int
	request recordings.LiveRecordingTargetRequest
}

func (planner *currentBoardTargetPlanner) PlanLiveRecordingTarget(request recordings.LiveRecordingTargetRequest) (recordings.LiveRecordingTarget, error) {
	planner.calls++
	planner.request = request
	return planner.target, planner.err
}

func TestFreshCurrentBoardReservationPreservesSelectionAndFailure(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"fresh", "retained", "restored predecessor", "explicit", "cancelled", "reservation failure", "empty target"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			planner := &currentBoardTargetPlanner{target: recordings.LiveRecordingTarget{ServicePath: "fresh.json"}}
			selection := &factorysessions.SessionRuntimeSelection{
				Recording: factorysessions.SessionRecordingSelection{ImplicitCurrentBoard: true},
				Mode:      factorysessions.SessionRuntimeModeService, Host: factorysessions.RuntimeHostRequest{Port: 1234},
				SystemConfigHome: "profile", CanonicalSessionID: "canonical-id",
			}
			opening := &sessionRuntimeOpening{sessionID: "~default", sessionSelection: selection}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			wantCalls, wantError, wantPath := 1, false, "fresh.json"
			switch name {
			case "retained":
				opening.configured.Recordings.RecordPath = "retained.json"
				selection.Recording.RecordPath = "retained.json"
				wantCalls, wantPath = 0, "retained.json"
			case "restored predecessor":
				opening.configured.Recordings.RecordPath = "predecessor.json"
				selection.Recording.RecordPath = "predecessor.json"
				opening.restoredWorldState = &factorydefinitions.FactoryWorldState{}
			case "explicit":
				selection.Recording.ImplicitCurrentBoard = false
				wantCalls, wantPath = 0, ""
			case "cancelled":
				cancel()
				wantCalls, wantError, wantPath = 0, true, ""
			case "reservation failure":
				planner.err = errors.New("controlled reservation failure")
				wantError, wantPath = true, ""
			case "empty target":
				planner.target = recordings.LiveRecordingTarget{}
				wantError, wantPath = true, ""
			}
			root := &Root{recordingsRuntime: planner}
			err := root.reserveFreshCurrentBoard(ctx, opening)
			if (err != nil) != wantError || planner.calls != wantCalls {
				t.Fatalf("reservation error/calls = %v/%d, want error=%v calls=%d", err, planner.calls, wantError, wantCalls)
			}
			if opening.configured.Recordings.RecordPath != wantPath || selection.Recording.RecordPath != wantPath {
				t.Fatal("reservation did not preserve or publish the selected target")
			}
			if planner.calls > 0 && (planner.request.HomeDir != "profile" || planner.request.CanonicalSessionID != "canonical-id" || planner.request.ReportedSessionID != "~default") {
				t.Fatalf("reservation lost invocation identity: %#v", planner.request)
			}
		})
	}
}

func TestReplayRequestsHistoricalInspectionUsesEffectiveListenerPort(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name string
		host factorysessions.RuntimeHostRequest
		want bool
	}{
		{
			name: "offline clears port but retains parsed auto port default",
			host: factorysessions.RuntimeHostRequest{AutoPort: true},
			want: true,
		},
		{
			name: "hosted replay has a resolved listener port",
			host: factorysessions.RuntimeHostRequest{Port: 7437, AutoPort: true},
			want: false,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if got := replayRequestsHistoricalInspection(testCase.host); got != testCase.want {
				t.Fatalf("replayRequestsHistoricalInspection() = %t, want %t", got, testCase.want)
			}
		})
	}
}

func TestResolveRuntimeRootNormalizesSharedProcessInputs(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core).With(zap.String("backend", "selected-process"))
	root, err := ResolveRuntimeRoot(filepath.Join(dir, "."), logger, "", func() string { return "runtime-id" }, os.UserHomeDir)
	if err != nil {
		t.Fatalf("resolve runtime root: %v", err)
	}
	if root.FactoryRootDir != filepath.Clean(dir) {
		t.Fatalf("root = %q, want %q", root.FactoryRootDir, filepath.Clean(dir))
	}
	root.BaseLogger.Info("runtime root selected")
	entries := logs.All()
	if len(entries) != 1 || entries[0].ContextMap()["backend"] != "selected-process" {
		t.Fatalf("runtime root diagnostics = %#v, want selected process backend", entries)
	}
	if root.RuntimeInstanceID != "runtime-id" {
		t.Fatalf("runtime instance ID = %q, want runtime-id", root.RuntimeInstanceID)
	}
}

func TestResolveRuntimeRootPreservesExplicitIdentityWithoutGenerator(t *testing.T) {
	root, err := ResolveRuntimeRoot(t.TempDir(), zap.NewNop(), "explicit-runtime", nil, os.UserHomeDir)
	if err != nil {
		t.Fatalf("ResolveRuntimeRoot: %v", err)
	}
	if root.RuntimeInstanceID != "explicit-runtime" {
		t.Fatalf("runtime instance ID = %q", root.RuntimeInstanceID)
	}
}

func TestEnsureBackendScopePreservesExplicitSelectionAndPropagatesFailures(t *testing.T) {
	if err := ensureBackendScope(nil, nil, nil); err == nil {
		t.Fatal("nil request accepted")
	}
	request := &factorysessions.SessionStartRequest{RuntimeSelection: &factorysessions.SessionRuntimeSelection{BackendScopeID: "existing"}}
	if err := ensureBackendScope(nil, request, nil); err != nil || request.RuntimeSelection.BackendScopeID != "existing" {
		t.Fatalf("explicit backend scope = %+v, error = %v", request.RuntimeSelection, err)
	}
	request.RuntimeSelection.BackendScopeID = ""
	if err := ensureBackendScope(nil, request, nil); err == nil {
		t.Fatal("missing Operator Settings ensurer accepted")
	}
	called := ""
	request.RuntimeSelection.SystemConfigPath = "/config/operator.json"
	err := ensureBackendScope(func(path string) (operatorconfig.ResolvedBackendScope, error) {
		called = path
		return operatorconfig.ResolvedBackendScope{BackendScopeID: "local-1"}, nil
	}, request, nil)
	if err != nil || called != "/config/operator.json" || request.RuntimeSelection.BackendScopeID != "local-1" {
		t.Fatalf("resolved backend scope = %+v, path = %q, error = %v", request.RuntimeSelection, called, err)
	}
	request.RuntimeSelection.BackendScopeID = ""
	request.RuntimeSelection.SystemConfigPath = ""
	request.RuntimeSelection.SystemConfigHome = ""
	if err := ensureBackendScope(func(string) (operatorconfig.ResolvedBackendScope, error) {
		return operatorconfig.ResolvedBackendScope{}, nil
	}, request, nil); err == nil {
		t.Fatal("missing operator config home accepted")
	}
	request.RuntimeSelection.SystemConfigHome = t.TempDir()
	want := errors.New("backend scope unavailable")
	if err := ensureBackendScope(func(string) (operatorconfig.ResolvedBackendScope, error) {
		return operatorconfig.ResolvedBackendScope{}, want
	}, request, nil); !errors.Is(err, want) {
		t.Fatalf("backend scope error = %v", err)
	}
}

func TestEnsureDefaultCanonicalSessionIDOnlyAllocatesForNewDefault(t *testing.T) {
	if err := ensureDefaultCanonicalSessionID(nil, "", nil); err != nil {
		t.Fatal(err)
	}
	request := &factorysessions.SessionStartRequest{}
	if err := ensureDefaultCanonicalSessionID(request, "recording.json", nil); err != nil {
		t.Fatalf("replay allocated a new identity: %v", err)
	}
	request.SessionID = "named-session"
	if err := ensureDefaultCanonicalSessionID(request, "", nil); err != nil {
		t.Fatalf("named session required a default identity: %v", err)
	}
	request.SessionID = factorysessions.DefaultSessionID
	if err := ensureDefaultCanonicalSessionID(request, "", nil); err == nil {
		t.Fatal("default session accepted missing identity generator")
	}
	if err := ensureDefaultCanonicalSessionID(request, "", func() string { return "  " }); err == nil {
		t.Fatal("default session accepted empty generated identity")
	}
	if err := ensureDefaultCanonicalSessionID(request, "", func() string { return "canonical-1" }); err != nil || request.RuntimeSelection.CanonicalSessionID != "canonical-1" {
		t.Fatalf("default canonical ID = %+v, error = %v", request.RuntimeSelection, err)
	}
	if err := ensureDefaultCanonicalSessionID(request, "", nil); err != nil {
		t.Fatalf("existing canonical ID was not retained: %v", err)
	}
}

func TestActivationRequestDefersCanonicalIdentityUntilRuntimeActivation(t *testing.T) {
	t.Parallel()

	const canonicalID = "550e8400-e29b-41d4-a716-446655440000"
	var canonicalCalls atomic.Int32
	factory := &Root{
		generateSessionID: func() string {
			canonicalCalls.Add(1)
			return canonicalID
		},
		generateRuntimeInstanceID: func() string { return "runtime-1" },
		snapshotSelection:         NewRuntimeSnapshotSelection((activationDefinitionsStub{snapshot: activationSnapshot()}).ResolveRuntimeSnapshot, nil, nil, nil, nil),
	}
	activation, err := factory.activationRequest(context.Background(), factorysessions.SessionStartRequest{
		FolderPath: "/factory",
	})
	if err != nil {
		t.Fatalf("activationRequest() error = %v", err)
	}
	if activation.FactorySessionID != factorysessions.DefaultSessionID || activation.RuntimeID != "runtime-1" {
		t.Fatalf("activation identity = %#v, want default/runtime-1", activation)
	}
	if activation.Inputs.Session.CanonicalSessionID != "" {
		t.Fatalf("canonical session identity = %q, want deferred allocation", activation.Inputs.Session.CanonicalSessionID)
	}
	if got := canonicalCalls.Load(); got != 0 {
		t.Fatalf("canonical session ID generator calls = %d, want 0 before Runtime activation", got)
	}
}

func TestActivationOpeningDefersCanonicalIdentityUntilDefinitionAdmission(t *testing.T) {
	t.Parallel()

	const canonicalID = "550e8400-e29b-41d4-a716-446655440000"
	var canonicalCalls atomic.Int32
	factory := &Root{
		generateSessionID: func() string {
			canonicalCalls.Add(1)
			return canonicalID
		},
		generateRuntimeInstanceID: func() string { return "runtime-1" },
	}
	runtimeSelection := factoryruntime.RuntimeSelection{}
	session := sessionOwnerFixture{FactorySessionID: factorysessions.DefaultSessionID}
	runtimeID, err := factory.ensureActivationRuntimeID(&runtimeSelection)
	if err != nil {
		t.Fatalf("ensureActivationRuntimeID(resume) error = %v", err)
	}
	if runtimeID != "runtime-1" {
		t.Fatalf("runtime ID = %q, want runtime-1", runtimeID)
	}
	if session.CanonicalSessionID != "" {
		t.Fatalf("successor canonical session ID = %q, want empty before definition admission", session.CanonicalSessionID)
	}
	if got := canonicalCalls.Load(); got != 0 {
		t.Fatalf("canonical session ID generator calls = %d, want 0 before definition admission", got)
	}
}

func TestActivationOpeningDefersCanonicalIdentityForAliasOnlyResume(t *testing.T) {
	t.Parallel()

	const canonicalID = "550e8400-e29b-41d4-a716-446655440000"
	factory := &Root{
		generateRuntimeInstanceID: func() string { return canonicalID },
	}
	runtimeSelection := factoryruntime.RuntimeSelection{}
	session := sessionOwnerFixture{}
	_, err := factory.ensureActivationRuntimeID(&runtimeSelection)
	if err != nil {
		t.Fatalf("ensureActivationRuntimeID(alias-only resume) error = %v", err)
	}
	if session.CanonicalSessionID != "" {
		t.Fatalf("alias-only successor canonical session ID = %q, want empty before definition admission", session.CanonicalSessionID)
	}
}

func TestResolveRuntimeRootFailsClosedWithoutRequiredIdentityGenerator(t *testing.T) {
	_, err := ResolveRuntimeRoot(t.TempDir(), zap.NewNop(), "", nil, os.UserHomeDir)
	if err == nil || !strings.Contains(err.Error(), "ID generator is required") {
		t.Fatalf("error = %v, want missing ID generator failure", err)
	}
	_, err = ResolveRuntimeRoot(t.TempDir(), zap.NewNop(), "", func() string { return "  " }, os.UserHomeDir)
	if err == nil || !strings.Contains(err.Error(), "empty identity") {
		t.Fatalf("error = %v, want empty generated identity failure", err)
	}
}

func TestResolveDefinitionPathPreservesReplayAndExplicitSourceSelection(t *testing.T) {
	t.Parallel()

	replay := factorydefinitions.RuntimeSelection{Directory: "factory-root"}
	got, err := resolveDefinitionPath(&replay, "recording.json", nil, nil)
	if err != nil || got != "factory-root" {
		t.Fatalf("replay path = (%q, %v), want (factory-root, nil)", got, err)
	}

	sourcePath := filepath.Join(t.TempDir(), "factory.yaml")
	explicit := factorydefinitions.RuntimeSelection{
		Directory:  "factory-root",
		SourcePath: sourcePath,
	}
	got, err = resolveDefinitionPath(&explicit, "", nil, func() (string, error) {
		return t.TempDir(), nil
	})
	if err != nil || got != filepath.Clean(sourcePath) {
		t.Fatalf("explicit source path = (%q, %v), want (%q, nil)", got, err, sourcePath)
	}
	if explicit.Directory != "factory-root" {
		t.Fatalf("explicit source changed runtime root to %q", explicit.Directory)
	}
}

func TestResolveDefinitionPathResolvesCurrentFactoryAndErrors(t *testing.T) {
	t.Parallel()

	definition := factorydefinitions.RuntimeSelection{Directory: "factory-root"}
	currentDir := filepath.Join(t.TempDir(), "current")
	got, err := resolveDefinitionPath(
		&definition,
		"",
		func(root string) (string, error) {
			if root != "factory-root" {
				t.Fatalf("current root = %q, want factory-root", root)
			}
			return currentDir, nil
		},
		func() (string, error) { return t.TempDir(), nil },
	)
	if err != nil || got != filepath.Clean(currentDir) || definition.Directory != got {
		t.Fatalf("current Factory path = (%q, %v, directory %q)", got, err, definition.Directory)
	}

	if _, err := resolveDefinitionPath(
		&factorydefinitions.RuntimeSelection{Directory: "factory-root"},
		"",
		nil,
		nil,
	); err == nil || !strings.Contains(err.Error(), "named Factory path resolver is required") {
		t.Fatalf("missing current resolver error = %v", err)
	}

	want := errors.New("current unavailable")
	if _, err := resolveDefinitionPath(
		&factorydefinitions.RuntimeSelection{Directory: "factory-root"},
		"",
		func(string) (string, error) { return "", want },
		nil,
	); !errors.Is(err, want) {
		t.Fatalf("current resolver error = %v, want %v", err, want)
	}

	if _, err := resolveDefinitionPath(
		&factorydefinitions.RuntimeSelection{SourcePath: "~\\factory.yaml"},
		"",
		nil,
		func() (string, error) { return "", want },
	); !errors.Is(err, want) {
		t.Fatalf("source home resolver error = %v, want %v", err, want)
	}
}

func TestOpenActivatedRuntimeRoutesRoleCleanupThroughRuntimeDeactivation(t *testing.T) {
	t.Parallel()

	root := &cleanupRoutingRoot{}
	factory := &Root{
		runtimeRoot:               root,
		generateRuntimeInstanceID: func() string { return "runtime-1" },
		snapshotSelection:         NewRuntimeSnapshotSelection((activationDefinitionsStub{snapshot: activationSnapshot()}).ResolveRuntimeSnapshot, nil, nil, nil, nil),
	}

	products, err := factory.openActivatedRuntime(context.Background(), factorysessions.SessionStartRequest{
		FolderPath: "/factory",
	})
	if err != nil {
		t.Fatalf("openActivatedRuntime() error = %v", err)
	}
	if root.activations != 1 {
		t.Fatalf("Runtime root activations = %d, want exactly one", root.activations)
	}

	roleCleanups := []struct {
		name  string
		close func() error
	}{
		{name: "application", close: products.closeArtifacts},
		{name: "invocation", close: products.closeArtifacts},
		{name: "execution", close: products.closeArtifacts},
	}
	for _, role := range roleCleanups {
		if role.close == nil {
			t.Fatalf("%s cleanup edge = nil, want the Runtime deactivation operation", role.name)
		}
	}
	if root.deactivations != 0 {
		t.Fatalf("Runtime deactivations before cleanup = %d, want zero", root.deactivations)
	}

	for _, role := range roleCleanups {
		if err := role.close(); err != nil {
			t.Fatalf("%s cleanup error = %v", role.name, err)
		}
	}
	if root.deactivations != 1 {
		t.Fatalf(
			"Runtime deactivations after draining every role cleanup = %d, want exactly one Runtime-routed deactivation",
			root.deactivations,
		)
	}

	// Opening publishes the Runtime root itself; it does not hand callers a
	// Sessions-retained runtime handle recovered from the opening products.
	if products.factoryRuntime != factoryruntime.Service(root) {
		t.Fatalf(
			"opened application FactoryRuntime = %T, want the Runtime root %T",
			products.factoryRuntime,
			root,
		)
	}
}

type cleanupRoutingRoot struct {
	factoryruntime.Service
	activations   int
	deactivations int
}

func (root *cleanupRoutingRoot) Activate(
	context.Context,
	factoryruntime.RuntimeActivationRequest,
	factoryruntime.RuntimeActivationOperation,
) (factoryruntime.RuntimeActivationResult, error) {
	root.activations++
	return factoryruntime.RuntimeActivationResult{
		RuntimeID: "runtime-1",
		Runtime: factoryruntime.RuntimeActivationView{
			RuntimeID: "runtime-1",
			Service:   root,
		},
	}, nil
}

func (root *cleanupRoutingRoot) Deactivate(
	context.Context,
	factoryruntime.RuntimeDeactivationRequest,
) (factoryruntime.RuntimeDeactivationResult, error) {
	root.deactivations++
	return factoryruntime.RuntimeDeactivationResult{}, nil
}

func TestWarnReplayMetadataMismatchesResolvesCurrentOperatorDefaults(t *testing.T) {
	t.Parallel()

	defaults := operatorconfig.ResolvedDefaults{
		WorkerModelProvider: "CODEX",
		WorkerModel:         "replay-model",
	}
	factoryConfig := &factorydefinitions.FactoryConfig{
		Name: "factory",
		Workers: []factorydefinitions.FactoryWorkerConfig{{
			Name: "worker",
			Type: factorydefinitions.WorkerTypeModel,
		}},
	}
	factoryDir := t.TempDir()
	recorded, err := factorydefinitionfixtures.NewLoadedSource(
		factoryDir, factoryConfig, nil, nil,
	)
	if err != nil {
		t.Fatalf("construct recorded source: %v", err)
	}
	if err := applyOperatorDefaults(recorded, defaults); err != nil {
		t.Fatalf("apply recorded operator defaults: %v", err)
	}
	capture := runtimeLoadedFactorySnapshotCapturer()
	artifactFactory, err := capture(recorded, factoryDir, nil)
	if err != nil {
		t.Fatalf("capture recorded source: %v", err)
	}

	current, err := factorydefinitionfixtures.NewLoadedSource(
		factoryDir, factoryConfig, nil, nil,
	)
	if err != nil {
		t.Fatalf("construct current source: %v", err)
	}
	core, logs := observer.New(zapcore.WarnLevel)
	NewRuntimeInputLoading(func(string, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
		return current, nil
	},
		factorydefinitionfixtures.NewLoadedSource,
		runtimeInputUnusedDecoder,
		runtimeInputReplayFunc(func(recordings.LoadReplayInputRequest) (recordings.LoadReplayInputResult, error) {
			panic("unexpected replay read")
		}),
		capture,
		runtimeInputSessionLogger,
		zap.New(core)).warnReplayMetadataMismatches(factoryDir, "recording.replay.json", &factorydefinitions.ReplayArtifact{Factory: artifactFactory}, zap.New(core), defaults)
	if logs.Len() != 0 {
		t.Fatalf("equivalent effective config emitted replay metadata warnings: %v", logs.All())
	}
}

func TestWarnReplayMetadataMismatchesReturnsStructuredComponentWarnings(t *testing.T) {
	t.Parallel()

	factoryDir := t.TempDir()
	recordedConfig := &factorydefinitions.FactoryConfig{
		Name: "factory",
		Workers: []factorydefinitions.FactoryWorkerConfig{{
			Name: "worker", Type: factorydefinitions.WorkerTypeModel, Body: "recorded prompt",
		}},
	}
	currentConfig := &factorydefinitions.FactoryConfig{
		Name: "factory",
		Workers: []factorydefinitions.FactoryWorkerConfig{{
			Name: "worker", Type: factorydefinitions.WorkerTypeModel, Body: "changed prompt",
		}},
	}
	recorded, err := factorydefinitionfixtures.NewLoadedSource(factoryDir, recordedConfig, nil, nil)
	if err != nil {
		t.Fatalf("construct recorded source: %v", err)
	}
	current, err := factorydefinitionfixtures.NewLoadedSource(factoryDir, currentConfig, nil, nil)
	if err != nil {
		t.Fatalf("construct current source: %v", err)
	}
	capture := runtimeLoadedFactorySnapshotCapturer()
	artifactFactory, err := capture(recorded, factoryDir, nil)
	if err != nil {
		t.Fatalf("capture recorded source: %v", err)
	}
	core, logs := observer.New(zapcore.WarnLevel)
	warnings := NewRuntimeInputLoading(func(string, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
		return current, nil
	},
		factorydefinitionfixtures.NewLoadedSource,
		runtimeInputUnusedDecoder,
		runtimeInputReplayFunc(func(recordings.LoadReplayInputRequest) (recordings.LoadReplayInputResult, error) {
			panic("unexpected replay read")
		}),
		capture,
		runtimeInputSessionLogger,
		zap.New(core)).warnReplayMetadataMismatches(factoryDir, "recording.replay.json", &factorydefinitions.ReplayArtifact{Factory: artifactFactory}, zap.New(core), operatorconfig.ResolvedDefaults{})
	gotKeys := make(map[string]bool, len(warnings))
	for _, warning := range warnings {
		gotKeys[warning.Key] = true
	}
	for _, key := range []string{"factory_hash", "workers_hash", "runtime_config_hash"} {
		if !gotKeys[key] {
			t.Fatalf("metadata warnings = %#v, missing %q", warnings, key)
		}
	}
	if logs.Len() != len(warnings) {
		t.Fatalf("structured warning log count = %d, want %d", logs.Len(), len(warnings))
	}
	for _, entry := range logs.All() {
		if entry.ContextMap()["metadata_key"] == nil {
			t.Fatalf("structured warning omitted metadata_key: %#v", entry)
		}
	}
}

type canonicalProjectionReplayInputLoader struct {
	artifact         *recordings.ReplayArtifact
	state            recordings.FactoryWorldState
	loadCalls        int
	reconstructTicks []int
}

func (loader *canonicalProjectionReplayInputLoader) LoadReplayInput(
	request recordings.LoadReplayInputRequest,
) (recordings.LoadReplayInputResult, error) {
	loader.loadCalls++
	return recordings.LoadReplayInputResult{
		Legacy:       loader.artifact,
		LegacyFormat: string(recordings.RecordedSessionFormatV2JSONL),
	}, nil
}

func (loader *canonicalProjectionReplayInputLoader) ReconstructCanonicalFactoryWorldState(
	_ []recordings.FactoryEvent,
	selectedTick int,
) (recordings.FactoryWorldState, error) {
	loader.reconstructTicks = append(loader.reconstructTicks, selectedTick)
	return loader.state, nil
}

func TestLoadRuntimeRoutesStructurallyValidLegacyReplayToDetachedProjection(t *testing.T) {
	t.Parallel()

	factorySnapshot, err := factorydefinitions.NewFactorySnapshot(map[string]any{"name": "legacy-history"})
	if err != nil {
		t.Fatalf("NewFactorySnapshot: %v", err)
	}
	eventTime := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	artifact := &recordings.ReplayArtifact{
		SchemaVersion: "legacy", Factory: factorySnapshot,
		Events: []recordings.FactoryEvent{{
			SchemaVersion: recordings.FactoryEventSchemaVersionV1,
			Id:            "legacy-event-1", Type: recordings.FactoryEventTypeFactoryStateResponse,
			Context: recordings.FactoryEventContext{Tick: 7, Sequence: 0, EventTime: eventTime},
			Payload: json.RawMessage(`{"state":"RUNNING"}`),
		}},
	}
	loader := &canonicalProjectionReplayInputLoader{
		artifact: artifact,
		state:    recordings.FactoryWorldState{FactoryState: "RUNNING"},
	}

	loaded, err := NewRuntimeInputLoading(runtimeInputNoCurrentSource,
		factorydefinitionfixtures.NewLoadedSource,
		runtimeInputUnusedDecoder,
		loader,
		runtimeInputUnusedCapture,
		func(base *zap.Logger, _, _, _ string) *zap.Logger { return base },
		zap.NewNop()).Load(RuntimeInputLoadRequest{Dir: t.TempDir(),
		ExecutionBaseDir:     "",
		ReplayPath:           "legacy-replay.jsonl",
		OperatorDefaults:     operatorconfig.ResolvedDefaults{},
		FactoryRootDir:       t.TempDir(),
		HistoricalInspection: true})
	if err != nil {
		t.Fatalf("LoadRuntime: %v", err)
	}
	if loader.loadCalls != 1 || len(loader.reconstructTicks) != 1 || loader.reconstructTicks[0] != 7 {
		t.Fatalf("legacy loader calls/ticks = %d/%v, want one load and selected tick 7", loader.loadCalls, loader.reconstructTicks)
	}
	if loaded.ReplayArtifact != artifact || loaded.PortableRecording != nil || loaded.LoadedFactoryCfg != nil {
		t.Fatalf("legacy runtime load = %#v, want artifact plus detached replay only", loaded)
	}
	if loaded.HistoricalReplay == nil ||
		loaded.HistoricalReplay.FactoryProjection == nil ||
		loaded.HistoricalReplay.FactoryProjection.FactoryState != "RUNNING" {
		t.Fatalf("legacy historical replay = %#v, want canonical projection", loaded.HistoricalReplay)
	}
	inspection := factorysessions.HistoricalReplayFactoryProjection{
		Availability: factorysessions.HistoricalReplayFactoryProjectionAvailable,
		State:        loaded.HistoricalReplay.FactoryProjection,
	}
	if inspection.Availability != factorysessions.HistoricalReplayFactoryProjectionAvailable || inspection.State == nil {
		t.Fatalf("legacy projection availability = %#v, want AVAILABLE", inspection)
	}
}

type legacyProjectionRecordingsRoot struct {
	*recordingsRootConstructionStub
	loader *canonicalProjectionReplayInputLoader
}

func (root *legacyProjectionRecordingsRoot) LoadReplayInput(
	request recordings.LoadReplayInputRequest,
) (recordings.LoadReplayInputResult, error) {
	return root.loader.LoadReplayInput(request)
}

func (root *legacyProjectionRecordingsRoot) ReconstructCanonicalFactoryWorldState(
	events []recordings.FactoryEvent,
	selectedTick int,
) (recordings.FactoryWorldState, error) {
	return root.loader.ReconstructCanonicalFactoryWorldState(events, selectedTick)
}

func TestNewFactorySelectsLegacyHistoricalReplayBeforeLiveRuntimeAssembly(t *testing.T) {
	t.Parallel()

	factorySnapshot, err := factorydefinitions.NewFactorySnapshot(map[string]any{"name": "legacy-history"})
	if err != nil {
		t.Fatalf("NewFactorySnapshot: %v", err)
	}
	artifact := &recordings.ReplayArtifact{
		SchemaVersion: "legacy", Factory: factorySnapshot,
		Events: []recordings.FactoryEvent{{
			SchemaVersion: recordings.FactoryEventSchemaVersionV1,
			Id:            "legacy-event-1", Type: recordings.FactoryEventTypeFactoryStateResponse,
			Context: recordings.FactoryEventContext{Tick: 9, Sequence: 0, EventTime: time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)},
			Payload: json.RawMessage(`{"state":"RUNNING"}`),
		}},
	}
	loader := &canonicalProjectionReplayInputLoader{
		artifact: artifact,
		state:    recordings.FactoryWorldState{FactoryState: "RUNNING"},
	}
	root := &legacyProjectionRecordingsRoot{
		recordingsRootConstructionStub: &recordingsRootConstructionStub{}, loader: loader,
	}
	calls := 0
	dependencies := validRuntimeOpeningCollaborators(&calls)
	dependencies.RecordingsService = root
	dependencies.RecordingsRuntime = root
	// Metadata drift inspection may consult the current authored Factory, but
	// this selection test keeps the live-runtime fail-on-call counter focused
	// on activation collaborators.
	dependencies.LoadFactory = func(string, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
		return nil, nil
	}
	dependencies.GenerateRuntimeInstanceID = func() string { return "legacy-runtime" }
	dependencies.NewSessionLogger = func(*zap.Logger, string, string, string) *zap.Logger {
		return zap.NewNop()
	}
	factory, err := dependencies.newFactory()
	if err != nil {
		t.Fatalf("NewFactory: %v", err)
	}
	opened, historical, err := factory.InspectHistoricalApplication(t.Context(), factorysessions.SessionStartRequest{
		FolderPath:       t.TempDir(),
		RuntimeSelection: &factorysessions.SessionRuntimeSelection{Recording: factorysessions.SessionRecordingSelection{ReplayPath: "legacy-replay.jsonl"}},
	})
	if err != nil {
		t.Fatalf("InspectHistoricalApplication: %v", err)
	}
	if !historical || opened.Replay == nil || opened.Replay.FactoryProjection.Availability != factorysessions.HistoricalReplayFactoryProjectionAvailable {
		t.Fatalf("opened legacy replay = %#v, want AVAILABLE historical projection", opened.Replay)
	}
	if opened.Close == nil {
		t.Fatalf("opened historical inspection = %#v, want cleanup", opened)
	}
	if loader.loadCalls != 1 || len(loader.reconstructTicks) != 1 || loader.reconstructTicks[0] != 9 {
		t.Fatalf("legacy selection calls/ticks = %d/%v, want one load and selected tick 9", loader.loadCalls, loader.reconstructTicks)
	}
	if calls != 0 {
		t.Fatalf("legacy historical opening invoked %d live collaborators, want zero", calls)
	}
}

func TestLegacyCurrentBoardCanonicalSelection(t *testing.T) {
	t.Parallel()
	event := func(id, payload string) factorydefinitions.FactoryEvent {
		return factorydefinitions.FactoryEvent{Id: id, Type: factorydefinitions.FactoryEventTypeWorkRequest, Payload: json.RawMessage(payload)}
	}
	a, b, c := event("a", `{"text":"§ —","value":1}`), event("b", `{}`), event("c", `{}`)
	reordered := event("a", `{"value":1,"text":"§ —"}`)
	changed := event("a", `{"text":"secret","value":1}`)
	for _, tc := range []struct {
		name      string
		witness   []factorydefinitions.FactoryEvent
		histories map[string][]factorydefinitions.FactoryEvent
		want      string
	}{
		{"unique", []factorydefinitions.FactoryEvent{a}, map[string][]factorydefinitions.FactoryEvent{"board": {a, b}}, "board"},
		{"canonical JSON", []factorydefinitions.FactoryEvent{reordered}, map[string][]factorydefinitions.FactoryEvent{"board": {a, b}}, "board"},
		{"stale prefix", []factorydefinitions.FactoryEvent{a}, map[string][]factorydefinitions.FactoryEvent{"stale": {a}, "board": {a, b}}, ""},
		{"equal duplicate tie", []factorydefinitions.FactoryEvent{a}, map[string][]factorydefinitions.FactoryEvent{"copy": {a, b}, "board": {a, b}}, ""},
		{"incomparable branch", []factorydefinitions.FactoryEvent{a}, map[string][]factorydefinitions.FactoryEvent{"branch": {a, c}, "board": {a, b}}, ""},
		{"wrong payload", []factorydefinitions.FactoryEvent{a}, map[string][]factorydefinitions.FactoryEvent{"board": {changed, b}}, ""},
		{"missing", []factorydefinitions.FactoryEvent{a}, nil, ""},
		{"empty witness", nil, map[string][]factorydefinitions.FactoryEvent{"board": {a}}, ""},
		{"wrong order", []factorydefinitions.FactoryEvent{a, b}, map[string][]factorydefinitions.FactoryEvent{"board": {b, a}}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			matching := make(map[string]time.Time)
			for path, events := range tc.histories {
				if currentBoardContainsFacts(events, tc.witness) {
					matching[path] = time.Time{}
				}
			}
			got, err := selectNewestCurrentBoard(matching)
			if got != tc.want || (err != nil) != (tc.want == "") {
				t.Fatalf("selection=%q, %v; want %q", got, err, tc.want)
			}
			if err != nil && strings.Contains(err.Error(), "secret") {
				t.Fatal("diagnostic exposed payload")
			}
		})
	}
}

func TestLegacyCurrentBoardNewestWriteSelection(t *testing.T) {
	t.Parallel()
	old := time.Date(2026, time.July, 29, 0, 0, 0, 0, time.UTC)
	newest := old.Add(time.Hour)
	for _, tc := range []struct {
		name      string
		histories map[string]time.Time
		want      string
	}{
		{"newest wins opposed path order", map[string]time.Time{"z-old": old, "a-new": newest}, "a-new"},
		{"other path order", map[string]time.Time{"a-old": old, "z-new": newest}, "z-new"},
		{"newest tie", map[string]time.Time{"old": old, "new": newest, "copy": newest}, ""},
		{"older tie irrelevant", map[string]time.Time{"old": old, "copy": old, "new": newest}, "new"},
		{"unknown order", map[string]time.Time{"unknown": {}, "new": newest}, ""},
		{"single unknown", map[string]time.Time{"only": {}}, "only"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := selectNewestCurrentBoard(tc.histories)
			if got != tc.want || (err != nil) != (tc.want == "") {
				t.Fatalf("selection=%q error=%v, want %q", got, err, tc.want)
			}
		})
	}
}

func configureBoardReferenceCase(name string, owner *boardReferenceOwner, opening *sessionRuntimeOpening) (bool, bool) {
	bypass, wantError := false, false
	switch name {
	case "fresh":
		owner.path, owner.durable = "", false
	case "legacy requires history":
		owner.path, owner.durable, wantError = "", true, true
	case "read failure":
		owner.failure, wantError = errors.New("controlled reference failure"), true
	case "explicit":
		opening.sessionSelection.Recording.ImplicitCurrentBoard, bypass = false, true
	case "batch":
		opening.sessionSelection.Mode, bypass = factorysessions.SessionRuntimeModeBatch, true
	case "peer":
		opening.sessionID, bypass = "peer-session", true
	case "resume":
		opening.configured.Recordings.ResumePath, bypass = "resume.json", true
	case "replay":
		opening.configured.Recordings.ReplayPath, bypass = "replay.json", true
	}
	return bypass, wantError
}

func assertCurrentBoardPublication(t *testing.T, name string, owner *boardReferenceOwner, opening *sessionRuntimeOpening) {
	t.Helper()
	wantPath := owner.path
	if name == "fresh" {
		wantPath = ""
	}
	if opening.configured.Recordings.RecordPath != wantPath || opening.hasCurrentBoardReference != (name == "selected") {
		t.Fatalf("selected path/presence = %s/%v", opening.configured.Recordings.RecordPath, opening.hasCurrentBoardReference)
	}
	if owner.saves != 0 {
		t.Fatal("selection prematurely published reference")
	}
	if name == "fresh" {
		planner := &currentBoardTargetPlanner{target: recordings.LiveRecordingTarget{ServicePath: "fresh.json"}}
		if !opening.emptyCurrentBoard {
			t.Fatal("missing snapshot did not select empty startup")
		}
		if err := (&Root{recordingsRuntime: planner}).reserveFreshCurrentBoard(t.Context(), opening); err != nil {
			t.Fatal(err)
		}
		wantPath = "fresh.json"
	}
	if err := opening.publishCurrentBoardReference(t.Context()); err != nil || owner.saves != 1 || owner.savedFactory != "factory-directory" || owner.savedArtifact != wantPath {
		t.Fatalf("publication = %s/%s/%v", owner.savedFactory, owner.savedArtifact, err)
	}
}

func TestCurrentBoardMissingSnapshotSkipsStaleHistory(t *testing.T) {
	t.Parallel()
	for _, path := range []string{"retained.json", "missing.json", ""} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			owner := &boardReferenceOwner{path: path}
			opening := &sessionRuntimeOpening{
				sessionID: "~default",
				sessionSelection: &factorysessions.SessionRuntimeSelection{
					Mode:      factorysessions.SessionRuntimeModeService,
					Host:      factorysessions.RuntimeHostRequest{Port: 1234},
					Recording: factorysessions.SessionRecordingSelection{ImplicitCurrentBoard: true, RecordPath: "planned.json"},
				},
				load:             RuntimeLoad{LoadedFactoryCfg: boardReferenceSource{directory: "factory-directory"}},
				durableExecution: DurableExecution{Service: owner},
				configured:       preparedRuntime{Recordings: recordings.RuntimeSelection{RecordPath: "planned.json"}},
			}
			// No history reader is injected: missing state must never replay the
			// stale reference, even if its target is unreadable or absent.
			if err := (&Root{}).selectCurrentBoardReference(t.Context(), opening); err != nil {
				t.Fatal(err)
			}
			if err := (&Root{}).restoreSessionOpeningHistory(t.Context(), opening); err != nil {
				t.Fatal(err)
			}
			if !opening.emptyCurrentBoard || opening.hasCurrentBoardReference ||
				opening.configured.Recordings.RecordPath != "" || opening.sessionSelection.Recording.RecordPath != "" ||
				opening.restoredWorldState != nil || len(opening.restoredEventHistory) != 0 || owner.saves != 0 {
				t.Fatal("missing snapshot selected or published stale history")
			}
		})
	}
}

func (owner *boardReferenceOwner) SaveCurrentBoardIfAbsent(ctx context.Context, factory, artifact string) error {
	if owner.path != "" {
		return nil
	}
	return owner.SaveCurrentBoard(ctx, factory, artifact)
}

func TestCurrentBoardReferenceExplicitRestoreEligibility(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"restored", "existing", "fresh", "foreign", "batch", "peer", "resume", "replay", "no record", "no server", "publication failure"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			directory, artifact := filepath.Join(t.TempDir(), "factory"), filepath.Join(t.TempDir(), "board.json")
			identity, _ := json.Marshal(map[string]any{"factory": map[string]string{"factoryDirectory": directory}})
			owner := &boardReferenceOwner{}
			opening := &sessionRuntimeOpening{
				sessionID:            "~default",
				sessionSelection:     &factorysessions.SessionRuntimeSelection{Mode: factorysessions.SessionRuntimeModeService, Host: factorysessions.RuntimeHostRequest{Port: 1234}, Recording: factorysessions.SessionRecordingSelection{RecordPath: artifact}},
				configured:           preparedRuntime{Recordings: recordings.RuntimeSelection{RecordPath: artifact}},
				load:                 RuntimeLoad{LoadedFactoryCfg: boardReferenceSource{directory: directory}},
				durableExecution:     DurableExecution{Service: owner},
				restoredWorldState:   &factorydefinitions.FactoryWorldState{},
				restoredEventHistory: []factorydefinitions.FactoryEvent{{Type: factorydefinitions.FactoryEventTypeRunRequest, Payload: identity}},
			}
			wantSave := configureExplicitBoardPublication(name, owner, opening)
			err := opening.publishCurrentBoardReference(t.Context())
			wantLoads := 0
			if name == "resume" {
				wantLoads = 1
			}
			if (err != nil) != (name == "publication failure") || (owner.saves == 1) != wantSave || owner.loads != wantLoads {
				t.Fatalf("publication error=%v saves=%d loads=%d", err, owner.saves, owner.loads)
			}
			if wantSave && owner.savedArtifact != artifact {
				t.Fatal("publication changed selected path")
			}
		})
	}
}

func configureExplicitBoardPublication(name string, owner *boardReferenceOwner, opening *sessionRuntimeOpening) bool {
	wantSave := false
	switch name {
	case "restored":
		wantSave = true
	case "existing":
		owner.path = "existing invalid bytes"
	case "fresh":
		opening.restoredWorldState = nil
	case "foreign":
		opening.restoredEventHistory = nil
	case "batch":
		opening.sessionSelection.Mode = factorysessions.SessionRuntimeModeBatch
	case "peer":
		opening.sessionID = "peer"
	case "resume":
		opening.configured.Recordings.ResumePath = "resume.json"
		wantSave = true
	case "replay":
		opening.configured.Recordings.ReplayPath = "replay.json"
	case "no record":
		opening.sessionSelection.Recording.RecordPath = ""
	case "no server":
		opening.sessionSelection.Host.Port = 0
	case "publication failure":
		owner.failure = errors.New("controlled publication failure")
		wantSave = true
	}
	return wantSave
}
