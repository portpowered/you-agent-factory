package service

import (
	"context"
	"testing"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	runtimehost "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host"
)

func TestAcquireHostSlotRefreshesEndpointAfterHostIsReplaced(t *testing.T) {
	t.Parallel()

	host := &rotatingHost{
		endpoints: []string{
			"grpc://127.0.0.1:50051",
			"grpc://127.0.0.1:50052",
		},
	}
	service := &service{runtimeHost: host}
	request := models.InvokeModelRequest{ModelName: "llm"}

	first, err := service.acquireHostSlot(context.Background(), request)
	if err != nil {
		t.Fatalf("first acquireHostSlot: %v", err)
	}
	second, err := service.acquireHostSlot(context.Background(), request)
	if err != nil {
		t.Fatalf("second acquireHostSlot: %v", err)
	}
	if first.Reused || first.Endpoint != host.endpoints[0] {
		t.Fatalf("first host slot = %#v, want new first endpoint", first)
	}
	if !second.Reused || second.Endpoint != host.endpoints[1] {
		t.Fatalf("second host slot = %#v, want reused replacement endpoint", second)
	}
	if host.ensureCalls != 2 {
		t.Fatalf("EnsureModelHost calls = %d, want one call per host state", host.ensureCalls)
	}
}

type rotatingHost struct {
	runtimehost.Service
	endpoints   []string
	ensureCalls int
}

func (host *rotatingHost) EnsureModelHost(
	context.Context,
	models.EnsureModelHostRequest,
) (models.EnsureModelHostResult, error) {
	outcome := models.HostEnsureBecameReady
	if host.ensureCalls > 0 {
		outcome = models.HostEnsureAlreadyReady
	}
	host.ensureCalls++
	return models.EnsureModelHostResult{
		Host:    models.ModelHostSnapshot{ReadinessState: models.ReadinessStateReady},
		Outcome: outcome,
	}, nil
}

func (host *rotatingHost) InvocationEndpoint(context.Context, models.RuntimeScopeRef, string) (string, error) {
	if host.ensureCalls == 0 {
		return "", models.ErrHostRuntimeNotReady
	}
	return host.endpoints[host.ensureCalls-1], nil
}
