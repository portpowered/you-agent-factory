package service

import (
	"context"
	"errors"
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	authoringlayout "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout"
)

// Service executes authored Factory layout prepare/render and atomic writes
// through exact injected ports.
type Service struct {
	ports authoringlayout.Ports
}

var _ authoringlayout.Service = (*Service)(nil)

// New constructs the authoring_layout implementation. Nil ports are rejected.
func New(ports authoringlayout.Ports) *Service {
	if ports.Prepare == nil || ports.Flatten == nil || ports.Expand == nil ||
		ports.Create == nil || ports.Replace == nil {
		return nil
	}
	return &Service{ports: ports}
}

func (s *Service) PrepareFactoryLayout(
	ctx context.Context,
	request factorydefinitions.PrepareFactoryLayoutRequest,
) (factorydefinitions.PrepareFactoryLayoutResult, error) {
	prepared, err := s.ports.Prepare(ctx, request.Name, request.Payload)
	if err != nil {
		if errors.Is(err, factorydefinitions.ErrMalformedFactoryLayoutPayload) {
			return factorydefinitions.PrepareFactoryLayoutResult{}, err
		}
		return factorydefinitions.PrepareFactoryLayoutResult{}, fmt.Errorf(
			"%w: %v",
			factorydefinitions.ErrMalformedFactoryLayoutPayload,
			err,
		)
	}
	return factorydefinitions.PrepareFactoryLayoutResult{Prepared: prepared}, nil
}

func (s *Service) FlattenFactoryLayout(
	_ context.Context,
	request factorydefinitions.FlattenFactoryLayoutRequest,
) (factorydefinitions.FlattenFactoryLayoutResult, error) {
	canonical, err := s.ports.Flatten(request.Path)
	if err != nil {
		return factorydefinitions.FlattenFactoryLayoutResult{}, err
	}
	return factorydefinitions.FlattenFactoryLayoutResult{Canonical: canonical}, nil
}

func (s *Service) ExpandFactoryLayout(
	_ context.Context,
	request factorydefinitions.ExpandFactoryLayoutRequest,
) (factorydefinitions.ExpandFactoryLayoutResult, error) {
	factoryDir, report, err := s.ports.Expand(request.Path)
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
	request factorydefinitions.CreateNamedFactoryRequest,
) (factorydefinitions.CreateNamedFactoryResult, error) {
	factoryDir, err := s.ports.Create(request.RootDir, request.Name, request.Prepared)
	if err != nil {
		return factorydefinitions.CreateNamedFactoryResult{}, atomicWriteFailure(request.Name, factoryDir, err)
	}
	return factorydefinitions.CreateNamedFactoryResult{
		Name:       request.Name,
		FactoryDir: factoryDir,
	}, nil
}

func (s *Service) ReplaceNamedFactory(
	_ context.Context,
	request factorydefinitions.ReplaceNamedFactoryRequest,
) (factorydefinitions.ReplaceNamedFactoryResult, error) {
	factoryDir, err := s.ports.Replace(request.RootDir, request.Name, request.Prepared)
	if err != nil {
		return factorydefinitions.ReplaceNamedFactoryResult{}, atomicWriteFailure(request.Name, factoryDir, err)
	}
	return factorydefinitions.ReplaceNamedFactoryResult{
		Name:       request.Name,
		FactoryDir: factoryDir,
	}, nil
}

func atomicWriteFailure(name, factoryDir string, cause error) error {
	var existing *factorydefinitions.AtomicFactoryWriteFailure
	if errors.As(cause, &existing) {
		return cause
	}
	return &factorydefinitions.AtomicFactoryWriteFailure{
		Name:              name,
		FactoryDir:        factoryDir,
		PreviousPreserved: true,
		Cause:             cause,
	}
}
