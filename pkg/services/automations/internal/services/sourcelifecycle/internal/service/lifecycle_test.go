package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	automations "github.com/portpowered/infinite-you/pkg/services/automations"
	cron "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/cron"
	cursorscopes "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/cursorscopes"
	scriptpollers "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/script_pollers"
	sourcelifecycle "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/sourcelifecycle"
	definitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

type scriptAttempt struct {
	ctx          context.Context
	scope        scriptpollers.CursorScope
	automationID string
	joined       chan struct{}
}

type scriptFixture struct {
	scriptpollers.Service
	attempts chan scriptAttempt
}

func (f scriptFixture) StartScriptPoller(ctx context.Context, children *sync.WaitGroup, _ definitions.RuntimeConfigLookup,
	_ definitions.FactoryWorkstationConfig, _ *definitions.FactoryWorkerConfig,
	supervision scriptpollers.ScriptPollerSupervision, _ automations.WorkRequestSubmitter,
) {
	joined := make(chan struct{})
	children.Add(1)
	go func() { defer children.Done(); <-ctx.Done(); close(joined) }()
	f.attempts <- scriptAttempt{ctx: ctx, scope: supervision.CursorScope, automationID: supervision.AutomationID, joined: joined}
}

type cursorFixture struct {
	cursorscopes.CursorScopes
	releases chan cursorscopes.CursorScope
}

func (f cursorFixture) ReleaseScope(scope cursorscopes.CursorScope) { f.releases <- scope }

func TestLifecycleConfigureIsInertAndReleaseJoinsOnlyOwnedRuntime(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	attempts := make(chan scriptAttempt, 3)
	releases := make(chan cursorscopes.CursorScope, 3)
	lifecycle := New(zap.NewNop(), clockwork.NewFakeClock(), scriptFixture{attempts: attempts}, nil, nil, nil, cursorFixture{releases: releases}, nil)
	identity := automations.SourceIdentity{AutomationID: "shared", SourceID: "scheduler-sidecars"}
	for _, id := range []string{"A", "B"} {
		if err := lifecycle.ConfigureRuntimeSource(ctx, scriptConfiguration(id)); err != nil {
			t.Fatal(err)
		}
	}
	if len(attempts) != 0 || len(releases) != 0 {
		t.Fatal("construction/configuration executed source effects")
	}
	for _, id := range []string{"A", "B"} {
		startScript(t, ctx, lifecycle, id, identity)
	}
	a, b := <-attempts, <-attempts
	assertScriptAttemptIdentities(t, a, b)
	releaseAndAssertJoined(t, ctx, lifecycle, "A", a)
	if b.ctx.Err() != nil {
		t.Fatal("A release cancelled B")
	}
	if scope := <-releases; scope != a.scope {
		t.Fatalf("released scope = %+v, want %+v", scope, a.scope)
	}
	if err := lifecycle.ConfigureRuntimeSource(ctx, scriptConfiguration("A")); err != nil {
		t.Fatal(err)
	}
	startScript(t, ctx, lifecycle, "A", identity)
	restarted := <-attempts
	if restarted.scope != a.scope || restarted.ctx.Err() != nil {
		t.Fatal("A did not restart in its original scope")
	}
	releaseAndAssertJoined(t, ctx, lifecycle, "B", b)
	if restarted.ctx.Err() != nil {
		t.Fatal("B release cancelled restarted A")
	}
	releaseAndAssertJoined(t, ctx, lifecycle, "A", restarted)

}

func scriptConfiguration(id string) sourcelifecycle.RuntimeSourceConfiguration {
	snapshot := definitions.RuntimeSnapshot{FactoryDir: "factory", RuntimeBaseDir: "factory", EffectiveFactory: definitions.FactoryConfig{
		Workers:      []definitions.FactoryWorkerConfig{{Name: "script", Type: definitions.WorkerTypeScript}},
		Workstations: []definitions.FactoryWorkstationConfig{{Name: "poll", Kind: definitions.WorkstationKindPoller, WorkerTypeName: "script"}},
	}}
	snapshot.Invocation.WorkflowID = "shared"
	return sourcelifecycle.RuntimeSourceConfiguration{RuntimeID: id, CursorBaseDir: "selected-cursor-base", WorkflowID: "selected-script-id", Snapshot: snapshot,
		Inputs: automations.RuntimeActivationInputs{StartSchedulers: true, Submitter: func(context.Context, work.WorkRequest) error { return nil }}}
}

