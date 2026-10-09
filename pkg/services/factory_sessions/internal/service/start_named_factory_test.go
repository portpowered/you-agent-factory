package service

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livechange"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
	legacyservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionservice"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestRuntimeOpeningRetainsSelectedFactsWithoutAliasing(t *testing.T) {
	for _, name := range []string{"session-a", "session-b", "session-c", "session-d"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			settings := &factoryruntime.JavaScriptWorkerSettings{DefaultModel: name}
			recovery := &currentBoardStartupRecovery{file: name, quarantinedFile: name + ".quarantined", cause: "CORRUPT_STATE"}
			warning := recordings.MetadataMismatchWarning{Key: name, Artifact: "recorded", Current: "authored"}
			opening := &sessionRuntimeOpening{
				configured:           preparedRuntime{Recordings: recordings.RuntimeSelection{RecordPath: name + ".recording"}},
				operatorSettingsPath: name + ".operator", skippedBoardRecordings: []string{name + ".skipped"},
				startupRecovery: recovery, durableExecution: DurableExecution{WorkerSettings: settings},
				load: RuntimeLoad{ReplayMetadataWarnings: []recordings.MetadataMismatchWarning{warning}},
			}
			selected := &runtimebinding.SessionState{}
			opening.bindSelectedState(selected)
			if selected.CurrentBoardRecordPath != name+".recording" || selected.OperatorSettingsPath != name+".operator" ||
				len(selected.SkippedBoardRecordings) != 1 || selected.SkippedBoardRecordings[0] != name+".skipped" ||
				len(selected.ReplayMetadataWarnings) != 1 || selected.ReplayMetadataWarnings[0] != warning {
				t.Fatalf("selected opening facts = %+v", selected)
			}
			if selected.StartupRecovery == nil || *selected.StartupRecovery != (factorysessions.StartupRecovery{
				Code: "DURABLE_STATE_QUARANTINED", File: name, QuarantinedFile: name + ".quarantined", Cause: "CORRUPT_STATE",
			}) {
				t.Fatalf("selected recovery = %+v", selected.StartupRecovery)
			}
			settings.DefaultModel = "changed"
			recovery.cause = "changed"
			opening.skippedBoardRecordings[0] = "changed"
			opening.load.ReplayMetadataWarnings[0].Key = "changed"
			if selected.WorkerSettingsSnapshot().DefaultModel != name || selected.StartupRecovery.Cause != "CORRUPT_STATE" ||
				selected.SkippedBoardRecordings[0] != name+".skipped" || selected.ReplayMetadataWarnings[0] != warning {
				t.Fatal("later opening mutation changed retained session facts")
			}
			peer := &runtimebinding.SessionState{}
			(&sessionRuntimeOpening{}).bindSelectedState(peer)
			if peer.StartupRecovery != nil || peer.WorkerSettingsSnapshot() != nil || len(peer.ReplayMetadataWarnings) != 0 ||
				selected.WorkerSettingsSnapshot().DefaultModel != name {
				t.Fatal("empty peer opening inherited or changed selected facts")
			}
			(&sessionRuntimeOpening{}).bindSelectedState(selected)
			if selected.StartupRecovery != nil || selected.WorkerSettingsSnapshot() != nil ||
				selected.CurrentBoardRecordPath != "" || selected.OperatorSettingsPath != "" ||
				len(selected.SkippedBoardRecordings) != 0 || len(selected.ReplayMetadataWarnings) != 0 {
				t.Fatal("empty opening retained stale facts")
			}
		})
	}
}

