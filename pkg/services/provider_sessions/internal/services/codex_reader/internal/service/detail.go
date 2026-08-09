package service

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	providersessionsinternal "github.com/portpowered/infinite-you/pkg/services/provider_sessions/internal"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

var safeProviderSessionIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
var codexTimestampPrefixedSessionPattern = regexp.MustCompile(`^rollout-(\d{4}-\d{2}-\d{2}T\d{2}-\d{2}-\d{2})-([A-Za-z0-9_-]+)\.jsonl$`)

// DefaultSessionsRoot returns the conventional Codex session storage root.
func DefaultSessionsRoot(resolveHome providersessionsinternal.ResolveHomeDirectory) (string, error) {
	home, err := resolveHome()
	if err != nil {
		return "", fmt.Errorf("home directory: %w", err)
	}
	if strings.TrimSpace(home) == "" {
		return "", fmt.Errorf("home directory is empty")
	}
	return filepath.Join(home, ".codex", "sessions"), nil
}

// LoadDetails resolves and parses one Codex session from the configured root.
func LoadDetails(files providersessionsinternal.FileSystem, walkDirectory providersessionsinternal.CodexWalkDirectory, resolveSymlinks providersessionsinternal.CodexResolveSymlinks, root, id string) (providersessions.Detail, error) {
	return loadDetails(context.Background(), files, walkDirectory, resolveSymlinks, root, id)
}

func loadDetails(ctx context.Context, files providersessionsinternal.FileSystem, walkDirectory providersessionsinternal.CodexWalkDirectory, resolveSymlinks providersessionsinternal.CodexResolveSymlinks, root, id string) (providersessions.Detail, error) {
	if err := ctx.Err(); err != nil {
		return providersessions.Detail{}, err
	}
	normalizedID := strings.TrimSpace(id)
	if !safeProviderSessionIDPattern.MatchString(normalizedID) {
		return providersessions.Detail{}, providersessions.ErrInvalidIdentifier
	}

	resolved, err := resolveCodexSessionFile(ctx, files, walkDirectory, resolveSymlinks, root, normalizedID)
	if err != nil {
		return providersessions.Detail{}, err
	}
	if err := ctx.Err(); err != nil {
		return providersessions.Detail{}, err
	}

	file, err := files.Open(resolved.absolutePath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return providersessions.Detail{}, providersessions.ErrSessionNotFound
		}
		return providersessions.Detail{}, providersessions.ErrSessionStorageUnavailable
	}
	defer file.Close()
	if err := ctx.Err(); err != nil {
		return providersessions.Detail{}, err
	}

	parsed, err := parseCodexSessionDetailsForSession(ctx, file, normalizedID)
	if err != nil {
		return providersessions.Detail{}, err
	}

	return detachDetail(providersessions.Detail{
		ProviderSession: providersessions.Ref{
			Provider: providersessions.ProviderCodex,
			Kind:     providers.SessionIDKind,
			ID:       normalizedID,
		},
		Source: providersessions.SourceMetadata{
			RelativePath: resolved.relativePath,
			SizeBytes:    resolved.sizeBytes,
			ModifiedAt:   resolved.modifiedAt,
		},
		Parse:      parsed.Summary,
		Transcript: parsed.Transcript,
	}), nil
}

type resolvedCodexSessionFile struct {
	absolutePath string
	relativePath string
	sizeBytes    int64
	modifiedAt   *time.Time
	layout       codexSessionFileLayout
}

type codexSessionFileLayout int

const (
	codexSessionFileLayoutExact codexSessionFileLayout = iota + 1
	codexSessionFileLayoutTimestampPrefixed
)

