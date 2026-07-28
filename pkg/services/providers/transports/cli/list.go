package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
)

// ListConfig holds parameters for the providers list command.
type ListConfig struct {
	Context     context.Context
	JSON        bool
	Verbose     bool
	Output      io.Writer
	Diagnostics io.Writer
}

// List delegates catalog list intent to the Providers-owned CLI adapter Service
// and surfaces typed results and cancellation failures for CLI consumption.
func List(cfg ListConfig, root providers.Service) error {
	adapter := New(root)
	if adapter == nil {
		return fmt.Errorf("providers service is required")
	}
	return adapter.List(cfg)
}

func (service *service) List(cfg ListConfig) error {
	if cfg.Context == nil {
		return fmt.Errorf("context is required")
	}
	if cfg.Output == nil {
		return fmt.Errorf("output writer is required")
	}
	if err := cfg.Context.Err(); err != nil {
		return err
	}
	clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "providers list request")
	result, err := service.root.ListProviders(cfg.Context, providers.ListProvidersRequest{})
	if err != nil {
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "providers list failed err=%v", err)
		return err
	}
	clidiag.Printf(
		cfg.Diagnostics,
		cfg.Verbose,
		"providers list complete count=%d",
		len(result.Providers),
	)
	if cfg.JSON {
		return json.NewEncoder(cfg.Output).Encode(listResultToJSON(result))
	}
	return renderListResult(cfg.Output, result)
}

type listJSONResult struct {
	Providers []listJSONProvider `json:"providers"`
}

type listJSONProvider struct {
	ID            string                 `json:"id"`
	DisplayName   string                 `json:"displayName,omitempty"`
	Aliases       []string               `json:"aliases,omitempty"`
	Availability  string                 `json:"availability"`
	Readiness     string                 `json:"readiness"`
	Capabilities  []string               `json:"capabilities,omitempty"`
	Prerequisites []listJSONPrerequisite `json:"prerequisites,omitempty"`
}

type listJSONPrerequisite struct {
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Status      string `json:"status"`
	Description string `json:"description,omitempty"`
}

func listResultToJSON(result providers.ListProvidersResult) listJSONResult {
	providers := append([]providers.Descriptor(nil), result.Providers...)
	sortProviders(providers)
	entries := make([]listJSONProvider, 0, len(providers))
	for _, provider := range providers {
		entries = append(entries, descriptorToJSON(provider))
	}
	return listJSONResult{Providers: entries}
}

func descriptorToJSON(descriptor providers.Descriptor) listJSONProvider {
	entry := listJSONProvider{
		ID:           descriptor.ID.String(),
		DisplayName:  descriptor.DisplayName,
		Availability: string(descriptor.Availability),
		Readiness:    string(descriptor.Readiness),
	}
	if len(descriptor.Aliases) > 0 {
		entry.Aliases = append([]string(nil), descriptor.Aliases...)
	}
	if len(descriptor.Capabilities) > 0 {
		entry.Capabilities = make([]string, 0, len(descriptor.Capabilities))
		for _, capability := range descriptor.Capabilities {
			entry.Capabilities = append(entry.Capabilities, string(capability))
		}
	}
	if len(descriptor.Prerequisites) > 0 {
		entry.Prerequisites = make([]listJSONPrerequisite, 0, len(descriptor.Prerequisites))
		for _, prerequisite := range descriptor.Prerequisites {
			entry.Prerequisites = append(entry.Prerequisites, listJSONPrerequisite{
				Kind:        string(prerequisite.Kind),
				Name:        prerequisite.Name,
				Status:      string(prerequisite.Status),
				Description: prerequisite.Description,
			})
		}
	}
	return entry
}

func renderListResult(output io.Writer, result providers.ListProvidersResult) error {
	if _, err := fmt.Fprintln(output, "ID\tDISPLAY NAME\tAVAILABILITY\tREADINESS\tALIASES"); err != nil {
		return err
	}
	providers := append([]providers.Descriptor(nil), result.Providers...)
	sortProviders(providers)
	for _, provider := range providers {
		if _, err := fmt.Fprintf(
			output,
			"%s\t%s\t%s\t%s\t%s\n",
			provider.ID.String(),
			provider.DisplayName,
			provider.Availability,
			provider.Readiness,
			formatProviderAliases(provider.Aliases),
		); err != nil {
			return err
		}
	}
	return nil
}

func sortProviders(descriptors []providers.Descriptor) {
	sort.Slice(descriptors, func(i, j int) bool {
		return descriptors[i].ID.String() < descriptors[j].ID.String()
	})
}

func formatProviderAliases(aliases []string) string {
	if len(aliases) == 0 {
		return "none"
	}
	return strings.Join(aliases, ", ")
}
