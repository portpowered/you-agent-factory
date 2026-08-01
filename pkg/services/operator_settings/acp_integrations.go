package operatorsettings

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var ErrACPIntegrationNotFound = errors.New("ACP integration not found")

// AddACPIntegration returns a validated document with one additional ACP
// provider while preserving all unrelated operator settings.
func (service ConfigDocumentService) AddACPIntegration(document ConfigDocument, integration ACPIntegration) (ConfigDocument, error) {
	config := document.FileConfig()
	config.Workers.ACP.Integrations = append(config.Workers.ACP.Integrations, integration)
	normalized, err := config.Normalize()
	if err != nil {
		return ConfigDocument{}, err
	}
	return ConfigDocument{config: normalized}, nil
}

// DeleteACPIntegration removes one integration by canonical provider name.
func (service ConfigDocumentService) DeleteACPIntegration(document ConfigDocument, name string) (ConfigDocument, error) {
	name = strings.TrimSpace(name)
	config := document.FileConfig()
	filtered := make([]ACPIntegration, 0, len(config.Workers.ACP.Integrations))
	found := false
	for _, integration := range config.Workers.ACP.Integrations {
		if integration.Name == name {
			found = true
			continue
		}
		filtered = append(filtered, integration)
	}
	if !found {
		return ConfigDocument{}, fmt.Errorf("%w: %q", ErrACPIntegrationNotFound, name)
	}
	config.Workers.ACP.Integrations = filtered
	normalized, err := config.Normalize()
	if err != nil {
		return ConfigDocument{}, err
	}
	return ConfigDocument{config: normalized}, nil
}

// ConfigureACPIntegrationAdd loads, updates, and persists one ACP integration.
func (service ConfigDocumentService) ConfigureACPIntegrationAdd(ctx context.Context, path string, integration ACPIntegration) (ConfigDocument, error) {
	return service.configureACPIntegrations(ctx, path, func(document ConfigDocument) (ConfigDocument, error) {
		return service.AddACPIntegration(document, integration)
	})
}

// ConfigureACPIntegrationDelete loads, removes, and persists one ACP integration.
func (service ConfigDocumentService) ConfigureACPIntegrationDelete(ctx context.Context, path, name string) (ConfigDocument, error) {
	return service.configureACPIntegrations(ctx, path, func(document ConfigDocument) (ConfigDocument, error) {
		return service.DeleteACPIntegration(document, name)
	})
}

// EnsurePackagedACPIntegrations materializes defaults only when the ACP
// integration list is absent. An explicitly present list, including an empty
// list, is customer-owned and remains unchanged.
func (service ConfigDocumentService) EnsurePackagedACPIntegrations(ctx context.Context, path string, defaults []ACPIntegration) (ConfigDocument, error) {
	return service.configureACPIntegrations(ctx, path, func(document ConfigDocument) (ConfigDocument, error) {
		config := document.FileConfig()
		if config.Workers.ACP.Integrations != nil {
			return document, nil
		}
		config.Workers.ACP.Integrations = append([]ACPIntegration(nil), defaults...)
		normalized, err := config.Normalize()
		if err != nil {
			return ConfigDocument{}, err
		}
		return ConfigDocument{config: normalized}, nil
	})
}

func (service ConfigDocumentService) configureACPIntegrations(
	ctx context.Context,
	path string,
	update func(ConfigDocument) (ConfigDocument, error),
) (ConfigDocument, error) {
	if err := operationContextError(ctx); err != nil {
		return ConfigDocument{}, err
	}
	if err := ctx.Err(); err != nil {
		return ConfigDocument{}, err
	}
	document, err := service.Load(path)
	if err != nil {
		return ConfigDocument{}, err
	}
	candidate, err := update(document)
	if err != nil {
		return ConfigDocument{}, err
	}
	if err := service.Persist(ctx, path, candidate); err != nil {
		return ConfigDocument{}, err
	}
	return candidate, nil
}
