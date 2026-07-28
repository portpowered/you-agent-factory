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

// ShowConfig holds parameters for the providers show command.
type ShowConfig struct {
	Context     context.Context
	ProviderID  string
	JSON        bool
	Verbose     bool
	Output      io.Writer
	Diagnostics io.Writer
}

// Show delegates catalog show intent to the Providers-owned CLI adapter Service
// and surfaces typed results and cancellation failures for CLI consumption.
func Show(cfg ShowConfig, root providers.Service) error {
	adapter := New(root)
	if adapter == nil {
		return fmt.Errorf("providers service is required")
	}
	return adapter.Show(cfg)
}

func (service *service) Show(cfg ShowConfig) error {
	if cfg.Context == nil {
		return fmt.Errorf("context is required")
	}
	if cfg.Output == nil {
		return fmt.Errorf("output writer is required")
	}
	if err := cfg.Context.Err(); err != nil {
		return err
	}
	providerID := providers.ID(strings.TrimSpace(cfg.ProviderID))
	clidiag.Printf(
		cfg.Diagnostics,
		cfg.Verbose,
		"providers show request providerId=%s",
		providerID,
	)
	result, err := service.root.GetProvider(cfg.Context, providers.GetProviderRequest{ID: providerID})
	if err != nil {
		clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "providers show failed err=%v", err)
		return err
	}
	clidiag.Printf(
		cfg.Diagnostics,
		cfg.Verbose,
		"providers show complete providerId=%s",
		result.Provider.ID,
	)
	if cfg.JSON {
		return json.NewEncoder(cfg.Output).Encode(descriptorToJSON(result.Provider))
	}
	return renderShowResult(cfg.Output, result.Provider)
}

func renderShowResult(output io.Writer, descriptor providers.Descriptor) error {
	rows := []struct {
		label string
		value string
	}{
		{label: "ID", value: descriptor.ID.String()},
		{label: "Display name", value: descriptor.DisplayName},
		{label: "Availability", value: string(descriptor.Availability)},
		{label: "Readiness", value: string(descriptor.Readiness)},
		{label: "Aliases", value: formatProviderAliases(descriptor.Aliases)},
		{label: "Capabilities", value: formatProviderCapabilities(descriptor.Capabilities)},
		{label: "Prerequisites", value: formatProviderPrerequisites(descriptor.Prerequisites)},
	}
	for _, row := range rows {
		if _, err := fmt.Fprintf(output, "%s:\t%s\n", row.label, row.value); err != nil {
			return err
		}
	}
	return nil
}

func formatProviderCapabilities(capabilities []providers.Capability) string {
	if len(capabilities) == 0 {
		return "none"
	}
	names := make([]string, 0, len(capabilities))
	for _, capability := range capabilities {
		names = append(names, string(capability))
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

func formatProviderPrerequisites(prerequisites []providers.Prerequisite) string {
	if len(prerequisites) == 0 {
		return "none"
	}
	entries := make([]string, 0, len(prerequisites))
	for _, prerequisite := range prerequisites {
		entry := fmt.Sprintf(
			"%s/%s=%s",
			prerequisite.Kind,
			prerequisite.Name,
			prerequisite.Status,
		)
		if strings.TrimSpace(prerequisite.Description) != "" {
			entry += " (" + prerequisite.Description + ")"
		}
		entries = append(entries, entry)
	}
	sort.Strings(entries)
	return strings.Join(entries, "; ")
}
