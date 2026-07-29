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
	Settings   operatorsettings.ConfigDocumentService
	Providers  providers.Service
	GenerateID operatorsettings.IDGenerator
}

type WorkerCatalog struct {
	Providers []providers.Descriptor
	ACP       map[providers.ID]bool
	Custom    map[providers.ID]bool
}

func (service Service) ListWorkers(ctx context.Context, home string) (WorkerCatalog, error) {
	if service.Providers == nil {
		return WorkerCatalog{}, fmt.Errorf("Providers service is required")
	}
	document, err := service.Settings.Load(operatorsettings.DefaultConfigPath(home))
	if err != nil {
		return WorkerCatalog{}, err
	}
	if err := service.configure(ctx, document); err != nil {
		return WorkerCatalog{}, err
	}
	listed, err := service.Providers.ListProviders(ctx, providers.ListProvidersRequest{})
	if err != nil {
		return WorkerCatalog{}, err
	}
	acpProviders := make(map[providers.ID]bool)
	customProviders := make(map[providers.ID]bool)
	for _, integration := range document.FileConfig().Workers.ACP.Integrations {
		customProviders[providers.ID(integration.Name)] = true
	}
	for _, descriptor := range filterACPProviders(listed, document.FileConfig().Workers.ACP.Integrations).Providers {
		acpProviders[descriptor.ID] = true
	}
	return WorkerCatalog{Providers: listed.Providers, ACP: acpProviders, Custom: customProviders}, nil
}

// Configure applies an already-loaded operator ACP configuration to the live
// Providers root. It is used by run before any worker selection occurs.
func (service Service) Configure(ctx context.Context, configured []operatorsettings.ACPIntegration) error {
	if len(configured) == 0 && service.Providers == nil {
		return nil
	}
	return service.configureIntegrations(ctx, configured)
}

func (service Service) Add(ctx context.Context, home, name, transport, command string) error {
	if service.GenerateID == nil {
		return fmt.Errorf("ACP integration ID generator is required")
	}
	document, err := service.Settings.ConfigureACPIntegrationAdd(ctx, operatorsettings.DefaultConfigPath(home), operatorsettings.ACPIntegration{
		ID: service.GenerateID(), Name: name, Transport: transport, Command: command,
	})
	if err != nil {
		return err
	}
	return service.configure(ctx, document)
}

func (service Service) Delete(ctx context.Context, home, name string) error {
	document, err := service.Settings.ConfigureACPIntegrationDelete(ctx, operatorsettings.DefaultConfigPath(home), name)
	if err != nil {
		return err
	}
	return service.configure(ctx, document)
}

func (service Service) List(ctx context.Context, home string) (providers.ListProvidersResult, error) {
	if service.Providers == nil {
		return providers.ListProvidersResult{}, fmt.Errorf("Providers service is required")
	}
	document, err := service.Settings.Load(operatorsettings.DefaultConfigPath(home))
	if err != nil {
		return providers.ListProvidersResult{}, err
	}
	if err := service.configure(ctx, document); err != nil {
		return providers.ListProvidersResult{}, err
	}
	result, err := service.Providers.ListProviders(ctx, providers.ListProvidersRequest{})
	if err != nil {
		return providers.ListProvidersResult{}, err
	}
	return filterACPProviders(result, document.FileConfig().Workers.ACP.Integrations), nil
}

func (service Service) configure(ctx context.Context, document operatorsettings.ConfigDocument) error {
	return service.configureIntegrations(ctx, document.FileConfig().Workers.ACP.Integrations)
}

func (service Service) configureIntegrations(ctx context.Context, configured []operatorsettings.ACPIntegration) error {
	configurator, ok := service.Providers.(providers.ACPConfiguration)
	if !ok {
		return fmt.Errorf("Providers ACP configuration role is required")
	}
	integrations := make([]providers.ACPIntegration, len(configured))
	for index, value := range configured {
		integrations[index] = providers.ACPIntegration{ID: value.ID, Name: providers.ID(value.Name), Transport: value.Transport, Command: value.Command}
	}
	return configurator.ConfigureACPIntegrations(ctx, integrations)
}

func filterACPProviders(
	result providers.ListProvidersResult,
	configured []operatorsettings.ACPIntegration,
) providers.ListProvidersResult {
	identities := make(map[providers.ID]struct{}, len(configured))
	for _, integration := range configured {
		identities[providers.ID(integration.Name)] = struct{}{}
	}
	filtered := make([]providers.Descriptor, 0, len(result.Providers))
	for _, descriptor := range result.Providers {
		_, configuredProvider := identities[descriptor.ID]
		if configuredProvider || strings.HasSuffix(descriptor.ID.String(), "-acp") {
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
