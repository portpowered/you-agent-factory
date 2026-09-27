package acp

import (
	"reflect"
	"testing"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/services/providers"
)

func TestFilterACPProvidersKeepsPresetsAndConfiguredIdentities(t *testing.T) {
	t.Parallel()

	result := filterACPProviders(providers.ListProvidersResult{Providers: []providers.Descriptor{
		{ID: "codex"},
		{ID: "cursor"},
		{ID: "customer-agent"},
		{ID: "opencode"},
	}}, []operatorsettings.ACPIntegration{{Name: "customer-agent"}})

	want := []providers.Descriptor{
		{ID: "cursor"},
		{ID: "customer-agent"},
		{ID: "opencode"},
	}
	if !reflect.DeepEqual(result.Providers, want) {
		t.Fatalf("filtered providers = %#v, want %#v", result.Providers, want)
	}
}
