package cli

import (
	"bytes"
	"context"
	"io"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerscli "github.com/portpowered/infinite-you/pkg/services/providers/transports/cli"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
)

func TestProductionProvidersCommandWiresGeneratedHandlersAndHelp(t *testing.T) {
	providerService := providerscli.New(&providerServiceStub{})
	command, err := newProductionProvidersCommand(
		&cliDiagnosticsOptions{},
		CommandFactory{ProvidersCLI: providerService},
	)
	if err != nil {
		t.Fatalf("newProductionProvidersCommand() error = %v", err)
	}
	if command.RunE != nil {
		t.Fatal("you providers must remain non-runnable")
	}
	list, _, err := command.Find([]string{"list"})
	if err != nil {
		t.Fatalf("find you providers list: %v", err)
	}
	if list.RunE == nil {
		t.Fatal("you providers list must attach its resolved manifest handler")
	}

	manifest, err := generated.ProvidersFamilyManifest()
	if err != nil {
		t.Fatalf("ProvidersFamilyManifest() error = %v", err)
	}
	if manifest.Commands["you.providers.list"].Documentation.Documentation.Title.CanonicalEnglish != "List provider capabilities" {
		t.Fatalf("providers list title = %q", manifest.Commands["you.providers.list"].Documentation.Documentation.Title.CanonicalEnglish)
	}

	var help bytes.Buffer
	command.SetOut(&help)
	command.SetErr(io.Discard)
	command.SetArgs([]string{"--help"})
	if err := command.Execute(); err != nil {
		t.Fatalf("execute providers help: %v", err)
	}
	for _, want := range []string{"Discover provider capabilities", "you providers list", "Available Commands"} {
		if !bytes.Contains(help.Bytes(), []byte(want)) {
			t.Fatalf("providers help missing %q:\n%s", want, help.String())
		}
	}
}

// providerServiceStub uses the published root interface without giving the
// CLI package a peer implementation or a second service graph. This test only
// exercises command projection and never invokes a Providers operation.
type providerServiceStub struct {
	providers.Service
}

var _ providers.Service = (*providerServiceStub)(nil)

func (providerServiceStub) ListProviders(context.Context, providers.ListProvidersRequest) (providers.ListProvidersResult, error) {
	return providers.ListProvidersResult{}, nil
}
