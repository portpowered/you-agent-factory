package factory

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

const absentFactoryLocation = "-"

// ListConfig holds parameters for the factory list command.
type ListConfig struct {
	Context     context.Context
	ProjectRoot string
	GlobalRoot  string
	JSON        bool
	Output      io.Writer
	Diagnostics io.Writer
}

// ListEntry is the detached CLI representation of one effective Factory.
type ListEntry struct {
	Name              string `json:"name"`
	FactoryDirectory  string `json:"factoryDirectory"`
	Current           bool   `json:"current"`
	Description       string `json:"description,omitempty"`
	InvocationExample string `json:"invocationExample,omitempty"`
}

// NewList binds the effective Factory catalog and current-pointer reader to the
// CLI representation.
func NewList(
	catalog factorydefinitions.EffectiveFactoryCatalogOperation,
	readCurrent factorydefinitions.CurrentFactoryPointerReader,
) func(ListConfig) error {
	return func(cfg ListConfig) error {
		return List(catalog, readCurrent, cfg)
	}
}

// List prints the precedence-resolved effective Factory catalog.
func List(
	catalog factorydefinitions.EffectiveFactoryCatalogOperation,
	readCurrent factorydefinitions.CurrentFactoryPointerReader,
	cfg ListConfig,
) error {
	if err := validateListConfig(catalog, readCurrent, cfg); err != nil {
		return err
	}
	ctx := cfg.Context
	if err := ctx.Err(); err != nil {
		return err
	}

	result, err := catalog(ctx, factorydefinitions.ListEffectiveFactoriesRequest{
		ProjectRoot: cfg.ProjectRoot,
		GlobalRoot:  cfg.GlobalRoot,
	})
	if err != nil {
		return err
	}
	current, err := effectiveCurrentFactoryName(ctx, readCurrent, cfg.ProjectRoot, cfg.GlobalRoot)
	if err != nil {
		return err
	}
	entries, err := ProjectEffectiveFactoryList(ctx, result, current)
	if err != nil {
		return err
	}
	return renderListResult(ctx, entries, result.Diagnostics, cfg)
}

