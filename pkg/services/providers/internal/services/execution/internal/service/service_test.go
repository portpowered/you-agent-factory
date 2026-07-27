package service_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	catalog "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog/wire"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
	executionwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/wire"
)

func TestNewRejectsInvalidRegistrationSets(t *testing.T) {
	t.Parallel()

	validAttempt := func(
		context.Context,
		providers.ExecuteRequest,
	) (providers.ExecuteResult, error) {
		return providers.ExecuteResult{}, nil
	}
	tests := []struct {
		name          string
		catalog       catalog.Service
		registrations []execution.Registration
	}{
		{name: "nil catalog"},
		{
			name:    "invalid provider",
			catalog: mustCatalog(t),
			registrations: []execution.Registration{{
				Attempt: validAttempt,
			}},
		},
		{
			name:    "nil adapter",
			catalog: mustCatalog(t),
			registrations: []execution.Registration{{
				Provider: providers.IDCodex,
			}},
		},
		{
			name:    "duplicate adapter",
			catalog: mustCatalog(t),
			registrations: []execution.Registration{
				{Provider: providers.IDCodex, Attempt: validAttempt},
				{Provider: providers.IDCodex, Attempt: validAttempt},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service, err := executionwire.NewService(test.catalog, test.registrations...)
			if err == nil || service != nil {
				t.Fatalf("NewService() = (%v, %v), want construction error", service, err)
			}
		})
	}
}

func TestExecuteValidatesThenResolvesCanonicalAdapterExactlyOnce(t *testing.T) {
	t.Parallel()

	catalogService := mustCatalog(t)
	calls := 0
	var received providers.ExecuteRequest
	executionService, err := executionwire.NewService(
		catalogService,
		execution.Registration{
			Provider: providers.IDCursor,
			Attempt: func(
				_ context.Context,
				request providers.ExecuteRequest,
			) (providers.ExecuteResult, error) {
				calls++
				received = request.Clone()
				request.ResumeSession.ID = "adapter-mutated"
				return providers.ExecuteResult{Content: "done"}, nil
			},
		},
	)
	if err != nil {
		t.Fatalf("NewService() = %v", err)
	}
	resume := &providers.SessionRef{
		Provider: providers.IDCursor,
		Kind:     providers.SessionIDKind,
		ID:       "session-1",
	}
	request := providers.ExecuteRequest{
		Provider:         providers.ID("cursor"),
		AttemptID:        "attempt-1",
		SystemPrompt:     "system",
		UserMessage:      "user",
		OutputSchema:     `{"type":"string"}`,
		ResumeSession:    resume,
		WorkingDirectory: "C:/workspace",
		Worktree:         "C:/workspace/tree",
	}

	result, err := executionService.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	if result.Content != "done" {
		t.Fatalf("Execute().Content = %q, want done", result.Content)
	}
	if calls != 1 {
		t.Fatalf("adapter calls = %d, want 1", calls)
	}
	want := request.Clone()
	want.Provider = providers.IDCursor
	if !reflect.DeepEqual(received, want) {
		t.Fatalf("adapter request = %#v, want %#v", received, want)
	}
	if received.ResumeSession == request.ResumeSession {
		t.Fatal("adapter received caller-owned ResumeSession pointer")
	}
	if request.ResumeSession.ID != "session-1" {
		t.Fatalf("caller ResumeSession.ID = %q, want session-1", request.ResumeSession.ID)
	}
}