type timeoutPolicy struct {
	definitions.WorkstationExecutionPolicyService
	err error
}

func (p timeoutPolicy) ExecutionTimeout(*definitions.FactoryWorkstationConfig) (time.Duration, error) {
	return time.Millisecond, p.err
}

type deadlineCron struct {
	cron.Service
	attempts *int
}

func (f deadlineCron) SubmitCronTick(ctx context.Context, _ cron.WorkRequestSubmitter, _ string, _ definitions.FactoryWorkstationConfig, _ time.Time) (cron.CronTickSubmission, error) {
	*f.attempts++
	<-ctx.Done()
	return cron.CronTickSubmission{}, ctx.Err()
}

func TestLifecycleCronUsesInjectedExecutionPolicyAndPreservesDeadlineError(t *testing.T) {
	t.Parallel()
	config := &runtimeSnapshotConfig{config: definitions.FactoryConfig{Workstations: []definitions.FactoryWorkstationConfig{{Name: "cron"}}}}
	ws := definitions.FactoryWorkstationConfig{Name: "cron"}
	attempts := 0
	owner := &service{executionPolicy: timeoutPolicy{}, cron: deadlineCron{attempts: &attempts}, logger: zap.NewNop()}
	err := owner.submitCronTickAttempt(context.Background(), config, "shared", func(context.Context, work.WorkRequest) error { return nil }, ws, time.Now())
	if !errors.Is(err, context.DeadlineExceeded) || attempts != 1 {
		t.Fatalf("attempts=%d error=%v, want one bounded deadline failure", attempts, err)
	}
	sentinel := errors.New("invalid canonical limit")
	owner.executionPolicy = timeoutPolicy{err: sentinel}

	// Use an admission collaborator so the canonical policy owns validation.
	err = owner.submitCronTickAttempt(context.Background(), config, "shared", func(context.Context, work.WorkRequest) error { return nil }, ws, time.Now())
	if !errors.Is(err, sentinel) || attempts != 1 {
		t.Fatalf("attempts=%d error=%v, want exact policy failure without admission", attempts, err)
	}
}

func startScript(t *testing.T, ctx context.Context, lifecycle sourcelifecycle.SourceLifecycle, id string, identity automations.SourceIdentity) {
	t.Helper()
	if err := lifecycle.Start(ctx, sourcelifecycle.StartEffect{RuntimeID: id, Kind: schedulerSourceKind, Observation: automations.SourceObservation{Identity: identity}}); err != nil {
		t.Fatal(err)
	}
}
func releaseAndAssertJoined(t *testing.T, ctx context.Context, lifecycle sourcelifecycle.SourceLifecycle, id string, attempt scriptAttempt) {
	t.Helper()
	if err := lifecycle.ReleaseRuntimeSource(ctx, id); err != nil {
		t.Fatal(err)
	}
	select {
	case <-attempt.joined:
	default:
		t.Fatalf("release returned before %s joined", id)
	}
}

func TestLifecyclePreservesBlankScriptIdentity(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	attempts := make(chan scriptAttempt, 1)
	lifecycle := New(zap.NewNop(), clockwork.NewFakeClock(), scriptFixture{attempts: attempts}, nil, nil, nil, cursorFixture{releases: make(chan cursorscopes.CursorScope, 1)}, nil)
	configuration := scriptConfiguration("A")
	configuration.WorkflowID = ""
	if err := lifecycle.ConfigureRuntimeSource(ctx, configuration); err != nil {
		t.Fatal(err)
	}
	startScript(t, ctx, lifecycle, "A", automations.SourceIdentity{AutomationID: "shared", SourceID: "scheduler-sidecars"})
	attempt := <-attempts
	if attempt.automationID != "" {
		t.Fatal("blank script identity gained a scheduler-identity fallback")
	}
	releaseAndAssertJoined(t, ctx, lifecycle, "A", attempt)
}

type failedHostedStart struct {
	joined chan struct{}
	err    error
}

func (f failedHostedStart) ValidateLinearPoller(definitions.RuntimeConfigLookup, definitions.FactoryWorkstationConfig, *definitions.FactoryWorkerConfig, automations.HostedWorkSubmitter) error {
	return nil
}

func (f failedHostedStart) StartLinearPoller(ctx context.Context, children *sync.WaitGroup, _ definitions.RuntimeConfigLookup,
	_ definitions.FactoryWorkstationConfig, _ *definitions.FactoryWorkerConfig, _ automations.HostedWorkSubmitter,
) error {
	children.Add(1)
	go func() { defer children.Done(); <-ctx.Done(); close(f.joined) }()
	return f.err
}