func TestRuntimeOpeningRetainsSelectedResumeMetadata(t *testing.T) {
	for _, name := range []string{"session-a", "session-b", "session-c", "session-d"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			input := &recordings.LoadResumeInputResult{RecoveryMetadata: recordings.ResumeRecoveryMetadata{
				SourceRecordingID: name + "-source", RecordedDefinitionID: name + "-definition",
				PreviousRecordedAt: time.Date(2026, 9, 19, 18, 0, 0, 0, time.UTC),
			}}
			opening := &sessionRuntimeOpening{
				configured:  preparedRuntime{Runtime: factoryruntime.RuntimeSelection{RuntimeInstanceID: name + "-runtime"}},
				resumeInput: input,
			}
			selected := &runtimebinding.SessionState{}
			opening.bindSelectedState(selected)
			want := input.RecoveryMetadata
			want.SuccessorRecordingID = recoveryRecordingID(name + "-runtime")
			if selected.ResumeRecoveryMetadata == nil || *selected.ResumeRecoveryMetadata != want {
				t.Fatalf("selected resume identities/timestamp = %+v, want %+v", selected.ResumeRecoveryMetadata, want)
			}
			if input.RecoveryMetadata.SuccessorRecordingID != "" {
				t.Fatal("binding changed source recording metadata")
			}
			input.RecoveryMetadata.SourceRecordingID = "changed"
			opening.configured.Runtime.RuntimeInstanceID = "changed"
			bindStartedSessionState(selected, &sessionActivation{}, " request ", nil)
			if *selected.ResumeRecoveryMetadata != want || selected.StartRequestID() != "request" {
				t.Fatal("later source mutation or lifecycle binding changed selected resume metadata")
			}
			peer := &runtimebinding.SessionState{}
			(&sessionRuntimeOpening{}).bindSelectedState(peer)
			if peer.ResumeRecoveryMetadata != nil || *selected.ResumeRecoveryMetadata != want {
				t.Fatal("fresh peer inherited or changed selected resume metadata")
			}
			(&sessionRuntimeOpening{}).bindSelectedState(selected)
			if selected.ResumeRecoveryMetadata != nil {
				t.Fatal("fresh opening retained stale resume metadata")
			}
		})
	}
}

func TestStartRejectsInvalidLiveRequestsBeforeOpening(t *testing.T) {
	root, err := newRootForTest(livechange.NewCoordinator())
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name    string
		request factorysessions.SessionStartRequest
		field   string
	}{
		{"negative wait", factorysessions.SessionStartRequest{Mode: factorysessions.SessionOperationModeLive, Wait: factorysessions.SessionOperationWait{TimeoutMillis: -1}}, "wait.timeoutMillis"},
		{"validate and initialize", factorysessions.SessionStartRequest{Mode: factorysessions.SessionOperationModeLive, ValidateOnly: true, InitNewFactory: true}, "initNewFactory"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := root.Start(context.Background(), test.request)
			var detached *factorysessions.DetachedRequestError
			if !errors.As(err, &detached) || detached.Field != test.field {
				t.Fatalf("Start() error = %v, want detached %s", err, test.field)
			}
		})
	}
	var nilRoot *Root
	if _, err := nilRoot.Start(context.Background(), factorysessions.SessionStartRequest{}); err == nil {
		t.Fatal("nil root accepted a start")
	}
}

