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
	automations "github.com/portpowered/infinite-you/pkg/services/automations"
	automationswire "github.com/portpowered/infinite-you/pkg/services/automations/wire"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

type constructionPorts struct {
	logger           *zap.Logger
	clock            clockwork.Clock
	commandRunner    workers.CommandRunner
	hostedSources    automations.HostedSourcesFactory
	hostedClock      automations.HostedLinearClock
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
		hostedSources: automationswire.NewHostedSourcesFactory(store),
		hostedClock:   clockwork.NewFakeClock(),
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
		ports.hostedSources,
		nil,
		ports.hostedClock,
		nil,
		nil,
		"",
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

func (stubCommandRunner) Run(context.Context, workers.CommandRequest) (workers.CommandResult, error) {
	return workers.CommandResult{}, nil
}

type recordingCommandRunner struct {
	calls *int
}

func (runner recordingCommandRunner) Run(context.Context, workers.CommandRequest) (workers.CommandResult, error) {
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
			name:   "hosted-sources factory",
			mutate: func(ports *constructionPorts) { ports.hostedSources = nil },
			want:   "construct Automations: hosted-sources factory is required",
		},
		{
			name:   "hosted poller clock",
			mutate: func(ports *constructionPorts) { ports.hostedClock = nil },
			want:   "construct Automations: hosted poller clock is required",
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
				ports.hostedSources,
				nil,
				ports.hostedClock,
				nil,
				nil,
				"",
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

func TestNewServiceConstructsInertRoot(t *testing.T) {
	t.Parallel()

	calls := &inertConstructionCalls{}
	ports := inertConstructionPorts(t, calls)

	runtime.GC()
	time.Sleep(20 * time.Millisecond)
	baseline := runtime.NumGoroutine()

	service, err := automationswire.NewService(
		ports.logger, ports.clock, ports.commandRunner, "automations-wire-inert", "",
		ports.hostedSources, nil, ports.hostedClock, nil, nil, "",
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
	hostedFactory        int
	startLinearPoller    int
	validateLinearPoller int
}

func inertConstructionPorts(t *testing.T, calls *inertConstructionCalls) constructionPorts {
	t.Helper()

	ports := validConstructionPorts(t)
	ports.commandRunner = recordingCommandRunner{calls: &calls.commandRunner}
	ports.hostedSources = func(
		*zap.Logger,
		automations.HostedLinearClock,
		automations.HostedLinearHTTPDoer,
		automations.HostedLinearSecretResolver,
		string,
	) automations.HostedPollers {
		calls.hostedFactory++
		return recordingHostedPollers{
			startCalls:    &calls.startLinearPoller,
			validateCalls: &calls.validateLinearPoller,
		}
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
	if calls.hostedFactory != 1 {
		t.Fatalf("hosted-sources factory calls = %d, want exactly one composition call", calls.hostedFactory)
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