// ProjectEffectiveFactoryList maps one service-owned catalog result to the
// detached list representation without performing discovery.
func ProjectEffectiveFactoryList(
	ctx context.Context,
	result factorydefinitions.ListEffectiveFactoriesResult,
	current string,
) ([]ListEntry, error) {
	if ctx == nil {
		return nil, fmt.Errorf("context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return projectListEntries(ctx, result.Entries, current)
}

func validateListConfig(
	catalog factorydefinitions.EffectiveFactoryCatalogOperation,
	readCurrent factorydefinitions.CurrentFactoryPointerReader,
	cfg ListConfig,
) error {
	if cfg.Output == nil {
		return fmt.Errorf("output writer is required")
	}
	if strings.TrimSpace(cfg.ProjectRoot) == "" {
		return fmt.Errorf("project Factory root is required")
	}
	if strings.TrimSpace(cfg.GlobalRoot) == "" {
		return fmt.Errorf("global Factory root is required")
	}
	if catalog == nil {
		return fmt.Errorf("Factory Definitions effective catalog is required")
	}
	if readCurrent == nil {
		return fmt.Errorf("Factory Definitions current-pointer reader is required")
	}
	if cfg.Context == nil {
		return fmt.Errorf("context is required")
	}
	return nil
}

func renderListResult(
	ctx context.Context,
	entries []ListEntry,
	catalogDiagnostics []factorydefinitions.EffectiveFactoryCatalogDiagnostic,
	cfg ListConfig,
) error {
	var output bytes.Buffer
	var err error
	if cfg.JSON {
		err = json.NewEncoder(&output).Encode(entries)
	} else {
		err = renderFactoryList(entries, &output)
	}
	if err != nil {
		return err
	}
	var diagnostics bytes.Buffer
	if err := renderCatalogDiagnostics(ctx, catalogDiagnostics, &diagnostics); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := io.Copy(cfg.Output, &output); err != nil {
		return err
	}
	if cfg.Diagnostics != nil {
		_, err = io.Copy(cfg.Diagnostics, &diagnostics)
	}
	return err
}

func effectiveCurrentFactoryName(
	ctx context.Context,
	readCurrent factorydefinitions.CurrentFactoryPointerReader,
	projectRoot string,
	globalRoot string,
) (string, error) {
	for _, root := range []string{projectRoot, globalRoot} {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		name, err := readCurrent(root)
		if contextErr := ctx.Err(); contextErr != nil {
			return "", contextErr
		}
		if err == nil {
			return name, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("read current Factory selection: %w", err)
		}
	}
	return "", nil
}

func projectListEntries(
	ctx context.Context,
	catalog []factorydefinitions.EffectiveFactoryCatalogEntry,
	current string,
) ([]ListEntry, error) {
	entries := make([]ListEntry, 0, len(catalog))
	for _, entry := range catalog {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		location := absentFactoryLocation
		if entry.Location != nil {
			location = *entry.Location
		}
		entries = append(entries, ListEntry{
			Name:              entry.Name,
			FactoryDirectory:  location,
			Current:           entry.Name == current,
			Description:       effectiveFactoryDescription(entry.Definition),
			InvocationExample: effectiveFactoryInvocationExample(entry),
		})
	}
	return entries, nil
}

func effectiveFactoryDescription(definition *factorydefinitions.FactoryConfig) string {
	if definition == nil || definition.Description == nil {
		return ""
	}
	return strings.TrimSpace(definition.Description.Value)
}

func effectiveFactoryInvocationExample(
	entry factorydefinitions.EffectiveFactoryCatalogEntry,
) string {
	if entry.InvocationSignature == nil {
		return ""
	}
	var example strings.Builder
	example.WriteString("you run --named ")
	example.WriteString(entry.Name)
	for _, parameter := range entry.InvocationSignature.Parameters {
		if !parameter.Required {
			continue
		}
		appendInvocationParameterExample(&example, parameter)
	}
	return example.String()
}

func appendInvocationParameterExample(
	example *strings.Builder,
	parameter factorydefinitions.InvocationParameterConfig,
) {
	placeholder := "<" + parameter.Name + ">"
	for _, binding := range parameter.Bindings {
		switch binding.Kind {
		case work.InvocationParameterBindingKindNamed:
			externalName := strings.TrimSpace(parameter.ExternalName)
			if externalName == "" {
				externalName = parameter.Name
			}
			example.WriteString(" --")
			example.WriteString(externalName)
			example.WriteString(" ")
			example.WriteString(placeholder)
			return
		case work.InvocationParameterBindingKindPositional:
			example.WriteString(" ")
			example.WriteString(placeholder)
			return
		case work.InvocationParameterBindingKindStdin:
			example.WriteString(" < ")
			example.WriteString(placeholder)
			return
		}
	}
}

func renderCatalogDiagnostics(
	ctx context.Context,
	diagnostics []factorydefinitions.EffectiveFactoryCatalogDiagnostic,
	output io.Writer,
) error {
	for _, diagnostic := range diagnostics {
		if err := ctx.Err(); err != nil {
			return err
		}
		name := diagnostic.Name
		if name == "" {
			name = "unknown"
		}
		if _, err := fmt.Fprintf(
			output,
			"Factory catalog %s %s (%s): %s\n",
			diagnostic.Source,
			name,
			diagnostic.Code,
			diagnostic.Message,
		); err != nil {
			return err
		}
	}
	return nil
}

func renderFactoryList(entries []ListEntry, output io.Writer) error {
	if len(entries) == 0 {
		_, err := fmt.Fprintln(output, "No factories found.")
		return err
	}

	if _, err := fmt.Fprintln(output, "NAME\tFACTORY DIRECTORY\tCURRENT"); err != nil {
		return err
	}
	for _, entry := range entries {
		current := ""
		if entry.Current {
			current = "yes"
		}
		if _, err := fmt.Fprintf(
			output,
			"%s\t%s\t%s\n",
			entry.Name,
			entry.FactoryDirectory,
			current,
		); err != nil {
			return err
		}
	}
	return nil
}
