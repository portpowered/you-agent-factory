package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	runtimehost "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host"
	internalservice "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host/internal/service"
)

func TestEnsureModelHostBoundsBlockedLoadModelNegotiation(t *testing.T) {
	t.Parallel()

	cacheDirectory := t.TempDir()
	writeCacheFixture(t, cacheDirectory, true)
	scopes := newScopes(t, "readiness-loadmodel-deadline")
	ref := openScope(t, scopes, cacheDirectory, managedLocalAIConfig(models.LoadPolicyOnDemand))
	launcher := &controlledProcessLauncher{}
	negotiator := &blockedReadinessNegotiator{deadline: make(chan bool, 1)}
	host := internalservice.NewWithHostTestConfig(
		scopes,
		mustAssetsService(t, scopes),
		launcher,
		nil,
		realHostClock{},
		nil,
		nil,
		internalservice.SupervisorTestConfig{ReadinessTimeout: 50 * time.Millisecond},
		internalservice.HostPolicyTestConfig{},
		runtimehost.Options{
			Platform:             managedHostPlatform(),
			CompatibilityChecker: &testCompatibilityChecker{},
			ProtocolNegotiator:   negotiator,
		},
	)
	t.Cleanup(func() { _ = internalservice.ShutdownHost(context.Background(), host) })

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result, err := host.EnsureModelHost(ctx, models.EnsureModelHostRequest{
		Scope: ref,
		Name:  managedModelName,
	})
	if !errors.Is(err, models.ErrHostLoadingTimeout) {
		t.Fatalf("EnsureModelHost error = %v, want typed readiness timeout", err)
	}
	if errors.Is(err, models.ErrHostCancelled) {
		t.Fatalf("readiness deadline was classified as caller cancellation: %v", err)
	}
	if result.Host.ReadinessState == models.ReadinessStateReady {
		t.Fatalf("host became ready after blocked LoadModel: %#v", result.Host)
	}
	select {
	case hasDeadline := <-negotiator.deadline:
		if !hasDeadline {
			t.Fatal("LoadModel negotiator context did not carry a readiness deadline")
		}
	default:
		t.Fatal("protocol negotiator did not observe the readiness call")
	}
	select {
	case <-launcher.process(0).stopped:
	case <-time.After(time.Second):
		t.Fatal("readiness timeout returned without stopping the owned process")
	}
}

type blockedReadinessNegotiator struct {
	deadline chan bool
}

func (negotiator *blockedReadinessNegotiator) Negotiate(
	ctx context.Context,
	_ string,
	_ modelseffects.HostProtocolNegotiationRequest,
) (modelseffects.HostProtocolNegotiationResult, error) {
	_, hasDeadline := ctx.Deadline()
	negotiator.deadline <- hasDeadline
	<-ctx.Done()
	return modelseffects.HostProtocolNegotiationResult{}, ctx.Err()
}

var _ modelseffects.HostProtocolNegotiator = (*blockedReadinessNegotiator)(nil)
