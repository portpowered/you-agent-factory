// Package service implements the Provider Sessions composed root contract.
package service

import (
	"context"
	"strings"

	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

type inspectionService struct {
	captured capturedProvider
}

var _ providersessions.Service = (*inspectionService)(nil)

// New constructs an inert captured-only inspection service.
func New(captured recordings.WorkerCapturedActivityReader) (providersessions.Service, error) {
	return &inspectionService{captured: capturedProvider{reader: captured}}, nil
}

// Details loads one provider session and returns provider-independent
// inspection data.
func (s *inspectionService) Details(provider, kind, id string) (providersessions.Detail, error) {
	providerID, err := normalizeProvider(provider)
	if err != nil {
		return providersessions.Detail{}, err
	}
	return s.detailsForRef(context.Background(), providers.SessionRef{Provider: providerID, Kind: kind, ID: id})
}

// Inspect validates and inspects a detached typed SessionRef through the same
// selected-profile lookup path as Details, returning a plain InspectResult.
func (s *inspectionService) Inspect(req providersessions.InspectRequest) (providersessions.InspectResult, error) {
	detail, err := s.detailsForRef(req.Context, req.Session)
	if err != nil {
		return providersessions.InspectResult{}, err
	}
	return providersessions.InspectResult{
		Session: req.Session.Clone(),
		Source:  detail.Source,
	}, nil
}

// Project projects provider-independent transcript/detail facts for a detached
// typed SessionRef through the same selected-profile lookup path as Details.
func (s *inspectionService) Project(req providersessions.ProjectRequest) (providersessions.ProjectResult, error) {
	detail, err := s.detailsForRef(req.Context, req.Session)
	if err != nil {
		return providersessions.ProjectResult{}, err
	}
	return providersessions.ProjectResult{
		Session: req.Session.Clone(),
		Detail:  detail,
	}, nil
}

func (s *inspectionService) detailsForRef(ctx context.Context, ref providers.SessionRef) (providersessions.Detail, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := validateSessionRef(ref); err != nil {
		return providersessions.Detail{}, err
	}
	detail, err := s.captured.Details(ctx, ref)
	if err != nil {
		return providersessions.Detail{}, &providersessions.LookupError{
			Provider: providersessions.Provider(ref.Provider), SessionID: ref.ID, Err: err,
		}
	}
	return detail, nil
}

func normalizeProvider(provider string) (providers.ID, error) {
	switch provider {
	case string(providersessions.ProviderCodex):
		return providers.IDCodex, nil
	case string(providersessions.ProviderCursor):
		return providers.IDCursor, nil
	default:
		return "", providersessions.ErrUnsupportedProvider
	}
}

func validateSessionRef(session providers.SessionRef) error {
	if strings.TrimSpace(session.ID) == "" {
		return providersessions.ErrInvalidIdentifier
	}
	switch session.Provider {
	case providers.IDCodex, providers.IDCursor:
	default:
		return providersessions.ErrUnsupportedProvider
	}
	if session.Kind != providers.SessionIDKind {
		return providersessions.ErrUnsupportedKind
	}
	return nil
}
