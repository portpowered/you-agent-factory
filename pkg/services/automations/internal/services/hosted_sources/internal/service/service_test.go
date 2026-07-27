package service

import (
	"context"
	"net/http"
	"sync"
	"testing"

	"github.com/jonboulle/clockwork"
	factorydefinitioncomposition "github.com/portpowered/infinite-you/internal/testutil/factorydefinitionfixtures"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	hostedlinear "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/hosted_sources/internal/linear"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

func TestNewConstructsHostedSourcesServiceWithDefaults(t *testing.T) {
	t.Parallel()

	checkpoints, err := hostedlinear.NewCheckpointStore(platformfilesystem.Local{})
	if err != nil {
		t.Fatalf("NewCheckpointStore() error = %v", err)
	}

	service := New(
		nil,
		clockwork.NewFakeClock(),
		&http.Client{Timeout: hostedlinear.DefaultRequestTimeout},
		hostedlinear.NewSecretResolver(func(string) string { return "" }, nil),
		"",
		checkpoints,
	)
	if service == nil {
		t.Fatal("New() returned nil")
	}
	if service.logger == nil {
		t.Fatal("New() did not apply default logger")
	}
	if service.linearEndpoint != hostedlinear.DefaultEndpoint {
		t.Fatalf("linearEndpoint = %q, want %q", service.linearEndpoint, hostedlinear.DefaultEndpoint)
	}
	if service.checkpoints != checkpoints {
		t.Fatal("New() did not retain injected checkpoint store")
	}
}

func validLinearPollerFixture(t *testing.T) (
	interfaces.RuntimeConfigLookup,
	interfaces.FactoryWorkstationConfig,
	*interfaces.FactoryWorkerConfig,
	hostedlinear.CheckpointStore,
) {
	t.Helper()

	checkpoints, err := hostedlinear.NewCheckpointStore(platformfilesystem.Local{})
	if err != nil {
		t.Fatalf("NewCheckpointStore() error = %v", err)
	}
	runtimeCfg, err := factorydefinitioncomposition.NewLoadedSource(
		t.TempDir(),
		&interfaces.FactoryConfig{},
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("NewLoadedFactoryConfig: %v", err)
	}
	worker := &interfaces.FactoryWorkerConfig{
		Name:     "linear-poller",
		Type:     interfaces.WorkerTypeHosted,
		Provider: interfaces.HostedWorkerProviderLinear,
		Auth:     &interfaces.HostedWorkerAuthConfig{SecretRef: "secrets/linear-api-key"},
		Linear: &interfaces.HostedLinearWorkerConfig{
			PollInterval: "1h",
			Mapping: interfaces.HostedLinearWorkerMappingConfig{
				WorkType: "story",
				State:    "init",
			},
		},
	}
	workstation := interfaces.FactoryWorkstationConfig{
		Name:           "linear-ingress",
		Kind:           interfaces.WorkstationKindPoller,
		WorkerTypeName: worker.Name,
	}
	return runtimeCfg, workstation, worker, checkpoints
}

func TestNewLinearPollerRejectsMissingDependencies(t *testing.T) {
	t.Parallel()

	runtimeCfg, workstation, worker, checkpoints := validLinearPollerFixture(t)
	httpClient := &http.Client{Timeout: hostedlinear.DefaultRequestTimeout}
	secretResolver := hostedlinear.NewSecretResolver(func(string) string { return "" }, nil)
	clock := clockwork.NewFakeClock()
	submitter := Submitter(func(context.Context, work.WorkRequest) error { return nil })

	dependencyCases := []struct {
		name string
		run  func() error
	}{
		{
			name: "clock",
			run: func() error {
				_, err := NewLinearPoller(nil, nil, httpClient, secretResolver, checkpoints, "", runtimeCfg, workstation, worker, submitter)
				return err
			},
		},
		{
			name: "http client",
			run: func() error {
				_, err := NewLinearPoller(nil, clock, nil, secretResolver, checkpoints, "", runtimeCfg, workstation, worker, submitter)
				return err
			},
		},
		{
			name: "secret resolver",
			run: func() error {
				_, err := NewLinearPoller(nil, clock, httpClient, nil, checkpoints, "", runtimeCfg, workstation, worker, submitter)
				return err
			},
		},
		{
			name: "checkpoint store",
			run: func() error {
				_, err := NewLinearPoller(nil, clock, httpClient, secretResolver, nil, "", runtimeCfg, workstation, worker, submitter)
				return err
			},
		},
		{
			name: "runtime config",
			run: func() error {
				_, err := NewLinearPoller(nil, clock, httpClient, secretResolver, checkpoints, "", nil, workstation, worker, submitter)
				return err
			},
		},
		{
			name: "worker",
			run: func() error {
				_, err := NewLinearPoller(nil, clock, httpClient, secretResolver, checkpoints, "", runtimeCfg, workstation, nil, submitter)
				return err
			},
		},
		{
			name: "submitter",
			run: func() error {
				_, err := NewLinearPoller(nil, clock, httpClient, secretResolver, checkpoints, "", runtimeCfg, workstation, worker, nil)
				return err
			},
		},
	}

	for _, tc := range dependencyCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := tc.run(); err == nil {
				t.Fatalf("NewLinearPoller() accepted missing %s dependency", tc.name)
			}
		})
	}
}

func TestServiceValidateLinearPollerDelegates(t *testing.T) {
	t.Parallel()

	runtimeCfg, workstation, worker, checkpoints := validLinearPollerFixture(t)
	service := New(
		nil,
		clockwork.NewFakeClock(),
		&http.Client{Timeout: hostedlinear.DefaultRequestTimeout},
		hostedlinear.NewSecretResolver(func(string) string { return "" }, nil),
		"https://linear.example/graphql",
		checkpoints,
	)

	if err := service.ValidateLinearPoller(
		runtimeCfg,
		workstation,
		worker,
		func(context.Context, work.WorkRequest) error { return nil },
	); err != nil {
		t.Fatalf("ValidateLinearPoller() error = %v", err)
	}
}

func TestServiceStartLinearPollerRejectsMissingAuth(t *testing.T) {
	t.Parallel()

	runtimeCfg, workstation, worker, checkpoints := validLinearPollerFixture(t)
	worker.Auth = nil
	service := New(
		nil,
		clockwork.NewFakeClock(),
		&http.Client{Timeout: hostedlinear.DefaultRequestTimeout},
		hostedlinear.NewSecretResolver(func(string) string { return "" }, nil),
		"",
		checkpoints,
	)

	var sidecars sync.WaitGroup
	err := service.StartLinearPoller(
		context.Background(),
		&sidecars,
		runtimeCfg,
		workstation,
		worker,
		func(context.Context, work.WorkRequest) error { return nil },
	)
	if err == nil {
		t.Fatal("StartLinearPoller() accepted missing auth configuration")
	}
}
