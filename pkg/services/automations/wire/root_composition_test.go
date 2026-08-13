package wire

import (
	"context"
	"errors"
	"sync"
	"testing"

	hostedsources "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/hosted_sources"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

type hostedPollersForwardingStub struct {
	startErr    error
	validateErr error
	started     bool
	validated   bool
}

type hostedRuntimePathsStub struct{}

func (hostedRuntimePathsStub) FactoryDir() string     { return "/factory" }
func (hostedRuntimePathsStub) RuntimeBaseDir() string { return "/runtime" }

func (s *hostedPollersForwardingStub) StartLinearPoller(
	context.Context,
	*sync.WaitGroup,
	factorydefinitions.RuntimeConfigLookup,
	factorydefinitions.FactoryWorkstationConfig,
	*factorydefinitions.FactoryWorkerConfig,
	hostedsources.WorkSubmitter,
) error {
	s.started = true
	return s.startErr
}

func (s *hostedPollersForwardingStub) ValidateLinearPoller(
	factorydefinitions.RuntimeConfigLookup,
	factorydefinitions.FactoryWorkstationConfig,
	*factorydefinitions.FactoryWorkerConfig,
	hostedsources.WorkSubmitter,
) error {
	s.validated = true
	return s.validateErr
}

func TestHostedPollersRootAdapterForwardsLifecycleCalls(t *testing.T) {
	startErr := errors.New("start failed")
	validateErr := errors.New("validation failed")
	inner := &hostedPollersForwardingStub{startErr: startErr, validateErr: validateErr}
	adapter := hostedPollersRootAdapter{inner: inner}

	if err := adapter.StartLinearPoller(
		context.Background(),
		&sync.WaitGroup{},
		nil,
		factorydefinitions.FactoryWorkstationConfig{},
		nil,
		func(context.Context, work.WorkRequest) error { return nil },
	); !errors.Is(err, startErr) {
		t.Fatalf("StartLinearPoller() error = %v, want %v", err, startErr)
	}
	if !inner.started {
		t.Fatal("StartLinearPoller() did not reach hosted poller")
	}

	if err := adapter.ValidateLinearPoller(
		nil,
		factorydefinitions.FactoryWorkstationConfig{},
		nil,
		func(context.Context, work.WorkRequest) error { return nil },
	); !errors.Is(err, validateErr) {
		t.Fatalf("ValidateLinearPoller() error = %v, want %v", err, validateErr)
	}
	if !inner.validated {
		t.Fatal("ValidateLinearPoller() did not reach hosted poller")
	}
}

func TestAdaptSecretResolverPreservesNilAndDelegatesValues(t *testing.T) {
	if got := adaptSecretResolver(nil); got != nil {
		t.Fatal("adaptSecretResolver(nil) returned a non-nil resolver")
	}

	called := false
	adapted := adaptSecretResolver(func(
		context.Context,
		hostedsources.HostedRuntimePaths,
		string,
	) (string, error) {
		called = true
		return "secret-value", nil
	})
	got, err := adapted(context.Background(), hostedRuntimePathsStub{}, "token")
	if err != nil || got != "secret-value" {
		t.Fatalf("adapted resolver = %q, %v; want secret-value, nil", got, err)
	}
	if !called {
		t.Fatal("adapted resolver did not call the supplied resolver")
	}
}
