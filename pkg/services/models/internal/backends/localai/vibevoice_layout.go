package localai

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
)

const (
	vibeVoiceBackendID     = "localai-vibevoice"
	vibeVoiceTokenizerName = "tokenizer.gguf"
	vibeVoiceTokenizerKey  = "tokenizer"
)

var errVibeVoiceLayout = errors.New("vibevoice tokenizer layout is invalid")

func vibeVoiceLoadOptions(
	request modelseffects.HostProtocolNegotiationRequest,
) ([]string, error) {
	if !strings.EqualFold(strings.TrimSpace(request.Backend), vibeVoiceBackendID) ||
		!strings.EqualFold(strings.TrimSpace(request.ModelName), models.BuiltInModelNameTTS) {
		return nil, nil
	}
	tokenizer, err := confinedVibeVoiceTokenizer(request.ModelPath, request.ModelFiles)
	if err != nil {
		return nil, errVibeVoiceLayout
	}
	return []string{vibeVoiceTokenizerKey + "=" + tokenizer}, nil
}

func confinedVibeVoiceTokenizer(modelPath string, modelFiles []string) (string, error) {
	modelFile := strings.TrimSpace(modelPath)
	if modelFile == "" || len(modelFiles) == 0 {
		return "", errVibeVoiceLayout
	}
	modelFile = filepath.Clean(filepath.FromSlash(modelFile))
	root := filepath.Dir(modelFile)
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return "", errVibeVoiceLayout
	}
	modelRootName := filepath.ToSlash(filepath.Clean(filepath.Dir(filepath.FromSlash(modelFile))))

	candidates := make([]string, 0, 1)
	for _, raw := range modelFiles {
		value := strings.TrimSpace(raw)
		if value == "" {
			return "", errVibeVoiceLayout
		}
		candidate := filepath.Clean(filepath.FromSlash(value))
		if !filepath.IsAbs(candidate) {
			if filepath.ToSlash(filepath.Clean(filepath.FromSlash(value))) != filepath.ToSlash(value) {
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
			!resolvedVibeVoicePathWithinRoot(absoluteRoot, absoluteCandidate) {
			return "", errVibeVoiceLayout
		}
		if filepath.Base(candidate) == vibeVoiceTokenizerName {
			candidates = append(candidates, candidate)
		}
	}
	if len(candidates) != 1 {
		return "", errVibeVoiceLayout
	}
	return candidates[0], nil
}

func pathWithinVibeVoiceRoot(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err != nil || filepath.IsAbs(relative) {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func resolvedVibeVoicePathWithinRoot(root, candidate string) bool {
	resolvedRoot, rootErr := filepath.EvalSymlinks(root)
	if rootErr != nil {
		if os.IsNotExist(rootErr) {
			return true
		}
		return false
	}
	for current := candidate; ; current = filepath.Dir(current) {
		if _, err := os.Lstat(current); err == nil {
			resolvedCandidate, candidateErr := filepath.EvalSymlinks(current)
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
