package service

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livechange"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

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
	root.resolveHome = func() (string, error) { return "/home/operator", nil }
	root.inheritCurrentSelection("other", &factorysessions.SessionRuntimeSelection{}) // missing current remains safe
	selection := factorysessions.SessionRuntimeSelection{}
	// The source selection is copied, so normalization cannot mutate its caller.
	request := factorysessions.SessionStartRequest{FolderPath: "/project", RuntimeSelection: &selection}
	selected, err := root.prepareLiveStartRequest(context.Background(), request, factorysessions.DefaultSessionID)
	if err != nil {
		t.Fatal(err)
	}
	if selected.RuntimeSelection.SystemConfigHome != "/home/operator" || selected.RuntimeSelection.ExecutionBaseDir != "/project" || selected.RuntimeSelection.LogPolicy != factorysessions.SessionArtifactPolicyDisabled || selection.SystemConfigHome != "" {
		t.Fatalf("normalized selection = %+v; original = %+v", selected.RuntimeSelection, selection)
	}
	named := request
	named.Target = &factorysessions.TargetRef{Kind: factorysessions.TargetKindNamed, Name: "  reviews  "}
	selected, err = root.prepareLiveStartRequest(context.Background(), named, "session-1")
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
		if _, err := root.prepareLiveStartRequest(context.Background(), invalid, "session-1"); err == nil {
			t.Fatalf("accepted invalid target %+v", target)
		}
	}
	root.resolveHome = func() (string, error) { return "", errors.New("home unavailable") }
	if _, err := root.prepareLiveStartRequest(context.Background(), request, "session-1"); err == nil || !strings.Contains(err.Error(), "home unavailable") {
		t.Fatalf("home resolution error = %v", err)
	}
}

type startLifecycleStub struct {
	roles.LifecycleRuntime
	startErr    error
	workerErr   error
	completeErr error
	stops       int
}

func (s *startLifecycleStub) StartLifecycle(context.Context, context.Context) error {
	return s.startErr
}
func (s *startLifecycleStub) StartWorkerLifecycle(context.Context) (factorysessions.RuntimeStop, error) {
	return func(context.Context) error { s.stops++; return nil }, s.workerErr
}
func (s *startLifecycleStub) CompleteStartup(context.Context) error { return s.completeErr }
func (s *startLifecycleStub) StopLifecycle(context.Context) error   { s.stops++; return nil }

func TestStartSessionLifecycleCleansFailedPhases(t *testing.T) {
	ctx := context.Background()
	for _, failure := range []struct {
		name string
		set  func(*startLifecycleStub)
	}{
		{"start", func(s *startLifecycleStub) { s.startErr = errors.New("start failed") }},
		{"worker", func(s *startLifecycleStub) { s.workerErr = errors.New("worker failed") }},
		{"complete", func(s *startLifecycleStub) { s.completeErr = errors.New("complete failed") }},
	} {
		t.Run(failure.name, func(t *testing.T) {
			lifecycle := &startLifecycleStub{}
			failure.set(lifecycle)
			artifacts := 0
			_, err := startSessionLifecycle(ctx, runtimeProducts{lifecycle: lifecycle, closeArtifacts: func() error { artifacts++; return nil }})
			if err == nil || artifacts != 1 || lifecycle.stops == 0 {
				t.Fatalf("failed lifecycle: error=%v artifacts=%d stops=%d", err, artifacts, lifecycle.stops)
			}
		})
	}
	closed := 0
	if _, err := startSessionLifecycle(ctx, runtimeProducts{closeArtifacts: func() error { closed++; return nil }}); err == nil || closed != 1 {
		t.Fatalf("missing lifecycle: error=%v closes=%d", err, closed)
	}
	lifecycle := &startLifecycleStub{}
	activation, err := startSessionLifecycle(ctx, runtimeProducts{lifecycle: lifecycle})
	if err != nil || activation == nil || activation.stopWorker == nil {
		t.Fatalf("successful lifecycle: activation=%+v error=%v", activation, err)
	}
	if err := activation.Close(ctx); err != nil || lifecycle.stops != 1 {
		t.Fatalf("close activation: error=%v stops=%d", err, lifecycle.stops)
	}
}

