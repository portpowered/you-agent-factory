package cli

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

type listJSONResult struct {
	Providers []listJSONProvider `json:"providers"`
}

type listJSONProvider struct {
	ID                         string                 `json:"id"`
	DisplayName                string                 `json:"displayName"`
	Aliases                    []string               `json:"aliases"`
	Availability               string                 `json:"availability"`
	Readiness                  string                 `json:"readiness"`
	TechnicalSupportLevel      string                 `json:"technicalSupportLevel"`
	ImplementationAvailability string                 `json:"implementationAvailability"`
	Prerequisites              []listJSONPrerequisite `json:"prerequisites"`
	Models                     []listJSONModel        `json:"models"`
	Tools                      []listJSONTool         `json:"tools"`
	KnownLimits                []listJSONKnownLimit   `json:"knownLimits"`
	Capabilities               []string               `json:"capabilities"`
}

type listJSONPrerequisite struct {
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Status      string `json:"status"`
	Description string `json:"description"`
}

type listJSONModel struct {
	ID         string             `json:"id"`
	Efforts    []string           `json:"efforts"`
	Modalities []listJSONModality `json:"modalities"`
}

type listJSONModality struct {
	Direction string `json:"direction"`
	Modality  string `json:"modality"`
	Support   string `json:"support"`
	Transport string `json:"transport"`
}

type listJSONTool struct {
	Name        string `json:"name"`
	Support     string `json:"support"`
	Description string `json:"description"`
}

type listJSONKnownLimit struct {
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Unit        string `json:"unit"`
	Description string `json:"description"`
	Maximum     *int64 `json:"maximum"`
	Default     *int64 `json:"default"`
	Value       string `json:"value"`
}

func listResultToJSON(result providers.ListProvidersResult) listJSONResult {
	descriptors := orderedDescriptors(result.Providers)
	entries := make([]listJSONProvider, 0, len(descriptors))
	for _, descriptor := range descriptors {
		entries = append(entries, descriptorToListJSON(descriptor))
	}
	return listJSONResult{Providers: entries}
}

func descriptorToListJSON(descriptor providers.Descriptor) listJSONProvider {
	entry := listJSONProvider{
		ID:                         descriptor.ID.String(),
		DisplayName:                descriptor.DisplayName,
		Aliases:                    append([]string{}, descriptor.Aliases...),
		Availability:               string(descriptor.Availability),
		Readiness:                  string(descriptor.Readiness),
		TechnicalSupportLevel:      string(descriptor.TechnicalSupportLevel),
		ImplementationAvailability: string(descriptor.ImplementationAvailability),
		Prerequisites:              make([]listJSONPrerequisite, 0, len(descriptor.Prerequisites)),
		Models:                     make([]listJSONModel, 0, len(descriptor.Models)),
		Tools:                      make([]listJSONTool, 0, len(descriptor.Tools)),
		KnownLimits:                make([]listJSONKnownLimit, 0, len(descriptor.KnownLimits)),
		Capabilities:               make([]string, 0, len(descriptor.Capabilities)),
	}
	for _, prerequisite := range descriptor.Prerequisites {
		entry.Prerequisites = append(entry.Prerequisites, listJSONPrerequisite{
			Kind:        string(prerequisite.Kind),
			Name:        prerequisite.Name,
			Status:      string(prerequisite.Status),
			Description: prerequisite.Description,
		})
	}
	for _, model := range descriptor.Models {
		jsonModel := listJSONModel{
			ID:         model.ID,
			Efforts:    make([]string, 0, len(model.Efforts)),
			Modalities: make([]listJSONModality, 0, len(model.Modalities)),
		}
		for _, effort := range model.Efforts {
			jsonModel.Efforts = append(jsonModel.Efforts, string(effort))
		}
		for _, modality := range model.Modalities {
			jsonModel.Modalities = append(jsonModel.Modalities, listJSONModality{
				Direction: string(modality.Direction),
				Modality:  string(modality.Kind),
				Support:   string(modality.Support),
				Transport: string(modality.Transport),
			})
		}
		entry.Models = append(entry.Models, jsonModel)
	}
	for _, tool := range descriptor.Tools {
		entry.Tools = append(entry.Tools, listJSONTool{
			Name:        tool.Name,
			Support:     string(tool.Support),
			Description: tool.Description,
		})
	}
	for _, limit := range descriptor.KnownLimits {
		entry.KnownLimits = append(entry.KnownLimits, listJSONKnownLimit{
			Name:        limit.Name,
			Kind:        string(limit.Kind),
			Unit:        limit.Unit,
			Description: limit.Description,
			Maximum:     cloneInt64(limit.Maximum),
			Default:     cloneInt64(limit.Default),
			Value:       limit.Value,
		})
	}
	for _, capability := range descriptor.Capabilities {
		entry.Capabilities = append(entry.Capabilities, string(capability))
	}
	return entry
}

