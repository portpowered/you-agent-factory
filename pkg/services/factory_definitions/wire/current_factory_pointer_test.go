package wire

import (
	"context"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestResolveCurrentFactoryDirectory_UsesRootServicePointer(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	var service factorydefinitions.Service = currentFactoryPointerPeer{
		entries: []factorydefinitions.NamedFactoryListEntry{{
			Name:       "alpha",
			FactoryDir: "/factories/alpha",
			Current:    true,
		}},
	}

	got, err := ResolveCurrentFactoryDirectory(ctx, service, "/factories")
	if err != nil {
		t.Fatalf("ResolveCurrentFactoryDirectory: %v", err)
	}
	if got != "/factories/alpha" {
		t.Fatalf("ResolveCurrentFactoryDirectory = %q, want /factories/alpha", got)
	}
}

type currentFactoryPointerPeer struct {
	factorydefinitions.UnimplementedService
	entries []factorydefinitions.NamedFactoryListEntry
}

func (p currentFactoryPointerPeer) GetCurrentFactoryPointer(
	context.Context,
	factorydefinitions.GetCurrentFactoryPointerRequest,
) (factorydefinitions.GetCurrentFactoryPointerResult, error) {
	if len(p.entries) == 0 {
		return factorydefinitions.GetCurrentFactoryPointerResult{}, nil
	}
	entry := p.entries[0]
	return factorydefinitions.GetCurrentFactoryPointerResult{
		Name:       entry.Name,
		FactoryDir: entry.FactoryDir,
	}, nil
}
