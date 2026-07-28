package internal

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog"
)

func TestExecuteRejectsUnavailableProviderBeforeAdapter(t *testing.T) {
	t.Parallel()

	execution := &recordingExecution{}
	service := New(staticCatalog{descriptor: catalog.Descriptor{Provider: providers.Provider{
		ID:            "cursor-acp",
		ExecutionKind: providers.ExecutionKindACP,
		Availability:  providers.Availability{State: providers.AvailabilityUnavailable, Detail: "executable not found"},
	}}}, execution)

	_, err := service.Execute(context.Background(), providers.ExecuteRequest{ProviderID: "cursor-acp"})
	if !errors.Is(err, providers.ErrUnavailableProvider) {
		t.Fatalf("Execute() error = %v, want ErrUnavailableProvider", err)
	}
	if execution.calls != 0 {
		t.Fatalf("execution adapter calls = %d, want zero", execution.calls)
	}
}

type staticCatalog struct{ descriptor catalog.Descriptor }

func (value staticCatalog) List(context.Context) ([]catalog.Descriptor, error) {
	return []catalog.Descriptor{value.descriptor}, nil
}

func (value staticCatalog) Get(context.Context, providers.ID) (catalog.Descriptor, error) {
	return value.descriptor, nil
}

type recordingExecution struct{ calls int }

func (execution *recordingExecution) Execute(context.Context, catalog.Descriptor, providers.ExecuteRequest) (providers.ExecuteResponse, error) {
	execution.calls++
	return providers.ExecuteResponse{}, nil
}

func (execution *recordingExecution) ExecuteStream(context.Context, catalog.Descriptor, providers.ExecuteRequest) (*providers.ExecutionStream, error) {
	execution.calls++
	return &providers.ExecutionStream{}, nil
}