func TestLifecycleFailedStartJoinsAcquiredChildrenAndPreservesCause(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	sentinel := errors.New("controlled hosted launch failure")
	joined := make(chan struct{})
	lifecycle := New(zap.NewNop(), clockwork.NewFakeClock(), nil, nil, nil, failedHostedStart{joined: joined, err: sentinel}, cursorFixture{releases: make(chan cursorscopes.CursorScope, 1)}, nil)
	configuration := scriptConfiguration("A")
	configuration.Snapshot.EffectiveFactory.Workers[0].Type = definitions.WorkerTypeHosted
	configuration.Snapshot.EffectiveFactory.Workers[0].Provider = definitions.HostedWorkerProviderLinear
	if err := lifecycle.ConfigureRuntimeSource(ctx, configuration); err != nil {
		t.Fatal(err)
	}
	err := lifecycle.Start(ctx, sourcelifecycle.StartEffect{RuntimeID: "A", Kind: schedulerSourceKind, Observation: automations.SourceObservation{
		Identity: automations.SourceIdentity{AutomationID: "shared", SourceID: "scheduler-sidecars"},
	}})
	if !errors.Is(err, sentinel) {
		t.Fatalf("start error = %v, want exact launch cause", err)
	}
	select {
	case <-joined:
	default:
		t.Fatal("failed launch returned before its acquired child joined")
	}
	if err := lifecycle.ReleaseRuntimeSource(ctx, "A"); err != nil {
		t.Fatal(err)
	}
}

func assertScriptAttemptIdentities(t *testing.T, a, b scriptAttempt) {
	t.Helper()
	for id, attempt := range map[string]scriptAttempt{"A": a, "B": b} {
		expected := scriptpollers.CursorScope{RuntimeID: id, BaseDir: "selected-cursor-base"}
		if attempt.automationID != "selected-script-id" || attempt.scope != expected {
			t.Fatalf("%s source identity=%q scope=%+v, want selected script identity and %+v", id, attempt.automationID, attempt.scope, expected)
		}
	}
}

func TestStartPollersForRuntime_LogsDisabledPaths(t *testing.T) {
	logCore, observedLogs := observer.New(zap.WarnLevel)
	svc := &service{logger: zap.New(logCore)}

	missingBinding := interfaces.FactoryWorkstationConfig{
		Name: "missing-binding",
		Kind: interfaces.WorkstationKindPoller,
	}
	missingWorker := interfaces.FactoryWorkstationConfig{
		Name:           "missing-worker",
		Kind:           interfaces.WorkstationKindPoller,
		WorkerTypeName: "unknown-worker",
	}
	unsupportedHosted := interfaces.FactoryWorkstationConfig{
		Name:           "unsupported-hosted",
		Kind:           interfaces.WorkstationKindPoller,
		WorkerTypeName: "github-poller",
	}
	githubWorker := &interfaces.FactoryWorkerConfig{
		Name:     "github-poller",
		Type:     interfaces.WorkerTypeHosted,
		Provider: "github",
	}

	factoryCfg := &interfaces.FactoryConfig{
		Workers: []interfaces.FactoryWorkerConfig{{Name: githubWorker.Name}},
		Workstations: []interfaces.FactoryWorkstationConfig{
			missingBinding,
			missingWorker,
			unsupportedHosted,
		},
	}
	runtimeCfg := runtimefixtures.RuntimeConfigLookupFixture{Workers: map[string]*interfaces.FactoryWorkerConfig{githubWorker.Name: githubWorker}}

	var sidecars sync.WaitGroup
	svc.startPollersForRuntime(sourcelifecycle.RuntimeSourceConfiguration{},
		context.Background(),
		&sidecars,
		factoryCfg,
		runtimeCfg,
		func(context.Context, work.WorkRequest) error { return nil },
	)

	if observedLogs.FilterMessage("script poller disabled").Len() != 2 {
		t.Fatalf("script poller disabled logs = %d, want 2", observedLogs.FilterMessage("script poller disabled").Len())
	}
	if observedLogs.FilterMessage("hosted poller disabled").Len() != 1 {
		t.Fatalf("hosted poller disabled logs = %d, want 1", observedLogs.FilterMessage("hosted poller disabled").Len())
	}
}
