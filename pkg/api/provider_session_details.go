package api

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"go.uber.org/zap"
)

const (
	loadableProviderSessionProvider = "codex"
	loadableProviderSessionKind     = "session_id"
)

var safeProviderSessionIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

var (
	errInvalidProviderSessionIdentifier = errors.New("invalid provider session identifier")
	errProviderSessionNotFound          = errors.New("provider session not found")
)

func (s *Server) GetProviderSessionDetails(
	w http.ResponseWriter,
	r *http.Request,
	params factoryapi.GetProviderSessionDetailsParams,
) {
	details, err := loadProviderSessionDetails(
		s.codexSessionsRoot,
		string(params.Provider),
		string(params.Kind),
		string(params.Id),
	)
	if err != nil {
		switch {
		case errors.Is(err, errInvalidProviderSessionIdentifier):
			s.writeError(w, http.StatusBadRequest, "provider session must be a codex session_id identifier without path separators", "BAD_REQUEST")
			return
		case errors.Is(err, errProviderSessionNotFound):
			s.writeError(w, http.StatusNotFound, "provider session not found", "NOT_FOUND")
			return
		default:
			s.logger.Error("load provider session details failed", zap.Error(err))
			s.writeError(w, http.StatusInternalServerError, "failed to load provider session details", "INTERNAL_ERROR")
			return
		}
	}

	s.writeJSON(w, http.StatusOK, details)
}

func loadProviderSessionDetails(root, provider, kind, id string) (factoryapi.ProviderSessionDetailResponse, error) {
	normalizedProvider := strings.ToLower(strings.TrimSpace(provider))
	normalizedKind := strings.TrimSpace(kind)
	normalizedID := strings.TrimSpace(id)
	if normalizedProvider != loadableProviderSessionProvider ||
		normalizedKind != loadableProviderSessionKind ||
		!safeProviderSessionIDPattern.MatchString(normalizedID) {
		return factoryapi.ProviderSessionDetailResponse{}, errInvalidProviderSessionIdentifier
	}

	resolved, err := resolveCodexSessionFile(root, normalizedID)
	if err != nil {
		return factoryapi.ProviderSessionDetailResponse{}, err
	}

	file, err := os.Open(resolved.absolutePath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return factoryapi.ProviderSessionDetailResponse{}, errProviderSessionNotFound
		}
		return factoryapi.ProviderSessionDetailResponse{}, fmt.Errorf("open provider session file: %w", err)
	}
	defer file.Close()

	parse, err := parseCodexSessionSummary(file)
	if err != nil {
		return factoryapi.ProviderSessionDetailResponse{}, err
	}

	return factoryapi.ProviderSessionDetailResponse{
		ProviderSession: factoryapi.ProviderSessionMetadata{
			Provider: &normalizedProvider,
			Kind:     &normalizedKind,
			Id:       &normalizedID,
		},
		Source: factoryapi.ProviderSessionSourceMetadata{
			RelativePath: resolved.relativePath,
			SizeBytes:    resolved.sizeBytes,
			ModifiedAt:   resolved.modifiedAt,
		},
		Parse: parse,
	}, nil
}

type resolvedCodexSessionFile struct {
	absolutePath string
	relativePath string
	sizeBytes    int64
	modifiedAt   *time.Time
}

func resolveCodexSessionFile(root, id string) (resolvedCodexSessionFile, error) {
	cleanRoot, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return resolvedCodexSessionFile{}, fmt.Errorf("resolve codex sessions root: %w", err)
	}

	rootInfo, err := os.Stat(cleanRoot)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return resolvedCodexSessionFile{}, errProviderSessionNotFound
		}
		return resolvedCodexSessionFile{}, fmt.Errorf("stat codex sessions root: %w", err)
	}
	if !rootInfo.IsDir() {
		return resolvedCodexSessionFile{}, fmt.Errorf("codex sessions root is not a directory: %s", cleanRoot)
	}

	targetName := "rollout-" + id + ".jsonl"
	matches := make([]string, 0, 1)
	err = filepath.WalkDir(cleanRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&fs.ModeType != 0 && entry.Type()&fs.ModeSymlink == 0 {
			return nil
		}
		if filepath.Base(path) == targetName {
			matches = append(matches, path)
		}
		return nil
	})
	if err != nil {
		return resolvedCodexSessionFile{}, fmt.Errorf("walk codex sessions root: %w", err)
	}
	if len(matches) == 0 {
		return resolvedCodexSessionFile{}, errProviderSessionNotFound
	}
	sort.Strings(matches)

	resolvedRoot, err := filepath.EvalSymlinks(cleanRoot)
	if err != nil {
		return resolvedCodexSessionFile{}, fmt.Errorf("resolve codex sessions root symlinks: %w", err)
	}
	for _, match := range matches {
		resolvedMatch, err := filepath.EvalSymlinks(match)
		if err != nil {
			return resolvedCodexSessionFile{}, fmt.Errorf("resolve provider session symlink: %w", err)
		}
		if !pathInsideRoot(resolvedRoot, resolvedMatch) {
			return resolvedCodexSessionFile{}, errInvalidProviderSessionIdentifier
		}
		info, err := os.Stat(resolvedMatch)
		if err != nil {
			return resolvedCodexSessionFile{}, fmt.Errorf("stat provider session file: %w", err)
		}
		rel, err := filepath.Rel(cleanRoot, match)
		if err != nil {
			return resolvedCodexSessionFile{}, fmt.Errorf("rel provider session file: %w", err)
		}
		modifiedAt := info.ModTime().UTC()
		return resolvedCodexSessionFile{
			absolutePath: resolvedMatch,
			relativePath: filepath.ToSlash(rel),
			sizeBytes:    info.Size(),
			modifiedAt:   &modifiedAt,
		}, nil
	}

	return resolvedCodexSessionFile{}, errProviderSessionNotFound
}

func pathInsideRoot(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func parseCodexSessionSummary(reader io.Reader) (factoryapi.CodexSessionParseSummary, error) {
	summary := factoryapi.CodexSessionParseSummary{}
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		summary.LineCount++
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			summary.MalformedLineCount++
			continue
		}
		summary.EventCount++
		if _, ok := event["type"].(string); !ok {
			summary.UnknownEventCount++
		}
	}
	if err := scanner.Err(); err != nil {
		return factoryapi.CodexSessionParseSummary{}, fmt.Errorf("read provider session stream: %w", err)
	}
	return summary, nil
}

func defaultCodexSessionsRoot() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Clean(".codex/sessions")
	}
	return filepath.Join(home, ".codex", "sessions")
}
