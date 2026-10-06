package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	providerservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/service"
	"github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog"
	"github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
)

type validationCatalog struct {
	catalog.Service
	unavailable bool
}

func (c validationCatalog) ResolveProviderID(id providers.ID) (providers.ID, error) {
	if id == "unknown" {
		return "", providers.ErrUnknownProvider
	}
	return id, nil
}

func (c validationCatalog) GetProvider(_ context.Context, req providers.GetProviderRequest) (providers.GetProviderResult, error) {
	if c.unavailable {
		return providers.GetProviderResult{}, providers.ErrProviderUnavailable
	}
	return providers.GetProviderResult{Provider: providers.Descriptor{ID: req.ID}}, nil
}

type validationExecution struct {
	execution.ContinuationService
	calls int
}

func (e *validationExecution) Execute(context.Context, providers.ExecuteRequest) (providers.ExecuteResult, error) {
	e.calls++
	return providers.ExecuteResult{Content: "controlled"}, nil
}

func TestT7ProviderPreflightSharesPolicyWithoutExecution(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, effort string
		provider     providers.ID
		unavailable  bool
		want         error
	}{
		{name: "codex canonical effort", provider: providers.IDCodex, effort: " HIGH "},
		{name: "default effort opaque model", provider: providers.IDClaude},
		{name: "Claude minimal rejected", provider: providers.IDClaude, effort: "MINIMAL", want: providers.ErrExecuteFailed},
		{name: "Agy separate effort rejected", provider: providers.IDAntigravity, effort: "low", want: providers.ErrExecuteFailed},
		{name: "ACP separate effort rejected", provider: "cursor-acp", effort: "high", want: providers.ErrExecuteFailed},
		{name: "ACP exact model accepted", provider: "cursor-acp"},
		{name: "unknown effort", provider: providers.IDCodex, effort: "invalid", want: providers.ErrExecuteFailed},
		{name: "unknown provider", provider: "unknown", want: providers.ErrUnknownProvider},
		{name: "unavailable", provider: providers.IDCodex, unavailable: true, want: providers.ErrProviderUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			native := &validationExecution{}
			acp := &stubACPService{provider: "cursor-acp"}
			svc, err := providerservice.NewWithACP(validationCatalog{unavailable: tc.unavailable}, native, acp, nil, logging.NoopLogger{}, nil)
			if err != nil {
				t.Fatal(err)
			}
			request := providers.ExecuteRequest{Provider: tc.provider, AttemptID: "attempt", Model: "opaque-model-with-effort", ReasoningEffort: tc.effort}
			err = svc.ValidateExecution(context.Background(), request)
			if !errors.Is(err, tc.want) {
				t.Fatalf("ValidateExecution = %v, want %v", err, tc.want)
			}
			if native.calls != 0 || acp.executeCalls != 0 {
				t.Fatal("preflight executed provider")
			}
			if errors.Is(tc.want, providers.ErrExecuteFailed) {
				_, err = svc.Execute(context.Background(), request)
				if !errors.Is(err, tc.want) {
					t.Fatalf("Execute policy = %v, want %v", err, tc.want)
				}
				if native.calls != 0 || acp.executeCalls != 0 {
					t.Fatal("invalid execution reached provider")
				}
			}
		})
	}
}