func resolveCodexSessionFile(ctx context.Context, files providersessionsinternal.FileSystem, walkDirectory providersessionsinternal.CodexWalkDirectory, resolveSymlinks providersessionsinternal.CodexResolveSymlinks, root, id string) (resolvedCodexSessionFile, error) {
	if walkDirectory == nil {
		return resolvedCodexSessionFile{}, fmt.Errorf("codex session directory walker is required")
	}
	if resolveSymlinks == nil {
		return resolvedCodexSessionFile{}, fmt.Errorf("codex session symlink resolver is required")
	}
	if err := ctx.Err(); err != nil {
		return resolvedCodexSessionFile{}, err
	}
	cleanRoot, resolvedRoot, err := resolveCodexSessionsRoot(ctx, files, resolveSymlinks, root)
	if err != nil {
		return resolvedCodexSessionFile{}, err
	}

	targetName := "rollout-" + id + ".jsonl"
	matches, err := collectCodexSessionMatches(ctx, walkDirectory, cleanRoot, id, targetName)
	if err != nil {
		return resolvedCodexSessionFile{}, err
	}
	if len(matches) == 0 {
		return resolvedCodexSessionFile{}, providersessions.ErrSessionNotFound
	}
	sort.Strings(matches)
	return buildResolvedCodexSessionCandidates(ctx, files, resolveSymlinks, cleanRoot, resolvedRoot, matches, targetName)
}

// Resolve locates one Codex rollout without opening or parsing it.
func Resolve(files providersessionsinternal.FileSystem, walkDirectory providersessionsinternal.CodexWalkDirectory, resolveSymlinks providersessionsinternal.CodexResolveSymlinks, root, id string) (providersessions.SourceMetadata, error) {
	resolved, err := resolveCodexSessionFile(context.Background(), files, walkDirectory, resolveSymlinks, root, id)
	if err != nil {
		return providersessions.SourceMetadata{}, err
	}
	return providersessions.SourceMetadata{
		ModifiedAt:   resolved.modifiedAt,
		RelativePath: resolved.relativePath,
		SizeBytes:    resolved.sizeBytes,
	}, nil
}

func resolveCodexSessionsRoot(ctx context.Context, files providersessionsinternal.FileSystem, resolveSymlinks providersessionsinternal.CodexResolveSymlinks, root string) (string, string, error) {
	if strings.TrimSpace(root) == "" {
		return "", "", providersessions.ErrSessionStorageUnavailable
	}
	cleanRoot, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return "", "", providersessions.ErrSessionStorageUnavailable
	}
	if err := ctx.Err(); err != nil {
		return "", "", err
	}
	rootInfo, err := files.Stat(cleanRoot)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", "", providersessions.ErrSessionNotFound
		}
		return "", "", providersessions.ErrSessionStorageUnavailable
	}
	if !rootInfo.IsDir() {
		return "", "", providersessions.ErrSessionStorageUnavailable
	}
	if err := ctx.Err(); err != nil {
		return "", "", err
	}
	resolvedRoot, err := resolveSymlinks(cleanRoot)
	if err != nil {
		return "", "", providersessions.ErrSessionStorageUnavailable
	}
	if err := ctx.Err(); err != nil {
		return "", "", err
	}
	return cleanRoot, resolvedRoot, nil
}

func collectCodexSessionMatches(ctx context.Context, walkDirectory providersessionsinternal.CodexWalkDirectory, cleanRoot, id, targetName string) ([]string, error) {
	matches := make([]string, 0, 1)
	err := walkDirectory(cleanRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return walkErr
		}
		if path != cleanRoot && matchesCodexSessionBaseName(filepath.Base(path), id, targetName) {
			if len(matches) >= maxCodexWalkCandidates {
				return errCodexWalkCandidateLimit
			}
			matches = append(matches, path)
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&fs.ModeType != 0 && entry.Type()&fs.ModeSymlink == 0 {
			return nil
		}
		return nil
	})
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, ctxErr
	}
	if errors.Is(err, errCodexWalkCandidateLimit) {
		return nil, providersessions.ErrSessionStorageUnavailable
	}
	if err != nil {
		return nil, providersessions.ErrSessionStorageUnavailable
	}
	return matches, nil
}

var errCodexWalkCandidateLimit = errors.New("codex session walk candidate limit reached")