func TestStartAppliesDurableDefaultsAndReportsUnavailableExecution(t *testing.T) {
	root, err := newRootForTest(livechange.NewCoordinator())
	if err != nil {
		t.Fatal(err)
	}
	request := factorysessions.SessionStartRequest{Mode: factorysessions.SessionOperationModeDurable, FolderPath: t.TempDir()}
	prepared := root.prepareDurableStartRequest(request)
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
	root.setStartedSessionTarget("missing", selected)
	if _, err := root.bindStartedSession(context.Background(), "missing", selected, runtimeProducts{}, &sessionActivation{lifecycle: &startLifecycleStub{}}, "request", nil); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("binding absent session error = %v", err)
	}
	if _, found := root.startedForRequestID("request"); found {
		t.Fatal("unstarted request appeared in idempotency lookup")
	}
	session := &livesession.LiveSession{ID: "canonical-1", SessionState: livesession.SessionState{FactoryDir: "/project/factory", FolderPath: "/project/factory/reviews"}, Target: *selected.Target}
	result := liveStartResult(session)
	if result.Live == nil || result.Live.Session == nil || result.Live.Session.Target.Name != "reviews" || result.SessionID != "canonical-1" {
		t.Fatalf("live start result = %+v", result)
	}
	bound := &runtimebinding.SessionState{}
	bindSessionProducts(bound, runtimeProducts{operatorSettingsPath: "/project/operator.yaml", closeArtifacts: func() error { return nil }}, &sessionActivation{}, " request ", nil)
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
	root := &Root{factoryDefinitions: definitions}
	project := t.TempDir()
	home := t.TempDir()
	request := factorysessions.SessionStartRequest{
		FolderPath: project,
		Source:     factorysessions.Source{Kind: factoryruntime.WorkflowSourceKindFactoryID, FactoryID: "@you/subagent"},
		Args:       map[string]any{"workingRoot": project},
	}
	got, err := root.resolveStartFolder(context.Background(), request, factorysessions.SessionRuntimeSelection{SystemConfigHome: home})
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
	root := &Root{factoryDefinitions: definitions}
	project := t.TempDir()
	factoryDir := filepath.Join(project, "factory", "@you", "subagent")
	request := factorysessions.SessionStartRequest{
		FolderPath: factoryDir,
		Source:     factorysessions.Source{Kind: factoryruntime.WorkflowSourceKindFactoryID, FactoryID: "@you/subagent"},
		Args:       map[string]any{"workingRoot": project},
	}
	got, err := root.resolveStartFolder(context.Background(), request, factorysessions.SessionRuntimeSelection{})
	if err != nil || got != factoryDir || definitions.calls != 0 {
		t.Fatalf("resolved Factory directory = %q, calls = %d, error = %v", got, definitions.calls, err)
	}
}

func TestActivationOnlyStartAllocatesDistinctSessionIdentities(t *testing.T) {
	generated := 0
	root := &Root{generateSessionID: func() string {
		generated++
		return "chat-session-" + string(rune('0'+generated))
	}}
	request := factorysessions.SessionStartRequest{ActivationOnly: true}
	first, err := root.sessionIDForStart(request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := root.sessionIDForStart(request)
	if err != nil {
		t.Fatal(err)
	}
	if first == second || first == factorysessions.DefaultSessionID || second == factorysessions.DefaultSessionID {
		t.Fatalf("activation IDs = %q, %q; want distinct non-default IDs", first, second)
	}
	request.SessionID = first
	reused, err := root.sessionIDForStart(request)
	if err != nil || reused != first || generated != 2 {
		t.Fatalf("explicit activation ID = %q, generated = %d, error = %v", reused, generated, err)
	}
}
