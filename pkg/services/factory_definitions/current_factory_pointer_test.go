package factorydefinitions_test

import (
	"context"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestResolveCurrentFactoryDirectory_UsesRootServicePointer(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	var service factorydefinitions.Service = fakeDefinitionsPeer{
		entries: []factorydefinitions.NamedFactoryListEntry{{
			Name:       "alpha",
			FactoryDir: "/factories/alpha",
			Current:    true,
		}},
	}

	got, err := factorydefinitions.ResolveCurrentFactoryDirectory(ctx, service, "/factories")
	if err != nil {
		t.Fatalf("ResolveCurrentFactoryDirectory: %v", err)
	}
	if got != "/factories/alpha" {
		t.Fatalf("ResolveCurrentFactoryDirectory = %q, want /factories/alpha", got)
	}
}
