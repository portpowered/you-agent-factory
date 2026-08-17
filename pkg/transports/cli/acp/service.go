// Package acp contains protocol-facing ACP CLI value types and validation.
// Cross-service configuration and catalog composition are injected by Wire as
// owner operations; this package does not join Operator Settings and
// Providers roots.
package acp

import (
	"context"
	"fmt"
	"strings"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/services/providers"
)

type WorkerCatalog struct {
	Providers []providers.Descriptor
	ACP       map[providers.ID]bool
	Custom    map[providers.ID]bool
}

type ListWorkersOperation func(context.Context, string) (WorkerCatalog, error)
type ConfigureOperation func(context.Context, []operatorsettings.ACPIntegration) error
type AddOperation func(context.Context, string, string, string, string) error
type DeleteOperation func(context.Context, string, string) error

// Operations is the CLI's injected ACP operation surface. Wire composes these
// operations from the owner roots; command handlers only parse flags and
// invoke one operation.
type Operations struct {
	ListWorkersOperation ListWorkersOperation
	ConfigureOperation   ConfigureOperation
	AddOperation         AddOperation
	DeleteOperation      DeleteOperation
}

func (operations Operations) ListWorkers(ctx context.Context, home string) (WorkerCatalog, error) {
	if operations.ListWorkersOperation == nil {
		return WorkerCatalog{}, fmt.Errorf("Providers service is required")
	}
	return operations.ListWorkersOperation(ctx, home)
}

func (operations Operations) Configure(ctx context.Context, configured []operatorsettings.ACPIntegration) error {
	if operations.ConfigureOperation == nil {
		return fmt.Errorf("ACP configuration operation is required")
	}
	return operations.ConfigureOperation(ctx, configured)
}

func (operations Operations) Add(ctx context.Context, home, name, transport, command string) error {
	if operations.AddOperation == nil {
		return fmt.Errorf("ACP integration ID generator is required")
	}
	return operations.AddOperation(ctx, home, name, transport, command)
}

func (operations Operations) Delete(ctx context.Context, home, name string) error {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	if operations.DeleteOperation == nil {
		return fmt.Errorf("Operator Settings service is required")
	}
	return operations.DeleteOperation(ctx, home, name)
}

// FilterACPProviders retains descriptors selected by the effective ACP
// configuration. Providers owns the effective application; this function only
// projects its detached catalog for the existing CLI table shape.
func FilterACPProviders(
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

// Kept private for the package's focused projection characterization.
func filterACPProviders(
	result providers.ListProvidersResult,
	configured []operatorsettings.ACPIntegration,
) providers.ListProvidersResult {
	return FilterACPProviders(result, configured)
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
