package service

import (
	"context"
	"fmt"

	factorysessions "github.com/portpowered/infinite-you/pkg/factory/sessions"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/controlplane"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/dataplane"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/stream"
)

// Service is the canonical Factory Session application gateway for open, read, and lifecycle behavior.
type Service struct {
	host          Host
	liveOpener    *dataplane.LiveOpener
	liveLifecycle *dataplane.LiveLifecycle
	streams       *stream.Manager
}

// New constructs a session gateway with explicit host and dataplane dependencies.
func New(host Host) *Service {
	if host == nil {
		return nil
	}
	return &Service{
		host:          host,
		liveOpener:    dataplane.NewLiveOpener(host),
		liveLifecycle: dataplane.NewLiveLifecycle(host),
		streams:       stream.NewManager(host),
	}
}

// OpenFactorySession runs an owner-defined open request through control-plane
// policy and live dataplane startup.
func (s *Service) OpenFactorySession(
	ctx context.Context,
	request factorysessions.OpenRequest,
) (*factorysessions.OpenResult, error) {
	if s == nil || s.host == nil {
		return nil, fmt.Errorf("factory session gateway is required")
	}
	if request.ValidateOnly && request.InitNewFactory {
		return nil, factorysessions.NewValidationError(
			factorysessions.ValidationReasonRequired,
			"initNewFactory",
			fmt.Errorf("initNewFactory cannot be combined with validateOnly"),
		)
	}
	return s.OpenFactorySessionFromFolder(
		ctx,
		request.FolderPath,
		request.Target,
		request.ValidateOnly,
		request.InitNewFactory,
	)
}

// OpenFactorySessionFromFolder runs folder-scoped open policy without transport mapping.
func (s *Service) OpenFactorySessionFromFolder(
	ctx context.Context,
	folderPath string,
	target *factorysessions.TargetRef,
	validateOnly bool,
	initNewFactory bool,
) (*factorysessions.OpenResult, error) {
	if s == nil || s.host == nil {
		return nil, fmt.Errorf("factory session gateway is required")
	}
	return controlplane.OpenFromFolder(
		ctx,
		s.host,
		s.liveOpener,
		folderPath,
		target,
		validateOnly,
		initNewFactory,
	)
}
