// pkgmaintcheck:ignore-file-lines consolidated same-package service tests remain on root-only runtime seams until dedicated service test seams are extracted.
// backendsizecheck:ignore-file consolidated same-package service tests remain on root-only runtime seams until dedicated service test seams are extracted.
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	factorytoken "github.com/portpowered/infinite-you/pkg/factory/token"
	"github.com/portpowered/infinite-you/pkg/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/work"
	"github.com/portpowered/infinite-you/pkg/workers"
	workerapplication "github.com/portpowered/infinite-you/pkg/workers/application"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
	hostedworkers "github.com/portpowered/infinite-you/pkg/workers/hosted"
	workersservice "github.com/portpowered/infinite-you/pkg/workers/service"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

const (
	canonicalScriptPollerWorkstationName = "linear-ingress"
	canonicalScriptPollerWorkerName      = "poller-script"
	canonicalScriptPollerCommand         = "factory/scripts/poller.sh"
)

func initializeWorkersSchedulerForTest(svc *FactoryService) {
	cfg := &FactoryServiceConfig{
		Dir:        svc.policy.dir,
		WorkflowID: svc.policy.workflowID,
	}
	if svc.cfg != nil && svc.cfg.WorkerApplication.Valid() {
		cfg.WorkerApplication = svc.cfg.WorkerApplication
	} else {
		cfg, _ = ConfigWithWorkerApplication(cfg)
	}
	svc.workersScheduler = workersservice.NewWorkersSchedulerService(workersSchedulerServiceConfig(cfg, svc.clock, svc.logger, svc.hostedWorkers))
}

func TestWorkersSchedulerServiceConfigAppliesDefaultsAndExplicitInputs(t *testing.T) {
	t.Parallel()

	defaults := workersSchedulerServiceConfig(nil, nil, nil, hostedworkers.Config{})
	if defaults.Logger == nil || defaults.CommandRunner == nil || defaults.WorkflowID != "" || defaults.DefaultFactoryDir != "" {
		t.Fatalf("default worker scheduler config = %#v, want usable defaults", defaults)
	}

	runner := workers.ExecCommandRunner{}
	cfg := &FactoryServiceConfig{
		Dir:        "/factory",
		WorkflowID: "workflow-a",
		WorkerApplication: workerapplication.Components{
			ScriptCommandRunner: runner,
		},
	}
	logger := zap.NewNop()
	explicit := workersSchedulerServiceConfig(cfg, factory.EnsureClock(nil), logger, hostedworkers.Config{})
	if explicit.Logger != logger || explicit.CommandRunner != runner ||
		explicit.WorkflowID != "workflow-a" || explicit.DefaultFactoryDir != "/factory" {
		t.Fatalf("explicit worker scheduler config = %#v, want supplied inputs", explicit)
	}
}

func TestFactoryService_StartLiveRuntimeSidecars_RequiresInitializedWorkerSidecarOwner(t *testing.T) {
	svc := &FactoryService{
		policy: serviceCoordinatorPolicyFromConfig(&FactoryServiceConfig{RuntimeMode: interfaces.RuntimeModeService}),
		logger: zap.NewNop(),
	}
	runtimeCfg := newScriptPollerLoadedRuntimeConfigForServiceTest(
		t,
		t.TempDir(),
		scriptPollerRuntimeConfigOptions{poller: newCanonicalScriptPollerWorkstation()},
	)
	handle := &liveRuntimeHandle{Bundle: &factoryRuntimeBundle{
		Factory:    &aggregateSnapshotFactory{},
		RuntimeCfg: runtimeCfg,
	}}

	err := svc.startLiveRuntimeSidecars(context.Background(), handle)
	if err == nil || !strings.Contains(err.Error(), "worker sidecar owner is not initialized") {
		t.Fatalf("startLiveRuntimeSidecars error = %v, want missing worker sidecar owner", err)
	}
	if handle.SidecarCancel != nil {
		t.Fatal("failed sidecar attachment retained a cancellation function")
	}
}

func TestFactoryService_StartLiveRuntimeSidecars_StartsScriptPollerForPollerRunWorkstationType(t *testing.T) {
	start := time.Date(2026, time.June, 16, 9, 0, 0, 0, time.UTC)
	fakeClock := clockwork.NewFakeClockAt(start)
	factoryDir := t.TempDir()
	runner := &pollerSequenceCommandRunner{
		outcomes: []pollerRunOutcome{{result: workers.CommandResult{}}},
	}
	logCore, _ := observer.New(zap.InfoLevel)
	svc := &FactoryService{
		policy: serviceCoordinatorPolicyFromConfig(&FactoryServiceConfig{RuntimeMode: interfaces.RuntimeModeService}),
		cfg:    serviceTestConfigWithWorkerEdges(t, &FactoryServiceConfig{}, workerapplication.Edges{ScriptCommandRunner: runner}),
		logger: zap.New(logCore),
		clock:  fakeClock,
	}
	initializeWorkersSchedulerForTest(svc)
	poller := interfaces.FactoryWorkstationConfig{
		Name:           canonicalScriptPollerWorkstationName,
		Type:           interfaces.WorkstationTypePoller,
		WorkerTypeName: canonicalScriptPollerWorkerName,
	}
	config.NormalizeCanonicalWorkstationRuntime(&poller)
	if poller.Kind != interfaces.WorkstationKindPoller {
		t.Fatalf("normalized poller kind = %q, want %q", poller.Kind, interfaces.WorkstationKindPoller)
	}

	runtimeCfg := newScriptPollerLoadedRuntimeConfigForServiceTest(
		t,
		factoryDir,
		scriptPollerRuntimeConfigOptions{
			poller:       poller,
			pollerWorker: newCanonicalScriptPollerWorker("--mode", "watch"),
		},
	)
	handle := &liveRuntimeHandle{Bundle: &factoryRuntimeBundle{
		Factory:    &aggregateSnapshotFactory{},
		RuntimeCfg: runtimeCfg,
	},
	}
	sidecarCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := svc.startLiveRuntimeSidecars(sidecarCtx, handle); err != nil {
		t.Fatalf("startLiveRuntimeSidecars: %v", err)
	}
	defer svc.stopLiveRuntimeSidecars(handle)

	waitForPollerRunnerCalls(t, runner, 1, time.Second)
}

func TestFactoryService_StartLiveRuntimeSidecars_StartsOnlyScriptPollersAndRestartsUnexpectedExit(t *testing.T) {
	start := time.Date(2026, time.May, 22, 9, 0, 0, 0, time.UTC)
	fakeClock := clockwork.NewFakeClockAt(start)
	factoryDir := t.TempDir()
	runner := &pollerSequenceCommandRunner{
		outcomes: []pollerRunOutcome{
			{result: workers.CommandResult{}},
			{waitForCancel: true},
		},
	}
	logCore, observedLogs := observer.New(zap.InfoLevel)
	svc := &FactoryService{
		policy: serviceCoordinatorPolicyFromConfig(&FactoryServiceConfig{RuntimeMode: interfaces.RuntimeModeService}),
		cfg:    serviceTestConfigWithWorkerEdges(t, &FactoryServiceConfig{}, workerapplication.Edges{ScriptCommandRunner: runner}),
		logger: zap.New(logCore),
		clock:  fakeClock,
	}
	initializeWorkersSchedulerForTest(svc)
	poller := newCanonicalScriptPollerWorkstation()
	standard := interfaces.FactoryWorkstationConfig{
		Name:           "processor",
		Kind:           interfaces.WorkstationKindStandard,
		WorkerTypeName: canonicalScriptPollerWorkerName,
	}
	runtimeCfg := newScriptPollerLoadedRuntimeConfigForServiceTest(
		t,
		factoryDir,
		scriptPollerRuntimeConfigOptions{
			poller:       poller,
			pollerWorker: newCanonicalScriptPollerWorker("--mode", "watch"),
			additionalWorkers: []*workerconfig.Config{
				{
					Name:    "non-poller-script",
					Type:    interfaces.WorkerTypeScript,
					Command: "factory/scripts/processor.sh",
				},
			},
			additionalWorkstations: []interfaces.FactoryWorkstationConfig{standard},
		},
	)
	handle := &liveRuntimeHandle{Bundle: &factoryRuntimeBundle{
		Factory:    &aggregateSnapshotFactory{},
		RuntimeCfg: runtimeCfg,
	},
	}
	sidecarCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := svc.startLiveRuntimeSidecars(sidecarCtx, handle); err != nil {
		t.Fatalf("startLiveRuntimeSidecars: %v", err)
	}
	defer svc.stopLiveRuntimeSidecars(handle)

	waitForPollerRunnerCalls(t, runner, 1, time.Second)
	waitForFakeClockWaiters(t, fakeClock, 1)
	fakeClock.Advance(workersservice.ScriptPollerRestartBackoffMin)
	waitForPollerRunnerCalls(t, runner, 2, time.Second)

	reqs := runner.requests()
	if len(reqs) < 2 {
		t.Fatalf("poller runner requests = %d, want at least 2", len(reqs))
	}
	first := reqs[0]
	if first.WorkstationName != "linear-ingress" {
		t.Fatalf("poller workstation name = %q, want linear-ingress", first.WorkstationName)
	}
	if first.WorkerType != "poller-script" {
		t.Fatalf("poller worker type = %q, want poller-script", first.WorkerType)
	}
	if first.Command != filepath.Join(factoryDir, "scripts", "poller.sh") {
		t.Fatalf("poller command = %q, want resolved factory script path", first.Command)
	}
	if strings.Join(first.Args, " ") != "--mode watch" {
		t.Fatalf("poller args = %#v, want [--mode watch]", first.Args)
	}
	if observedLogs.FilterMessage("script poller restarting").Len() == 0 {
		t.Fatal("expected poller restart log after unexpected exit")
	}
}

func TestFactoryService_StartLiveRuntimeSidecars_BatchModeDoesNotStartScriptPollers(t *testing.T) {
	runner := &pollerSequenceCommandRunner{
		outcomes: []pollerRunOutcome{{waitForCancel: true}},
	}
	svc := &FactoryService{
		policy: serviceCoordinatorPolicyFromConfig(&FactoryServiceConfig{RuntimeMode: interfaces.RuntimeModeBatch}),
		cfg:    serviceTestConfigWithWorkerEdges(t, &FactoryServiceConfig{}, workerapplication.Edges{ScriptCommandRunner: runner}),
		logger: zap.NewNop(),
	}
	initializeWorkersSchedulerForTest(svc)
	poller := newCanonicalScriptPollerWorkstation()
	runtimeCfg := newScriptPollerLoadedRuntimeConfigForServiceTest(
		t,
		t.TempDir(),
		scriptPollerRuntimeConfigOptions{
			poller: poller,
		},
	)
	handle := &liveRuntimeHandle{Bundle: &factoryRuntimeBundle{
		Factory:    &aggregateSnapshotFactory{},
		RuntimeCfg: runtimeCfg,
	},
	}

	if err := svc.startLiveRuntimeSidecars(context.Background(), handle); err != nil {
		t.Fatalf("startLiveRuntimeSidecars: %v", err)
	}
	defer svc.stopLiveRuntimeSidecars(handle)

	time.Sleep(50 * time.Millisecond)
	if runner.callCount() != 0 {
		t.Fatalf("poller runner calls = %d, want 0 in batch mode", runner.callCount())
	}
}

