package service

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	factorydefinitioncomposition "github.com/portpowered/infinite-you/internal/testutil/factorydefinitionfixtures"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformrandom "github.com/portpowered/infinite-you/pkg/platform/random"
	hostedlinear "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/hosted_sources/internal/linear"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestNewConstructsHostedSourcesServiceWithExplicitLogger(t *testing.T) {
	t.Parallel()

	checkpoints, err := hostedlinear.NewCheckpointStore(platformfilesystem.Local{})
	if err != nil {
		t.Fatalf("NewCheckpointStore() error = %v", err)
	}

	service := New(
		zap.NewNop(),
		clockwork.NewFakeClock(),
		&http.Client{Timeout: hostedlinear.DefaultRequestTimeout},
		hostedlinear.NewSecretResolver(func(string) string { return "" }, nil),
		"",
		platformrandom.CryptoSource{},
		checkpoints,
	)
	if service == nil {
		t.Fatal("New() returned nil")
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
				_, err := NewLinearPoller(zap.NewNop(), nil, httpClient, secretResolver, checkpoints, "", runtimeCfg, workstation, worker, nil, submitter)
				return err
			},
		},
		{
			name: "http client",
			run: func() error {
				_, err := NewLinearPoller(zap.NewNop(), clock, nil, secretResolver, checkpoints, "", runtimeCfg, workstation, worker, nil, submitter)
				return err
			},
		},
		{
			name: "secret resolver",
			run: func() error {
				_, err := NewLinearPoller(zap.NewNop(), clock, httpClient, nil, checkpoints, "", runtimeCfg, workstation, worker, nil, submitter)
				return err
			},
		},
		{
			name: "checkpoint store",
			run: func() error {
				_, err := NewLinearPoller(zap.NewNop(), clock, httpClient, secretResolver, nil, "", runtimeCfg, workstation, worker, nil, submitter)
				return err
			},
		},
		{
			name: "runtime config",
			run: func() error {
				_, err := NewLinearPoller(zap.NewNop(), clock, httpClient, secretResolver, checkpoints, "", nil, workstation, worker, nil, submitter)
				return err
			},
		},
		{
			name: "worker",
			run: func() error {
				_, err := NewLinearPoller(zap.NewNop(), clock, httpClient, secretResolver, checkpoints, "", runtimeCfg, workstation, nil, nil, submitter)
				return err
			},
		},
		{
			name: "submitter",
			run: func() error {
				_, err := NewLinearPoller(zap.NewNop(), clock, httpClient, secretResolver, checkpoints, "", runtimeCfg, workstation, worker, nil, nil)
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
		zap.NewNop(),
		clockwork.NewFakeClock(),
		&http.Client{Timeout: hostedlinear.DefaultRequestTimeout},
		hostedlinear.NewSecretResolver(func(string) string { return "" }, nil),
		"https://linear.example/graphql",
		platformrandom.CryptoSource{},
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
		zap.NewNop(),
		clockwork.NewFakeClock(),
		&http.Client{Timeout: hostedlinear.DefaultRequestTimeout},
		hostedlinear.NewSecretResolver(func(string) string { return "" }, nil),
		"",
		platformrandom.CryptoSource{},
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

// hostedHTTPFunc keeps the network boundary controlled without a local server.
type hostedHTTPFunc func(*http.Request) (*http.Response, error)

func (f hostedHTTPFunc) Do(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestServiceInjectedLoggerPreservesScopedRedactedDiagnostics(t *testing.T) {
	t.Parallel()
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core).With(zap.String("origin", "selected-backend"))
	for _, name := range []string{"source-a", "source-b"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			runtimeCfg, workstation, worker, checkpoints := validLinearPollerFixture(t)
			workstation.Name, worker.Name = name, name+"-worker"
			clock := clockwork.NewFakeClock()
			var effects atomic.Int32
			secret := "private-" + name
			service := New(logger, clock, hostedHTTPFunc(func(request *http.Request) (*http.Response, error) {
				effects.Add(1)
				return nil, fmt.Errorf("transport failure for %s", request.Header.Get("Authorization"))
			}), func(context.Context, hostedlinear.RuntimePaths, string) (string, error) {
				effects.Add(1)
				return secret, nil
			}, "https://linear.example/graphql", platformrandom.SourceFunc(func(bound int64) (int64, error) {
				effects.Add(1)
				return bound / 2, nil
			}), checkpoints)
			submitter := Submitter(func(context.Context, work.WorkRequest) error {
				t.Error("failed provider request admitted Work")
				return nil
			})
			if err := service.ValidateLinearPoller(runtimeCfg, workstation, worker, submitter); err != nil {
				t.Fatal(err)
			}
			if effects.Load() != 0 || logs.FilterField(zap.String("workstation", name)).Len() != 0 {
				t.Fatal("construction or validation executed a hosted effect")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			var sidecars sync.WaitGroup
			t.Cleanup(func() { cancel(); sidecars.Wait() })
			if err := service.StartLinearPoller(ctx, &sidecars, runtimeCfg, workstation, worker, submitter); err != nil {
				t.Fatal(err)
			}
			// Registration follows the restart log, so this observes completed failure handling.
			if err := clock.BlockUntilContext(ctx, 1); err != nil {
				t.Fatal(err)
			}
			cancel()
			sidecars.Wait()
			scoped := logs.FilterField(zap.String("workstation", name))
			for _, message := range []string{"hosted linear poller started", "hosted linear poller restarting", "hosted linear poller stopped"} {
				entries := scoped.FilterMessage(message).All()
				if len(entries) != 1 {
					t.Fatalf("%s: got %d diagnostics", message, len(entries))
				}
				fields := entries[0].ContextMap()
				if fields["origin"] != "selected-backend" || fields["worker"] != worker.Name || fields["provider"] != interfaces.HostedWorkerProviderLinear {
					t.Fatalf("%s lost injected source context: %v", message, fields)
				}
			}
			failure := fieldString(scoped.FilterMessage("hosted linear poller restarting").All()[0].ContextMap()["error"])
			if strings.Contains(failure, secret) || !strings.Contains(failure, "[REDACTED]") {
				t.Fatalf("unsafe or missing failure diagnostic: %q", failure)
			}
		})
	}
}
