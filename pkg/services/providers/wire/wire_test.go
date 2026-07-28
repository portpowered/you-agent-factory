package wire

import (
	"context"
	"errors"
	"strings"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	catalog "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog/wire"
)

func TestNewServiceBuildsUsableRoot(t *testing.T) {
	root, err := NewService()
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	result, err := root.ListProviders(
		context.Background(),
		providers.ListProvidersRequest{},
	)
	if err != nil || len(result.Providers) == 0 {
		t.Fatalf("ListProviders() = (%#v, %v), want catalog entries", result, err)
	}
}

func TestNewServiceBindsCodexAndClaudeFromCatalogWithoutEffects(t *testing.T) {
	t.Parallel()

	probeCalls := 0
	root, err := NewService(CatalogOption(catalogwire.WithProbeQuery(func(
		_ context.Context,
		descriptor providers.Descriptor,
	) (catalog.ProbeFacts, error) {
		probeCalls++
		return catalog.ProbeFacts{
			Readiness:     descriptor.Readiness,
			Prerequisites: descriptor.Prerequisites,
		}, nil
	})))
	if err != nil {
		t.Fatalf("NewService() = %v", err)
	}
	if probeCalls != 0 {
		t.Fatalf("construction probe calls = %d, want 0", probeCalls)
	}

	for _, test := range []struct {
		id   providers.ID
		name string
	}{
		{id: providers.IDCodex, name: "Codex"},
		{id: providers.IDClaude, name: "Claude"},
	} {
		_, executeErr := root.Execute(context.Background(), providers.ExecuteRequest{
			Provider:  test.id,
			AttemptID: "composition-attempt",
		})
		var failure providers.ExecuteFailure
		if !errors.As(executeErr, &failure) ||
			failure.Kind != providers.ExecuteFailureKindDependency ||
			!strings.Contains(failure.Message, test.name) {
			t.Fatalf(
				"Execute(%q) error = %#v, want matching private adapter",
				test.id,
				executeErr,
			)
		}
	}
	if probeCalls != 2 {
		t.Fatalf("execution probe calls = %d, want one per explicit selection", probeCalls)
	}
}

func TestNewRootRejectsMissingCatalog(t *testing.T) {
	root, err := newRoot(nil, nil, nil)
	if err == nil || root != nil {
		t.Fatalf("newRoot(nil) = (%v, %v), want construction error", root, err)
	}
}
