// Package service implements the Provider Sessions service contract.
package service

import (
	"fmt"
	"path/filepath"
	"strings"

	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/provider_sessions/codex"
	cursorreader "github.com/portpowered/infinite-you/pkg/services/provider_sessions/internal/services/cursor_reader"
	cursorreaderwire "github.com/portpowered/infinite-you/pkg/services/provider_sessions/internal/services/cursor_reader/wire"
	"github.com/portpowered/infinite-you/pkg/services/providers"
)

type inspectionService struct {
	codexRoot            string
	codexWalkDirectory   providersessions.CodexWalkDirectory
	codexResolveSymlinks providersessions.CodexResolveSymlinks
	cursorReader         cursorreader.Service
	files                providersessions.FileSystem
}

// Compile-time proof that production inspectionService seals the singular
// peer root (Details + Inspect + Project) without exposing construction ports
// or private Codex/Cursor reader types through Service method signatures.
var _ providersessions.Service = (*inspectionService)(nil)

// New constructs Provider Sessions from explicit process edges and the
// provider-owned default storage-root policy.
func New(
	files providersessions.FileSystem,
	resolveHome providersessions.ResolveHomeDirectory,
	codexWalkDirectory providersessions.CodexWalkDirectory,
	codexResolveSymlinks providersessions.CodexResolveSymlinks,
	cursorWalkDirectory providersessions.CursorWalkDirectory,
	cursorResolveSymlinks providersessions.CursorResolveSymlinks,
	cursorOpenDatabase providersessions.CursorOpenSQLDatabase,
	cursorOperatingSystem providersessions.OperatingSystem,
) (providersessions.Service, error) {
	if err := validateDependencies(files, resolveHome, codexWalkDirectory, codexResolveSymlinks, cursorWalkDirectory, cursorResolveSymlinks, cursorOpenDatabase, cursorOperatingSystem); err != nil {
		return nil, err
	}
	codexRoot, err := codex.DefaultSessionsRoot(resolveHome)
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
	files providersessions.FileSystem,
	codexWalkDirectory providersessions.CodexWalkDirectory,
	codexResolveSymlinks providersessions.CodexResolveSymlinks,
	cursorWalkDirectory providersessions.CursorWalkDirectory,
	cursorResolveSymlinks providersessions.CursorResolveSymlinks,
	cursorOpenDatabase providersessions.CursorOpenSQLDatabase,
	codexRoot, cursorRoot string,
) (providersessions.Service, error) {
	if err := validateStorageDependencies(files, codexWalkDirectory, codexResolveSymlinks, cursorWalkDirectory, cursorResolveSymlinks, cursorOpenDatabase); err != nil {
		return nil, err
	}
	return newForRoots(files, codexWalkDirectory, codexResolveSymlinks, cursorWalkDirectory, cursorResolveSymlinks, cursorOpenDatabase, codexRoot, cursorRoot)
}

func newForRoots(files providersessions.FileSystem, codexWalkDirectory providersessions.CodexWalkDirectory, codexResolveSymlinks providersessions.CodexResolveSymlinks, cursorWalkDirectory providersessions.CursorWalkDirectory, cursorResolveSymlinks providersessions.CursorResolveSymlinks, cursorOpenDatabase providersessions.CursorOpenSQLDatabase, codexRoot, cursorRoot string) (providersessions.Service, error) {
	cursorReader, err := cursorreaderwire.NewService(files, cursorWalkDirectory, cursorResolveSymlinks, cursorOpenDatabase, cursorRoot)
	if err != nil {
		return nil, err
	}
	codexRoot = filepath.Clean(codexRoot)
	return &inspectionService{
		codexRoot:            codexRoot,
		codexWalkDirectory:   codexWalkDirectory,
		codexResolveSymlinks: codexResolveSymlinks,
		cursorReader:         cursorReader,
		files:                files,
	}, nil
}

func validateDependencies(files providersessions.FileSystem, resolveHome providersessions.ResolveHomeDirectory, codexWalkDirectory providersessions.CodexWalkDirectory, codexResolveSymlinks providersessions.CodexResolveSymlinks, cursorWalkDirectory providersessions.CursorWalkDirectory, cursorResolveSymlinks providersessions.CursorResolveSymlinks, cursorOpenDatabase providersessions.CursorOpenSQLDatabase, cursorOperatingSystem providersessions.OperatingSystem) error {
	if resolveHome == nil {
		return fmt.Errorf("provider-session home resolver is required")
	}
	if strings.TrimSpace(string(cursorOperatingSystem)) == "" {
		return fmt.Errorf("provider-session operating system is required")
	}
	return validateStorageDependencies(files, codexWalkDirectory, codexResolveSymlinks, cursorWalkDirectory, cursorResolveSymlinks, cursorOpenDatabase)
}

func validateStorageDependencies(files providersessions.FileSystem, codexWalkDirectory providersessions.CodexWalkDirectory, codexResolveSymlinks providersessions.CodexResolveSymlinks, cursorWalkDirectory providersessions.CursorWalkDirectory, cursorResolveSymlinks providersessions.CursorResolveSymlinks, cursorOpenDatabase providersessions.CursorOpenSQLDatabase) error {
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
	return s.detailsForRef(providers.SessionRef{Provider: providerID, Kind: kind, ID: id})
}

// Inspect validates and inspects a detached typed SessionRef through the same
// storage-backed lookup path as Details, returning a plain InspectResult.
func (s *inspectionService) Inspect(req providersessions.InspectRequest) (providersessions.InspectResult, error) {
	detail, err := s.detailsForRef(req.Session)
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
	detail, err := s.detailsForRef(req.Session)
	if err != nil {
		return providersessions.ProjectResult{}, err
	}
	return providersessions.ProjectResult{
		Session: req.Session.Clone(),
		Detail:  detail,
	}, nil
}

func (s *inspectionService) detailsForRef(ref providers.SessionRef) (providersessions.Detail, error) {
	if ref.Provider != providers.IDCodex && ref.Provider != providers.IDCursor {
		return providersessions.Detail{}, providersessions.ErrUnsupportedProvider
	}
	if strings.TrimSpace(ref.Kind) != providers.SessionIDKind {
		return providersessions.Detail{}, providersessions.ErrUnsupportedKind
	}
	if strings.TrimSpace(ref.ID) == "" {
		return providersessions.Detail{}, providersessions.ErrInvalidIdentifier
	}
	if ref.Provider == providers.IDCursor {
		return s.cursorReader.Read(ref)
	}

	detail, err := codex.LoadDetails(s.files, s.codexWalkDirectory, s.codexResolveSymlinks, s.codexRoot, ref.ID)
	if err == nil {
		return detail, nil
	}
	return providersessions.Detail{}, &providersessions.LookupError{
		Provider: providersessions.ProviderCodex,
		Root:     s.codexRoot,
		Err:      err,
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
