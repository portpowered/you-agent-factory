package wire_test

import (
	"context"
	"errors"
	"io/fs"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/portpowered/infinite-you/internal/testutil/factorydefinitionfixtures"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	automations "github.com/portpowered/infinite-you/pkg/services/automations"
	hostedsourceswire "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/hosted_sources/wire"
	scriptpollers "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/script_pollers"
	automationswire "github.com/portpowered/infinite-you/pkg/services/automations/wire"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

type constructionPorts struct {
	logger           *zap.Logger
	clock            clockwork.Clock
	commandRunner    platformprocess.CommandRunner
	hostedPollers    automations.HostedPollers
	resolveTemplates workers.TemplateFieldResolver
	executionPolicy  factorydefinitions.WorkstationExecutionPolicyService
}

type runtimeAutomationService interface {
	automations.Service
	StartSchedulerSidecarsForRuntime(
		context.Context,
		*sync.WaitGroup,
		string,
		*factorydefinitions.FactoryConfig,
		factorydefinitions.RuntimeConfigLookup,
		automations.WorkRequestSubmitter,
	) error
	NewFilesystemWatcher(automations.FilesystemWatcherConfig) automations.FilesystemWatcher
}

func validConstructionPorts(t *testing.T) constructionPorts {
	t.Helper()

	store, err := automationswire.NewHostedLinearCheckpointStore(platformfilesystem.Local{})
	if err != nil {
		t.Fatalf("NewHostedLinearCheckpointStore() error = %v", err)
	}

	return constructionPorts{
		logger:        zap.NewNop(),
		clock:         clockwork.NewFakeClock(),
		commandRunner: stubCommandRunner{},
		hostedPollers: hostedsourceswire.NewHostedPollers(
			zap.NewNop(), clockwork.NewFakeClock(), nil, nil, "", store,
		),
		resolveTemplates: func(
			string,
			map[string]string,
			[]workers.Token,
			*workers.Context,
			string,
		) (*workers.ResolvedTemplateFields, error) {
			return &workers.ResolvedTemplateFields{}, nil
		},
		executionPolicy: factorydefinitionfixtures.WorkstationExecutionPolicy{
			Resolve: func(*factorydefinitions.FactoryWorkstationConfig) (time.Duration, error) {
				return 0, nil
			},
		},
	}
}

func (ports constructionPorts) newService(t *testing.T) runtimeAutomationService {
	t.Helper()

	service, err := automationswire.NewService(
		ports.logger,
		ports.clock,
		ports.commandRunner,
		"automations-wire",
		"",
		ports.hostedPollers,
		ports.resolveTemplates,
		ports.executionPolicy,
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if service == nil {
		t.Fatal("NewService() returned nil service")
	}
	runtimeService, ok := service.(runtimeAutomationService)
	if !ok {
		t.Fatal("NewService() returned a service without runtime automation capabilities")
	}
	return runtimeService
}

type stubCommandRunner struct{}

func (stubCommandRunner) Run(context.Context, platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	return platformprocess.CommandResult{}, nil
}

type recordingCommandRunner struct {
	calls *int
}

func (runner recordingCommandRunner) Run(context.Context, platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	*runner.calls++
	panic("command runner invoked during inert construction")
}

type recordingHostedPollers struct {
	startCalls    *int
	validateCalls *int
}

func (pollers recordingHostedPollers) StartLinearPoller(
	context.Context,
	*sync.WaitGroup,
	factorydefinitions.RuntimeConfigLookup,
	factorydefinitions.FactoryWorkstationConfig,
	*factorydefinitions.FactoryWorkerConfig,
	automations.HostedWorkSubmitter,
) error {
	*pollers.startCalls++
	panic("hosted linear poller started during inert construction")
}

func (pollers recordingHostedPollers) ValidateLinearPoller(
	factorydefinitions.RuntimeConfigLookup,
	factorydefinitions.FactoryWorkstationConfig,
	*factorydefinitions.FactoryWorkerConfig,
	automations.HostedWorkSubmitter,
) error {
	*pollers.validateCalls++
	panic("hosted linear poller validated during inert construction")
}

type rootPeer interface {
	Root() automations.Root
}

type stubFilesystemInputReader struct{}

func (stubFilesystemInputReader) ReadDir(string) ([]fs.DirEntry, error) { return nil, nil }
func (stubFilesystemInputReader) ReadFile(string) ([]byte, error)       { return nil, nil }
func (stubFilesystemInputReader) Stat(string) (fs.FileInfo, error)      { return nil, nil }

type secretRuntimePathsStub struct{}

func (secretRuntimePathsStub) FactoryDir() string     { return "/factory" }
func (secretRuntimePathsStub) RuntimeBaseDir() string { return "/runtime" }

func TestHostedLinearSecretResolverWrapperDelegatesToInjectedEffects(t *testing.T) {
	resolver := automationswire.NewHostedLinearSecretResolver(
		func(string) string { return "env-secret" },
		func(string) ([]byte, error) { return nil, nil },
	)
	got, err := resolver(context.Background(), secretRuntimePathsStub{}, "secrets/api-key")
	if err != nil || got != "env-secret" {
		t.Fatalf("NewHostedLinearSecretResolver() = %q, %v; want env-secret, nil", got, err)
	}
}

// Peer-behavior coverage pairs Service and Root success/failure paths in one test.
func TestNewServiceServesPublishedPeerBehavior(t *testing.T) {
	t.Parallel()

	service := validConstructionPorts(t).newService(t)
	var published automations.Service = service
	if published == nil {
		t.Fatal("constructed service is nil")
	}

	peer, ok := service.(rootPeer)
	if !ok {
		t.Fatal("constructed service does not expose Root() for published peer inspection")
	}
	root := peer.Root()

	reconciled, err := root.Reconcile(context.Background(), automations.ReconcileRequest{
		Desired: []automations.DesiredSpec{{
			AutomationID: "automations-wire",
			SourceID:     "source-a",
			Kind:         "schedule",
			State:        automations.DesiredLifecycleRunning,
		}},
		Observed: []automations.ObservedInstance{{
			AutomationID: "automations-wire",
			SourceID:     "source-a",
			InstanceID:   "instance-a",
			State:        automations.ObservedLifecycleRunning,
		}},
	})
	if err != nil {
		t.Fatalf("Root().Reconcile() = %v", err)
	}
	if len(reconciled.Outcomes) != 1 {
		t.Fatalf("Root().Reconcile() outcomes = %+v, want one converged source", reconciled.Outcomes)
	}
	if reconciled.Outcomes[0].Convergence != automations.ConvergenceStatusConverged {
		t.Fatalf(
			"Root().Reconcile() convergence = %q, want %q",
			reconciled.Outcomes[0].Convergence,
			automations.ConvergenceStatusConverged,
		)
	}

	watcher := service.NewFilesystemWatcher(automations.FilesystemWatcherConfig{
		Dir: t.TempDir(),
		Submitter: func(context.Context, work.WorkRequest) error {
			return nil
		},
		Files:          stubFilesystemInputReader{},
		WalkDirectory:  func(string, fs.WalkDirFunc) error { return nil },
		WorkRequestIDs: func() string { return "wire-peer-test" },
	})
	if watcher == nil {
		t.Fatal("NewFilesystemWatcher returned nil watcher")
	}

	_, err = root.SourceStatus(context.Background(), automations.SourceStatusRequest{
		Identity: automations.SourceIdentity{
			AutomationID: "automations-wire",
			SourceID:     "missing-source",
		},
	})
	var automationsErr *automations.Error
	if !errors.As(err, &automationsErr) || automationsErr.Code != automations.ErrorCodeNotFound {
		t.Fatalf("Root().SourceStatus() = %v, want typed not-found error", err)
	}
	if !errors.Is(err, automations.ErrNotFound) {
		t.Fatalf("Root().SourceStatus() = %v, want ErrNotFound", err)
	}

	_, err = root.Reconcile(context.Background(), automations.ReconcileRequest{
		Desired: []automations.DesiredSpec{{
			SourceID: "source-a",
			Kind:     "schedule",
			State:    automations.DesiredLifecycleRunning,
		}},
	})
	if !errors.As(err, &automationsErr) || automationsErr.Code != automations.ErrorCodeInvalid {
		t.Fatalf("Root().Reconcile(invalid) = %v, want typed invalid error", err)
	}
	if !errors.Is(err, automations.ErrInvalidRequest) {
		t.Fatalf("Root().Reconcile(invalid) = %v, want ErrInvalidRequest", err)
	}
}

func TestNewServiceRejectsMissingRequiredDependencies(t *testing.T) {
	t.Parallel()

	base := validConstructionPorts(t)
	tests := []struct {
		name   string
		mutate func(*constructionPorts)
		want   string
	}{
		{
			name:   "logger",
			mutate: func(ports *constructionPorts) { ports.logger = nil },
			want:   "construct Automations: logger is required",
		},
		{
			name:   "clock",
			mutate: func(ports *constructionPorts) { ports.clock = nil },
			want:   "construct Automations: clock is required",
		},
		{
			name:   "command runner",
			mutate: func(ports *constructionPorts) { ports.commandRunner = nil },
			want:   "construct Automations: command runner is required",
		},
		{
			name:   "hosted pollers",
			mutate: func(ports *constructionPorts) { ports.hostedPollers = nil },
			want:   "construct Automations: hosted pollers are required",
		},
		{
			name:   "template field resolver",
			mutate: func(ports *constructionPorts) { ports.resolveTemplates = nil },
			want:   "construct Automations: template field resolver is required",
		},
		{
			name:   "workstation execution policy",
			mutate: func(ports *constructionPorts) { ports.executionPolicy = nil },
			want:   "construct Automations: workstation execution policy is required",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ports := base
			test.mutate(&ports)

			service, err := automationswire.NewService(
				ports.logger,
				ports.clock,
				ports.commandRunner,
				"automations-wire",
				"",
				ports.hostedPollers,
				ports.resolveTemplates,
				ports.executionPolicy,
			)
			if err == nil {
				t.Fatalf("NewService() error = nil, want missing %s dependency", test.name)
			}
			if err.Error() != test.want {
				t.Fatalf("NewService() error = %q, want %q", err.Error(), test.want)
			}
			if service != nil {
				t.Fatalf("NewService() = %#v, want nil service", service)
			}
		})
	}
}

func TestNewServiceConstructsPublishedRoot(t *testing.T) {
	t.Parallel()

	service := validConstructionPorts(t).newService(t)

	var root automations.Service = service
	if root == nil {
		t.Fatal("constructed service is not assignable to automations.Service")
	}
}

func TestNewRootComposesHostedEffectsAndPublishesRuntimeCapabilities(t *testing.T) {
	t.Parallel()

	ports := validConstructionPorts(t)
	store, err := automationswire.NewHostedLinearCheckpointStore(platformfilesystem.Local{})
	if err != nil {
		t.Fatalf("NewHostedLinearCheckpointStore() error = %v", err)
	}
	root, err := automationswire.NewRoot(
		ports.logger,
		ports.clock,
		ports.commandRunner,
		"automations-root",
		"",
		automationswire.HostedSourceInputs{
			Clock:            ports.clock,
			SecretResolver:   func(context.Context, automations.HostedRuntimePaths, string) (string, error) { return "secret", nil },
			LinearEndpoint:   "",
			CheckpointStore:  store,
			CursorFileSystem: platformfilesystem.Local{},
		},
		ports.resolveTemplates,
		ports.executionPolicy,
	)
	if err != nil {
		t.Fatalf("NewRoot() error = %v", err)
	}
	if root.Operations == nil || root.Lifecycle == nil || root.Runtime == nil {
		t.Fatalf("NewRoot() = %#v, want operations, lifecycle, and runtime capabilities", root)
	}
}

func TestNewRootRequiresCursorPersistenceEffect(t *testing.T) {
	t.Parallel()

	ports := validConstructionPorts(t)
	store, err := automationswire.NewHostedLinearCheckpointStore(platformfilesystem.Local{})
	if err != nil {
		t.Fatalf("NewHostedLinearCheckpointStore() error = %v", err)
	}
	_, err = automationswire.NewRoot(
		ports.logger,
		ports.clock,
		ports.commandRunner,
		"automations-root",
		"",
		automationswire.HostedSourceInputs{
			Clock:           ports.clock,
			SecretResolver:  func(context.Context, automations.HostedRuntimePaths, string) (string, error) { return "secret", nil },
			CheckpointStore: store,
		},
		ports.resolveTemplates,
		ports.executionPolicy,
	)
	if err == nil || err.Error() != "construct Automations: script poller cursor filesystem is required" {
		t.Fatalf("NewRoot() error = %v, want missing cursor persistence effect", err)
	}
}

func TestNewRootUsesDurableCursorRecorderAcrossReconstruction(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	ports := validConstructionPorts(t)
	store, err := automationswire.NewHostedLinearCheckpointStore(platformfilesystem.Local{})
	if err != nil {
		t.Fatalf("NewHostedLinearCheckpointStore() error = %v", err)
	}
	inputs := automationswire.HostedSourceInputs{
		Clock:            ports.clock,
		SecretResolver:   func(context.Context, automations.HostedRuntimePaths, string) (string, error) { return "secret", nil },
		CheckpointStore:  store,
		CursorFileSystem: platformfilesystem.Local{},
	}
	first, err := automationswire.NewRoot(
		ports.logger,
		ports.clock,
		cursorCommandRunner{stdout: durableCursorStdout},
		"workflow-durable-root",
		baseDir,
		inputs,
		ports.resolveTemplates,
		ports.executionPolicy,
	)
	if err != nil {
		t.Fatalf("NewRoot(first): %v", err)
	}
	operation, ok := first.Operations.(interface {
		RunScriptPoller(
			context.Context,
			platformprocess.CommandRunner,
			factorydefinitions.RuntimeConfigLookup,
			factorydefinitions.FactoryWorkstationConfig,
			*factorydefinitions.FactoryWorkerConfig,
			automations.WorkRequestSubmitter,
		) error
	})
	if !ok {
		t.Fatal("NewRoot(first) operations do not expose script-poller execution")
	}
	poller := factorydefinitions.FactoryWorkstationConfig{
		Name:           "durable-poller",
		Kind:           factorydefinitions.WorkstationKindPoller,
		WorkerTypeName: "durable-script",
	}
	worker := &factorydefinitions.FactoryWorkerConfig{
		Name:    "durable-script",
		Type:    factorydefinitions.WorkerTypeScript,
		Command: "poller.sh",
	}
	if err := operation.RunScriptPoller(
		context.Background(),
		cursorCommandRunner{stdout: durableCursorStdout},
		cursorRuntimeConfig{factoryDir: baseDir, worker: worker, workstation: poller},
		poller,
		worker,
		func(context.Context, work.WorkRequest) error { return nil },
	); err == nil {
		t.Fatal("RunScriptPoller() error = nil, want terminal poller exit after commit")
	}

	second, err := automationswire.NewRoot(
		ports.logger,
		ports.clock,
		cursorCommandRunner{stdout: durableCursorStdout},
		"workflow-durable-root",
		baseDir,
		inputs,
		ports.resolveTemplates,
		ports.executionPolicy,
	)
	if err != nil {
		t.Fatalf("NewRoot(second): %v", err)
	}
	instanceID := scriptpollers.SupervisionFor("workflow-durable-root", poller.Name).InstanceID
	got, err := second.GetCursor(context.Background(), automations.GetCursorRequest{InstanceID: instanceID})
	if err != nil {
		t.Fatalf("second.GetCursor(): %v", err)
	}
	if got.AutomationID != "workflow-durable-root" || got.InstanceID != instanceID ||
		got.Cursor != "durable-cursor" || got.Checkpoint != "durable-checkpoint" {
		t.Fatalf("second.GetCursor() = %+v, want exact recovered cursor facts", got)
	}
}

func TestNewRootRoutesActivatedRuntimeCursorToFactoryLocalRecorder(t *testing.T) {
	t.Parallel()

	const (
		workflowID = "workflow-runtime-local-cursor"
		runtimeID  = "runtime-runtime-local-cursor"
	)
	factoryDir := t.TempDir()
	runner := &runtimeCursorCommandRunner{
		stdout:  []byte(`{"requestId":"runtime-local-cursor","type":"FACTORY_REQUEST_BATCH","works":[{"name":"runtime-local-work","workTypeName":"task"}],"cursor":"runtime-local-cursor","checkpoint":"runtime-local-checkpoint"}`),
		started: make(chan struct{}),
	}
	ports := validConstructionPorts(t)
	composition := newRuntimeCursorComposition(t, factoryDir, workflowID, runtimeID, ports.clock)
	first := newRuntimeCursorRoot(t, ports, runner, workflowID, composition.inputs)
	if _, err := first.ActivateRuntime(context.Background(), composition.request); err != nil {
		t.Fatalf("first.ActivateRuntime(): %v", err)
	}
	if err := first.StartRuntime(context.Background(), runtimeID); err != nil {
		t.Fatalf("first.StartRuntime(): %v", err)
	}
	select {
	case <-runner.started:
	case <-time.After(2 * time.Second):
		t.Fatal("script poller did not start")
	}
	supervision := scriptpollers.SupervisionFor(workflowID, composition.poller.Name)
	got := waitForRuntimeCursor(t, first, supervision.InstanceID)
	assertRuntimeCursor(t, got, workflowID, supervision.InstanceID, "first.GetCursor()")
	if _, err := first.DeactivateRuntime(context.Background(), automations.RuntimeDeactivationRequest{RuntimeID: runtimeID}); err != nil {
		t.Fatalf("first.DeactivateRuntime(): %v", err)
	}

	second := newRuntimeCursorRoot(t, ports, stubCommandRunner{}, workflowID, composition.inputs)
	if _, err := second.ActivateRuntime(context.Background(), composition.request); err != nil {
		t.Fatalf("second.ActivateRuntime(): %v", err)
	}
	defer func() {
		_, _ = second.DeactivateRuntime(context.Background(), automations.RuntimeDeactivationRequest{RuntimeID: runtimeID})
	}()
	recovered := waitForRuntimeCursor(t, second, supervision.InstanceID)
	assertRuntimeCursor(t, recovered, workflowID, supervision.InstanceID, "second.GetCursor()")
}

type runtimeCursorComposition struct {
	inputs  automationswire.HostedSourceInputs
	request automations.RuntimeActivationRequest
	poller  factorydefinitions.FactoryWorkstationConfig
}

func newRuntimeCursorComposition(
	t *testing.T,
	factoryDir, workflowID, runtimeID string,
	clock clockwork.Clock,
) runtimeCursorComposition {
	t.Helper()
	store, err := automationswire.NewHostedLinearCheckpointStore(platformfilesystem.Local{})
	if err != nil {
		t.Fatalf("NewHostedLinearCheckpointStore() error = %v", err)
	}
	inputs := automationswire.HostedSourceInputs{
		Clock:            clock,
		SecretResolver:   func(context.Context, automations.HostedRuntimePaths, string) (string, error) { return "secret", nil },
		CheckpointStore:  store,
		CursorFileSystem: platformfilesystem.Local{},
	}
	poller := factorydefinitions.FactoryWorkstationConfig{
		Name:           "runtime-local-poller",
		Kind:           factorydefinitions.WorkstationKindPoller,
		WorkerTypeName: "runtime-local-script",
	}
	worker := factorydefinitions.FactoryWorkerConfig{
		Name:    "runtime-local-script",
		Type:    factorydefinitions.WorkerTypeScript,
		Command: "poller.sh",
	}
	return runtimeCursorComposition{
		inputs: inputs,
		poller: poller,
		request: automations.RuntimeActivationRequest{
			RuntimeID:        runtimeID,
			FactorySessionID: "session-runtime-local-cursor",
			Snapshot: factorydefinitions.RuntimeSnapshot{
				FactoryDir:     factoryDir,
				RuntimeBaseDir: factoryDir,
				Invocation: factorydefinitions.RuntimeSnapshotInvocationContext{
					FactorySessionID: "session-runtime-local-cursor",
					WorkflowID:       workflowID,
				},
				EffectiveFactory: factorydefinitions.FactoryConfig{
					Name:         "runtime-local-factory",
					Workers:      []factorydefinitions.FactoryWorkerConfig{worker},
					Workstations: []factorydefinitions.FactoryWorkstationConfig{poller},
				},
			},
			Inputs: automations.RuntimeActivationInputs{
				StartSchedulers: true,
				Submitter:       func(context.Context, work.WorkRequest) error { return nil },
			},
		},
	}
}

func newRuntimeCursorRoot(
	t *testing.T,
	ports constructionPorts,
	runner platformprocess.CommandRunner,
	workflowID string,
	inputs automationswire.HostedSourceInputs,
) automations.Root {
	t.Helper()
	root, err := automationswire.NewRoot(
		ports.logger,
		ports.clock,
		runner,
		workflowID,
		"",
		inputs,
		ports.resolveTemplates,
		ports.executionPolicy,
	)
	if err != nil {
		t.Fatalf("NewRoot(): %v", err)
	}
	return root
}

func assertRuntimeCursor(
	t *testing.T,
	got automations.GetCursorResult,
	workflowID, instanceID, label string,
) {
	t.Helper()
	if got.AutomationID != workflowID || got.InstanceID != instanceID ||
		got.Cursor != "runtime-local-cursor" || got.Checkpoint != "runtime-local-checkpoint" {
		t.Fatalf("%s = %+v, want exact factory-local runtime facts", label, got)
	}
}

func waitForRuntimeCursor(
	t *testing.T,
	root automations.Root,
	instanceID string,
) automations.GetCursorResult {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		cursor, err := root.GetCursor(context.Background(), automations.GetCursorRequest{InstanceID: instanceID})
		if err == nil {
			return cursor
		}
		select {
		case <-deadline:
			t.Fatalf("GetCursor() did not recover within deadline: %v", err)
		case <-time.After(time.Millisecond):
		}
	}
}

