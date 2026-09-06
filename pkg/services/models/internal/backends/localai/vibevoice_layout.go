package localai

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/models"
	modelartifacts "github.com/portpowered/infinite-you/pkg/services/models/internal/artifacts"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
)

const (
	vibeVoiceBackendID    = "localai-vibevoice"
	vibeVoiceTokenizerKey = "tokenizer"
	vibeVoiceVoiceKey     = "voice"
)

var errVibeVoiceLayout = errors.New("vibevoice role layout is invalid")

func vibeVoiceLoadOptions(
	request modelseffects.HostProtocolNegotiationRequest,
	resolveSymlinks modelseffects.HostResolveSymlinks,
) ([]string, error) {
	if !strings.EqualFold(strings.TrimSpace(request.Backend), vibeVoiceBackendID) ||
		!strings.EqualFold(strings.TrimSpace(request.ModelName), models.BuiltInModelNameTTS) {
		return nil, nil
	}
	manifest, err := modelartifacts.DefaultModelRoleManifest()
	if err != nil {
		return nil, errVibeVoiceLayout
	}
	definition, ok := manifest.Model(models.BuiltInModelNameTTS)
	if !ok || (strings.TrimSpace(request.Revision) != "" &&
		strings.TrimSpace(request.Revision) != definition.Publication.Revision) {
		return nil, errVibeVoiceLayout
	}
	paths, err := confinedVibeVoiceRolePaths(
		request.ModelPath, request.ModelFiles, definition, resolveSymlinks,
	)
	if err != nil {
		return nil, errVibeVoiceLayout
	}
	return []string{
		vibeVoiceTokenizerKey + "=" + paths["tokenizer"],
		vibeVoiceVoiceKey + "=" + paths["voice"],
	}, nil
}

func confinedVibeVoiceRolePaths(
	modelPath string,
	modelFiles []string,
	definition modelartifacts.ModelRoleModel,
	resolveSymlinks modelseffects.HostResolveSymlinks,
) (map[string]string, error) {
	modelFile := strings.TrimSpace(modelPath)
	if modelFile == "" || len(modelFiles) != len(definition.Artifacts) {
		return nil, errVibeVoiceLayout
	}
	modelFile = filepath.Clean(filepath.FromSlash(modelFile))
	root := filepath.Dir(modelFile)
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, errVibeVoiceLayout
	}
	modelRootName := filepath.ToSlash(filepath.Clean(filepath.Dir(filepath.FromSlash(modelFile))))
	expectedPaths := make(map[string]string, len(definition.Artifacts))
	for _, artifact := range definition.Artifacts {
		expectedPaths[filepath.ToSlash(filepath.Clean(filepath.FromSlash(artifact.Path)))] = artifact.Role
	}

	paths := make(map[string]string, len(expectedPaths))
	seenCandidates := make(map[string]struct{}, len(modelFiles))
	for _, raw := range modelFiles {
		candidate, err := confinedVibeVoiceCandidate(
			raw, root, absoluteRoot, modelRootName, resolveSymlinks,
		)
		if err != nil {
			return nil, errVibeVoiceLayout
		}
		absoluteCandidate, err := filepath.Abs(candidate)
		if err != nil {
			return nil, errVibeVoiceLayout
		}
		relative, err := filepath.Rel(absoluteRoot, absoluteCandidate)
		if err != nil || filepath.IsAbs(relative) {
			return nil, errVibeVoiceLayout
		}
		relativeName := filepath.ToSlash(relative)
		role, ok := expectedPaths[relativeName]
		if !ok || relativeName == "." {
			return nil, errVibeVoiceLayout
		}
		if _, duplicate := seenCandidates[absoluteCandidate]; duplicate {
			return nil, errVibeVoiceLayout
		}
		seenCandidates[absoluteCandidate] = struct{}{}
		if _, duplicate := paths[role]; duplicate {
			return nil, errVibeVoiceLayout
		}
		paths[role] = candidate
	}
	if len(paths) != len(expectedPaths) {
		return nil, errVibeVoiceLayout
	}
	modelCandidate := paths["model"]
	modelAbsolute, err := filepath.Abs(modelFile)
	if err != nil {
		return nil, errVibeVoiceLayout
	}
	candidateAbsolute, err := filepath.Abs(modelCandidate)
	if err != nil || filepath.Clean(modelAbsolute) != filepath.Clean(candidateAbsolute) {
		return nil, errVibeVoiceLayout
	}
	return paths, nil
}

func confinedVibeVoiceCandidate(
	raw, root, absoluteRoot, modelRootName string,
	resolveSymlinks modelseffects.HostResolveSymlinks,
) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", errVibeVoiceLayout
	}
	candidate := filepath.Clean(filepath.FromSlash(value))
	if !filepath.IsAbs(candidate) {
		if filepath.ToSlash(candidate) != filepath.ToSlash(value) {
			return "", errVibeVoiceLayout
		}
		valueName := filepath.ToSlash(value)
		if modelRootName != "." &&
			(valueName == modelRootName || strings.HasPrefix(valueName, modelRootName+"/")) {
			candidate = filepath.Clean(filepath.FromSlash(value))
		} else {
			candidate = filepath.Clean(filepath.Join(root, candidate))
		}
	}
	absoluteCandidate, err := filepath.Abs(candidate)
	if err != nil || !pathWithinVibeVoiceRoot(absoluteRoot, absoluteCandidate) ||
		!resolvedVibeVoicePathWithinRoot(resolveSymlinks, absoluteRoot, absoluteCandidate) {
		return "", errVibeVoiceLayout
	}
	return candidate, nil
}

func pathWithinVibeVoiceRoot(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err != nil || filepath.IsAbs(relative) {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func resolvedVibeVoicePathWithinRoot(
	resolveSymlinks modelseffects.HostResolveSymlinks,
	root, candidate string,
) bool {
	if resolveSymlinks == nil {
		return true
	}
	resolvedRoot, rootErr := resolveSymlinks(root)
	if rootErr != nil {
		if os.IsNotExist(rootErr) {
			return true
		}
		return false
	}
	for current := candidate; ; current = filepath.Dir(current) {
		if _, err := os.Lstat(current); err == nil {
			resolvedCandidate, candidateErr := resolveSymlinks(current)
			if candidateErr != nil {
				return false
			}
			return pathWithinVibeVoiceRoot(resolvedRoot, resolvedCandidate)
		} else if !os.IsNotExist(err) {
			return false
		}
		parent := filepath.Dir(current)
		if parent == current {
			return false
		}
	}
}
