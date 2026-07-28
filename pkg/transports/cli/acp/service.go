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

func (service Service) List(ctx context.Context, home string) (providers.ListResponse, error) {
	if service.ProvidersFactory == nil {
		return providers.ListResponse{}, fmt.Errorf("Providers factory is required")
	}
	document, err := service.Settings.Load(operatorsettings.DefaultConfigPath(home))
	if err != nil {
		return providers.ListResponse{}, err
	}
	configured := document.FileConfig().Workers.ACP.Integrations
	integrations := make([]providers.Integration, len(configured))
	for index, value := range configured {
		integrations[index] = providers.Integration{
			ID: value.ID, Name: providers.ID(value.Name), Transport: value.Transport, Command: value.Command,
		}
	}
	root, err := service.ProvidersFactory(integrations)
	if err != nil {
		return providers.ListResponse{}, err
	}
	return root.List(ctx, providers.ListRequest{})
}

func ValidateAdd(name, transport, command string) error {
	if err := providers.ID(strings.TrimSpace(name)).Validate(); err != nil {
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