type runtimeCursorCommandRunner struct {
	stdout  []byte
	started chan struct{}
	once    sync.Once
}

func (runner *runtimeCursorCommandRunner) Run(ctx context.Context, _ platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	runner.once.Do(func() { close(runner.started) })
	select {
	case <-ctx.Done():
		return platformprocess.CommandResult{}, ctx.Err()
	default:
		return platformprocess.CommandResult{Stdout: runner.stdout}, nil
	}
}

const durableCursorStdout = `{"requestId":"durable-request","type":"FACTORY_REQUEST_BATCH","works":[{"name":"durable-work","workTypeName":"task"}],"cursor":"durable-cursor","checkpoint":"durable-checkpoint"}`

type cursorCommandRunner struct {
	stdout string
}

func (r cursorCommandRunner) Run(context.Context, platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	return platformprocess.CommandResult{Stdout: []byte(r.stdout)}, nil
}

type cursorRuntimeConfig struct {
	factoryDir  string
	worker      *factorydefinitions.FactoryWorkerConfig
	workstation factorydefinitions.FactoryWorkstationConfig
}

func (c cursorRuntimeConfig) FactoryDir() string { return c.factoryDir }

func (c cursorRuntimeConfig) RuntimeBaseDir() string { return c.factoryDir }

