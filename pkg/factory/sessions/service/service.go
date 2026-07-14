package service

import (
	"context"
	"fmt"
	"strings"

	factorysessions "github.com/portpowered/infinite-you/pkg/factory/sessions"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/controlplane"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/dataplane"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/stream"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
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

// OpenFactorySession maps the public open request through control-plane policy and
// live dataplane startup.
func (s *Service) OpenFactorySession(
	ctx context.Context,
	request factoryapi.OpenFactorySessionRequest,
) (factoryapi.OpenFactorySessionResponse, error) {
	if s == nil || s.host == nil {
		return factoryapi.OpenFactorySessionResponse{}, fmt.Errorf("factory session gateway is required")
	}
	var target *factorysessions.TargetRef
	if request.Target != nil {
		targetName := ""
		if request.Target.Name != nil {
			targetName = strings.TrimSpace(*request.Target.Name)
		}
		target = &factorysessions.TargetRef{
			Kind: factorysessions.TargetKind(request.Target.Kind),
			Name: targetName,
		}
	}
	validateOnly := request.ValidateOnly != nil && *request.ValidateOnly
	initNewFactory := request.InitNewFactory != nil && *request.InitNewFactory
	if validateOnly && initNewFactory {
		return factoryapi.OpenFactorySessionResponse{}, factorysessions.NewValidationError(
			factorysessions.ValidationReasonRequired,
			"initNewFactory",
			fmt.Errorf("initNewFactory cannot be combined with validateOnly"),
		)
	}
	result, err := s.OpenFactorySessionFromFolder(ctx, request.FolderPath, target, validateOnly, initNewFactory)
	if err != nil {
		return factoryapi.OpenFactorySessionResponse{}, err
	}
	return openResultToAPI(s.host, result)
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

func openResultToAPI(host Host, result *factorysessions.OpenResult) (factoryapi.OpenFactorySessionResponse, error) {
	response := factoryapi.OpenFactorySessionResponse{}
	if result == nil {
		return response, nil
	}
	if result.InitsNewFactory {
		initsNewFactory := true
		response.InitsNewFactory = &initsNewFactory
		if folderPath := strings.TrimSpace(result.FolderPath); folderPath != "" {
			response.FolderPath = &folderPath
		}
	}
	if len(result.Targets) > 0 {
		targets := factorysessions.TargetsResponse(result.Targets)
		response.Targets = &targets
	}
	if result.SessionID != "" {
		session, err := host.RequireSession(result.SessionID)
		if err != nil {
			return factoryapi.OpenFactorySessionResponse{}, err
		}
		summary := factorysessions.SummaryResponse(session)
		response.Session = &summary
	}
	return response, nil
}