func TestFactoryService_StartLiveRuntimeSidecars_StartsHostedLinearPoller(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data": {
				"issues": {
					"nodes": [
						{
							"id": "issue-new",
							"identifier": "ENG-101",
							"title": "Newest issue",
							"description": "First",
							"updatedAt": "2026-05-22T07:10:00Z",
							"url": "https://linear.app/example/issue/ENG-101",
							"team": {"id": "team-1", "key": "ENG", "name": "Engineering"},
							"state": {"id": "state-1", "name": "Todo", "type": "unstarted"},
							"assignee": null
						}
					],
					"pageInfo": {"hasNextPage": false, "endCursor": ""}
				}
			}
		}`))
	}))
	defer server.Close()

	factoryDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(factoryDir, "secrets"), 0o755); err != nil {
		t.Fatalf("mkdir secrets: %v", err)
	}
	if err := os.WriteFile(filepath.Join(factoryDir, "secrets", "linear-api-key"), []byte("runtime-linear-key\n"), 0o600); err != nil {
		t.Fatalf("write secret file: %v", err)
	}

	submitted := &aggregateSnapshotFactory{}
	svcCfg := serviceTestConfigWithWorkerEdges(t, &FactoryServiceConfig{
		RuntimeMode: interfaces.RuntimeModeService,
	}, workerapplication.Edges{HostedHTTPClient: server.Client(), HostedLinearEndpoint: server.URL})
	svc := &FactoryService{
		policy:        serviceCoordinatorPolicyFromConfig(svcCfg),
		cfg:           svcCfg,
		logger:        zap.NewNop(),
		hostedWorkers: buildHostedWorkersConfigForServiceTest(svcCfg, zap.NewNop(), nil),
	}
	initializeWorkersSchedulerForTest(svc)
	poller := interfaces.FactoryWorkstationConfig{
		Name:           "linear-ingress",
		Kind:           interfaces.WorkstationKindPoller,
		WorkerTypeName: "linear-poller",
	}
	runtimeCfg := newLoadedFactoryConfigForServiceTest(
		t,
		factoryDir,
		&interfaces.FactoryConfig{
			Workers:      []workerconfig.Config{{Name: "linear-poller"}},
			Workstations: []interfaces.FactoryWorkstationConfig{poller},
		},
		map[string]*workerconfig.Config{
			"linear-poller": {
				Name:     "linear-poller",
				Type:     interfaces.WorkerTypeHosted,
				Provider: interfaces.HostedWorkerProviderLinear,
				Auth:     &workerconfig.HostedWorkerAuthConfig{SecretRef: "secrets/linear-api-key"},
				Linear: &workerconfig.HostedLinearWorkerConfig{
					PollInterval: "1h",
					Mapping: workerconfig.HostedLinearWorkerMappingConfig{
						WorkType: "story",
						State:    "init",
					},
				},
			},
		},
		map[string]*interfaces.FactoryWorkstationConfig{poller.Name: &poller},
	)
	handle := &liveRuntimeHandle{Bundle: &factoryRuntimeBundle{
		RuntimeCfg: runtimeCfg,
		Factory:    submitted,
	},
	}
	sidecarCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := svc.startLiveRuntimeSidecars(sidecarCtx, handle); err != nil {
		t.Fatalf("startLiveRuntimeSidecars: %v", err)
	}
	defer svc.stopLiveRuntimeSidecars(handle)

	waitForHostedPollerSubmission(t, submitted, 1, time.Second)
	calls, submissions := submitted.submissionSnapshot()
	if calls != 1 {
		t.Fatalf("submit calls = %d, want 1", calls)
	}
	if got := submissions[0].Works[0].WorkID; got != "linear:issue-new" {
		t.Fatalf("submitted work id = %q, want linear:issue-new", got)
	}
}

func TestFactoryService_StartLiveRuntimeSidecars_SubmitsMultipleHostedLinearIssuesPerPollCycle(t *testing.T) {
	server := newHostedLinearIssuesGraphQLServer(t,
		hostedLinearIssueFixture{
			ID: "issue-new", Identifier: "ENG-101", Title: "Newest issue",
			Description: "First", UpdatedAt: "2026-05-22T07:10:00Z",
		},
		hostedLinearIssueFixture{
			ID: "issue-old", Identifier: "ENG-55", Title: "Older issue",
			Description: "Second", UpdatedAt: "2026-05-22T07:00:00Z",
		},
	)
	defer server.Close()

	fixture := newHostedLinearPollerServiceFixture(t, server, func(linear *workerconfig.HostedLinearWorkerConfig) {
		linear.TeamIDs = []string{"team-1"}
		linear.StateIDs = []string{"state-1"}
	})
	stop := startHostedLinearPollerSidecars(t, fixture)
	defer stop()

	waitForHostedPollerSubmission(t, fixture.submitted, 1, time.Second)
	assertHostedLinearBatchWorks(t, fixture.submitted, "linear:issue-old", "linear:issue-new")
}

func TestFactoryService_StartLiveRuntimeSidecars_RunsHostedAndScriptPollersConcurrently(t *testing.T) {
	server := newHostedLinearIssuesGraphQLServer(t, hostedLinearIssueFixture{
		ID: "issue-hosted", Identifier: "ENG-200", Title: "Hosted issue",
		Description: "From hosted poller", UpdatedAt: "2026-05-22T07:10:00Z",
	})
	defer server.Close()

	fixture := newConcurrentHostedAndScriptPollerFixture(t, server)
	stop := startHostedLinearPollerSidecars(t, fixture.hostedLinearPollerServiceFixture)
	defer stop()

	waitForConcurrentHostedAndScriptPollerSubmissions(t, fixture.submitted, 2*time.Second)
	assertConcurrentHostedAndScriptPollerSubmissions(t, fixture.submitted)
}

func TestFactoryService_StopLiveRuntimeSidecars_StopsHostedLinearPollerAndLogsLifecycle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data": {
				"issues": {
					"nodes": [],
					"pageInfo": {"hasNextPage": false, "endCursor": ""}
				}
			}
		}`))
	}))
	defer server.Close()

	factoryDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(factoryDir, "secrets"), 0o755); err != nil {
		t.Fatalf("mkdir secrets: %v", err)
	}
	if err := os.WriteFile(filepath.Join(factoryDir, "secrets", "linear-api-key"), []byte("runtime-linear-key\n"), 0o600); err != nil {
		t.Fatalf("write secret file: %v", err)
	}

	logCore, observedLogs := observer.New(zap.InfoLevel)
	svcCfg := serviceTestConfigWithWorkerEdges(t, &FactoryServiceConfig{
		RuntimeMode: interfaces.RuntimeModeService,
		Logger:      zap.New(logCore),
	}, workerapplication.Edges{HostedHTTPClient: server.Client(), HostedLinearEndpoint: server.URL})
	svc := &FactoryService{
		policy:        serviceCoordinatorPolicyFromConfig(svcCfg),
		cfg:           svcCfg,
		logger:        zap.New(logCore),
		hostedWorkers: buildHostedWorkersConfigForServiceTest(svcCfg, zap.New(logCore), nil),
	}
	initializeWorkersSchedulerForTest(svc)
	poller := interfaces.FactoryWorkstationConfig{
		Name:           "linear-ingress",
		Kind:           interfaces.WorkstationKindPoller,
		WorkerTypeName: "linear-poller",
	}
	runtimeCfg := newLoadedFactoryConfigForServiceTest(
		t,
		factoryDir,
		&interfaces.FactoryConfig{
			Workers:      []workerconfig.Config{{Name: "linear-poller"}},
			Workstations: []interfaces.FactoryWorkstationConfig{poller},
		},
		map[string]*workerconfig.Config{
			"linear-poller": {
				Name:     "linear-poller",
				Type:     interfaces.WorkerTypeHosted,
				Provider: interfaces.HostedWorkerProviderLinear,
				Auth:     &workerconfig.HostedWorkerAuthConfig{SecretRef: "secrets/linear-api-key"},
				Linear: &workerconfig.HostedLinearWorkerConfig{
					PollInterval: "1h",
					Mapping: workerconfig.HostedLinearWorkerMappingConfig{
						WorkType: "story",
						State:    "init",
					},
				},
			},
		},
		map[string]*interfaces.FactoryWorkstationConfig{poller.Name: &poller},
	)
	handle := &liveRuntimeHandle{Bundle: &factoryRuntimeBundle{
		RuntimeCfg: runtimeCfg,
		Factory:    &aggregateSnapshotFactory{},
	},
	}

	if err := svc.startLiveRuntimeSidecars(context.Background(), handle); err != nil {
		t.Fatalf("startLiveRuntimeSidecars: %v", err)
	}

	waitForObservedLogMessage(t, observedLogs, "hosted linear poller started", time.Second)
	svc.stopLiveRuntimeSidecars(handle)

	stopped := observedLogs.FilterMessage("hosted linear poller stopped").All()
	if len(stopped) != 1 {
		t.Fatalf("hosted linear poller stopped log count = %d, want 1", len(stopped))
	}
	if got := fieldString(stopped[0].ContextMap()["reason"]); got != "context canceled" {
		t.Fatalf("hosted linear poller stop reason = %q, want context canceled", got)
	}
}

