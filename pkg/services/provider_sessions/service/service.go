// Package service implements the Provider Sessions service contract.
package service

import (
	"fmt"
	"path/filepath"
	"strings"

	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/provider_sessions/codex"
	"github.com/portpowered/infinite-you/pkg/services/provider_sessions/cursor"
)

type inspectionService struct {
	codexRoot             string
	codexWalkDirectory    providersessions.CodexWalkDirectory
	codexResolveSymlinks  providersessions.CodexResolveSymlinks
	cursorRoot            cursor.AgentStorageRoot
	cursorWalkDirectory   providersessions.CursorWalkDirectory
	cursorResolveSymlinks providersessions.CursorResolveSymlinks
	cursorOpenDatabase    providersessions.CursorOpenSQLDatabase
	files                 providersessions.FileSystem
}

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
	cursorRoot, err := cursor.DefaultAgentStorageRoot(resolveHome, files, cursorOperatingSystem)
	if err != nil {
		return nil, err
	}
	return newForRoots(files, codexWalkDirectory, codexResolveSymlinks, cursorWalkDirectory, cursorResolveSymlinks, cursorOpenDatabase, codexRoot, string(cursorRoot)), nil
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
	return newForRoots(files, codexWalkDirectory, codexResolveSymlinks, cursorWalkDirectory, cursorResolveSymlinks, cursorOpenDatabase, codexRoot, cursorRoot), nil
}

func newForRoots(files providersessions.FileSystem, codexWalkDirectory providersessions.CodexWalkDirectory, codexResolveSymlinks providersessions.CodexResolveSymlinks, cursorWalkDirectory providersessions.CursorWalkDirectory, cursorResolveSymlinks providersessions.CursorResolveSymlinks, cursorOpenDatabase providersessions.CursorOpenSQLDatabase, codexRoot, cursorRoot string) *inspectionService {
	codexRoot = filepath.Clean(codexRoot)
	return &inspectionService{
		codexRoot:             codexRoot,
		codexWalkDirectory:    codexWalkDirectory,
		codexResolveSymlinks:  codexResolveSymlinks,
		cursorRoot:            cursor.AgentStorageRoot(cursorRoot),
		cursorWalkDirectory:   cursorWalkDirectory,
		cursorResolveSymlinks: cursorResolveSymlinks,
		cursorOpenDatabase:    cursorOpenDatabase,
		files:                 files,
	}
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
		detail, err = codex.LoadDetails(s.files, s.codexWalkDirectory, s.codexResolveSymlinks, s.codexRoot, id)
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

func (s *inspectionService) rootFor(provider providersessions.Provider) string {
	switch provider {
	case providersessions.ProviderCursor:
		return string(s.cursorRoot)
	default:
		return s.codexRoot
	}
}