func TestPrepareLiveStartRequestNormalizesSelectionAndValidatesTarget(t *testing.T) {
	root, err := newRootForTest(livechange.NewCoordinator())
	if err != nil {
		t.Fatal(err)
	}
	root.opening.resolveHome = func() (string, error) { return "/home/operator", nil }
	root.opening.inheritCurrentSelection("other", &factorysessions.SessionRuntimeSelection{}) // missing current remains safe
	selection := factorysessions.SessionRuntimeSelection{}
	// The source selection is copied, so normalization cannot mutate its caller.
	request := factorysessions.SessionStartRequest{FolderPath: "/project", RuntimeSelection: &selection}
	selected, err := root.opening.prepareLiveStartRequest(context.Background(), request, factorysessions.DefaultSessionID)
	if err != nil {
		t.Fatal(err)
	}
	if selected.RuntimeSelection.SystemConfigHome != "/home/operator" || selected.RuntimeSelection.ExecutionBaseDir != "/project" || selected.RuntimeSelection.LogPolicy != factorysessions.SessionArtifactPolicyDisabled || selection.SystemConfigHome != "" {
		t.Fatalf("normalized selection = %+v; original = %+v", selected.RuntimeSelection, selection)
	}
	named := request
	named.Target = &factorysessions.TargetRef{Kind: factorysessions.TargetKindNamed, Name: "  reviews  "}
	selected, err = root.opening.prepareLiveStartRequest(context.Background(), named, "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if selected.Target.Name != "reviews" || selected.RuntimeSelection.DefinitionSourcePath != filepath.Join("/project", "reviews", factorydefinitions.FactoryConfigFile) {
		t.Fatalf("named selection = %+v", selected)
	}
	for _, target := range []factorysessions.TargetRef{
		{Kind: factorysessions.TargetKindDefault, Name: "unexpected"},
		{Kind: factorysessions.TargetKindNamed, Name: "nested/name"},
		{Kind: "UNKNOWN"},
	} {
		invalid := request
		invalid.Target = &target
		if _, err := root.opening.prepareLiveStartRequest(context.Background(), invalid, "session-1"); err == nil {
			t.Fatalf("accepted invalid target %+v", target)
		}
	}
	root.opening.resolveHome = func() (string, error) { return "", errors.New("home unavailable") }
	if _, err := root.opening.prepareLiveStartRequest(context.Background(), request, "session-1"); err == nil || !strings.Contains(err.Error(), "home unavailable") {
		t.Fatalf("home resolution error = %v", err)
	}
}

type startLifecycleStub struct {
	roles.LifecycleRuntime
	startErr      error
	workerErr     error
	completeErr   error
	stops         int
	stopErr       error
	workerStopErr error
	events        []string
	runContext    context.Context
}

func (s *startLifecycleStub) StartLifecycle(_ context.Context, runContext context.Context) error {
	s.runContext = runContext
	s.events = append(s.events, "start")
	return s.startErr
}
func (s *startLifecycleStub) StartWorkerLifecycle(context.Context) (factorysessions.RuntimeStop, error) {
	s.events = append(s.events, "worker")
	return func(context.Context) error {
		s.stops++
		s.events = append(s.events, "stop-worker")
		return s.workerStopErr
	}, s.workerErr
}
func (s *startLifecycleStub) CompleteStartup(context.Context) error {
	s.events = append(s.events, "complete")
	return s.completeErr
}
func (s *startLifecycleStub) StopLifecycle(context.Context) error {
	s.stops++
	s.events = append(s.events, "stop")
	return s.stopErr
}

func TestStartSessionLifecycleCleansFailedPhases(t *testing.T) {
	t.Parallel()
	for _, phase := range []string{"start", "worker", "complete"} {
		t.Run(phase, func(t *testing.T) {
			t.Parallel()
			cause := errors.New("selected phase failed")
			cleanupCause := errors.New("artifact cleanup failed")
			stopCause := errors.New("lifecycle stop failed")
			workerStopCause := errors.New("worker stop failed")
			lifecycle := &startLifecycleStub{stopErr: stopCause, workerStopErr: workerStopCause}
			switch phase {
			case "start":
				lifecycle.startErr = cause
			case "worker":
				lifecycle.workerErr = cause
			case "complete":
				lifecycle.completeErr = cause
			}
			cleanup := func() error {
				lifecycle.events = append(lifecycle.events, "artifacts")
				if !errors.Is(lifecycle.runContext.Err(), context.Canceled) {
					t.Error("cleanup ran before run cancellation")
				}
				return cleanupCause
			}
			activation, err := startSessionLifecycle(t.Context(), lifecycle, cleanup, false)
			if activation != nil || !errors.Is(err, cause) || !errors.Is(err, cleanupCause) || !errors.Is(err, stopCause) {
				t.Fatalf("failed phase lost cause/cleanup: activation=%v error=%v", activation, err)
			}
			want := []string{"start"}
			if phase != "start" {
				want = append(want, "worker")
				if phase == "complete" {
					want = append(want, "complete")
				}
				want = append(want, "stop-worker")
				if !errors.Is(err, workerStopCause) {
					t.Fatalf("lost worker cleanup cause: %v", err)
				}
			}
			want = append(want, "artifacts", "stop")
			if !reflect.DeepEqual(lifecycle.events, want) {
				t.Fatalf("phase order = %v, want %v", lifecycle.events, want)
			}
		})
	}
	closed := 0
	if _, err := startSessionLifecycle(t.Context(), nil, func() error { closed++; return nil }, false); err == nil || closed != 1 {
		t.Fatalf("missing lifecycle: error=%v closes=%d", err, closed)
	}
	lifecycle := &startLifecycleStub{}
	activation, err := startSessionLifecycle(t.Context(), lifecycle, nil, false)
	if err != nil || activation == nil || activation.stopWorker == nil {
		t.Fatalf("successful lifecycle: activation=%+v error=%v", activation, err)
	}
	if err := activation.Close(t.Context()); err != nil || lifecycle.stops != 1 {
		t.Fatalf("close activation: error=%v stops=%d", err, lifecycle.stops)
	}
	if !reflect.DeepEqual(lifecycle.events, []string{"start", "worker", "complete", "stop-worker"}) {
		t.Fatalf("successful order = %v", lifecycle.events)
	}
}

