package service

import (
	"context"
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	authoringlayout "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout"
)

// Service is the private nested authoring_layout implementation behind the
// CTR-DEF root authoring slice.
type Service struct {
	validator            factorydefinitions.Validator
	mapInput             factorydefinitions.FactoryLayoutPayloadMapper
	prepare              factorydefinitions.FactoryLayoutPayloadPreparer
	write                func(string, *factorydefinitions.PreparedFactoryLayoutPayload, string) error
	validate             func(string) error
	flatten              factorydefinitions.FactoryLayoutFlattener
	expand               factorydefinitions.FactoryLayoutExpander
	fileSystem           factorydefinitions.PersistenceFileSystem
	requireDefinitionDir factorydefinitions.DefinitionDirectoryRequirer
	directories          factorydefinitions.DirectoryReplacementStore
}

var _ authoringlayout.Service = (*Service)(nil)

// New constructs the authoring_layout implementation from exact injected ports.
func New(deps authoringlayout.Dependencies) *Service {
	if deps.FileSystem == nil || deps.RequireDefinitionDir == nil {
		return nil
	}
	if deps.Validator == nil ||
		deps.MapInput == nil ||
		deps.Prepare == nil ||
		deps.Write == nil ||
		deps.Validate == nil ||
		deps.Flatten == nil ||
		deps.Expand == nil ||
		deps.Directories == nil {
		return nil
	}
	return &Service{
		validator:            deps.Validator,
		mapInput:             deps.MapInput,
		prepare:              deps.Prepare,
		write:                deps.Write,
		validate:             deps.Validate,
		flatten:              deps.Flatten,
		expand:               deps.Expand,
		fileSystem:           deps.FileSystem,
		requireDefinitionDir: deps.RequireDefinitionDir,
		directories:          deps.Directories,
	}
}

func (s *Service) PrepareFactoryLayout(
	_ context.Context,
	_ factorydefinitions.PrepareFactoryLayoutRequest,
) (factorydefinitions.PrepareFactoryLayoutResult, error) {
	if err := s.requirePorts(); err != nil {
		return factorydefinitions.PrepareFactoryLayoutResult{}, err
	}
	return factorydefinitions.PrepareFactoryLayoutResult{},
		factorydefinitions.ErrMalformedFactoryLayoutPayload
}

func (s *Service) FlattenFactoryLayout(
	_ context.Context,
	_ factorydefinitions.FlattenFactoryLayoutRequest,
) (factorydefinitions.FlattenFactoryLayoutResult, error) {
	if err := s.requirePorts(); err != nil {
		return factorydefinitions.FlattenFactoryLayoutResult{}, err
	}
	return factorydefinitions.FlattenFactoryLayoutResult{},
		fmt.Errorf("factory layout collaborator is required")
}

func (s *Service) ExpandFactoryLayout(
	_ context.Context,
	_ factorydefinitions.ExpandFactoryLayoutRequest,
) (factorydefinitions.ExpandFactoryLayoutResult, error) {
	if err := s.requirePorts(); err != nil {
		return factorydefinitions.ExpandFactoryLayoutResult{}, err
	}
	return factorydefinitions.ExpandFactoryLayoutResult{},
		fmt.Errorf("factory layout collaborator is required")
}

func (s *Service) CreateNamedFactory(
	_ context.Context,
	_ factorydefinitions.CreateNamedFactoryRequest,
) (factorydefinitions.CreateNamedFactoryResult, error) {
	if err := s.requirePorts(); err != nil {
		return factorydefinitions.CreateNamedFactoryResult{}, err
	}
	return factorydefinitions.CreateNamedFactoryResult{}, &factorydefinitions.AtomicFactoryWriteFailure{
		PreviousPreserved: true,
		Cause:             fmt.Errorf("factory layout collaborator is required"),
	}
}

func (s *Service) ReplaceNamedFactory(
	_ context.Context,
	_ factorydefinitions.ReplaceNamedFactoryRequest,
) (factorydefinitions.ReplaceNamedFactoryResult, error) {
	if err := s.requirePorts(); err != nil {
		return factorydefinitions.ReplaceNamedFactoryResult{}, err
	}
	return factorydefinitions.ReplaceNamedFactoryResult{}, &factorydefinitions.AtomicFactoryWriteFailure{
		PreviousPreserved: true,
		Cause:             fmt.Errorf("factory layout collaborator is required"),
	}
}

func (s *Service) requirePorts() error {
	if s == nil || s.fileSystem == nil {
		return fmt.Errorf("Factory Definitions persistence filesystem is required")
	}
	if s.requireDefinitionDir == nil {
		return fmt.Errorf("Factory Definition directory validator is required")
	}
	if s.validator == nil {
		return fmt.Errorf("Factory Definitions validator is required")
	}
	if s.mapInput == nil {
		return fmt.Errorf("Factory Definitions payload mapper is required")
	}
	if s.prepare == nil {
		return fmt.Errorf("Factory Definitions layout preparer is required")
	}
	if s.write == nil {
		return fmt.Errorf("Factory Definitions layout writer is required")
	}
	if s.validate == nil {
		return fmt.Errorf("Factory Definitions layout validator is required")
	}
	if s.flatten == nil {
		return fmt.Errorf("Factory Definitions layout flattener is required")
	}
	if s.expand == nil {
		return fmt.Errorf("Factory Definitions layout expander is required")
	}
	if s.directories == nil {
		return fmt.Errorf("directory replacement store is required")
	}
	return nil
}