func TestFactoryService_StartLiveRuntimeSidecars_DisablesUnsupportedHostedProvider(t *testing.T) {
	logCore, observedLogs := observer.New(zap.WarnLevel)
	svcCfg := &FactoryServiceConfig{RuntimeMode: interfaces.RuntimeModeService}
	svc := &FactoryService{
		policy:        serviceCoordinatorPolicyFromConfig(svcCfg),
		cfg:           svcCfg,
		logger:        zap.New(logCore),
		hostedWorkers: buildHostedWorkersConfigForServiceTest(svcCfg, zap.New(logCore), nil),
	}
	initializeWorkersSchedulerForTest(svc)
	poller := interfaces.FactoryWorkstationConfig{
		Name:           "custom-ingress",
		Kind:           interfaces.WorkstationKindPoller,
		WorkerTypeName: "custom-hosted",
	}
	runtimeCfg := newLoadedFactoryConfigForServiceTest(
		t,
		t.TempDir(),
		&interfaces.FactoryConfig{
			Workers:      []workerconfig.Config{{Name: "custom-hosted"}},
			Workstations: []interfaces.FactoryWorkstationConfig{poller},
		},
		map[string]*workerconfig.Config{
			"custom-hosted": {
				Name:     "custom-hosted",
				Type:     interfaces.WorkerTypeHosted,
				Provider: "CUSTOM",
			},
		},
		map[string]*interfaces.FactoryWorkstationConfig{poller.Name: &poller},
	)
	handle := &liveRuntimeHandle{Bundle: &factoryRuntimeBundle{
		RuntimeCfg: runtimeCfg,
		Factory:    &aggregateSnapshotFactory{},
	},
	}
	sidecarCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := svc.startLiveRuntimeSidecars(sidecarCtx, handle); err != nil {
		t.Fatalf("startLiveRuntimeSidecars: %v", err)
	}
	defer svc.stopLiveRuntimeSidecars(handle)

	time.Sleep(50 * time.Millisecond)
	disabled := observedLogs.FilterMessage("hosted poller disabled").All()
	if len(disabled) != 1 {
		t.Fatalf("hosted poller disabled log count = %d, want 1", len(disabled))
	}
	fields := disabled[0].ContextMap()
	if fieldString(fields["workstation"]) != "custom-ingress" {
		t.Fatalf("disabled workstation = %#v, want custom-ingress", fields["workstation"])
	}
	if fieldString(fields["reason"]) != "unsupported hosted provider" {
		t.Fatalf("disabled reason = %#v, want unsupported hosted provider", fields["reason"])
	}
	if fieldString(fields["provider"]) != "CUSTOM" {
		t.Fatalf("disabled provider = %#v, want CUSTOM", fields["provider"])
	}
}

func TestFactoryService_StartLiveRuntimeSidecars_RestartsScriptPollerOnMalformedOutput(t *testing.T) {
	fakeClock := clockwork.NewFakeClock()
	runner := &pollerSequenceCommandRunner{
		outcomes: []pollerRunOutcome{
			{result: workers.CommandResult{Stdout: []byte("not-json\n")}},
			{waitForCancel: true},
		},
	}
	logCore, observedLogs := observer.New(zap.InfoLevel)
	svc := &FactoryService{
		policy: serviceCoordinatorPolicyFromConfig(&FactoryServiceConfig{RuntimeMode: interfaces.RuntimeModeService}),
		cfg:    serviceTestConfigWithWorkerEdges(t, &FactoryServiceConfig{}, workerapplication.Edges{ScriptCommandRunner: runner}),
		logger: zap.New(logCore),
		clock:  fakeClock,
	}
	initializeWorkersSchedulerForTest(svc)
	poller := newCanonicalScriptPollerWorkstation()
	runtimeCfg := newScriptPollerLoadedRuntimeConfigForServiceTest(
		t,
		t.TempDir(),
		scriptPollerRuntimeConfigOptions{
			poller: poller,
		},
	)
	handle := &liveRuntimeHandle{Bundle: &factoryRuntimeBundle{
		Factory:    &aggregateSnapshotFactory{},
		RuntimeCfg: runtimeCfg,
	},
	}
	sidecarCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := svc.startLiveRuntimeSidecars(sidecarCtx, handle); err != nil {
		t.Fatalf("startLiveRuntimeSidecars: %v", err)
	}
	defer svc.stopLiveRuntimeSidecars(handle)

	waitForPollerRunnerCalls(t, runner, 1, time.Second)
	waitForFakeClockWaiters(t, fakeClock, 1)
	fakeClock.Advance(workersservice.ScriptPollerRestartBackoffMin)
	waitForPollerRunnerCalls(t, runner, 2, time.Second)

	if observedLogs.FilterMessage("script poller restarting").Len() == 0 {
		t.Fatal("expected restart log for malformed poller output")
	}
	entry := observedLogs.FilterMessage("script poller restarting").All()[0]
	if fieldString(entry.ContextMap()["error"]) == "" || !strings.Contains(fieldString(entry.ContextMap()["error"]), "malformed stdout") {
		t.Fatalf("restart error = %#v, want malformed stdout context", entry.ContextMap()["error"])
	}
}

func TestFactoryService_StopLiveRuntimeSidecars_StopsScriptPollerAndLogsLifecycle(t *testing.T) {
	runner := &pollerSequenceCommandRunner{
		outcomes: []pollerRunOutcome{{waitForCancel: true}},
	}
	logCore, observedLogs := observer.New(zap.InfoLevel)
	svc := &FactoryService{
		policy: serviceCoordinatorPolicyFromConfig(&FactoryServiceConfig{RuntimeMode: interfaces.RuntimeModeService}),
		cfg:    serviceTestConfigWithWorkerEdges(t, &FactoryServiceConfig{}, workerapplication.Edges{ScriptCommandRunner: runner}),
		logger: zap.New(logCore),
	}
	initializeWorkersSchedulerForTest(svc)
	poller := newCanonicalScriptPollerWorkstation()
	runtimeCfg := newScriptPollerLoadedRuntimeConfigForServiceTest(
		t,
		t.TempDir(),
		scriptPollerRuntimeConfigOptions{
			poller: poller,
		},
	)
	handle := &liveRuntimeHandle{Bundle: &factoryRuntimeBundle{
		Factory:    &aggregateSnapshotFactory{},
		RuntimeCfg: runtimeCfg,
	},
	}

	if err := svc.startLiveRuntimeSidecars(context.Background(), handle); err != nil {
		t.Fatalf("startLiveRuntimeSidecars: %v", err)
	}

	waitForPollerRunnerCalls(t, runner, 1, time.Second)
	svc.stopLiveRuntimeSidecars(handle)

	if observedLogs.FilterMessage("script poller started").Len() != 1 {
		t.Fatalf("script poller started log count = %d, want 1", observedLogs.FilterMessage("script poller started").Len())
	}
	stopped := observedLogs.FilterMessage("script poller stopped").All()
	if len(stopped) != 1 {
		t.Fatalf("script poller stopped log count = %d, want 1", len(stopped))
	}
	if got := fieldString(stopped[0].ContextMap()["reason"]); got != "context canceled" {
		t.Fatalf("script poller stop reason = %q, want context canceled", got)
	}
	if runner.callCount() != 1 {
		t.Fatalf("script poller runner calls after stop = %d, want 1", runner.callCount())
	}
}

func TestFactoryService_StopLiveRuntimeSidecars_StopsPriorScriptPollerBeforeReplacementStart(t *testing.T) {
	runner := &pollerSequenceCommandRunner{
		outcomes: []pollerRunOutcome{
			{waitForCancel: true},
			{waitForCancel: true},
		},
	}
	svc := &FactoryService{
		policy: serviceCoordinatorPolicyFromConfig(&FactoryServiceConfig{RuntimeMode: interfaces.RuntimeModeService}),
		cfg:    serviceTestConfigWithWorkerEdges(t, &FactoryServiceConfig{}, workerapplication.Edges{ScriptCommandRunner: runner}),
		logger: zap.NewNop(),
	}
	initializeWorkersSchedulerForTest(svc)
	oldPoller := interfaces.FactoryWorkstationConfig{
		Name:           "linear-ingress-old",
		Kind:           interfaces.WorkstationKindPoller,
		WorkerTypeName: "poller-script",
	}
	newPoller := interfaces.FactoryWorkstationConfig{
		Name:           "linear-ingress-new",
		Kind:           interfaces.WorkstationKindPoller,
		WorkerTypeName: "poller-script",
	}
	oldHandle := newScriptPollerRuntimeHandleForWorkstation(t, oldPoller, &aggregateSnapshotFactory{})
	newHandle := newScriptPollerRuntimeHandleForWorkstation(t, newPoller, &aggregateSnapshotFactory{})

	if err := svc.startLiveRuntimeSidecars(context.Background(), oldHandle); err != nil {
		t.Fatalf("startLiveRuntimeSidecars(old): %v", err)
	}
	waitForPollerRunnerCalls(t, runner, 1, time.Second)

	svc.stopLiveRuntimeSidecars(oldHandle)

	if err := svc.startLiveRuntimeSidecars(context.Background(), newHandle); err != nil {
		t.Fatalf("startLiveRuntimeSidecars(new): %v", err)
	}
	defer svc.stopLiveRuntimeSidecars(newHandle)
	waitForPollerRunnerCalls(t, runner, 2, time.Second)

	reqs := runner.requests()
	if len(reqs) != 2 {
		t.Fatalf("poller runner requests = %d, want 2", len(reqs))
	}
	if reqs[0].WorkstationName != oldPoller.Name {
		t.Fatalf("first poller workstation = %q, want %q", reqs[0].WorkstationName, oldPoller.Name)
	}
	if reqs[1].WorkstationName != newPoller.Name {
		t.Fatalf("replacement poller workstation = %q, want %q", reqs[1].WorkstationName, newPoller.Name)
	}
}

func TestFactoryService_StopLiveRuntimeSidecars_WaitsForScriptPollerSubmitBeforeReplacementStart(t *testing.T) {
	workRequestJSON := []byte(`{
		"requestId":"linear-issue-batch-3",
		"type":"FACTORY_REQUEST_BATCH",
		"works":[{"name":"issue-125","workTypeName":"task","payload":{"id":"ISSUE-125"}}]
	}`)
	runner := &pollerSequenceCommandRunner{
		outcomes: []pollerRunOutcome{
			{result: workers.CommandResult{Stdout: workRequestJSON}},
			{waitForCancel: true},
		},
	}
	submitStarted := make(chan struct{})
	releaseSubmit := make(chan struct{})
	oldFactory := &aggregateSnapshotFactory{
		submitFunc: func(context.Context, work.WorkRequest) error {
			close(submitStarted)
			<-releaseSubmit
			return nil
		},
	}
	newFactory := &aggregateSnapshotFactory{}
	svc := &FactoryService{
		policy: serviceCoordinatorPolicyFromConfig(&FactoryServiceConfig{RuntimeMode: interfaces.RuntimeModeService}),
		cfg:    serviceTestConfigWithWorkerEdges(t, &FactoryServiceConfig{}, workerapplication.Edges{ScriptCommandRunner: runner}),
		logger: zap.NewNop(),
	}
	initializeWorkersSchedulerForTest(svc)
	oldHandle := newScriptPollerRuntimeHandle(t, "linear-ingress-old", oldFactory)
	newHandle := newScriptPollerRuntimeHandle(t, "linear-ingress-new", newFactory)

	if err := svc.startLiveRuntimeSidecars(context.Background(), oldHandle); err != nil {
		t.Fatalf("startLiveRuntimeSidecars(old): %v", err)
	}
	<-submitStarted

	stopped := make(chan struct{})
	go func() {
		svc.stopLiveRuntimeSidecars(oldHandle)
		close(stopped)
	}()

	select {
	case <-stopped:
		t.Fatal("stopLiveRuntimeSidecars(old) completed before in-flight submit drained")
	case <-time.After(100 * time.Millisecond):
	}

	close(releaseSubmit)

	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for stopLiveRuntimeSidecars(old) to finish")
	}

	if err := svc.startLiveRuntimeSidecars(context.Background(), newHandle); err != nil {
		t.Fatalf("startLiveRuntimeSidecars(new): %v", err)
	}
	defer svc.stopLiveRuntimeSidecars(newHandle)
	waitForPollerRunnerCalls(t, runner, 2, time.Second)

	if oldFactory.submitCalls != 1 {
		t.Fatalf("old runtime submit calls = %d, want 1", oldFactory.submitCalls)
	}
	if newFactory.submitCalls != 0 {
		t.Fatalf("replacement runtime submit calls before replacement poller restart = %d, want 0", newFactory.submitCalls)
	}
	reqs := runner.requests()
	if len(reqs) != 2 {
		t.Fatalf("poller runner requests = %d, want 2", len(reqs))
	}
	if reqs[0].WorkstationName != "linear-ingress-old" {
		t.Fatalf("first poller workstation = %q, want %q", reqs[0].WorkstationName, "linear-ingress-old")
	}
	if reqs[1].WorkstationName != "linear-ingress-new" {
		t.Fatalf("replacement poller workstation = %q, want %q", reqs[1].WorkstationName, "linear-ingress-new")
	}
}

