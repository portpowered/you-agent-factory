package internal

import (
	"context"
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	authoringlayout "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout"
)

type authoringLayoutService struct {
	factorydefinitions.Service
	authoring authoringlayout.Service
}

// AttachAuthoringLayout returns the Factory Definitions root with CTR-DEF
// authoring operations delegated to the private authoring_layout subservice
// while preserving every other root operation.
func AttachAuthoringLayout(
	service factorydefinitions.Service,
	authoring authoringlayout.Service,
) (factorydefinitions.Service, error) {
	if service == nil {
		return nil, errFactoryDefinitionsServiceRequired()
	}
	if authoring == nil {
		return nil, errAuthoringLayoutServiceRequired()
	}
	return authoringLayoutService{Service: service, authoring: authoring}, nil
}

func (s authoringLayoutService) PrepareFactoryLayout(
	ctx context.Context,
	request factorydefinitions.PrepareFactoryLayoutRequest,
) (factorydefinitions.PrepareFactoryLayoutResult, error) {
	return s.authoring.PrepareFactoryLayout(ctx, request)
}

func (s authoringLayoutService) FlattenFactoryLayout(
	ctx context.Context,
	request factorydefinitions.FlattenFactoryLayoutRequest,
) (factorydefinitions.FlattenFactoryLayoutResult, error) {
	return s.authoring.FlattenFactoryLayout(ctx, request)
}

func (s authoringLayoutService) ExpandFactoryLayout(
	ctx context.Context,
	request factorydefinitions.ExpandFactoryLayoutRequest,
) (factorydefinitions.ExpandFactoryLayoutResult, error) {
	return s.authoring.ExpandFactoryLayout(ctx, request)
}

func (s authoringLayoutService) CreateNamedFactory(
	ctx context.Context,
	request factorydefinitions.CreateNamedFactoryRequest,
) (factorydefinitions.CreateNamedFactoryResult, error) {
	return s.authoring.CreateNamedFactory(ctx, request)
}

func (s authoringLayoutService) ReplaceNamedFactory(
	ctx context.Context,
	request factorydefinitions.ReplaceNamedFactoryRequest,
) (factorydefinitions.ReplaceNamedFactoryResult, error) {
	return s.authoring.ReplaceNamedFactory(ctx, request)
}

func errFactoryDefinitionsServiceRequired() error {
	return fmt.Errorf("Factory Definitions service is required")
}

func errAuthoringLayoutServiceRequired() error {
	return fmt.Errorf("Factory Definitions authoring_layout subservice is required")
}
