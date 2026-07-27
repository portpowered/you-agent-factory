// Package service implements the Provider Sessions service contract.
package service

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/provider_sessions/cursor"
	codexreader "github.com/portpowered/infinite-you/pkg/services/provider_sessions/internal/services/codex_reader"
	codexreaderwire "github.com/portpowered/infinite-you/pkg/services/provider_sessions/internal/services/codex_reader/wire"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

type inspectionService struct {
	codex                 codexreader.Service
	codexRoot             string
	cursorRoot            cursor.AgentStorageRoot
	cursorWalkDirectory   providersessions.CursorWalkDirectory
	cursorResolveSymlinks providersessions.CursorResolveSymlinks
	cursorOpenDatabase    providersessions.CursorOpenSQLDatabase
	files                 providersessions.FileSystem
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
	codexRoot, err := codexreaderwire.DefaultSessionsRoot(resolveHome)
	if err != nil {
		return nil, err
	}
	cursorRoot, err := cursor.DefaultAgentStorageRoot(resolveHome, files, cursorOperatingSystem)
	if err != nil {
		return nil, err
	}
	return newForRoots(files, codexWalkDirectory, codexResolveSymlinks, cursorWalkDirectory, cursorResolveSymlinks, cursorOpenDatabase, codexRoot, string(cursorRoot))
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
		codex:                 codexService,
		codexRoot:             codexRoot,
		cursorRoot:            cursor.AgentStorageRoot(cursorRoot),
		cursorWalkDirectory:   cursorWalkDirectory,
		cursorResolveSymlinks: cursorResolveSymlinks,
		cursorOpenDatabase:    cursorOpenDatabase,
		files:                 files,
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
	normalizedProvider, err := normalizeProvider(provider)
	if err != nil {
		return providersessions.Detail{}, err
	}
	if strings.TrimSpace(kind) != providersessions.SessionIDKind {
		return providersessions.Detail{}, providersessions.ErrUnsupportedKind
	}

	var detail providersessions.Detail
	switch normalizedProvider {
	case providersessions.ProviderCodex:
		detail, err = s.codex.Details(context.Background(), providers.SessionRef{
			Provider: providers.IDCodex,
			Kind:     kind,
			ID:       id,
		})
	case providersessions.ProviderCursor:
		detail, err = cursor.LoadDetails(s.files, s.cursorWalkDirectory, s.cursorResolveSymlinks, s.cursorOpenDatabase, s.cursorRoot, id)
	}
	if err == nil {
		return detail, nil
	}
	return providersessions.Detail{}, &providersessions.LookupError{
		Provider: normalizedProvider,
		Root:     s.rootFor(normalizedProvider),
		Err:      err,
	}
}

// Inspect validates and inspects a detached typed SessionRef through the same
// storage-backed lookup path as Details, returning a plain InspectResult.
func (s *inspectionService) Inspect(req providersessions.InspectRequest) (providersessions.InspectResult, error) {
	if err := validateSessionRef(req.Session); err != nil {
		return providersessions.InspectResult{}, err
	}
	detail, err := s.Details(req.Session.Provider.String(), req.Session.Kind, req.Session.ID)
	if err != nil {
		return providersessions.InspectResult{}, err
	}
	return providersessions.InspectResult{
		Session: providers.SessionRef{
			Provider: canonicalProviderID(detail.ProviderSession.Provider),
			Kind:     detail.ProviderSession.Kind,
			ID:       detail.ProviderSession.ID,
		},
		Source: detail.Source,
	}, nil
}

// Project projects provider-independent transcript/detail facts for a detached
// typed SessionRef through the same storage-backed lookup path as Details.
func (s *inspectionService) Project(req providersessions.ProjectRequest) (providersessions.ProjectResult, error) {
	if err := validateSessionRef(req.Session); err != nil {
		return providersessions.ProjectResult{}, err
	}
	detail, err := s.Details(req.Session.Provider.String(), req.Session.Kind, req.Session.ID)
	if err != nil {
		return providersessions.ProjectResult{}, err
	}
	return providersessions.ProjectResult{
		Session: providers.SessionRef{
			Provider: canonicalProviderID(detail.ProviderSession.Provider),
			Kind:     detail.ProviderSession.Kind,
			ID:       detail.ProviderSession.ID,
		},
		Detail: detail,
	}, nil
}

func normalizeProvider(provider string) (providersessions.Provider, error) {
	switch strings.TrimSpace(provider) {
	case string(providersessions.ProviderCodex):
		return providersessions.ProviderCodex, nil
	case string(providersessions.ProviderCursor), "agent", "cursor-agent":
		return providersessions.ProviderCursor, nil
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

func canonicalProviderID(provider providersessions.Provider) providers.ID {
	if provider == providersessions.ProviderCursor {
		return providers.IDCursor
	}
	return providers.IDCodex
}

func (s *inspectionService) rootFor(provider providersessions.Provider) string {
	switch provider {
	case providersessions.ProviderCursor:
		return string(s.cursorRoot)
	default:
		return s.codexRoot
	}
}