func newScriptPollerRuntimeHandle(t *testing.T, workstationName string, activeFactory *aggregateSnapshotFactory) *liveRuntimeHandle {
	t.Helper()

	return newScriptPollerRuntimeHandleForWorkstation(t, interfaces.FactoryWorkstationConfig{
		Name:           workstationName,
		Kind:           interfaces.WorkstationKindPoller,
		WorkerTypeName: "poller-script",
	}, activeFactory)
}

func newScriptPollerRuntimeHandleForWorkstation(
	t *testing.T,
	poller interfaces.FactoryWorkstationConfig,
	activeFactory *aggregateSnapshotFactory,
) *liveRuntimeHandle {
	t.Helper()

	return &liveRuntimeHandle{Bundle: &factoryRuntimeBundle{
		Factory: activeFactory,
		RuntimeCfg: newScriptPollerLoadedRuntimeConfigForServiceTest(
			t,
			t.TempDir(),
			scriptPollerRuntimeConfigOptions{
				poller: poller,
			},
		),
	},
	}
}

func newCanonicalScriptPollerWorkstation() interfaces.FactoryWorkstationConfig {
	return interfaces.FactoryWorkstationConfig{
		Name:           canonicalScriptPollerWorkstationName,
		Kind:           interfaces.WorkstationKindPoller,
		WorkerTypeName: canonicalScriptPollerWorkerName,
	}
}

func newCanonicalScriptPollerWorker(args ...string) *workerconfig.Config {
	return &workerconfig.Config{
		Name:    canonicalScriptPollerWorkerName,
		Type:    interfaces.WorkerTypeScript,
		Command: canonicalScriptPollerCommand,
		Args:    args,
	}
}

type scriptPollerRuntimeConfigOptions struct {
	poller                 interfaces.FactoryWorkstationConfig
	pollerWorker           *workerconfig.Config
	additionalWorkers      []*workerconfig.Config
	additionalWorkstations []interfaces.FactoryWorkstationConfig
}

func newScriptPollerLoadedRuntimeConfigForServiceTest(
	t *testing.T,
	factoryDir string,
	options scriptPollerRuntimeConfigOptions,
) *config.LoadedFactoryConfig {
	t.Helper()

	poller := options.poller
	if poller.Name == "" {
		poller = newCanonicalScriptPollerWorkstation()
	}
	pollerWorker := options.pollerWorker
	if pollerWorker == nil {
		pollerWorker = newCanonicalScriptPollerWorker()
	}

	factoryCfg := &interfaces.FactoryConfig{
		Workers:      []workerconfig.Config{{Name: pollerWorker.Name}},
		Workstations: []interfaces.FactoryWorkstationConfig{poller},
	}
	workerConfigs := map[string]*workerconfig.Config{
		pollerWorker.Name: pollerWorker,
	}
	workstationConfigs := map[string]*interfaces.FactoryWorkstationConfig{
		poller.Name: &poller,
	}

	for _, worker := range options.additionalWorkers {
		if worker == nil {
			continue
		}
		factoryCfg.Workers = append(factoryCfg.Workers, workerconfig.Config{Name: worker.Name})
		workerConfigs[worker.Name] = worker
	}
	for i := range options.additionalWorkstations {
		workstation := options.additionalWorkstations[i]
		factoryCfg.Workstations = append(factoryCfg.Workstations, workstation)
		workstationCopy := workstation
		workstationConfigs[workstation.Name] = &workstationCopy
	}

	return newLoadedFactoryConfigForServiceTest(t, factoryDir, factoryCfg, workerConfigs, workstationConfigs)
}

type pollerRunOutcome struct {
	result        workers.CommandResult
	err           error
	waitForCancel bool
}

type pollerSequenceCommandRunner struct {
	mu       sync.Mutex
	calls    int
	reqs     []workers.CommandRequest
	outcomes []pollerRunOutcome
}

func (r *pollerSequenceCommandRunner) Run(ctx context.Context, req workers.CommandRequest) (workers.CommandResult, error) {
	r.mu.Lock()
	r.calls++
	r.reqs = append(r.reqs, req)
	index := r.calls - 1
	var outcome pollerRunOutcome
	if index < len(r.outcomes) {
		outcome = r.outcomes[index]
	} else if len(r.outcomes) > 0 {
		outcome = r.outcomes[len(r.outcomes)-1]
	}
	r.mu.Unlock()

	if outcome.waitForCancel {
		<-ctx.Done()
		return outcome.result, ctx.Err()
	}
	return outcome.result, outcome.err
}

func (r *pollerSequenceCommandRunner) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func (r *pollerSequenceCommandRunner) requests() []workers.CommandRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]workers.CommandRequest, len(r.reqs))
	copy(out, r.reqs)
	return out
}

func waitForPollerRunnerCalls(t *testing.T, runner *pollerSequenceCommandRunner, want int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if runner.callCount() >= want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d poller runner call(s); got %d", want, runner.callCount())
}

func fieldString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case error:
		return typed.Error()
	default:
		return ""
	}
}

type hostedLinearIssueFixture struct {
	ID          string
	Identifier  string
	Title       string
	Description string
	UpdatedAt   string
}

type hostedLinearPollerServiceFixture struct {
	svc        *FactoryService
	submitted  *aggregateSnapshotFactory
	runtimeCfg *config.LoadedFactoryConfig
}

type concurrentHostedAndScriptPollerFixture struct {
	hostedLinearPollerServiceFixture
}

func newHostedLinearIssuesGraphQLServer(t *testing.T, issues ...hostedLinearIssueFixture) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		nodes := make([]string, 0, len(issues))
		for _, issue := range issues {
			description := issue.Description
			if description == "" {
				description = issue.Title
			}
			nodes = append(nodes, fmt.Sprintf(`{
				"id": %q,
				"identifier": %q,
				"title": %q,
				"description": %q,
				"updatedAt": %q,
				"url": %q,
				"team": {"id": "team-1", "key": "ENG", "name": "Engineering"},
				"state": {"id": "state-1", "name": "Todo", "type": "unstarted"},
				"assignee": null
			}`, issue.ID, issue.Identifier, issue.Title, description, issue.UpdatedAt,
				fmt.Sprintf("https://linear.app/example/issue/%s", issue.Identifier)))
		}
		_, _ = w.Write([]byte(fmt.Sprintf(`{
			"data": {
				"issues": {
					"nodes": [%s],
					"pageInfo": {"hasNextPage": false, "endCursor": ""}
				}
			}
		}`, strings.Join(nodes, ","))))
	}))
}

func writeHostedLinearSecretForServiceTest(t *testing.T, factoryDir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(factoryDir, "secrets"), 0o755); err != nil {
		t.Fatalf("mkdir secrets: %v", err)
	}
	if err := os.WriteFile(filepath.Join(factoryDir, "secrets", "linear-api-key"), []byte("runtime-linear-key\n"), 0o600); err != nil {
		t.Fatalf("write secret file: %v", err)
	}
}

func newHostedLinearPollerServiceFixture(
	t *testing.T,
	server *httptest.Server,
	mutateLinear func(*workerconfig.HostedLinearWorkerConfig),
) hostedLinearPollerServiceFixture {
	t.Helper()

	factoryDir := t.TempDir()
	writeHostedLinearSecretForServiceTest(t, factoryDir)
	submitted := &aggregateSnapshotFactory{}
	svcCfg := serviceTestConfigWithWorkerEdges(t, &FactoryServiceConfig{
		RuntimeMode: interfaces.RuntimeModeService,
	}, workerapplication.Edges{
		HostedHTTPClient:     server.Client(),
		HostedLinearEndpoint: server.URL,
	})
	linearCfg := &workerconfig.HostedLinearWorkerConfig{
		PollInterval: "1h",
		Mapping: workerconfig.HostedLinearWorkerMappingConfig{
			WorkType: "story",
			State:    "init",
		},
	}
	if mutateLinear != nil {
		mutateLinear(linearCfg)
	}
	poller := interfaces.FactoryWorkstationConfig{
		Name:           "linear-ingress",
		Kind:           interfaces.WorkstationKindPoller,
		WorkerTypeName: "linear-poller",
	}
	runtimeCfg := newLoadedFactoryConfigForServiceTest(
		t,
		factoryDir,
		&interfaces.FactoryConfig{
			Workers:      []workerconfig.Config{{Name: "linear-poller"}},
			Workstations: []interfaces.FactoryWorkstationConfig{poller},
		},
		map[string]*workerconfig.Config{
			"linear-poller": {
				Name:     "linear-poller",
				Type:     interfaces.WorkerTypeHosted,
				Provider: interfaces.HostedWorkerProviderLinear,
				Auth:     &workerconfig.HostedWorkerAuthConfig{SecretRef: "secrets/linear-api-key"},
				Linear:   linearCfg,
			},
		},
		map[string]*interfaces.FactoryWorkstationConfig{poller.Name: &poller},
	)
	svc := &FactoryService{
		policy:        serviceCoordinatorPolicyFromConfig(svcCfg),
		cfg:           svcCfg,
		logger:        zap.NewNop(),
		hostedWorkers: buildHostedWorkersConfigForServiceTest(svcCfg, zap.NewNop(), nil),
	}
	initializeWorkersSchedulerForTest(svc)
	return hostedLinearPollerServiceFixture{
		svc:        svc,
		submitted:  submitted,
		runtimeCfg: runtimeCfg,
	}
}

