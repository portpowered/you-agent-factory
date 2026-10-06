// Package service implements the Provider Sessions composed root contract.
package service

import (
	"context"
	"fmt"
	"strings"

	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	providersessionsinternal "github.com/portpowered/infinite-you/pkg/services/provider_sessions/internal"
	cursorreader "github.com/portpowered/infinite-you/pkg/services/provider_sessions/internal/services/cursor_reader"
	cursorreaderwire "github.com/portpowered/infinite-you/pkg/services/provider_sessions/internal/services/cursor_reader/wire"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

type inspectionService struct {
	codex        capturedCodex
	cursorReader cursorreader.Service
}

// Compile-time proof that production inspectionService seals the singular
// peer root (Details + Inspect + Project) without exposing construction ports
// or private Codex/Cursor reader types through Service method signatures.
var _ providersessions.Service = (*inspectionService)(nil)

// New constructs Provider Sessions from explicit process edges and the
// provider-owned default storage-root policy.
func New(
	files providersessionsinternal.FileSystem,
	resolveHome providersessionsinternal.ResolveHomeDirectory,
	cursorWalkDirectory providersessionsinternal.CursorWalkDirectory,
	cursorResolveSymlinks providersessionsinternal.CursorResolveSymlinks,
	cursorOpenDatabase providersessionsinternal.CursorOpenSQLDatabase,
	cursorOperatingSystem providersessionsinternal.OperatingSystem,
	captured recordings.WorkerCapturedActivityReader,
) (providersessions.Service, error) {
	if err := validateDependencies(files, resolveHome, cursorWalkDirectory, cursorResolveSymlinks, cursorOpenDatabase, cursorOperatingSystem, captured); err != nil {
		return nil, err
	}
	home, err := resolveHome()
	if err != nil {
		return nil, fmt.Errorf("home directory: %w", err)
	}
	if strings.TrimSpace(home) == "" {
		return nil, fmt.Errorf("provider-session home directory is required")
	}
	cursorRoot, err := cursorreaderwire.DefaultStorageRoot(func() (string, error) { return home, nil }, files, cursorOperatingSystem)
	if err != nil {
		return nil, err
	}
	return newForRoots(files, cursorWalkDirectory, cursorResolveSymlinks, cursorOpenDatabase, cursorRoot, captured)
}

// NewForRoots constructs Provider Sessions with explicit storage roots.
func NewForRoots(
	files providersessionsinternal.FileSystem,
	cursorWalkDirectory providersessionsinternal.CursorWalkDirectory,
	cursorResolveSymlinks providersessionsinternal.CursorResolveSymlinks,
	cursorOpenDatabase providersessionsinternal.CursorOpenSQLDatabase,
	cursorRoot string,
	captured recordings.WorkerCapturedActivityReader,
) (providersessions.Service, error) {
	if err := validateStorageDependencies(files, cursorWalkDirectory, cursorResolveSymlinks, cursorOpenDatabase, captured); err != nil {
		return nil, err
	}
	return newForRoots(files, cursorWalkDirectory, cursorResolveSymlinks, cursorOpenDatabase, cursorRoot, captured)
}

func newForRoots(files providersessionsinternal.FileSystem, cursorWalkDirectory providersessionsinternal.CursorWalkDirectory, cursorResolveSymlinks providersessionsinternal.CursorResolveSymlinks, cursorOpenDatabase providersessionsinternal.CursorOpenSQLDatabase, cursorRoot string, captured recordings.WorkerCapturedActivityReader) (providersessions.Service, error) {
	cursorReader, err := cursorreaderwire.NewService(files, cursorWalkDirectory, cursorResolveSymlinks, cursorOpenDatabase, cursorRoot)
	if err != nil {
		return nil, err
	}
	return &inspectionService{
		codex:        capturedCodex{reader: captured},
		cursorReader: cursorReader,
	}, nil
}

func validateDependencies(files providersessionsinternal.FileSystem, resolveHome providersessionsinternal.ResolveHomeDirectory, cursorWalkDirectory providersessionsinternal.CursorWalkDirectory, cursorResolveSymlinks providersessionsinternal.CursorResolveSymlinks, cursorOpenDatabase providersessionsinternal.CursorOpenSQLDatabase, cursorOperatingSystem providersessionsinternal.OperatingSystem, captured recordings.WorkerCapturedActivityReader) error {
	if resolveHome == nil {
		return fmt.Errorf("provider-session home resolver is required")
	}
	if strings.TrimSpace(string(cursorOperatingSystem)) == "" {
		return fmt.Errorf("provider-session operating system is required")
	}
	return validateStorageDependencies(files, cursorWalkDirectory, cursorResolveSymlinks, cursorOpenDatabase, captured)
}

func validateStorageDependencies(files providersessionsinternal.FileSystem, cursorWalkDirectory providersessionsinternal.CursorWalkDirectory, cursorResolveSymlinks providersessionsinternal.CursorResolveSymlinks, cursorOpenDatabase providersessionsinternal.CursorOpenSQLDatabase, captured recordings.WorkerCapturedActivityReader) error {
	if files == nil {
		return fmt.Errorf("provider-session filesystem is required")
	}
	if captured == nil {
		return fmt.Errorf("provider-session captured activity reader is required")
	}
	if cursorWalkDirectory == nil {
		return fmt.Errorf("provider-session Cursor directory walker is required")
	}
	if cursorResolveSymlinks == nil {
		return fmt.Errorf("provider-session Cursor symlink resolver is required")
	}
	if cursorOpenDatabase == nil {
		return fmt.Errorf("provider-session Cursor database opener is required")
	}
	return nil
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
	switch ref.Provider {
	case providers.IDCursor:
		return s.cursorReader.Read(ctx, ref)
	case providers.IDCodex:
		detail, err := s.codex.Details(ctx, ref)
		if err == nil {
			return detail, nil
		}
		return providersessions.Detail{}, &providersessions.LookupError{
			Provider:  providersessions.ProviderCodex,
			SessionID: ref.ID,
			Err:       err,
		}
	default:
		return providersessions.Detail{}, providersessions.ErrUnsupportedProvider
	}
}

func normalizeProvider(provider string) (providers.ID, error) {
	switch strings.TrimSpace(provider) {
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
	if strings.TrimSpace(session.Kind) != providers.SessionIDKind {
		return providersessions.ErrUnsupportedKind
	}
	return nil
}