func renderListResult(ctx context.Context, output io.Writer, result providers.ListProvidersResult) error {
	if _, err := fmt.Fprintln(output, "ID\tDISPLAY NAME\tAVAILABILITY\tREADINESS\tALIASES"); err != nil {
		return err
	}
	for _, descriptor := range orderedDescriptors(result.Providers) {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := renderProviderSummary(output, descriptor); err != nil {
			return err
		}
		if err := renderProviderDetails(ctx, output, descriptor); err != nil {
			return err
		}
	}
	return ctx.Err()
}

func renderProviderSummary(output io.Writer, descriptor providers.Descriptor) error {
	_, err := fmt.Fprintf(
		output,
		"%s\t%s\t%s\t%s\t%s\n",
		descriptor.ID.String(),
		descriptor.DisplayName,
		descriptor.Availability,
		descriptor.Readiness,
		formatProviderAliases(descriptor.Aliases),
	)
	return err
}

func renderProviderDetails(ctx context.Context, output io.Writer, descriptor providers.Descriptor) error {
	rows := []struct {
		label string
		value string
	}{
		{label: "  Technical support", value: string(descriptor.TechnicalSupportLevel)},
		{label: "  Implementation", value: string(descriptor.ImplementationAvailability)},
		{label: "  Capabilities", value: formatListProviderCapabilities(descriptor.Capabilities)},
	}
	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(output, "%s:\t%s\n", row.label, valueOrNone(row.value)); err != nil {
			return err
		}
	}
	if err := renderPrerequisites(ctx, output, descriptor.Prerequisites); err != nil {
		return err
	}
	if err := renderModels(ctx, output, descriptor.Models); err != nil {
		return err
	}
	if err := renderTools(ctx, output, descriptor.Tools); err != nil {
		return err
	}
	return renderKnownLimits(ctx, output, descriptor.KnownLimits)
}

func renderPrerequisites(ctx context.Context, output io.Writer, prerequisites []providers.Prerequisite) error {
	if _, err := fmt.Fprintln(output, "  Prerequisites:"); err != nil {
		return err
	}
	if len(prerequisites) == 0 {
		_, err := fmt.Fprintln(output, "    - none")
		return err
	}
	for _, prerequisite := range prerequisites {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(
			output,
			"    - %s/%s: %s — %s\n",
			prerequisite.Kind,
			prerequisite.Name,
			prerequisite.Status,
			oneLine(prerequisite.Description),
		); err != nil {
			return err
		}
	}
	return nil
}

func renderModels(ctx context.Context, output io.Writer, models []providers.ModelDescriptor) error {
	if _, err := fmt.Fprintln(output, "  Models:"); err != nil {
		return err
	}
	if len(models) == 0 {
		_, err := fmt.Fprintln(output, "    - none")
		return err
	}
	for _, model := range models {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(output, "    - %s\n", model.ID); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(output, "      Efforts:\t%s\n", formatEfforts(model.Efforts)); err != nil {
			return err
		}
		if err := renderModalities(ctx, output, model.Modalities); err != nil {
			return err
		}
	}
	return nil
}

func renderModalities(ctx context.Context, output io.Writer, modalities []providers.Modality) error {
	if err := renderModalityDirection(ctx, output, "Input modalities", modalities, providers.ModalityDirectionInput); err != nil {
		return err
	}
	if err := renderModalityDirection(ctx, output, "Output modalities", modalities, providers.ModalityDirectionOutput); err != nil {
		return err
	}
	return nil
}

func renderModalityDirection(
	ctx context.Context,
	output io.Writer,
	label string,
	modalities []providers.Modality,
	direction providers.ModalityDirection,
) error {
	if _, err := fmt.Fprintf(output, "      %s:\n", label); err != nil {
		return err
	}
	found := false
	for _, modality := range modalities {
		if modality.Direction != direction {
			continue
		}
		found = true
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(
			output,
			"        - %s: %s (transport: %s)\n",
			modality.Kind,
			modality.Support,
			modality.Transport,
		); err != nil {
			return err
		}
	}
	if !found {
		_, err := fmt.Fprintln(output, "        - none")
		return err
	}
	return nil
}

func renderTools(ctx context.Context, output io.Writer, tools []providers.Tool) error {
	if _, err := fmt.Fprintln(output, "  Tools:"); err != nil {
		return err
	}
	if len(tools) == 0 {
		_, err := fmt.Fprintln(output, "    - none")
		return err
	}
	for _, tool := range tools {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(
			output,
			"    - %s: %s — %s\n",
			tool.Name,
			tool.Support,
			oneLine(tool.Description),
		); err != nil {
			return err
		}
	}
	return nil
}