func buildResolvedCodexSessionCandidates(ctx context.Context, files providersessionsinternal.FileSystem, resolveSymlinks providersessionsinternal.CodexResolveSymlinks, cleanRoot, resolvedRoot string, matches []string, targetName string) (resolvedCodexSessionFile, error) {
	candidates := make([]resolvedCodexSessionFile, 0, len(matches))
	for _, match := range matches {
		if err := ctx.Err(); err != nil {
			return resolvedCodexSessionFile{}, err
		}
		candidate, err := resolvedCodexSessionCandidate(ctx, files, resolveSymlinks, cleanRoot, resolvedRoot, match, targetName)
		if err != nil {
			return resolvedCodexSessionFile{}, err
		}
		candidates = append(candidates, candidate)
	}
	if err := ctx.Err(); err != nil {
		return resolvedCodexSessionFile{}, err
	}
	return selectResolvedCodexSessionFile(candidates)
}

func resolvedCodexSessionCandidate(ctx context.Context, files providersessionsinternal.FileSystem, resolveSymlinks providersessionsinternal.CodexResolveSymlinks, cleanRoot, resolvedRoot, match, targetName string) (resolvedCodexSessionFile, error) {
	resolvedMatch, err := resolveSymlinks(match)
	if err != nil {
		return resolvedCodexSessionFile{}, providersessions.ErrSessionStorageUnavailable
	}
	if !pathInsideRoot(resolvedRoot, resolvedMatch) {
		return resolvedCodexSessionFile{}, providersessions.ErrSessionOutsideRoot
	}
	if err := ctx.Err(); err != nil {
		return resolvedCodexSessionFile{}, err
	}
	info, err := files.Stat(resolvedMatch)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return resolvedCodexSessionFile{}, providersessions.ErrSessionNotFound
		}
		return resolvedCodexSessionFile{}, providersessions.ErrSessionStorageUnavailable
	}
	if !info.Mode().IsRegular() {
		return resolvedCodexSessionFile{}, providersessions.ErrSessionSourceNotRegularFile
	}
	if err := ctx.Err(); err != nil {
		return resolvedCodexSessionFile{}, err
	}
	rel, err := filepath.Rel(cleanRoot, match)
	if err != nil {
		return resolvedCodexSessionFile{}, providersessions.ErrSessionStorageUnavailable
	}
	modifiedAt := info.ModTime().UTC()
	return resolvedCodexSessionFile{
		absolutePath: resolvedMatch,
		relativePath: filepath.ToSlash(rel),
		sizeBytes:    info.Size(),
		modifiedAt:   &modifiedAt,
		layout:       classifyCodexSessionFileLayout(filepath.Base(match), targetName),
	}, nil
}

func matchesCodexSessionBaseName(baseName, id, exactName string) bool {
	if baseName == exactName {
		return true
	}
	matches := codexTimestampPrefixedSessionPattern.FindStringSubmatch(baseName)
	if matches == nil {
		return false
	}
	return matches[2] == id
}

// MatchesSessionBaseName reports whether a file name is a supported Codex
// rollout layout for the requested session.
func MatchesSessionBaseName(baseName, id, exactName string) bool {
	return matchesCodexSessionBaseName(baseName, id, exactName)
}

func classifyCodexSessionFileLayout(baseName, exactName string) codexSessionFileLayout {
	if baseName == exactName {
		return codexSessionFileLayoutExact
	}
	return codexSessionFileLayoutTimestampPrefixed
}

func selectResolvedCodexSessionFile(candidates []resolvedCodexSessionFile) (resolvedCodexSessionFile, error) {
	exactMatches := make([]resolvedCodexSessionFile, 0, 1)
	timestampMatches := make([]resolvedCodexSessionFile, 0, 1)
	for _, candidate := range candidates {
		switch candidate.layout {
		case codexSessionFileLayoutExact:
			exactMatches = append(exactMatches, candidate)
		case codexSessionFileLayoutTimestampPrefixed:
			timestampMatches = append(timestampMatches, candidate)
		}
	}

	switch {
	case len(exactMatches) == 1:
		return exactMatches[0], nil
	case len(exactMatches) > 1:
		return resolvedCodexSessionFile{}, providersessions.ErrAmbiguousSessionFile
	case len(timestampMatches) == 1:
		return timestampMatches[0], nil
	case len(timestampMatches) > 1:
		return resolvedCodexSessionFile{}, providersessions.ErrAmbiguousSessionFile
	default:
		return resolvedCodexSessionFile{}, providersessions.ErrSessionNotFound
	}
}

func pathInsideRoot(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}