func newConcurrentHostedAndScriptPollerFixture(t *testing.T, server *httptest.Server) concurrentHostedAndScriptPollerFixture {
	t.Helper()

	factoryDir := t.TempDir()
	writeHostedLinearSecretForServiceTest(t, factoryDir)
	scriptWorkRequestJSON := []byte(`{
		"requestId":"script-poller-batch-1",
		"type":"FACTORY_REQUEST_BATCH",
		"works":[{"name":"script-issue","workTypeName":"task","payload":{"id":"SCRIPT-1"}}]
	}`)
	runner := &pollerSequenceCommandRunner{
		outcomes: []pollerRunOutcome{{result: workers.CommandResult{Stdout: scriptWorkRequestJSON}}},
	}
	submitted := &aggregateSnapshotFactory{}
	svcCfg := serviceTestConfigWithWorkerEdges(t, &FactoryServiceConfig{
		RuntimeMode: interfaces.RuntimeModeService,
	}, workerapplication.Edges{
		ScriptCommandRunner: runner, HostedHTTPClient: server.Client(), HostedLinearEndpoint: server.URL,
	})
	hostedPoller := interfaces.FactoryWorkstationConfig{
		Name:           "linear-hosted-ingress",
		Kind:           interfaces.WorkstationKindPoller,
		WorkerTypeName: "linear-poller",
	}
	scriptPoller := interfaces.FactoryWorkstationConfig{
		Name:           "script-ingress",
		Kind:           interfaces.WorkstationKindPoller,
		WorkerTypeName: canonicalScriptPollerWorkerName,
	}
	runtimeCfg := newLoadedFactoryConfigForServiceTest(
		t,
		factoryDir,
		&interfaces.FactoryConfig{
			Workers: []workerconfig.Config{
				{Name: "linear-poller"},
				{Name: canonicalScriptPollerWorkerName},
			},
			Workstations: []interfaces.FactoryWorkstationConfig{hostedPoller, scriptPoller},
		},
		map[string]*workerconfig.Config{
			"linear-poller": {
				Name:     "linear-poller",
				Type:     interfaces.WorkerTypeHosted,
				Provider: interfaces.HostedWorkerProviderLinear,
				Auth:     &workerconfig.HostedWorkerAuthConfig{SecretRef: "secrets/linear-api-key"},
				Linear: &workerconfig.HostedLinearWorkerConfig{
					PollInterval: "1h",
					Mapping: workerconfig.HostedLinearWorkerMappingConfig{
						WorkType: "story",
						State:    "init",
					},
				},
			},
			canonicalScriptPollerWorkerName: newCanonicalScriptPollerWorker(),
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			hostedPoller.Name: &hostedPoller,
			scriptPoller.Name: &scriptPoller,
		},
	)
	svc := &FactoryService{
		policy:        serviceCoordinatorPolicyFromConfig(svcCfg),
		cfg:           svcCfg,
		logger:        zap.NewNop(),
		hostedWorkers: buildHostedWorkersConfigForServiceTest(svcCfg, zap.NewNop(), nil),
	}
	initializeWorkersSchedulerForTest(svc)
	return concurrentHostedAndScriptPollerFixture{
		hostedLinearPollerServiceFixture: hostedLinearPollerServiceFixture{
			svc:        svc,
			submitted:  submitted,
			runtimeCfg: runtimeCfg,
		},
	}
}

func startHostedLinearPollerSidecars(t *testing.T, fixture hostedLinearPollerServiceFixture) func() {
	t.Helper()
	handle := &liveRuntimeHandle{Bundle: &factoryRuntimeBundle{
		RuntimeCfg: fixture.runtimeCfg,
		Factory:    fixture.submitted,
	},
	}
	sidecarCtx, cancel := context.WithCancel(context.Background())
	if err := fixture.svc.startLiveRuntimeSidecars(sidecarCtx, handle); err != nil {
		cancel()
		t.Fatalf("startLiveRuntimeSidecars: %v", err)
	}
	return func() {
		fixture.svc.stopLiveRuntimeSidecars(handle)
		cancel()
	}
}

func assertHostedLinearBatchWorks(t *testing.T, submitted *aggregateSnapshotFactory, wantWorkIDs ...string) {
	t.Helper()
	calls, submissions := submitted.submissionSnapshot()
	if calls != 1 {
		t.Fatalf("submit calls = %d, want 1 batch submit for the poll cycle", calls)
	}
	if len(submissions) != 1 {
		t.Fatalf("submitted requests = %d, want 1", len(submissions))
	}
	works := submissions[0].Works
	if len(works) != len(wantWorkIDs) {
		t.Fatalf("submitted works = %d, want %d canonical outputs from one poll cycle", len(works), len(wantWorkIDs))
	}
	for i, wantWorkID := range wantWorkIDs {
		if works[i].WorkID != wantWorkID {
			t.Fatalf("submitted work ID[%d] = %q, want %q", i, works[i].WorkID, wantWorkID)
		}
	}
	if submissions[0].RequestID == "" || works[0].RequestID != works[1].RequestID {
		t.Fatalf("batch request IDs = [%q %q], want shared non-empty requestId", works[0].RequestID, works[1].RequestID)
	}
}

func assertConcurrentHostedAndScriptPollerSubmissions(t *testing.T, submitted *aggregateSnapshotFactory) {
	t.Helper()
	calls, submissions := submitted.submissionSnapshot()
	if calls < 2 {
		t.Fatalf("submit calls = %d, want at least 2 from concurrent pollers", calls)
	}
	if !hasConcurrentHostedAndScriptPollerSubmissions(submissions) {
		t.Fatalf("submitted works = %#v, want both hosted linear:issue-hosted and script-issue outputs", submissions)
	}
}

func waitForConcurrentHostedAndScriptPollerSubmissions(t *testing.T, submitted *aggregateSnapshotFactory, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		_, submissions := submitted.submissionSnapshot()
		if hasConcurrentHostedAndScriptPollerSubmissions(submissions) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	_, submissions := submitted.submissionSnapshot()
	t.Fatalf("timed out waiting for both hosted linear:issue-hosted and script-issue outputs; submitted works = %#v", submissions)
}

func hasConcurrentHostedAndScriptPollerSubmissions(submissions []work.WorkRequest) bool {
	var hostedSubmitted, scriptSubmitted bool
	for _, request := range submissions {
		for _, work := range request.Works {
			hostedSubmitted = hostedSubmitted || work.WorkID == "linear:issue-hosted"
			scriptSubmitted = scriptSubmitted || work.Name == "script-issue"
		}
	}
	return hostedSubmitted && scriptSubmitted
}

func waitForHostedPollerSubmission(t *testing.T, submitted *aggregateSnapshotFactory, want int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		calls, _ := submitted.submissionSnapshot()
		if calls >= want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	calls, _ := submitted.submissionSnapshot()
	t.Fatalf("timed out waiting for %d hosted poller submission(s); got %d", want, calls)
}

func waitForObservedLogMessage(t *testing.T, logs *observer.ObservedLogs, message string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if logs.FilterMessage(message).Len() > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for log message %q", message)
}

func TestFactoryService_RequiredInputCronKeepsTimeWorkPendingWhenInputMissing(t *testing.T) {
	start := time.Date(2026, time.April, 18, 12, 30, 0, 0, time.UTC)
	fakeClock := clockwork.NewFakeClockAt(start)
	observedSubmissions := make(chan work.FactorySubmissionRecord, 16)
	svc, runCtx, errCh, cancelRun := buildCronServiceForIngressTest(
		t,
		fakeClock,
		requiredInputCronFactoryConfigWithExpiry("* * * * *", "40ms"),
		observedSubmissions,
	)
	defer cancelRun()

	ws := configuredCronWorkstationForServiceTest(t, svc, "poll-with-input")
	if err := svc.submitCronTick(runCtx, ws, start); err != nil {
		t.Fatalf("submitCronTick: %v", err)
	}

	firstRecord := waitForCronSubmission(t, observedSubmissions, time.Second)
	if firstRecord.Request.WorkTypeID != interfaces.SystemTimeWorkTypeID {
		t.Fatalf("required-input cron submission work type = %q, want %q", firstRecord.Request.WorkTypeID, interfaces.SystemTimeWorkTypeID)
	}
	if firstRecord.Request.Tags[interfaces.TimeWorkTagKeyCronWorkstation] != "poll-with-input" {
		t.Fatalf("required-input cron workstation tag = %q, want poll-with-input", firstRecord.Request.Tags[interfaces.TimeWorkTagKeyCronWorkstation])
	}

	pendingSnap := waitForPendingCronTimeToken(t, svc, firstRecord.Request.WorkID)
	if pendingSnap.InFlightCount != 0 || len(pendingSnap.Dispatches) != 0 {
		t.Fatalf("required-input cron dispatched while input was missing: inflight=%d dispatches=%#v", pendingSnap.InFlightCount, pendingSnap.Dispatches)
	}
	if tokens := pendingSnap.Marking.TokensInPlace("task:init"); len(tokens) != 0 {
		t.Fatalf("required-input cron created task output before input existed: %#v", tokens)
	}
	stopServiceModeRun(t, cancelRun, errCh)
}

