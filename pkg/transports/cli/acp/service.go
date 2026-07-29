// Package acp adapts customer ACP catalog commands to Operator Settings and
// the Providers root without introducing a CLI-owned registry.
package acp

import (
	"context"
	"fmt"
	"strings"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/services/providers"
)

type Service struct {
	Settings         operatorsettings.ConfigDocumentService
	ProvidersFactory providers.Factory
	GenerateID       operatorsettings.IDGenerator
}

func (service Service) Add(ctx context.Context, home, name, transport, command string) error {
	if service.GenerateID == nil {
		return fmt.Errorf("ACP integration ID generator is required")
	}
	_, err := service.Settings.ConfigureACPIntegrationAdd(ctx, operatorsettings.DefaultConfigPath(home), operatorsettings.ACPIntegration{
		ID: service.GenerateID(), Name: name, Transport: transport, Command: command,
	})
	return err
}

func (service Service) Delete(ctx context.Context, home, name string) error {
	_, err := service.Settings.ConfigureACPIntegrationDelete(ctx, operatorsettings.DefaultConfigPath(home), name)
	return err
}

func (service Service) List(ctx context.Context, home string) (providers.ListProvidersResult, error) {
	if service.ProvidersFactory == nil {
		return providers.ListProvidersResult{}, fmt.Errorf("Providers factory is required")
	}
	document, err := service.Settings.Load(operatorsettings.DefaultConfigPath(home))
	if err != nil {
		return providers.ListProvidersResult{}, err
	}
	configured := document.FileConfig().Workers.ACP.Integrations
	integrations := make([]providers.ACPIntegration, len(configured))
	for index, value := range configured {
		integrations[index] = providers.ACPIntegration{
			ID: value.ID, Name: providers.ID(value.Name), Transport: value.Transport, Command: value.Command,
		}
	}
	root, err := service.ProvidersFactory(integrations)
	if err != nil {
		return providers.ListProvidersResult{}, err
	}
	result, err := root.ListProviders(ctx, providers.ListProvidersRequest{})
	if err != nil {
		return providers.ListProvidersResult{}, err
	}
	return filterACPProviders(result, configured), nil
}

func filterACPProviders(
	result providers.ListProvidersResult,
	configured []operatorsettings.ACPIntegration,
) providers.ListProvidersResult {
	identities := map[providers.ID]struct{}{
		"cursor-acp":   {},
		"kiro-acp":     {},
		"opencode-acp": {},
	}
	for _, integration := range configured {
		identities[providers.ID(integration.Name)] = struct{}{}
	}
	filtered := make([]providers.Descriptor, 0, len(identities))
	for _, descriptor := range result.Providers {
		if _, ok := identities[descriptor.ID]; ok {
			filtered = append(filtered, descriptor)
		}
	}
	return providers.ListProvidersResult{Providers: filtered}
}

func ValidateAdd(name, transport, command string) error {
	name = strings.TrimSpace(name)
	if name != strings.ToLower(name) || strings.ContainsAny(name, " \t\r\n") {
		return fmt.Errorf("ACP provider name must be lowercase and contain no whitespace")
	}
	if err := providers.ID(name).Validate(); err != nil {
		return err
	}
	if strings.ToLower(strings.TrimSpace(transport)) != "stdio" {
		return fmt.Errorf("ACP transport must be stdio")
	}
	if strings.TrimSpace(command) == "" {
		return fmt.Errorf("ACP command is required")
	}
	return nil
}
