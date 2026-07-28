package wire_test

import (
	"context"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/portpowered/infinite-you/internal/testutil/factorydefinitionfixtures"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	automations "github.com/portpowered/infinite-you/pkg/services/automations"
	automationswire "github.com/portpowered/infinite-you/pkg/services/automations/wire"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

type constructionPorts struct {
	logger           *zap.Logger
	clock            clockwork.Clock
	commandRunner    workers.CommandRunner
	hostedSources    automations.HostedSourcesFactory
	hostedClock      workers.HostedPollerClock
	resolveTemplates workers.TemplateFieldResolver
	executionPolicy  factorydefinitions.WorkstationExecutionPolicyService
}

func validConstructionPorts(t *testing.T) constructionPorts {
	t.Helper()

	store, err := automations.NewHostedLinearCheckpointStore(platformfilesystem.Local{})
	if err != nil {
		t.Fatalf("NewHostedLinearCheckpointStore() error = %v", err)
	}

	return constructionPorts{
		logger:        zap.NewNop(),
		clock:         clockwork.NewFakeClock(),
		commandRunner: stubCommandRunner{},
		hostedSources: automations.NewHostedSourcesFactory(store),
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

func (ports constructionPorts) newService(t *testing.T) automations.Service {
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
	return service
}

type stubCommandRunner struct{}

func (stubCommandRunner) Run(context.Context, workers.CommandRequest) (workers.CommandResult, error) {
	return workers.CommandResult{}, nil
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
