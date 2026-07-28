package service_test

import (
	"context"
	"errors"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
	executionwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/wire"
)

func TestExecuteRejectsCatalogFactDriftWithoutInvokingAdapter(t *testing.T) {
	t.Parallel()

	adapterCalls := 0
	catalogService := &recordingCatalog{
		registration: func(id providers.ID) (providers.Descriptor, error) {
			return providers.Descriptor{
				ID:           id,
				Availability: providers.AvailabilitySelectable,
				Capabilities: []providers.Capability{
					providers.CapabilityPromptSubmission,
				},
			}, nil
		},
		get: func(
			context.Context,
			providers.GetProviderRequest,
		) (providers.GetProviderResult, error) {
			return providers.GetProviderResult{
				Provider: providers.Descriptor{
					ID:           providers.IDCodex,
					Availability: providers.AvailabilitySelectable,
					Capabilities: []providers.Capability{
						providers.CapabilityNativeStreaming,
					},
				},
			}, nil
		},
	}
	executionService, err := executionwire.NewService(
		catalogService,
		execution.Registration{
			Provider: providers.IDCodex,
			Attempt: func(
				context.Context,
				providers.ExecuteRequest,
			) (providers.ExecuteResult, error) {
				adapterCalls++
				return providers.ExecuteResult{Content: "wrong adapter"}, nil
			},
		},
	)
	if err != nil {
		t.Fatalf("NewService() = %v", err)
	}

	_, executeErr := executionService.Execute(
		context.Background(),
		providers.ExecuteRequest{
			Provider:  providers.IDCodex,
			AttemptID: "attempt-1",
		},
	)
	var failure providers.ExecuteFailure
	if !errors.As(executeErr, &failure) ||
		failure.Kind != providers.ExecuteFailureKindDependency {
		t.Fatalf("Execute() error = %#v, want catalog mismatch failure", executeErr)
	}
	if adapterCalls != 0 {
		t.Fatalf("adapter calls = %d, want 0", adapterCalls)
	}
}