func renderKnownLimits(ctx context.Context, output io.Writer, limits []providers.KnownLimit) error {
	if _, err := fmt.Fprintln(output, "  Known limits:"); err != nil {
		return err
	}
	if len(limits) == 0 {
		_, err := fmt.Fprintln(output, "    - none")
		return err
	}
	for _, limit := range limits {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(
			output,
			"    - %s [%s, %s] %s — %s\n",
			limit.Name,
			limit.Kind,
			limit.Unit,
			formatKnownLimitValue(limit),
			oneLine(limit.Description),
		); err != nil {
			return err
		}
	}
	return nil
}

func orderedDescriptors(descriptors []providers.Descriptor) []providers.Descriptor {
	ordered := make([]providers.Descriptor, len(descriptors))
	for index, descriptor := range descriptors {
		ordered[index] = descriptor.Clone()
		normalizeDescriptor(&ordered[index])
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].ID.String() < ordered[j].ID.String()
	})
	return ordered
}

func normalizeDescriptor(descriptor *providers.Descriptor) {
	sort.Strings(descriptor.Aliases)
	sort.SliceStable(descriptor.Prerequisites, func(i, j int) bool {
		return prerequisiteSortKey(descriptor.Prerequisites[i]) < prerequisiteSortKey(descriptor.Prerequisites[j])
	})
	sort.SliceStable(descriptor.Models, func(i, j int) bool {
		return descriptor.Models[i].ID < descriptor.Models[j].ID
	})
	for index := range descriptor.Models {
		sort.SliceStable(descriptor.Models[index].Efforts, func(i, j int) bool {
			return effortSortKey(descriptor.Models[index].Efforts[i]) < effortSortKey(descriptor.Models[index].Efforts[j])
		})
		sort.SliceStable(descriptor.Models[index].Modalities, func(i, j int) bool {
			return modalitySortKey(descriptor.Models[index].Modalities[i]) < modalitySortKey(descriptor.Models[index].Modalities[j])
		})
	}
	sort.SliceStable(descriptor.Tools, func(i, j int) bool {
		return descriptor.Tools[i].Name < descriptor.Tools[j].Name
	})
	sort.SliceStable(descriptor.KnownLimits, func(i, j int) bool {
		return descriptor.KnownLimits[i].Name < descriptor.KnownLimits[j].Name
	})
	sort.SliceStable(descriptor.Capabilities, func(i, j int) bool {
		return descriptor.Capabilities[i] < descriptor.Capabilities[j]
	})
}

func prerequisiteSortKey(value providers.Prerequisite) string {
	return strings.Join([]string{string(value.Kind), value.Name, string(value.Status), value.Description}, "\x00")
}

func effortSortKey(value providers.ReasoningEffort) string {
	for index, candidate := range []providers.ReasoningEffort{
		"minimal", "low", "medium", "high", "xhigh", "max",
	} {
		if value == candidate {
			return fmt.Sprintf("%02d", index)
		}
	}
	return "99:" + string(value)
}

func modalitySortKey(value providers.Modality) string {
	return strings.Join([]string{string(value.Direction), string(value.Kind), string(value.Support), string(value.Transport)}, "\x00")
}

func formatProviderAliases(aliases []string) string {
	if len(aliases) == 0 {
		return "none"
	}
	return strings.Join(aliases, ", ")
}

func formatListProviderCapabilities(capabilities []providers.Capability) string {
	if len(capabilities) == 0 {
		return "none"
	}
	values := make([]string, 0, len(capabilities))
	for _, capability := range capabilities {
		values = append(values, string(capability))
	}
	return strings.Join(values, ", ")
}

func formatEfforts(efforts []providers.ReasoningEffort) string {
	if len(efforts) == 0 {
		return "none"
	}
	values := make([]string, 0, len(efforts))
	for _, effort := range efforts {
		values = append(values, string(effort))
	}
	return strings.Join(values, ", ")
}

func formatKnownLimitValue(limit providers.KnownLimit) string {
	switch limit.Kind {
	case providers.KnownLimitMaximum:
		if limit.Maximum != nil {
			return fmt.Sprintf("maximum=%d", *limit.Maximum)
		}
	case providers.KnownLimitDefault:
		if limit.Default != nil {
			return fmt.Sprintf("default=%d", *limit.Default)
		}
	case providers.KnownLimitBehavior:
		if strings.TrimSpace(limit.Value) != "" {
			return "value=" + limit.Value
		}
	}
	return "value=none"
}

func valueOrNone(value string) string {
	if strings.TrimSpace(value) == "" {
		return "none"
	}
	return value
}

func oneLine(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
