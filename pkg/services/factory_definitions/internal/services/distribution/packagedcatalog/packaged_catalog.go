package packagedcatalog

import (
	"context"
	"fmt"
	"sort"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// Catalog owns deterministic public-name projection and selection over one
// already-validated embedded packaged Factory catalog.
type Catalog struct {
	definitions []factorydefinitions.PackagedDefinition
}

// NewCatalog constructs the direct catalog port consumed by the focused
// Packaging capability. It stores detached source and integrity facts in
// deterministic public-name order.
func NewCatalog(
	definitions []factorydefinitions.PackagedDefinition,
) (factorydefinitions.PackagedFactoryCatalog, error) {
	cloned := make([]factorydefinitions.PackagedDefinition, len(definitions))
	for index, definition := range definitions {
		cloned[index] = cloneDefinition(definition)
	}
	sort.Slice(cloned, func(i, j int) bool {
		return cloned[i].Name < cloned[j].Name
	})
	for index, definition := range cloned {
		if strings.TrimSpace(definition.Name) == "" {
			return nil, fmt.Errorf("construct packaged Factory catalog: public name is required")
		}
		if index > 0 && cloned[index-1].Name == definition.Name {
			return nil, fmt.Errorf(
				"construct packaged Factory catalog: duplicate public name %q",
				definition.Name,
			)
		}
		if len(definition.Formats) == 0 {
			return nil, fmt.Errorf(
				"construct packaged Factory catalog: %q has no published formats",
				definition.Name,
			)
		}
	}
	return &Catalog{definitions: cloned}, nil
}

// New retains the legacy callback bundle for compatibility callers. New
// consumers receive the direct PackagedFactoryCatalog port through NewCatalog.
func New(
	definitions []factorydefinitions.PackagedDefinition,
) (factorydefinitions.PackagedFactoryCatalogOperations, error) {
	catalog, err := NewCatalog(definitions)
	if err != nil {
		return factorydefinitions.PackagedFactoryCatalogOperations{}, err
	}
	return factorydefinitions.PackagedFactoryCatalogOperations{
		List:    catalog.ListBuiltInPackagedFactories,
		Resolve: catalog.ResolveBuiltInPackagedFactory,
	}, nil
}

func (s *Catalog) ListBuiltInPackagedFactories(
	ctx context.Context,
	_ factorydefinitions.ListBuiltInPackagedFactoriesRequest,
) (factorydefinitions.ListBuiltInPackagedFactoriesResult, error) {
	if err := contextError(ctx); err != nil {
		return factorydefinitions.ListBuiltInPackagedFactoriesResult{}, err
	}
	if s == nil {
		return factorydefinitions.ListBuiltInPackagedFactoriesResult{}, fmt.Errorf(
			"packaged Factory catalog is required",
		)
	}
	entries := make([]factorydefinitions.BuiltInPackagedFactoryEntry, len(s.definitions))
	for index, definition := range s.definitions {
		entries[index] = factorydefinitions.BuiltInPackagedFactoryEntry{
			Name:    definition.Name,
			Project: definition.Project,
			Formats: append([]factorydefinitions.PackagedFactoryFormat(nil), definition.Formats...),
		}
	}
	return factorydefinitions.ListBuiltInPackagedFactoriesResult{Entries: entries}, nil
}

func (s *Catalog) ResolveBuiltInPackagedFactory(
	ctx context.Context,
	request factorydefinitions.ResolveBuiltInPackagedFactoryRequest,
) (factorydefinitions.ResolveBuiltInPackagedFactoryResult, error) {
	if err := contextError(ctx); err != nil {
		return factorydefinitions.ResolveBuiltInPackagedFactoryResult{}, err
	}
	if s == nil {
		return factorydefinitions.ResolveBuiltInPackagedFactoryResult{}, fmt.Errorf(
			"packaged Factory catalog is required",
		)
	}
	if strings.TrimSpace(request.Name) == "" {
		return factorydefinitions.ResolveBuiltInPackagedFactoryResult{}, &factorydefinitions.UnknownPackagedFactoryError{
			Name:      request.Name,
			Available: s.names(),
		}
	}
	index := sort.Search(len(s.definitions), func(index int) bool {
		return s.definitions[index].Name >= request.Name
	})
	if index == len(s.definitions) || s.definitions[index].Name != request.Name {
		return factorydefinitions.ResolveBuiltInPackagedFactoryResult{}, &factorydefinitions.UnknownPackagedFactoryError{
			Name:      request.Name,
			Available: s.names(),
		}
	}
	definition := cloneDefinition(s.definitions[index])
	return factorydefinitions.ResolveBuiltInPackagedFactoryResult{
		Definition: definition,
		Formats:    append([]factorydefinitions.PackagedFactoryFormat(nil), definition.Formats...),
	}, nil
}

func (s *Catalog) names() []string {
	names := make([]string, len(s.definitions))
	for index, definition := range s.definitions {
		names[index] = definition.Name
	}
	return names
}

func cloneDefinition(
	definition factorydefinitions.PackagedDefinition,
) factorydefinitions.PackagedDefinition {
	definition.JSON = append([]byte(nil), definition.JSON...)
	definition.YAML = append([]byte(nil), definition.YAML...)
	definition.Formats = append([]factorydefinitions.PackagedFactoryFormat(nil), definition.Formats...)
	definition.Integrity.BundledFiles = append(
		[]factorydefinitions.PackagedFactoryArtifactIntegrity(nil),
		definition.Integrity.BundledFiles...,
	)
	return definition
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("packaged Factory catalog context is required")
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("read packaged Factory catalog: %w", err)
	}
	return nil
}
