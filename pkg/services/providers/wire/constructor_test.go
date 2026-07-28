package wire

import (
	"context"
	"os/exec"
	"reflect"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/providers"
)

func TestDefaultACPProvidersUseSuffixedIdentitiesAndDeterministicCommands(t *testing.T) {
	t.Parallel()

	service, err := New(exec.Command, allAvailableLocator{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	response, err := service.List(context.Background(), providers.ListRequest{})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(response.Providers) != 16 {
		t.Fatalf("default provider count = %d, want 16", len(response.Providers))
	}
	byID := make(map[providers.ID]providers.Provider, len(response.Providers))
	for _, provider := range response.Providers {
		byID[provider.ID] = provider
	}
	want := map[providers.ID]providers.Provider{
		"cursor-acp":   {ID: "cursor-acp", DisplayName: "Cursor ACP", ExecutionKind: providers.ExecutionKindACP, Command: "cursor-agent", Arguments: []string{"acp"}, Availability: providers.Availability{State: providers.AvailabilityAvailable}},
		"droid-acp":    {ID: "droid-acp", DisplayName: "Factory Droid ACP", ExecutionKind: providers.ExecutionKindACP, Command: "droid", Arguments: []string{"exec", "--output-format", "acp-daemon"}, Availability: providers.Availability{State: providers.AvailabilityAvailable}},
		"kiro-acp":     {ID: "kiro-acp", DisplayName: "Kiro ACP", ExecutionKind: providers.ExecutionKindACP, Command: "kiro-cli-chat", Arguments: []string{"acp"}, Availability: providers.Availability{State: providers.AvailabilityAvailable}},
		"opencode-acp": {ID: "opencode-acp", DisplayName: "OpenCode ACP", ExecutionKind: providers.ExecutionKindACP, Command: "npx", Arguments: []string{"-y", "opencode-ai", "acp"}, Availability: providers.Availability{State: providers.AvailabilityAvailable}},
	}
	for id, expected := range want {
		if got := byID[id]; !reflect.DeepEqual(got, expected) {
			t.Fatalf("default provider %q = %#v, want %#v", id, got, expected)
		}
	}
}

func TestConfiguredIntegrationOverridesDefaultAndSupportsQuotedArguments(t *testing.T) {
	t.Parallel()

	factory, err := NewFactory(exec.Command, allAvailableLocator{})
	if err != nil {
		t.Fatalf("NewFactory() error = %v", err)
	}
	service, err := factory([]providers.Integration{{
		ID: "settings-entry", Name: "cursor-acp", Transport: "stdio", Command: `custom-cursor --profile "team alpha" acp`,
	}})
	if err != nil {
		t.Fatalf("factory(configured) error = %v", err)
	}
	response, err := service.Get(context.Background(), providers.GetRequest{ID: "cursor-acp"})
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if response.Provider.Command != "custom-cursor" || !reflect.DeepEqual(response.Provider.Arguments, []string{"--profile", "team alpha", "acp"}) {
		t.Fatalf("configured provider command = %q %#v", response.Provider.Command, response.Provider.Arguments)
	}
}

type allAvailableLocator struct{}

func (allAvailableLocator) LookPath(file string) (string, error) { return file, nil }