func TestHostedStartDefersCompletionUntilTransportReadiness(t *testing.T) {
	t.Parallel()
	lifecycle := &startLifecycleStub{completeErr: errors.New("completion must wait for host")}
	activation, err := startSessionLifecycle(t.Context(), lifecycle, nil, true)
	if err != nil || activation == nil {
		t.Fatalf("hosted start completed before transport: %v", err)
	}
	if err := activation.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestStartAppliesDurableDefaultsAndReportsUnavailableExecution(t *testing.T) {
	root, err := newRootForTest(livechange.NewCoordinator())
	if err != nil {
		t.Fatal(err)
	}
	request := factorysessions.SessionStartRequest{Mode: factorysessions.SessionOperationModeDurable, FolderPath: t.TempDir()}
	prepared := root.opening.prepareDurableStartRequest(request)
	if prepared.WorkerSettings != nil || prepared.WorkerAttemptStarter != nil || prepared.WorkerResourceAdmission != nil {
		t.Fatalf("unopened Factory supplied execution capabilities: %+v", prepared)
	}
	if _, err := root.Start(context.Background(), request); err == nil {
		t.Fatal("durable start accepted without a bound execution service")
	}
	if _, err := root.StartSync(context.Background(), factorysessions.StartRequest{}); err == nil {
		t.Fatal("sync start accepted without a bound execution service")
	}
	if _, err := root.StartAsync(context.Background(), factorysessions.StartRequest{}); err == nil {
		t.Fatal("async start accepted without a bound execution service")
	}
}

func TestStartHelpersKeepSessionSelectionAndBindingDetached(t *testing.T) {
	selected := factorysessions.SessionStartRequest{FolderPath: "/project/factory/reviews", RuntimeSelection: &factorysessions.SessionRuntimeSelection{}}
	if err := selectStartTarget(factorysessions.SessionStartRequest{FolderPath: "/project/factory", Target: &factorysessions.TargetRef{Kind: factorysessions.TargetKindNamed, Name: "reviews"}}, &selected, selected.RuntimeSelection); err != nil {
		t.Fatal(err)
	}
	if selected.Target.Name != "reviews" || selected.RuntimeSelection.DefinitionSourcePath != filepath.Join("/project/factory", "reviews", factorydefinitions.FactoryConfigFile) {
		t.Fatalf("named target = %+v", selected)
	}
	root, err := newRootForTest(livechange.NewCoordinator())
	if err != nil {
		t.Fatal(err)
	}
	root.opening.setStartedSessionTarget("missing", selected)
	if _, err := root.opening.bindStartedSession(context.Background(), "missing", selected, &sessionActivation{lifecycle: &startLifecycleStub{}}, "request", nil); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("binding absent session error = %v", err)
	}
	if _, found := root.opening.startedForRequestID("request"); found {
		t.Fatal("unstarted request appeared in idempotency lookup")
	}
	session := &livesession.LiveSession{ID: "canonical-1", SessionState: livesession.SessionState{FactoryDir: "/project/factory", FolderPath: "/project/factory/reviews"}, Target: *selected.Target}
	result := liveStartResult(session)
	if result.Live == nil || result.Live.Session == nil || result.Live.Session.Target.Name != "reviews" || result.SessionID != "canonical-1" {
		t.Fatalf("live start result = %+v", result)
	}
	bound := &runtimebinding.SessionState{OperatorSettingsPath: "/project/operator.yaml"}
	bindStartedSessionState(bound, &sessionActivation{}, " request ", nil)
	if bound.StartRequestID() != "request" || bound.OperatorSettingsPath != "/project/operator.yaml" || bound.Activation == nil {
		t.Fatalf("bound state = %+v", bound)
	}
}

