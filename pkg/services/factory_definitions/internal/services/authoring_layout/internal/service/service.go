package service

import (
	"context"
	"errors"
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	authoringlayout "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout/expand"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout/flatten"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout/prepare"
)

// Service is the private nested authoring_layout implementation behind the
// CTR-DEF root authoring slice.
type Service struct {
	validator            factorydefinitions.Validator
	validateDefinition   factorydefinitions.DefinitionValidationOperation
	mapInput             factorydefinitions.FactoryLayoutPayloadMapper
	decodeFactory        factorydefinitions.FactoryConfigJSONDecoder
	normalizeAuthored    func(*factorydefinitions.FactoryConfig) (*factorydefinitions.FactoryConfig, error)
	encodeFactory        func(*factorydefinitions.FactoryConfig) ([]byte, error)
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
		deps.DecodeFactory == nil ||
		deps.NormalizeAuthored == nil ||
		deps.EncodeFactory == nil ||
		deps.Write == nil ||
		deps.Validate == nil ||
		deps.Flatten == nil ||
		deps.Expand == nil ||
		deps.Directories == nil {
		return nil
	}
	validateDefinition, _ := any(deps.Validator).(factorydefinitions.DefinitionValidationOperation)
	return &Service{
		validator:            deps.Validator,
		validateDefinition:   validateDefinition,
		mapInput:             deps.MapInput,
		decodeFactory:        deps.DecodeFactory,
		normalizeAuthored:    deps.NormalizeAuthored,
		encodeFactory:        deps.EncodeFactory,
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
	ctx context.Context,
	request factorydefinitions.PrepareFactoryLayoutRequest,
) (factorydefinitions.PrepareFactoryLayoutResult, error) {
	if err := s.requirePorts(); err != nil {
		return factorydefinitions.PrepareFactoryLayoutResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return factorydefinitions.PrepareFactoryLayoutResult{}, err
	}
	if s.validateDefinition == nil {
		return factorydefinitions.PrepareFactoryLayoutResult{},
			fmt.Errorf("Factory Definitions validation operation is required")
	}

	validationRequest, err := s.mapInput(request.Payload)
	if err != nil {
		return factorydefinitions.PrepareFactoryLayoutResult{}, err
	}
	validationRequest.Profile = factorydefinitions.ValidationProfilePrePersist
	result, err := s.validateDefinition.ValidateDefinition(ctx, validationRequest)
	if err != nil {
		if errors.Is(err, factorydefinitions.ErrInvalidNamedFactory) {
			return factorydefinitions.PrepareFactoryLayoutResult{}, err
		}
		return factorydefinitions.PrepareFactoryLayoutResult{},
			fmt.Errorf("%w: %v", factorydefinitions.ErrInvalidNamedFactory, err)
	}
	if targets := result.BlockingTargets(); len(targets) != 0 {
		return factorydefinitions.PrepareFactoryLayoutResult{},
			factorydefinitions.NewBlockingFactoryLoadError(
				factorydefinitions.ValidationResult{Targets: targets},
			)
	}

	prepared, err := prepare.FactoryLayout(
		ctx,
		request.Name,
		request.Payload,
		s.validator,
		s.decodeFactory,
		s.normalizeAuthored,
		s.encodeFactory,
	)
	if err != nil {
		return factorydefinitions.PrepareFactoryLayoutResult{}, err
	}
	return factorydefinitions.PrepareFactoryLayoutResult{Prepared: *prepared}, nil
}

func (s *Service) FlattenFactoryLayout(
	ctx context.Context,
	request factorydefinitions.FlattenFactoryLayoutRequest,
) (factorydefinitions.FlattenFactoryLayoutResult, error) {
	if err := s.requirePorts(); err != nil {
		return factorydefinitions.FlattenFactoryLayoutResult{}, err
	}
	canonical, err := flatten.FactoryLayout(ctx, request.Path, s.flatten)
	if err != nil {
		return factorydefinitions.FlattenFactoryLayoutResult{}, err
	}
	return factorydefinitions.FlattenFactoryLayoutResult{Canonical: canonical}, nil
}

func (s *Service) ExpandFactoryLayout(
	ctx context.Context,
	request factorydefinitions.ExpandFactoryLayoutRequest,
) (factorydefinitions.ExpandFactoryLayoutResult, error) {
	if err := s.requirePorts(); err != nil {
		return factorydefinitions.ExpandFactoryLayoutResult{}, err
	}
	factoryDir, report, err := expand.FactoryLayout(ctx, request.Path, s.expand)
	if err != nil {
		return factorydefinitions.ExpandFactoryLayoutResult{}, err
	}
	return factorydefinitions.ExpandFactoryLayoutResult{
		FactoryDir: factoryDir,
		Report:     report,
	}, nil
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
	if s.decodeFactory == nil {
		return fmt.Errorf("Factory Definitions decoder is required")
	}
	if s.normalizeAuthored == nil {
		return fmt.Errorf("Factory Definitions authored normalizer is required")
	}
	if s.encodeFactory == nil {
		return fmt.Errorf("Factory Definitions encoder is required")
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
