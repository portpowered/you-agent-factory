package cursor

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
)

var safeSessionIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

var (
	ErrInvalidSessionID = errors.New("invalid cursor session identifier")
	ErrSessionNotFound  = errors.New("cursor session not found")
	ErrAmbiguousSession = errors.New("ambiguous cursor session file")
)

// ValidateSessionID rejects path-like or unsafe session identifiers from API clients.
func ValidateSessionID(id string) error {
	normalized := strings.TrimSpace(id)
	if normalized == "" || !safeSessionIDPattern.MatchString(normalized) {
		return ErrInvalidSessionID
	}
	return nil
}

// ResolvedStoreDB is a cursor-agent store.db file resolved under a configured root.
type ResolvedStoreDB struct {
	SessionID    string
	AbsolutePath string
	RelativePath string
}

// ResolveStoreDB locates {root}/{hash}/{sessionID}/store.db without accepting client filesystem paths.
func ResolveStoreDB(files providersessions.FileSystem, walkDirectory providersessions.CursorWalkDirectory, resolveSymlinks providersessions.CursorResolveSymlinks, root AgentStorageRoot, sessionID string) (ResolvedStoreDB, error) {
	if walkDirectory == nil {
		return ResolvedStoreDB{}, fmt.Errorf("cursor session directory walker is required")
	}
	if resolveSymlinks == nil {
		return ResolvedStoreDB{}, fmt.Errorf("cursor session symlink resolver is required")
	}
	if err := ValidateSessionID(sessionID); err != nil {
		return ResolvedStoreDB{}, err
	}
	if string(root) == "" {
		return ResolvedStoreDB{}, ErrSessionNotFound
	}

	cleanRoot, resolvedRoot, err := resolveAgentStorageRoot(files, resolveSymlinks, string(root))
	if err != nil {
		return ResolvedStoreDB{}, err
	}

	targetName := "store.db"
	matches, err := collectStoreDBMatches(walkDirectory, cleanRoot, sessionID, targetName)
	if err != nil {
		return ResolvedStoreDB{}, err
	}
	if len(matches) == 0 {
		return ResolvedStoreDB{}, ErrSessionNotFound
	}
	sort.Strings(matches)

	candidates := make([]ResolvedStoreDB, 0, len(matches))
	for _, match := range matches {
		candidate, candidateErr := resolvedStoreDBCandidate(resolveSymlinks, cleanRoot, resolvedRoot, match, sessionID)
		if candidateErr != nil {
			return ResolvedStoreDB{}, candidateErr
		}
		candidates = append(candidates, candidate)
	}
	if len(candidates) == 1 {
		return candidates[0], nil
	}
	if len(candidates) > 1 {
		return ResolvedStoreDB{}, ErrAmbiguousSession
	}
	return ResolvedStoreDB{}, ErrSessionNotFound
}

func resolveAgentStorageRoot(files providersessions.FileSystem, resolveSymlinks providersessions.CursorResolveSymlinks, root string) (string, string, error) {
	cleanRoot, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return "", "", fmt.Errorf("resolve cursor storage root: %w", err)
	}
	rootInfo, err := files.Stat(cleanRoot)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", "", ErrSessionNotFound
		}
		return "", "", fmt.Errorf("stat cursor storage root: %w", err)
	}
	if !rootInfo.IsDir() {
		return "", "", fmt.Errorf("cursor storage root is not a directory: %s", cleanRoot)
	}
	resolvedRoot, err := resolveSymlinks(cleanRoot)
	if err != nil {
		return "", "", fmt.Errorf("resolve cursor storage root symlinks: %w", err)
	}
	return cleanRoot, resolvedRoot, nil
}

func collectStoreDBMatches(walkDirectory providersessions.CursorWalkDirectory, cleanRoot, sessionID, targetName string) ([]string, error) {
	matches := make([]string, 0, 1)
	err := walkDirectory(cleanRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Name() != targetName {
			return nil
		}
		if filepath.Base(filepath.Dir(path)) != sessionID {
			return nil
		}
		matches = append(matches, path)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk cursor storage root: %w", err)
	}
	return matches, nil
}

func resolvedStoreDBCandidate(resolveSymlinks providersessions.CursorResolveSymlinks, cleanRoot, resolvedRoot, match, sessionID string) (ResolvedStoreDB, error) {
	resolvedMatch, err := resolveSymlinks(match)
	if err != nil {
		return ResolvedStoreDB{}, fmt.Errorf("resolve cursor session symlink: %w", err)
	}
	if !pathInsideRoot(resolvedRoot, resolvedMatch) {
		return ResolvedStoreDB{}, ErrInvalidSessionID
	}
	rel, err := filepath.Rel(cleanRoot, match)
	if err != nil {
		return ResolvedStoreDB{}, fmt.Errorf("rel cursor session file: %w", err)
	}
	return ResolvedStoreDB{
		SessionID:    sessionID,
		AbsolutePath: resolvedMatch,
		RelativePath: filepath.ToSlash(rel),
	}, nil
}

func pathInsideRoot(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !hasPathPrefix(rel, ".."+string(filepath.Separator)))
}

func hasPathPrefix(value, prefix string) bool {
	if len(value) < len(prefix) {
		return false
	}
	return value[:len(prefix)] == prefix
}
