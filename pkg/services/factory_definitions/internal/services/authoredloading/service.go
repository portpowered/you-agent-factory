// Package authoredloading implements the Factory Definitions-owned authored
// source selection, loading, and blocking-validation capability.
package authoredloading

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// Service retains only the exact existing loading and validation collaborators
// needed by the focused public capability. Construction is inert; every effect
// occurs only when LoadValidatedAuthoredFactoryDefinition is invoked.
type Service struct {
	loadCurrent  factorydefinitions.LoadedFactoryLoader
	loadSelected factorydefinitions.LoadedFactoryLoader
	validator    factorydefinitions.Validator
}

var _ factorydefinitions.ValidatedAuthoredFactoryDefinitionLoader = (*Service)(nil)

// New constructs the focused authored loading capability from direct
// collaborators selected by Factory Definitions Wire.
func New(
	loadCurrent factorydefinitions.LoadedFactoryLoader,
	loadSelected factorydefinitions.LoadedFactoryLoader,
	validator factorydefinitions.Validator,
) *Service {
	if loadCurrent == nil || loadSelected == nil || validator == nil {
		return nil
	}
	return &Service{
		loadCurrent:  loadCurrent,
		loadSelected: loadSelected,
		validator:    validator,
	}
}

func (s *Service) LoadValidatedAuthoredFactoryDefinition(
	ctx context.Context,
	request factorydefinitions.LoadValidatedAuthoredFactoryDefinitionRequest,
) (factorydefinitions.LoadValidatedAuthoredFactoryDefinitionResult, error) {
	if err := s.validateContext(ctx); err != nil {
		return factorydefinitions.LoadValidatedAuthoredFactoryDefinitionResult{}, err
	}

	selected := authoredIdentity(request)
	loaded, err := s.load(request)
	if err != nil {
		return factorydefinitions.LoadValidatedAuthoredFactoryDefinitionResult{}, &factorydefinitions.AuthoredFactoryDefinitionLoadFailure{
			Kind:   factorydefinitions.AuthoredFactoryDefinitionLoadFailureDependency,
			Source: selected,
			Cause:  err,
		}
	}
	return s.validatedResult(ctx, request, selected, loaded)
}

func (s *Service) validatedResult(
	ctx context.Context,
	request factorydefinitions.LoadValidatedAuthoredFactoryDefinitionRequest,
	selected factorydefinitions.AuthoredFactoryDefinitionIdentity,
	loaded factorydefinitions.MutableLoadedFactorySource,
) (factorydefinitions.LoadValidatedAuthoredFactoryDefinitionResult, error) {
	if loaded == nil || loaded.FactoryConfig() == nil {
		return factorydefinitions.LoadValidatedAuthoredFactoryDefinitionResult{}, malformedFailure(selected)
	}
	if source, ok := loaded.(factorydefinitions.AuthoredFactorySourceIdentityProvider); ok {
		if identity := source.AuthoredFactorySourceIdentity(); identity.Path != "" {
			selected = identity
		}
	}
	if selected.Path == "" {
		selected.Path = loaded.FactoryDir()
	}
	validation := s.validator.ValidateBlockingLoad(ctx, loaded.FactoryConfig())
	if err := ctx.Err(); err != nil {
		return factorydefinitions.LoadValidatedAuthoredFactoryDefinitionResult{}, err
	}
	if validation.HasBlockingTargets() {
		return factorydefinitions.LoadValidatedAuthoredFactoryDefinitionResult{}, &factorydefinitions.AuthoredFactoryDefinitionLoadFailure{
			Kind:       factorydefinitions.AuthoredFactoryDefinitionLoadFailureValidation,
			Source:     selected,
			Validation: cloneValidation(validation),
			Cause:      factorydefinitions.NewBlockingFactoryLoadError(validation),
		}
	}

	definition, err := factorydefinitions.CloneFactoryConfig(loaded.FactoryConfig())
	if err != nil {
		return factorydefinitions.LoadValidatedAuthoredFactoryDefinitionResult{}, &factorydefinitions.AuthoredFactoryDefinitionLoadFailure{
			Kind:   factorydefinitions.AuthoredFactoryDefinitionLoadFailureDependency,
			Source: selected,
			Cause:  fmt.Errorf("clone effective Factory Definition: %w", err),
		}
	}
	runtimeBaseDir := strings.TrimSpace(request.ExecutionBaseDir)
	if runtimeBaseDir == "" {
		runtimeBaseDir = loaded.RuntimeBaseDir()
	}
	return factorydefinitions.LoadValidatedAuthoredFactoryDefinitionResult{
		Source:                  selected,
		Definition:              definition,
		FactoryDir:              loaded.FactoryDir(),
		RuntimeBaseDir:          runtimeBaseDir,
		BundledFileReplacements: append([]factorydefinitions.PortableBundledFileReplacement(nil), loaded.PortableBundledFileReplacements()...),
		Validation:              cloneValidation(validation),
	}, nil
}

func (s *Service) validateContext(ctx context.Context) error {
	if s == nil || s.loadCurrent == nil || s.loadSelected == nil || s.validator == nil {
		return dependencyFailure("Factory Definitions authored loading capability is required")
	}
	if ctx == nil {
		return dependencyFailure("Factory Definitions authored loading context is required")
	}
	return ctx.Err()
}

func malformedFailure(
	selected factorydefinitions.AuthoredFactoryDefinitionIdentity,
) error {
	return &factorydefinitions.AuthoredFactoryDefinitionLoadFailure{
		Kind:   factorydefinitions.AuthoredFactoryDefinitionLoadFailureMalformed,
		Source: selected,
	}
}

func dependencyFailure(message string) error {
	return &factorydefinitions.AuthoredFactoryDefinitionLoadFailure{
		Kind:  factorydefinitions.AuthoredFactoryDefinitionLoadFailureDependency,
		Cause: errors.New(message),
	}
}

func (s *Service) load(
	request factorydefinitions.LoadValidatedAuthoredFactoryDefinitionRequest,
) (factorydefinitions.MutableLoadedFactorySource, error) {
	if sourcePath := strings.TrimSpace(request.SourcePath); sourcePath != "" {
		return s.loadSelected(sourcePath, nil)
	}
	return s.loadCurrent(strings.TrimSpace(request.Directory), nil)
}

func authoredIdentity(
	request factorydefinitions.LoadValidatedAuthoredFactoryDefinitionRequest,
) factorydefinitions.AuthoredFactoryDefinitionIdentity {
	path := strings.TrimSpace(request.SourcePath)
	if path == "" {
		path = strings.TrimSpace(request.Directory)
	}
	return factorydefinitions.AuthoredFactoryDefinitionIdentity{
		Path:   path,
		Format: authoredFormat(path),
	}
}

func authoredFormat(path string) factorydefinitions.AuthoredFactoryFormat {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		return factorydefinitions.AuthoredFactoryFormatJSON
	case ".yaml", ".yml":
		return factorydefinitions.AuthoredFactoryFormatYAML
	default:
		return ""
	}
}

func cloneValidation(validation factorydefinitions.ValidationResult) factorydefinitions.ValidationResult {
	return factorydefinitions.ValidationResult{
		Targets: append([]factorydefinitions.ValidationTarget(nil), validation.Targets...),
	}
}
