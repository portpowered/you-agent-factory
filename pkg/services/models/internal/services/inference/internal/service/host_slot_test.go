package service

import (
	"context"
	"testing"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	runtimehost "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host"
)

func TestAcquireHostSlotInspectsLeasedHostAndRefreshesEndpoint(t *testing.T) {
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
	if !first.Reused || first.Endpoint != host.endpoints[0] {
		t.Fatalf("first host slot = %#v, want first endpoint", first)
	}
	if !second.Reused || second.Endpoint != host.endpoints[1] {
		t.Fatalf("second host slot = %#v, want reused replacement endpoint", second)
	}
	if host.inspectCalls != 2 {
		t.Fatalf("InspectModelHost calls = %d, want one call per host state", host.inspectCalls)
	}
}

type rotatingHost struct {
	runtimehost.Service
	endpoints    []string
	inspectCalls int
}

func (host *rotatingHost) InspectModelHost(
	context.Context,
	models.InspectModelHostRequest,
) (models.InspectModelHostResult, error) {
	host.inspectCalls++
	return models.InspectModelHostResult{
		Host: models.ModelHostSnapshot{ReadinessState: models.ReadinessStateReady},
	}, nil
}

func (host *rotatingHost) InvocationEndpoint(
	context.Context,
	models.RuntimeScopeRef,
	string,
) (string, error) {
	if host.inspectCalls == 0 {
		return "", models.ErrHostRuntimeNotReady
	}
	return host.endpoints[host.inspectCalls-1], nil
}