func (c cursorRuntimeConfig) FactoryConfig() *factorydefinitions.FactoryConfig {
	return &factorydefinitions.FactoryConfig{}
}

func (c cursorRuntimeConfig) Worker(name string) (*factorydefinitions.FactoryWorkerConfig, bool) {
	if c.worker != nil && c.worker.Name == name {
		return c.worker, true
	}
	return nil, false
}

func (c cursorRuntimeConfig) Workstation(name string) (*factorydefinitions.FactoryWorkstationConfig, bool) {
	if c.workstation.Name == name {
		workstation := c.workstation
		return &workstation, true
	}
	return nil, false
}

func TestNewServiceConstructsInertRoot(t *testing.T) {
	t.Parallel()

	calls := &inertConstructionCalls{}
	ports := inertConstructionPorts(t, calls)

	runtime.GC()
	time.Sleep(20 * time.Millisecond)
	baseline := runtime.NumGoroutine()

	service, err := automationswire.NewService(
		ports.logger, ports.clock, ports.commandRunner, "automations-wire-inert", "",
		ports.hostedPollers,
		ports.resolveTemplates, ports.executionPolicy,
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if service == nil {
		t.Fatal("NewService() returned nil service")
	}
	assertInertConstructionCalls(t, calls)

	runtime.GC()
	time.Sleep(20 * time.Millisecond)
	if leaked := runtime.NumGoroutine() - baseline; leaked > 4 {
		t.Fatalf(
			"goroutine leak after construction: baseline=%d current=%d delta=%d",
			baseline,
			runtime.NumGoroutine(),
			leaked,
		)
	}

	assertInertPublishedRoot(t, service)
}

type inertConstructionCalls struct {
	commandRunner        int
	templateResolver     int
	startLinearPoller    int
	validateLinearPoller int
}

func inertConstructionPorts(t *testing.T, calls *inertConstructionCalls) constructionPorts {
	t.Helper()

	ports := validConstructionPorts(t)
	ports.commandRunner = recordingCommandRunner{calls: &calls.commandRunner}
	ports.hostedPollers = recordingHostedPollers{
		startCalls:    &calls.startLinearPoller,
		validateCalls: &calls.validateLinearPoller,
	}
	ports.resolveTemplates = func(
		string,
		map[string]string,
		[]workers.Token,
		*workers.Context,
		string,
	) (*workers.ResolvedTemplateFields, error) {
		calls.templateResolver++
		panic("template resolver invoked during inert construction")
	}
	return ports
}

func assertInertConstructionCalls(t *testing.T, calls *inertConstructionCalls) {
	t.Helper()

	if calls.commandRunner != 0 || calls.templateResolver != 0 {
		t.Fatalf(
			"construction invoked effect stubs (command runner=%d template resolver=%d), want inert construction",
			calls.commandRunner, calls.templateResolver,
		)
	}
	if calls.startLinearPoller != 0 || calls.validateLinearPoller != 0 {
		t.Fatalf(
			"construction invoked hosted poller lifecycle (start=%d validate=%d), want inert construction",
			calls.startLinearPoller, calls.validateLinearPoller,
		)
	}
}

func assertInertPublishedRoot(t *testing.T, service automations.Service) {
	t.Helper()

	peer, ok := service.(rootPeer)
	if !ok {
		t.Fatal("constructed service does not expose Root() for published peer inspection")
	}
	_, err := peer.Root().SourceStatus(context.Background(), automations.SourceStatusRequest{
		Identity: automations.SourceIdentity{
			AutomationID: "automations-wire-inert",
			SourceID:     "missing-source",
		},
	})
	var automationsErr *automations.Error
	if !errors.As(err, &automationsErr) || automationsErr.Code != automations.ErrorCodeNotFound {
		t.Fatalf("SourceStatus() = %v, want typed not-found lifecycle state after inert construction", err)
	}
	if !errors.Is(err, automations.ErrNotFound) {
		t.Fatalf("SourceStatus() = %v, want ErrNotFound after inert construction", err)
	}
}