func TestExecuteRejectsInvalidRequestBeforeCatalogOrAdapterIO(t *testing.T) {
	t.Parallel()

	catalogCalls := 0
	adapterCalls := 0
	catalogService := &recordingCatalog{
		get: func(
			_ context.Context,
			_ providers.GetProviderRequest,
		) (providers.GetProviderResult, error) {
			catalogCalls++
			return providers.GetProviderResult{
				Provider: providers.Descriptor{ID: providers.IDCodex},
			}, nil
		},
	}
	executionService, err := executionwire.NewService(
		catalogService,
		execution.Registration{
			Provider: providers.IDCodex,
			Attempt: func(
				_ context.Context,
				_ providers.ExecuteRequest,
			) (providers.ExecuteResult, error) {
				adapterCalls++
				return providers.ExecuteResult{}, nil
			},
		},
	)
	if err != nil {
		t.Fatalf("NewService() = %v", err)
	}
	invalidRequests := []providers.ExecuteRequest{
		{AttemptID: "attempt-1"},
		{Provider: providers.IDCodex},
		{
			Provider:  providers.IDCodex,
			AttemptID: "attempt-1",
			ResumeSession: &providers.SessionRef{
				Provider: providers.IDCodex,
				Kind:     providers.SessionIDKind,
			},
		},
	}

	for _, request := range invalidRequests {
		if _, err := executionService.Execute(context.Background(), request); err == nil {
			t.Fatalf("Execute(%#v) error = nil, want validation failure", request)
		}
	}
	if catalogCalls != 0 || adapterCalls != 0 {
		t.Fatalf(
			"invalid requests caused catalog/adapter calls = %d/%d, want 0/0",
			catalogCalls,
			adapterCalls,
		)
	}
}

func TestExecuteNeverFallsBackForUnknownUnavailableOrUnregisteredProvider(t *testing.T) {
	t.Parallel()

	codexCalls := 0
	executionService, err := executionwire.NewService(
		mustCatalog(t),
		execution.Registration{
			Provider: providers.IDCodex,
			Attempt: func(
				_ context.Context,
				_ providers.ExecuteRequest,
			) (providers.ExecuteResult, error) {
				codexCalls++
				return providers.ExecuteResult{}, nil
			},
		},
	)
	if err != nil {
		t.Fatalf("NewService() = %v", err)
	}
	tests := []struct {
		name     string
		provider providers.ID
		want     error
	}{
		{name: "unknown", provider: "missing", want: providers.ErrUnknownProvider},
		{name: "catalog unavailable", provider: providers.IDAgy, want: providers.ErrProviderUnavailable},
		{name: "no registered adapter", provider: providers.IDClaude, want: providers.ErrProviderUnavailable},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, executeErr := executionService.Execute(
				context.Background(),
				providers.ExecuteRequest{
					Provider:  test.provider,
					AttemptID: "attempt-1",
				},
			)
			if !errors.Is(executeErr, test.want) {
				t.Fatalf("Execute() error = %v, want %v", executeErr, test.want)
			}
		})
	}
	if codexCalls != 0 {
		t.Fatalf("default codex adapter calls = %d, want 0", codexCalls)
	}
}

func TestExecuteReturnsFirstAdapterFailureWithoutRetry(t *testing.T) {
	t.Parallel()

	attemptErr := errors.New("attempt failed")
	calls := 0
	executionService, err := executionwire.NewService(
		mustCatalog(t),
		execution.Registration{
			Provider: providers.IDCodex,
			Attempt: func(
				_ context.Context,
				_ providers.ExecuteRequest,
			) (providers.ExecuteResult, error) {
				calls++
				return providers.ExecuteResult{}, attemptErr
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
	if !errors.Is(executeErr, attemptErr) {
		t.Fatalf("Execute() error = %v, want %v", executeErr, attemptErr)
	}
	if calls != 1 {
		t.Fatalf("adapter calls = %d, want 1", calls)
	}
}

func mustCatalog(t *testing.T) catalog.Service {
	t.Helper()

	catalogService, err := catalogwire.NewService()
	if err != nil {
		t.Fatalf("catalogwire.NewService() = %v", err)
	}
	return catalogService
}

type recordingCatalog struct {
	get func(
		context.Context,
		providers.GetProviderRequest,
	) (providers.GetProviderResult, error)
}

func (*recordingCatalog) ListProviders(
	context.Context,
	providers.ListProvidersRequest,
) (providers.ListProvidersResult, error) {
	return providers.ListProvidersResult{}, nil
}

func (catalog *recordingCatalog) GetProvider(
	ctx context.Context,
	request providers.GetProviderRequest,
) (providers.GetProviderResult, error) {
	return catalog.get(ctx, request)
}
