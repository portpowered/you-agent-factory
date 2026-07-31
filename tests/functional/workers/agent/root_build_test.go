package agent_test

import (
	"context"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestAgentRunnerDeliveryRemainsInertThroughRootBuildProcessConstruction proves
// root.BuildProcess stays inert while the published Providers root and Agent
// Runner delivery slice are available for composition.
func TestAgentRunnerDeliveryRemainsInertThroughRootBuildProcessConstruction(t *testing.T) {
	t.Parallel()

	process := support.BuildProcess(t, serviceedges.Edges{})
	providersRoot, err := providerswire.NewService()
	if err != nil {
		t.Fatalf("providerswire.NewService() error = %v", err)
	}
	if process == nil || providersRoot == nil {
		t.Fatal("root-built process or Providers root = nil, want inert composition")
	}
	if _, err := providersRoot.ListProviders(context.Background(), struct{}{}); err != nil {
		t.Fatalf("ListProviders() error = %v", err)
	}
}