func waitForPendingCronTimeToken(
	t *testing.T,
	svc *FactoryService,
	workID string,
) *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net] {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		snap, err := svc.GetEngineStateSnapshot(context.Background())
		if err != nil {
			t.Fatalf("GetEngineStateSnapshot pending time work: %v", err)
		}
		for _, token := range snap.Marking.TokensInPlace(interfaces.SystemTimePendingPlaceID) {
			if token.Color.WorkID == workID {
				return snap
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for required-input cron time token in %s", interfaces.SystemTimePendingPlaceID)
	return nil
}

func TestFactoryService_BatchModeDoesNotStartCronWatchers(t *testing.T) {
	dir := t.TempDir()
	writeFactoryJSON(t, dir, cronFactoryConfig("* * * * *"))
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	observedSubmissions := make(chan work.FactorySubmissionRecord, 1)
	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               dir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
		ExtraOptions: []factory.FactoryOption{
			factory.WithSubmissionRecorder(func(record work.FactorySubmissionRecord) {
				observedSubmissions <- record
			}),
		},
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := svc.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	select {
	case record := <-observedSubmissions:
		t.Fatalf("batch-mode cron watcher submitted unexpectedly: %#v", record)
	default:
	}
}

func cronFactoryConfig(schedule string) map[string]any {
	return map[string]any{
		"name": "factory",
		"workTypes": []map[string]any{
			{
				"name": "task",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workers": []map[string]string{{"name": "cron-worker"}},
		"workstations": []map[string]any{
			{
				"name":     "poll-for-work",
				"behavior": "CRON",
				"worker":   "cron-worker",
				"cron":     map[string]string{"schedule": schedule, "expiryWindow": "500ms"},
				"outputs":  []map[string]string{{"workType": "task", "state": "init"}},
			},
		},
	}
}

func cronFactoryConfigWithTriggerAtStart(schedule string, triggerAtStart bool) map[string]any {
	cfg := cronFactoryConfig(schedule)
	workstations := cfg["workstations"].([]map[string]any)
	workstations[0]["cron"] = map[string]any{
		"schedule":       schedule,
		"expiryWindow":   "500ms",
		"triggerAtStart": triggerAtStart,
	}
	return cfg
}

func cronLoadedFactoryConfigForServiceTest(t *testing.T, factoryDir string, triggerAtStart bool) *config.LoadedFactoryConfig {
	t.Helper()

	ws := interfaces.FactoryWorkstationConfig{
		Name: "poll-for-work",
		Kind: interfaces.WorkstationKindCron,
		Cron: &interfaces.CronConfig{
			Schedule:       "* * * * *",
			TriggerAtStart: triggerAtStart,
		},
		Outputs: []interfaces.IOConfig{{
			WorkTypeName: "task",
			StateName:    "init",
		}},
	}
	return newLoadedFactoryConfigForServiceTest(
		t,
		factoryDir,
		&interfaces.FactoryConfig{
			WorkTypes:    []interfaces.WorkTypeConfig{{Name: "task"}},
			Workstations: []interfaces.FactoryWorkstationConfig{ws},
		},
		nil,
		map[string]*interfaces.FactoryWorkstationConfig{ws.Name: &ws},
	)
}

func cronFactoryConfigWithOutputState(schedule, outputState string) map[string]any {
	cfg := cronFactoryConfig(schedule)
	workTypes := cfg["workTypes"].([]map[string]any)
	task := workTypes[0]
	task["states"] = []map[string]string{
		{"name": "init", "type": "INITIAL"},
		{"name": "ready", "type": "PROCESSING"},
		{"name": "complete", "type": "TERMINAL"},
		{"name": "failed", "type": "FAILED"},
	}
	workstations := cfg["workstations"].([]map[string]any)
	workstations[0]["outputs"] = []map[string]string{{"workType": "task", "state": outputState}}
	return cfg
}

func requiredInputCronFactoryConfigWithExpiry(schedule, expiryWindow string) map[string]any {
	cron := map[string]string{"schedule": schedule}
	if expiryWindow != "" {
		cron["expiryWindow"] = expiryWindow
	}
	return map[string]any{
		"name": "factory",
		"workTypes": []map[string]any{
			{
				"name": "signal",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
			{
				"name": "task",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workers": []map[string]string{{"name": "cron-worker"}},
		"workstations": []map[string]any{
			{
				"name":       "poll-with-input",
				"behavior":   "CRON",
				"worker":     "cron-worker",
				"cron":       cron,
				"inputs":     []map[string]string{{"workType": "signal", "state": "init"}},
				"outputs":    []map[string]string{{"workType": "task", "state": "init"}},
				"onContinue": []map[string]string{{"workType": "signal", "state": "complete"}},
			},
		},
	}
}

func TestFactoryService_ServiceModeCronScheduleConfigStartsAndStopsService(t *testing.T) {
	dir := t.TempDir()
	writeFactoryJSON(t, dir, cronFactoryConfig("* * * * *"))
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               dir,
		RuntimeMode:       interfaces.RuntimeModeService,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	runCtx, cancelRun := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.Run(runCtx)
	}()

	waitForSessionRuntimeStatus(t, svc, defaultFactorySessionID, interfaces.RuntimeStatusIdle, time.Second, "default cron runtime")
	stopServiceModeRun(t, cancelRun, errCh)
}

func TestIsCanceledServiceStartup(t *testing.T) {
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	if !isCanceledServiceStartup(canceledCtx, context.Canceled) {
		t.Fatal("expected canceled startup to be treated as graceful shutdown")
	}
	if isCanceledServiceStartup(context.Background(), context.Canceled) {
		t.Fatal("expected uncanceled parent context to preserve startup cancellation as an error")
	}
	if isCanceledServiceStartup(canceledCtx, context.DeadlineExceeded) {
		t.Fatal("expected non-cancellation startup errors to remain failures")
	}
}

func TestFactoryService_StartLiveRuntimeSidecars_SkipsNonCronAndTriggersOnlyCronWorkstations(t *testing.T) {
	start := time.Date(2026, time.April, 18, 12, 30, 0, 0, time.UTC)
	fakeClock := clockwork.NewFakeClockAt(start)
	logCore, observedLogs := observer.New(zap.InfoLevel)
	currentFactory := &aggregateSnapshotFactory{}
	replacementFactory := &aggregateSnapshotFactory{}
	validCron := interfaces.FactoryWorkstationConfig{
		Name: "valid-cron",
		Kind: interfaces.WorkstationKindCron,
		Cron: &interfaces.CronConfig{
			Schedule:       "* * * * *",
			TriggerAtStart: true,
		},
		Outputs: []interfaces.IOConfig{{
			WorkTypeName: "task",
			StateName:    "init",
		}},
	}
	manual := interfaces.FactoryWorkstationConfig{
		Name: "manual-step",
		Kind: interfaces.WorkstationKindStandard,
		Inputs: []interfaces.IOConfig{{
			WorkTypeName: "task",
			StateName:    "init",
		}},
		Outputs: []interfaces.IOConfig{{
			WorkTypeName: "task",
			StateName:    "complete",
		}},
	}
	runtimeCfg := newLoadedFactoryConfigForServiceTest(
		t,
		"factory-alpha",
		&interfaces.FactoryConfig{
			WorkTypes:    []interfaces.WorkTypeConfig{{Name: "task"}},
			Workstations: []interfaces.FactoryWorkstationConfig{manual, validCron},
		},
		nil,
		map[string]*interfaces.FactoryWorkstationConfig{
			manual.Name:    &manual,
			validCron.Name: &validCron,
		},
	)
	observedRequests := make(chan work.WorkRequest, 8)
	replacementFactory.submitFunc = func(_ context.Context, request work.WorkRequest) error {
		select {
		case observedRequests <- request:
		default:
			t.Fatalf("cron request channel overflow")
		}
		return nil
	}
	svc := &FactoryService{
		policy: serviceCoordinatorPolicyFromConfig(&FactoryServiceConfig{RuntimeMode: interfaces.RuntimeModeService}),
		logger: zap.New(logCore),
		clock:  fakeClock,
	}
	initializeWorkersSchedulerForTest(svc)
	handle := &liveRuntimeHandle{Bundle: &factoryRuntimeBundle{
		Factory:    replacementFactory,
		RuntimeCfg: runtimeCfg,
	},
	}
	sidecarCtx, cancelSidecars := context.WithCancel(context.Background())
	defer cancelSidecars()

	if err := svc.startLiveRuntimeSidecars(sidecarCtx, handle); err != nil {
		t.Fatalf("startLiveRuntimeSidecars: %v", err)
	}
	defer svc.stopLiveRuntimeSidecars(handle)

	startupRequest := waitForCronWorkRequest(t, observedRequests, time.Second)
	assertCronWorkRequestNominalAt(t, startupRequest, start)
	if got := startupRequest.Works[0].Tags[interfaces.TimeWorkTagKeyCronWorkstation]; got != "valid-cron" {
		t.Fatalf("startup cron workstation tag = %q, want valid-cron", got)
	}

	waitForFakeClockWaiters(t, fakeClock, 1)
	assertNoCronWorkRequestQueued(t, observedRequests)
	fakeClock.Advance(time.Minute)
	scheduledRequest := waitForCronWorkRequest(t, observedRequests, time.Second)
	assertCronWorkRequestNominalAt(t, scheduledRequest, start.Add(time.Minute))
	if got := scheduledRequest.Works[0].Tags[interfaces.TimeWorkTagKeyCronWorkstation]; got != "valid-cron" {
		t.Fatalf("scheduled cron workstation tag = %q, want valid-cron", got)
	}
	assertNoCronWorkRequestQueued(t, observedRequests)

	if currentFactory.submitCalls != 0 {
		t.Fatalf("current runtime submit calls = %d, want 0", currentFactory.submitCalls)
	}
	if replacementFactory.submitCalls != 2 {
		t.Fatalf("replacement runtime submit calls = %d, want 2", replacementFactory.submitCalls)
	}
	assertCronWatcherRegistrationLog(t, observedLogs, "valid-cron")
	assertCronSchedulerStartedLog(t, observedLogs, 1)
}

func TestFactoryService_ServiceModeCronSchedulerUsesFakeClockAndStopsOnCancel(t *testing.T) {
	start := time.Date(2026, time.April, 18, 12, 30, 0, 0, time.UTC)
	fakeClock := clockwork.NewFakeClockAt(start)
	dir := t.TempDir()
	writeFactoryJSON(t, dir, cronFactoryConfigWithTriggerAtStart("* * * * *", false))
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	observedSubmissions := make(chan work.FactorySubmissionRecord, 8)
	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               dir,
		RuntimeMode:       interfaces.RuntimeModeService,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
		Clock:             fakeClock,
		ExtraOptions: []factory.FactoryOption{
			factory.WithSubmissionRecorder(nonBlockingSubmissionRecorder(observedSubmissions)),
		},
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	runCtx, cancelRun := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.Run(runCtx)
	}()

	waitForFakeClockWaiters(t, fakeClock, 1)
	assertNoCronSubmissionQueued(t, observedSubmissions)

	fakeClock.Advance(time.Minute)
	record := waitForCronSubmission(t, observedSubmissions, time.Second)
	wantNominalAt := start.Add(time.Minute).Format(time.RFC3339Nano)
	if record.Request.Tags[interfaces.TimeWorkTagKeyNominalAt] != wantNominalAt {
		cancelRun()
		t.Fatalf("cron nominal_at tag = %q, want %q", record.Request.Tags[interfaces.TimeWorkTagKeyNominalAt], wantNominalAt)
	}
	if record.Request.Tags[interfaces.TimeWorkTagKeyCronWorkstation] != "poll-for-work" {
		cancelRun()
		t.Fatalf("cron workstation tag = %q, want poll-for-work", record.Request.Tags[interfaces.TimeWorkTagKeyCronWorkstation])
	}

	cancelRun()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run after cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for service-mode cron scheduler to stop")
	}

	fakeClock.Advance(time.Minute)
	assertNoCronSubmissionQueued(t, observedSubmissions)
}

func TestFactoryService_ServiceModeCronTriggerAtStartSubmitsOnceAndKeepsSchedule(t *testing.T) {
	start := time.Date(2026, time.April, 18, 12, 30, 0, 0, time.UTC)
	fakeClock := clockwork.NewFakeClockAt(start)
	dir := t.TempDir()
	writeFactoryJSON(t, dir, cronFactoryConfigWithTriggerAtStart("* * * * *", true))
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	observedSubmissions := make(chan work.FactorySubmissionRecord, 8)
	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               dir,
		RuntimeMode:       interfaces.RuntimeModeService,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
		Clock:             fakeClock,
		ExtraOptions: []factory.FactoryOption{
			factory.WithSubmissionRecorder(nonBlockingSubmissionRecorder(observedSubmissions)),
		},
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	runCtx, cancelRun := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.Run(runCtx)
	}()

	startupRecord := waitForCronSubmission(t, observedSubmissions, time.Second)
	assertCronSubmissionNominalAt(t, startupRecord, start)
	waitForCompletedDispatchConsumingWorkID(t, svc, startupRecord.Request.WorkID, time.Second)

	waitForFakeClockWaiters(t, fakeClock, 1)
	assertNoCronSubmissionQueued(t, observedSubmissions)
	fakeClock.Advance(time.Minute)
	scheduledRecord := waitForCronSubmission(t, observedSubmissions, time.Second)
	assertCronSubmissionNominalAt(t, scheduledRecord, start.Add(time.Minute))
	if scheduledRecord.Request.WorkID == startupRecord.Request.WorkID {
		cancelRun()
		t.Fatal("scheduled cron fire reused startup trigger work ID")
	}

	cancelRun()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run after cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for service-mode cron scheduler to stop")
	}
}

func TestFactoryService_StartLiveRuntimeSidecars_BindsCronTriggerAtStartToReplacementRuntime(t *testing.T) {
	fakeClock := clockwork.NewFakeClock()
	currentFactory := &aggregateSnapshotFactory{}
	replacementFactory := &aggregateSnapshotFactory{}
	svc := &FactoryService{
		policy: serviceCoordinatorPolicyFromConfig(&FactoryServiceConfig{RuntimeMode: interfaces.RuntimeModeService}),
		logger: zap.NewNop(),
		clock:  fakeClock,
	}
	initializeWorkersSchedulerForTest(svc)
	handle := &liveRuntimeHandle{Bundle: &factoryRuntimeBundle{
		Factory:    replacementFactory,
		RuntimeCfg: cronLoadedFactoryConfigForServiceTest(t, "beta", true),
	},
	}
	sidecarCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := svc.startLiveRuntimeSidecars(sidecarCtx, handle); err != nil {
		t.Fatalf("startLiveRuntimeSidecars: %v", err)
	}
	defer svc.stopLiveRuntimeSidecars(handle)

	if currentFactory.submitCalls != 0 {
		t.Fatalf("current runtime submit calls = %d, want 0", currentFactory.submitCalls)
	}
	if replacementFactory.submitCalls != 1 {
		t.Fatalf("replacement runtime submit calls = %d, want 1", replacementFactory.submitCalls)
	}
	if got := replacementFactory.submissions[0].Works[0].WorkTypeID; got != interfaces.SystemTimeWorkTypeID {
		t.Fatalf("replacement runtime submission work type = %q, want %q", got, interfaces.SystemTimeWorkTypeID)
	}
}

func TestFactoryService_CronTickSubmitsThroughEngineIngressAndAppearsInSnapshot(t *testing.T) {
	start := time.Date(2026, time.April, 18, 12, 30, 0, 0, time.UTC)
	fakeClock := clockwork.NewFakeClockAt(start)
	observedSubmissions := make(chan work.FactorySubmissionRecord, 16)
	svc, runCtx, errCh, cancelRun := buildCronServiceForIngressTest(t, fakeClock, cronFactoryConfig("* * * * *"), observedSubmissions)
	defer cancelRun()

	ws := configuredCronWorkstationForServiceTest(t, svc, "poll-for-work")
	if err := svc.submitCronTick(runCtx, ws, start); err != nil {
		t.Fatalf("submitCronTick: %v", err)
	}

	record := waitForCronSubmission(t, observedSubmissions, time.Second)
	assertCronSubmissionRecord(t, record, "poll-for-work")
	assertCronDispatchAndOutput(t, svc, record.Request.WorkID, "task:init")
	stopServiceModeRun(t, cancelRun, errCh)
}

func assertCronWatcherRegistrationLog(t *testing.T, observedLogs *observer.ObservedLogs, workstation string) {
	t.Helper()
	registered := observedLogs.FilterMessage("cron watcher registered").All()
	if len(registered) != 1 {
		t.Fatalf("registered cron watcher count = %d, want 1", len(registered))
	}
	if got := registered[0].ContextMap()["workstation"]; got != workstation {
		t.Fatalf("registered cron watcher workstation = %#v, want %s", got, workstation)
	}
}

func assertCronSchedulerStartedLog(t *testing.T, observedLogs *observer.ObservedLogs, jobs int64) {
	t.Helper()
	started := observedLogs.FilterMessage("cron scheduler started").All()
	if len(started) != 1 {
		t.Fatalf("cron scheduler started log count = %d, want 1", len(started))
	}
	if got := started[0].ContextMap()["jobs"]; got != jobs {
		t.Fatalf("cron scheduler started jobs = %#v, want %d", got, jobs)
	}
}

func buildCronServiceForIngressTest(
	t *testing.T,
	fakeClock *clockwork.FakeClock,
	cfg map[string]any,
	observedSubmissions chan work.FactorySubmissionRecord,
) (*FactoryService, context.Context, <-chan error, context.CancelFunc) {
	t.Helper()

	dir := t.TempDir()
	writeFactoryJSON(t, dir, cfg)
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               dir,
		RuntimeMode:       interfaces.RuntimeModeService,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
		Clock:             fakeClock,
		ExtraOptions: []factory.FactoryOption{
			factory.WithSubmissionRecorder(nonBlockingSubmissionRecorder(observedSubmissions)),
		},
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	runCtx, cancelRun := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.Run(runCtx)
	}()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		handle := svc.currentLiveRuntime()
		if handle != nil {
			startCtx, cancel := context.WithTimeout(context.Background(), time.Second)
			err := svc.waitForLiveRuntimeStart(startCtx, handle)
			cancel()
			if err != nil {
				t.Fatalf("wait for cron service startup: %v", err)
			}
			return svc, runCtx, errCh, cancelRun
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for cron service runtime handle")
	return svc, runCtx, errCh, cancelRun
}

func assertCronSubmissionRecord(t *testing.T, record work.FactorySubmissionRecord, workstation string) {
	t.Helper()
	if record.Source != "external-submit" {
		t.Fatalf("cron submission source = %q, want external-submit", record.Source)
	}
	if record.Request.WorkTypeID != interfaces.SystemTimeWorkTypeID {
		t.Fatalf("cron submission work type = %q, want %q", record.Request.WorkTypeID, interfaces.SystemTimeWorkTypeID)
	}
	if record.Request.TargetState != interfaces.SystemTimePendingState {
		t.Fatalf("cron submission target state = %q, want %q", record.Request.TargetState, interfaces.SystemTimePendingState)
	}
	if record.Request.Tags[interfaces.TimeWorkTagKeySource] != "cron" {
		t.Fatalf("cron submission source tag = %q, want cron", record.Request.Tags[interfaces.TimeWorkTagKeySource])
	}
	if record.Request.Tags[interfaces.TimeWorkTagKeyCronWorkstation] != workstation {
		t.Fatalf("cron submission workstation tag = %q, want %q", record.Request.Tags[interfaces.TimeWorkTagKeyCronWorkstation], workstation)
	}
}

func assertCronDispatchAndOutput(t *testing.T, svc *FactoryService, workID, outputPlace string) {
	t.Helper()
	dispatch := waitForCompletedDispatchConsumingWorkID(t, svc, workID, time.Second)
	matched := consumedTokenWithWorkID(dispatch.ConsumedTokens, workID)
	if matched == nil {
		t.Fatalf("completed cron dispatch did not retain consumed time token %q: %#v", workID, dispatch.ConsumedTokens)
	}
	if matched.Color.WorkTypeID != interfaces.SystemTimeWorkTypeID {
		t.Fatalf("cron token work type = %q, want %q", matched.Color.WorkTypeID, interfaces.SystemTimeWorkTypeID)
	}
	if matched.Color.TraceID == "" {
		t.Fatal("expected cron token to receive a trace ID")
	}
	if matched.Color.Name != "cron:poll-for-work" {
		t.Fatalf("cron token name = %q, want %q", matched.Color.Name, "cron:poll-for-work")
	}
	if matched.Color.Tags[interfaces.TimeWorkTagKeySource] != "cron" {
		t.Fatalf("cron token source tag = %q, want cron", matched.Color.Tags[interfaces.TimeWorkTagKeySource])
	}

	var payload map[string]string
	if err := json.Unmarshal(matched.Color.Payload, &payload); err != nil {
		t.Fatalf("cron token payload is not JSON: %v\npayload=%s", err, matched.Color.Payload)
	}
	if payload["cron_workstation"] != "poll-for-work" {
		t.Fatalf("cron payload workstation = %q, want poll-for-work", payload["cron_workstation"])
	}
	for _, key := range []string{"nominal_at", "due_at", "expires_at", "jitter", "source"} {
		if payload[key] == "" {
			t.Fatalf("expected cron payload to include %s, got %#v", key, payload)
		}
	}
	if tags := matched.Color.Tags; tags[interfaces.TimeWorkTagKeyNominalAt] == "" || tags[interfaces.TimeWorkTagKeyDueAt] == "" || tags[interfaces.TimeWorkTagKeyExpiresAt] == "" {
		t.Fatalf("expected cron timing tags, got %#v", tags)
	}

	output := waitForTokenInPlaceByParent(t, svc, outputPlace, workID, time.Second)
	if output.Color.WorkTypeID != "task" {
		t.Fatalf("cron worker-backed output work type = %q, want task", output.Color.WorkTypeID)
	}
}

func waitForFakeClockWaiters(t *testing.T, fakeClock *clockwork.FakeClock, waiters int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := fakeClock.BlockUntilContext(ctx, waiters); err != nil {
		t.Fatalf("timed out waiting for %d fake-clock waiter(s): %v", waiters, err)
	}
}

func waitForCronSubmission(t *testing.T, submissions <-chan work.FactorySubmissionRecord, timeout time.Duration) work.FactorySubmissionRecord {
	t.Helper()
	select {
	case record := <-submissions:
		if record.Request.WorkTypeID != interfaces.SystemTimeWorkTypeID {
			t.Fatalf("cron submission work type = %q, want %q", record.Request.WorkTypeID, interfaces.SystemTimeWorkTypeID)
		}
		return record
	case <-time.After(timeout):
		t.Fatal("timed out waiting for cron submission")
	}
	return work.FactorySubmissionRecord{}
}

func assertCronSubmissionNominalAt(t *testing.T, record work.FactorySubmissionRecord, want time.Time) {
	assertCronSubmissionNominalAtForWorkstation(t, record, want, "poll-for-work")
}

func assertCronSubmissionNominalAtForWorkstation(t *testing.T, record work.FactorySubmissionRecord, want time.Time, workstation string) {
	t.Helper()
	got := record.Request.Tags[interfaces.TimeWorkTagKeyNominalAt]
	if got != want.Format(time.RFC3339Nano) {
		t.Fatalf("cron nominal_at tag = %q, want %q", got, want.Format(time.RFC3339Nano))
	}
	if record.Request.Tags[interfaces.TimeWorkTagKeyCronWorkstation] != workstation {
		t.Fatalf("cron workstation tag = %q, want %s", record.Request.Tags[interfaces.TimeWorkTagKeyCronWorkstation], workstation)
	}
}

func assertNoCronSubmissionQueued(t *testing.T, submissions <-chan work.FactorySubmissionRecord) {
	t.Helper()
	select {
	case record := <-submissions:
		t.Fatalf("cron submission observed unexpectedly: %#v", record)
	default:
	}
}

func waitForCronWorkRequest(t *testing.T, requests <-chan work.WorkRequest, timeout time.Duration) work.WorkRequest {
	t.Helper()
	select {
	case request := <-requests:
		if len(request.Works) != 1 || request.Works[0].WorkTypeID != interfaces.SystemTimeWorkTypeID {
			t.Fatalf("cron work request works = %#v, want one internal time work item", request.Works)
		}
		return request
	case <-time.After(timeout):
		t.Fatal("timed out waiting for cron work request")
	}
	return work.WorkRequest{}
}

func assertCronWorkRequestNominalAt(t *testing.T, request work.WorkRequest, want time.Time) {
	t.Helper()
	if got := request.Works[0].Tags[interfaces.TimeWorkTagKeyNominalAt]; got != want.Format(time.RFC3339Nano) {
		t.Fatalf("cron nominal_at tag = %q, want %q", got, want.Format(time.RFC3339Nano))
	}
}

func assertNoCronWorkRequestQueued(t *testing.T, requests <-chan work.WorkRequest) {
	t.Helper()
	select {
	case request := <-requests:
		t.Fatalf("cron work request observed unexpectedly: %#v", request)
	default:
	}
}

func matchedTokenSnapshotTokensInPlace(t *testing.T, svc *FactoryService, placeID string) []factorytoken.Token {
	t.Helper()
	snap, err := svc.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	return snap.Marking.TokensInPlace(placeID)
}

func configuredCronWorkstationForServiceTest(t *testing.T, svc *FactoryService, name string) interfaces.FactoryWorkstationConfig {
	t.Helper()
	runtimeCfg := svc.currentRuntimeConfig()
	if svc == nil || runtimeCfg == nil {
		t.Fatal("expected loaded service runtime config")
	}
	ws, ok := runtimeCfg.Workstation(name)
	if !ok {
		t.Fatalf("expected cron workstation config %q", name)
	}
	return *ws
}

func waitForCompletedDispatchConsumingWorkID(t *testing.T, svc *FactoryService, workID string, timeout time.Duration) interfaces.CompletedDispatch {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		snap, err := svc.GetEngineStateSnapshot(context.Background())
		if err != nil {
			t.Fatalf("GetEngineStateSnapshot dispatch history: %v", err)
		}
		for _, dispatch := range snap.DispatchHistory {
			if consumedTokenWithWorkID(dispatch.ConsumedTokens, workID) != nil {
				return dispatch
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for completed dispatch consuming work %q", workID)
	return interfaces.CompletedDispatch{}
}

func consumedTokenWithWorkID(tokens []factorytoken.Token, workID string) *factorytoken.Token {
	for i := range tokens {
		if tokens[i].Color.WorkID == workID {
			return &tokens[i]
		}
	}
	return nil
}

func nonBlockingSubmissionRecorder(records chan<- work.FactorySubmissionRecord) func(work.FactorySubmissionRecord) {
	return func(record work.FactorySubmissionRecord) {
		select {
		case records <- record:
		default:
		}
	}
}

func waitForTokenInPlaceByParent(t *testing.T, svc *FactoryService, placeID string, parentID string, timeout time.Duration) factorytoken.Token {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		snap, err := svc.GetEngineStateSnapshot(context.Background())
		if err != nil {
			t.Fatalf("GetEngineStateSnapshot output token: %v", err)
		}
		for _, token := range snap.Marking.TokensInPlace(placeID) {
			if token.Color.ParentID == parentID {
				return token
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for token in %s with parent %q", placeID, parentID)
	return factorytoken.Token{}
}

func TestFactoryService_CronTickTargetsInternalTimePlaceDespiteConfiguredOutputState(t *testing.T) {
	start := time.Date(2026, time.April, 18, 12, 30, 0, 0, time.UTC)
	fakeClock := clockwork.NewFakeClockAt(start)
	observedSubmissions := make(chan work.FactorySubmissionRecord, 16)
	svc, runCtx, errCh, cancelRun := buildCronServiceForIngressTest(t, fakeClock, cronFactoryConfigWithOutputState("* * * * *", "ready"), observedSubmissions)
	defer cancelRun()

	ws := configuredCronWorkstationForServiceTest(t, svc, "poll-for-work")
	if err := svc.submitCronTick(runCtx, ws, start); err != nil {
		t.Fatalf("submitCronTick: %v", err)
	}

	record := waitForCronSubmission(t, observedSubmissions, time.Second)
	if record.Request.WorkTypeID != interfaces.SystemTimeWorkTypeID {
		t.Fatalf("cron submission work type = %q, want %q", record.Request.WorkTypeID, interfaces.SystemTimeWorkTypeID)
	}
	if record.Request.TargetState != interfaces.SystemTimePendingState {
		t.Fatalf("cron submission target state = %q, want %q", record.Request.TargetState, interfaces.SystemTimePendingState)
	}
	assertCronDispatchAndOutput(t, svc, record.Request.WorkID, "task:ready")
	if tokens := matchedTokenSnapshotTokensInPlace(t, svc, "task:init"); len(tokens) != 0 {
		t.Fatalf("cron created customer token in initial state despite configured output state: %#v", tokens)
	}
	stopServiceModeRun(t, cancelRun, errCh)
}

type rejectingWorkerExecutor struct{}

func (rejectingWorkerExecutor) Execute(context.Context, work.WorkDispatch) (workerexecution.WorkResult, error) {
	return workerexecution.WorkResult{}, errors.New("worker executor must not be invoked for workerless cron logical move")
}

func TestFactoryService_LogicalMoveCronTickConsumesTimeWorkWithoutWorkerExecutor(t *testing.T) {
	start := time.Date(2026, time.June, 3, 12, 0, 0, 0, time.UTC)
	fakeClock := clockwork.NewFakeClockAt(start)
	observedSubmissions := make(chan work.FactorySubmissionRecord, 16)

	dir := t.TempDir()
	writeFactoryJSON(t, dir, logicalMoveCronFactoryConfig("* * * * *"))
	writeLogicalMoveCronWorkstationAgentsMD(t, dir, "scheduled-route")
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               dir,
		RuntimeMode:       interfaces.RuntimeModeService,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
		Clock:             fakeClock,
		ExtraOptions: []factory.FactoryOption{
			factory.WithSubmissionRecorder(nonBlockingSubmissionRecorder(observedSubmissions)),
			factory.WithWorkerExecutor("cron-worker", rejectingWorkerExecutor{}),
		},
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	runCtx, cancelRun := context.WithCancel(context.Background())
	defer cancelRun()
	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.Run(runCtx)
	}()
	waitForCronServiceStartup(t, svc)

	ws := configuredCronWorkstationForServiceTest(t, svc, "scheduled-route")
	if ws.Type != interfaces.WorkstationTypeLogical {
		t.Fatalf("workstation type = %q, want %q", ws.Type, interfaces.WorkstationTypeLogical)
	}
	if err := svc.submitCronTick(runCtx, ws, start); err != nil {
		t.Fatalf("submitCronTick: %v", err)
	}

	record := waitForCronSubmission(t, observedSubmissions, time.Second)
	assertCronSubmissionRecord(t, record, "scheduled-route")
	assertLogicalMoveCronDispatchAndOutput(t, svc, record.Request.WorkID, "scheduled-route", "task:init")
	stopServiceModeRun(t, cancelRun, errCh)
}

func logicalMoveCronFactoryConfig(schedule string) map[string]any {
	return map[string]any{
		"name": "factory",
		"workTypes": []map[string]any{
			{
				"name": "task",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workstations": []map[string]any{
			{
				"name":     "scheduled-route",
				"type":     "LOGICAL_MOVE",
				"behavior": "CRON",
				"cron":     map[string]string{"schedule": schedule, "expiryWindow": "500ms"},
				"outputs":  []map[string]string{{"workType": "task", "state": "init"}},
			},
		},
	}
}

func writeLogicalMoveCronWorkstationAgentsMD(t *testing.T, factoryDir, workstationName string) {
	t.Helper()
	wsDir := filepath.Join(factoryDir, "workstations", workstationName)
	if err := os.MkdirAll(wsDir, 0o755); err != nil {
		t.Fatalf("create workstation dir: %v", err)
	}
	agentsMD := "---\ntype: LOGICAL_MOVE\n---\n"
	if err := os.WriteFile(filepath.Join(wsDir, "AGENTS.md"), []byte(agentsMD), 0o644); err != nil {
		t.Fatalf("write workstation AGENTS.md: %v", err)
	}
}

func waitForCronServiceStartup(t *testing.T, svc *FactoryService) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		handle := svc.currentLiveRuntime()
		if handle != nil {
			startCtx, cancel := context.WithTimeout(context.Background(), time.Second)
			err := svc.waitForLiveRuntimeStart(startCtx, handle)
			cancel()
			if err != nil {
				t.Fatalf("wait for cron service startup: %v", err)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for cron service runtime handle")
}

func assertLogicalMoveCronDispatchAndOutput(
	t *testing.T,
	svc *FactoryService,
	workID string,
	workstation string,
	outputPlace string,
) {
	t.Helper()
	dispatch := waitForCompletedDispatchConsumingWorkID(t, svc, workID, time.Second)
	if dispatch.WorkstationName != workstation {
		t.Fatalf("completed dispatch workstation = %q, want %q", dispatch.WorkstationName, workstation)
	}
	matched := consumedTokenWithWorkID(dispatch.ConsumedTokens, workID)
	if matched == nil {
		t.Fatalf("completed cron dispatch did not retain consumed time token %q: %#v", workID, dispatch.ConsumedTokens)
	}
	if matched.Color.WorkTypeID != interfaces.SystemTimeWorkTypeID {
		t.Fatalf("cron token work type = %q, want %q", matched.Color.WorkTypeID, interfaces.SystemTimeWorkTypeID)
	}

	output := waitForTokenInPlaceByParent(t, svc, outputPlace, workID, time.Second)
	if output.Color.WorkTypeID != "task" {
		t.Fatalf("cron logical move output work type = %q, want task", output.Color.WorkTypeID)
	}
}
