// Package wire constructs the inert Providers root and its private services.
package wire

import (
	"fmt"
	"strings"

	"github.com/mattn/go-shellwords"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	providersinternal "github.com/portpowered/infinite-you/pkg/services/providers/internal"
	"github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog/wire"
	executionwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/wire"
)

func New(newCommand platformprocess.CommandFactory, locator platformprocess.ExecutableLocator) (providers.Service, error) {
	factory, err := NewFactory(newCommand, locator)
	if err != nil {
		return nil, err
	}
	return factory(nil)
}

func NewFactory(newCommand platformprocess.CommandFactory, locator platformprocess.ExecutableLocator) (providers.Factory, error) {
	executionService, err := executionwire.NewACP(newCommand)
	if err != nil {
		return nil, err
	}
	if locator == nil {
		return nil, fmt.Errorf("provider executable locator is required")
	}
	return func(integrations []providers.Integration) (providers.Service, error) {
		descriptors, err := configuredDescriptors(integrations)
		if err != nil {
			return nil, err
		}
		catalogService, err := catalogwire.New(descriptors, locator)
		if err != nil {
			return nil, err
		}
		return providersinternal.New(catalogService, executionService), nil
	}, nil
}

func configuredDescriptors(integrations []providers.Integration) ([]catalog.Descriptor, error) {
	byID := make(map[providers.ID]catalog.Descriptor)
	for _, descriptor := range defaultDescriptors() {
		byID[descriptor.Provider.ID] = descriptor
	}
	for index, integration := range integrations {
		if err := integration.Name.Validate(); err != nil {
			return nil, fmt.Errorf("provider integration %d: %w", index, err)
		}
		if strings.ToLower(strings.TrimSpace(integration.Transport)) != "stdio" {
			return nil, fmt.Errorf("provider integration %q has unsupported transport %q", integration.Name, integration.Transport)
		}
		argv, err := shellwords.Parse(strings.TrimSpace(integration.Command))
		if err != nil {
			return nil, fmt.Errorf("provider integration %q has invalid command: %w", integration.Name, err)
		}
		if len(argv) == 0 {
			return nil, fmt.Errorf("provider integration %q has an empty command", integration.Name)
		}
		byID[integration.Name] = catalog.Descriptor{Provider: providers.Provider{
			ID: integration.Name, DisplayName: string(integration.Name), ExecutionKind: providers.ExecutionKindACP,
			Command: argv[0], Arguments: append([]string(nil), argv[1:]...),
		}}
	}
	result := make([]catalog.Descriptor, 0, len(byID))
	for _, descriptor := range byID {
		result = append(result, descriptor)
	}
	return result, nil
}

func defaultDescriptors() []catalog.Descriptor {
	return []catalog.Descriptor{
		{Provider: providers.Provider{ID: "cursor-acp", DisplayName: "Cursor ACP", ExecutionKind: providers.ExecutionKindACP, Command: "cursor-agent", Arguments: []string{"acp"}}},
		{Provider: providers.Provider{ID: "copilot-acp", DisplayName: "GitHub Copilot ACP", ExecutionKind: providers.ExecutionKindACP, Command: "copilot", Arguments: []string{"--acp", "--stdio"}}},
		{Provider: providers.Provider{ID: "droid-acp", DisplayName: "Factory Droid ACP", ExecutionKind: providers.ExecutionKindACP, Command: "droid", Arguments: []string{"exec", "--output-format", "acp-daemon"}}, Aliases: []providers.ID{"factory-droid", "factorydroid"}},
		{Provider: providers.Provider{ID: "gemini-acp", DisplayName: "Gemini ACP", ExecutionKind: providers.ExecutionKindACP, Command: "gemini", Arguments: []string{"--acp"}}},
		{Provider: providers.Provider{ID: "grok-build-acp", DisplayName: "Grok Build ACP", ExecutionKind: providers.ExecutionKindACP, Command: "grok", Arguments: []string{"agent", "stdio"}}},
		{Provider: providers.Provider{ID: "iflow-acp", DisplayName: "iFlow ACP", ExecutionKind: providers.ExecutionKindACP, Command: "iflow", Arguments: []string{"--experimental-acp"}}},
		{Provider: providers.Provider{ID: "kilocode-acp", DisplayName: "Kilo Code ACP", ExecutionKind: providers.ExecutionKindACP, Command: "npx", Arguments: []string{"-y", "@kilocode/cli", "acp"}}},
		{Provider: providers.Provider{ID: "kimi-acp", DisplayName: "Kimi ACP", ExecutionKind: providers.ExecutionKindACP, Command: "kimi", Arguments: []string{"acp"}}},
		{Provider: providers.Provider{ID: "kiro-acp", DisplayName: "Kiro ACP", ExecutionKind: providers.ExecutionKindACP, Command: "kiro-cli-chat", Arguments: []string{"acp"}}},
		{Provider: providers.Provider{ID: "openclaw-acp", DisplayName: "OpenClaw ACP", ExecutionKind: providers.ExecutionKindACP, Command: "openclaw", Arguments: []string{"acp"}}},
		{Provider: providers.Provider{ID: "opencode-acp", DisplayName: "OpenCode ACP", ExecutionKind: providers.ExecutionKindACP, Command: "npx", Arguments: []string{"-y", "opencode-ai", "acp"}}},
		{Provider: providers.Provider{ID: "pi-acp", DisplayName: "Pi ACP", ExecutionKind: providers.ExecutionKindACP, Command: "npx", Arguments: []string{"-y", "pi-acp"}}},
		{Provider: providers.Provider{ID: "qoder-acp", DisplayName: "Qoder ACP", ExecutionKind: providers.ExecutionKindACP, Command: "qodercli", Arguments: []string{"--acp"}}},
		{Provider: providers.Provider{ID: "qwen-acp", DisplayName: "Qwen ACP", ExecutionKind: providers.ExecutionKindACP, Command: "qwen", Arguments: []string{"--acp"}}},
		{Provider: providers.Provider{ID: "reasonix-acp", DisplayName: "Reasonix ACP", ExecutionKind: providers.ExecutionKindACP, Command: "reasonix", Arguments: []string{"acp"}}},
		{Provider: providers.Provider{ID: "trae-acp", DisplayName: "Trae ACP", ExecutionKind: providers.ExecutionKindACP, Command: "traecli", Arguments: []string{"acp", "serve"}}},
	}
}