func TestDurableStartInheritsOnlyMatchingCurrentFactoryMockWorkers(t *testing.T) {
	configured := workers.NewEmptyMockWorkersConfig()
	current := &livesession.LiveSession{
		SessionState: livesession.SessionState{FactoryDir: "/current"},
		Handle:       &runtimebinding.SessionState{},
	}
	runtimebinding.SessionStateFrom(current).SetMockWorkers(configured)
	request := factorysessions.SessionStartRequest{FolderPath: "/current"}
	matched := inheritCurrentMockWorkers(request, current)
	if matched.RuntimeSelection == nil || matched.RuntimeSelection.Workers.MockWorkers == nil {
		t.Fatal("matching Current Factory lost mock worker selection")
	}
	matched.RuntimeSelection.Workers.MockWorkers.MockWorkers = append(
		matched.RuntimeSelection.Workers.MockWorkers.MockWorkers,
		workers.MockWorkerConfig{ID: "changed"},
	)
	if len(configured.MockWorkers) != 0 {
		t.Fatal("durable request mutated the live Factory's mock configuration")
	}
	other := inheritCurrentMockWorkers(factorysessions.SessionStartRequest{FolderPath: "/other"}, current)
	if other.RuntimeSelection != nil {
		t.Fatal("another Factory inherited Current Factory mock workers")
	}
}

type startNamedDefinitions struct {
	factorydefinitions.Service
	request factorydefinitions.ResolveNamedFactoryRequest
	calls   int
}

func (s *startNamedDefinitions) ResolveNamedFactory(_ context.Context, request factorydefinitions.ResolveNamedFactoryRequest) (factorydefinitions.ResolveNamedFactoryResult, error) {
	s.request = request
	s.calls++
	return factorydefinitions.ResolveNamedFactoryResult{Resolution: factorydefinitions.NamedFactoryResolution{
		FactoryDir: filepath.Join(request.ProjectRoot, request.Name),
	}}, nil
}

func TestResolveStartFolderUsesRequestProjectForPackagedFactory(t *testing.T) {
	definitions := &startNamedDefinitions{}
	root := &runtimeOpeningTestRoot{opening: &RuntimeOpening{factoryDefinitions: definitions}}
	project := t.TempDir()
	home := t.TempDir()
	request := factorysessions.SessionStartRequest{
		FolderPath: project,
		Source:     factorysessions.Source{Kind: factoryruntime.WorkflowSourceKindFactoryID, FactoryID: "@you/subagent"},
		Args:       map[string]any{"workingRoot": project},
	}
	got, err := root.opening.resolveStartFolder(context.Background(), request, factorysessions.SessionRuntimeSelection{SystemConfigHome: home})
	if err != nil {
		t.Fatal(err)
	}
	projectFactories := filepath.Join(project, "factory")
	globalFactories := filepath.Join(home, ".you-agent-factory", "factories")
	if definitions.calls != 1 || definitions.request.ProjectRoot != projectFactories ||
		definitions.request.GlobalRoot != globalFactories ||
		definitions.request.Name != "@you/subagent" {
		t.Fatalf("named Factory resolution = %+v, calls = %d", definitions.request, definitions.calls)
	}
	if want := filepath.Join(projectFactories, "@you/subagent"); got != want {
		t.Fatalf("Factory directory = %q, want %q", got, want)
	}
}

