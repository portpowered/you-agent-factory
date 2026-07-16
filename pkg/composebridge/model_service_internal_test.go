package composebridge

import (
	"testing"

	localmodels "github.com/portpowered/infinite-you/pkg/models/local"
	"github.com/portpowered/infinite-you/pkg/runtimehost"
	"go.uber.org/zap"
)

func TestNewLocalModelDomainRetainsExplicitModelEdges(t *testing.T) {
	t.Parallel()

	assets := localmodels.NewAssetPuller(t.TempDir())
	runtime := localmodels.NewOmniVoiceRuntime(nil)
	domain, err := NewLocalModelDomain(&runtimehost.Config{
		Logger:                    zap.NewNop(),
		ModelAssets:               assets,
		LocalModelRuntimeOverride: runtime,
	})
	if err != nil {
		t.Fatalf("NewLocalModelDomain() error = %v", err)
	}
	if domain.Assets != assets || domain.Runtime != runtime || domain.Manager == nil || domain.Host == nil {
		t.Fatalf("NewLocalModelDomain() = %+v, want explicit edges and complete inert domain", domain)
	}
}
