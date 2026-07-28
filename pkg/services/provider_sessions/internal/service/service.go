// Package service implements the Provider Sessions composed root contract.
package service

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	providersessionsinternal "github.com/portpowered/infinite-you/pkg/services/provider_sessions/internal"
	codexreader "github.com/portpowered/infinite-you/pkg/services/provider_sessions/internal/services/codex_reader"
	codexreaderwire "github.com/portpowered/infinite-you/pkg/services/provider_sessions/internal/services/codex_reader/wire"
	cursorreader "github.com/portpowered/infinite-you/pkg/services/provider_sessions/internal/services/cursor_reader"
	cursorreaderwire "github.com/portpowered/infinite-you/pkg/services/provider_sessions/internal/services/cursor_reader/wire"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

type inspectionService struct {
	codex        codexreader.Service
	cursorReader cursorreader.Service
	files        providersessionsinternal.FileSystem
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
	codexWalkDirectory providersessionsinternal.CodexWalkDirectory,
	codexResolveSymlinks providersessionsinternal.CodexResolveSymlinks,
	cursorWalkDirectory providersessionsinternal.CursorWalkDirectory,
	cursorResolveSymlinks providersessionsinternal.CursorResolveSymlinks,
	cursorOpenDatabase providersessionsinternal.CursorOpenSQLDatabase,
	cursorOperatingSystem providersessionsinternal.OperatingSystem,
) (providersessions.Service, error) {
	if err := validateDependencies(files, resolveHome, codexWalkDirectory, codexResolveSymlinks, cursorWalkDirectory, cursorResolveSymlinks, cursorOpenDatabase, cursorOperatingSystem); err != nil {
		return nil, err
	}
	codexRoot, err := codexreaderwire.DefaultSessionsRoot(resolveHome)
	if err != nil {
		return nil, err
	}
	cursorRoot, err := cursorreaderwire.DefaultStorageRoot(resolveHome, files, cursorOperatingSystem)
	if err != nil {
		return nil, err
	}
	return newForRoots(files, codexWalkDirectory, codexResolveSymlinks, cursorWalkDirectory, cursorResolveSymlinks, cursorOpenDatabase, codexRoot, cursorRoot)
}

// NewForRoots constructs Provider Sessions with explicit storage roots.
func NewForRoots(
	files providersessionsinternal.FileSystem,
	codexWalkDirectory providersessionsinternal.CodexWalkDirectory,
	codexResolveSymlinks providersessionsinternal.CodexResolveSymlinks,
	cursorWalkDirectory providersessionsinternal.CursorWalkDirectory,
	cursorResolveSymlinks providersessionsinternal.CursorResolveSymlinks,
	cursorOpenDatabase providersessionsinternal.CursorOpenSQLDatabase,
	codexRoot, cursorRoot string,
) (providersessions.Service, error) {
	if err := validateStorageDependencies(files, codexWalkDirectory, codexResolveSymlinks, cursorWalkDirectory, cursorResolveSymlinks, cursorOpenDatabase); err != nil {
		return nil, err
	}
	return newForRoots(files, codexWalkDirectory, codexResolveSymlinks, cursorWalkDirectory, cursorResolveSymlinks, cursorOpenDatabase, codexRoot, cursorRoot)
}

func newForRoots(files providersessionsinternal.FileSystem, codexWalkDirectory providersessionsinternal.CodexWalkDirectory, codexResolveSymlinks providersessionsinternal.CodexResolveSymlinks, cursorWalkDirectory providersessionsinternal.CursorWalkDirectory, cursorResolveSymlinks providersessionsinternal.CursorResolveSymlinks, cursorOpenDatabase providersessionsinternal.CursorOpenSQLDatabase, codexRoot, cursorRoot string) (providersessions.Service, error) {
	cursorReader, err := cursorreaderwire.NewService(files, cursorWalkDirectory, cursorResolveSymlinks, cursorOpenDatabase, cursorRoot)
	if err != nil {
		return nil, err
	}
	codexRoot = filepath.Clean(codexRoot)
	codexService, err := codexreaderwire.NewService(codexreader.Dependencies{
		Files:           files,
		WalkDirectory:   codexWalkDirectory,
		ResolveSymlinks: codexResolveSymlinks,
		SessionsRoot:    codexRoot,
	})
	if err != nil {
		return nil, err
	}
	return &inspectionService{
		codex:        codexService,
		cursorReader: cursorReader,
		files:        files,
	}, nil
}

func validateDependencies(files providersessionsinternal.FileSystem, resolveHome providersessionsinternal.ResolveHomeDirectory, codexWalkDirectory providersessionsinternal.CodexWalkDirectory, codexResolveSymlinks providersessionsinternal.CodexResolveSymlinks, cursorWalkDirectory providersessionsinternal.CursorWalkDirectory, cursorResolveSymlinks providersessionsinternal.CursorResolveSymlinks, cursorOpenDatabase providersessionsinternal.CursorOpenSQLDatabase, cursorOperatingSystem providersessionsinternal.OperatingSystem) error {
	if resolveHome == nil {
		return fmt.Errorf("provider-session home resolver is required")
	}
	if strings.TrimSpace(string(cursorOperatingSystem)) == "" {
		return fmt.Errorf("provider-session operating system is required")
	}
	return validateStorageDependencies(files, codexWalkDirectory, codexResolveSymlinks, cursorWalkDirectory, cursorResolveSymlinks, cursorOpenDatabase)
}

func validateStorageDependencies(files providersessionsinternal.FileSystem, codexWalkDirectory providersessionsinternal.CodexWalkDirectory, codexResolveSymlinks providersessionsinternal.CodexResolveSymlinks, cursorWalkDirectory providersessionsinternal.CursorWalkDirectory, cursorResolveSymlinks providersessionsinternal.CursorResolveSymlinks, cursorOpenDatabase providersessionsinternal.CursorOpenSQLDatabase) error {
	if files == nil {
		return fmt.Errorf("provider-session filesystem is required")
	}
	if codexWalkDirectory == nil {
		return fmt.Errorf("provider-session Codex directory walker is required")
	}
	if codexResolveSymlinks == nil {
		return fmt.Errorf("provider-session Codex symlink resolver is required")
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
// storage-backed lookup path as Details, returning a plain InspectResult.
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
// typed SessionRef through the same storage-backed lookup path as Details.
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
			Provider: providersessions.ProviderCodex,
			Err:      err,
		}
	default:
		return providersessions.Detail{}, providersessions.ErrUnsupportedProvider
	}
}

func normalizeProvider(provider string) (providers.ID, error) {
	switch strings.TrimSpace(provider) {
	case string(providersessions.ProviderCodex):
		return providers.IDCodex, nil
	case string(providersessions.ProviderCursor), "agent", "cursor-agent":
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