func TestResolveStartFolderPreservesAlreadyResolvedFactory(t *testing.T) {
	definitions := &startNamedDefinitions{}
	root := &runtimeOpeningTestRoot{opening: &RuntimeOpening{factoryDefinitions: definitions}}
	project := t.TempDir()
	factoryDir := filepath.Join(project, "factory", "@you", "subagent")
	request := factorysessions.SessionStartRequest{
		FolderPath: factoryDir,
		Source:     factorysessions.Source{Kind: factoryruntime.WorkflowSourceKindFactoryID, FactoryID: "@you/subagent"},
		Args:       map[string]any{"workingRoot": project},
	}
	got, err := root.opening.resolveStartFolder(context.Background(), request, factorysessions.SessionRuntimeSelection{})
	if err != nil || got != factoryDir || definitions.calls != 0 {
		t.Fatalf("resolved Factory directory = %q, calls = %d, error = %v", got, definitions.calls, err)
	}
}

func TestActivationOnlyStartAllocatesDistinctSessionIdentities(t *testing.T) {
	generated := 0
	root := &runtimeOpeningTestRoot{opening: &RuntimeOpening{generateSessionID: func() string {
		generated++
		return "chat-session-" + string(rune('0'+generated))
	}}}
	request := factorysessions.SessionStartRequest{ActivationOnly: true}
	first, err := root.opening.sessionIDForStart(request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := root.opening.sessionIDForStart(request)
	if err != nil {
		t.Fatal(err)
	}
	if first == second || first == factorysessions.DefaultSessionID || second == factorysessions.DefaultSessionID {
		t.Fatalf("activation IDs = %q, %q; want distinct non-default IDs", first, second)
	}
	request.SessionID = first
	reused, err := root.opening.sessionIDForStart(request)
	if err != nil || reused != first || generated != 2 {
		t.Fatalf("explicit activation ID = %q, generated = %d, error = %v", reused, generated, err)
	}
}

func TestRuntimeOpeningStartAdmissionFailurePreservesCauseAndCanRetry(t *testing.T) {
	for _, id := range []string{"session-a", "session-b", "session-c", "session-d"} {
		t.Run(id, func(t *testing.T) {
			t.Parallel()
			cause := &factorysessions.DetachedRequestError{Field: "home", Message: id}
			homes, identities := 0, 0
			opening := &RuntimeOpening{
				assembly:          &legacyservice.Assembly{},
				resolveHome:       func() (string, error) { homes++; return "", cause },
				generateSessionID: func() string { identities++; return id },
			}
			request := factorysessions.SessionStartRequest{Mode: factorysessions.SessionOperationModeLive, FolderPath: "/factory"}
			result, err := opening.Start(t.Context(), request)
			var typed *factorysessions.DetachedRequestError
			if !errors.Is(err, cause) || !errors.As(err, &typed) || typed != cause || result.SessionID != "" || homes != 1 || identities != 0 {
				t.Fatalf("failed admission = %+v, error=%v, homes=%d identities=%d", result, err, homes, identities)
			}
			// Correcting home selection must progress to target validation on the same
			// owner, without allocating or acquiring a runtime on either failure.
			request.RuntimeSelection = &factorysessions.SessionRuntimeSelection{SystemConfigHome: "/selected/home"}
			request.Target = &factorysessions.TargetRef{Kind: factorysessions.TargetKindNamed, Name: "nested/name"}
			result, err = opening.Start(t.Context(), request)
			if err == nil || !strings.Contains(err.Error(), "invalid named Factory Session target") || errors.Is(err, cause) || result.SessionID != "" || homes != 1 || identities != 0 {
				t.Fatalf("corrected admission = %+v, error=%v, homes=%d identities=%d", result, err, homes, identities)
			}
		})
	}
}
