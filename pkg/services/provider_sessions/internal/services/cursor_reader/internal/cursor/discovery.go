package cursor

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
)

// UpstreamSourceCommit is the iksnae/cursor-session default-branch HEAD recorded at port time.
const UpstreamSourceCommit = "340f0f72a760ba8b454eac814f986d1a6a4c2f57"

// UpstreamRepositoryURL is the MIT-licensed source repository for cursor-agent CLI storage parsing.
const UpstreamRepositoryURL = "https://github.com/iksnae/cursor-session"

// AgentStorageRoot is the server-side root for cursor-agent CLI session storage (v1 required backend).
// Layout: {root}/{workspace-hash}/{session-id}/store.db
type AgentStorageRoot string

// DefaultAgentStorageRoot returns the default cursor-agent chats directory for the current OS.
// v1 reads cursor-agent CLI storage only; Cursor desktop globalStorage is out of scope.
func DefaultAgentStorageRoot(resolveHome providersessions.ResolveHomeDirectory, files providersessions.FileSystem, operatingSystem providersessions.OperatingSystem) (AgentStorageRoot, error) {
	if strings.TrimSpace(string(operatingSystem)) == "" {
		return "", fmt.Errorf("cursor operating system is required")
	}
	home, err := resolveHome()
	if err != nil {
		return "", fmt.Errorf("home directory: %w", err)
	}
	return defaultAgentStorageRoot(string(operatingSystem), home, files.Stat)
}

func defaultAgentStorageRoot(
	goos string,
	home string,
	stat func(string) (fs.FileInfo, error),
) (AgentStorageRoot, error) {
	candidates, err := defaultAgentStorageRootCandidates(goos, home)
	if err != nil {
		return "", err
	}
	for _, candidate := range candidates {
		if info, statErr := stat(candidate); statErr == nil && info.IsDir() {
			return AgentStorageRoot(candidate), nil
		}
	}
	if len(candidates) == 0 {
		return "", nil
	}
	return AgentStorageRoot(""), nil
}

func defaultAgentStorageRootCandidates(goos string, home string) ([]string, error) {
	switch goos {
	case "linux":
		return []string{
			filepath.Join(home, ".config", "cursor", "chats"),
			filepath.Join(home, ".cursor", "chats"),
		}, nil
	case "darwin":
		return []string{
			filepath.Join(home, ".cursor", "chats"),
			filepath.Join(home, ".config", "cursor", "chats"),
		}, nil
	case "windows":
		return []string{
			filepath.Join(home, ".cursor", "chats"),
			filepath.Join(home, ".config", "cursor", "chats"),
		}, nil
	default:
		return nil, fmt.Errorf("unsupported OS for cursor-agent storage: %s", goos)
	}
}

// NormalizeAgentStorageRoot cleans and validates a configured root directory.
func NormalizeAgentStorageRoot(resolveHome providersessions.ResolveHomeDirectory, files providersessions.FileSystem, operatingSystem providersessions.OperatingSystem, root string) (AgentStorageRoot, error) {
	trimmed := filepath.Clean(strings.TrimSpace(root))
	if trimmed == "" || trimmed == "." {
		defaultRoot, err := DefaultAgentStorageRoot(resolveHome, files, operatingSystem)
		if err != nil {
			return "", err
		}
		return defaultRoot, nil
	}
	clean, err := filepath.Abs(trimmed)
	if err != nil {
		return "", fmt.Errorf("resolve cursor storage root: %w", err)
	}
	info, err := files.Stat(clean)
	if err != nil {
		return "", fmt.Errorf("stat cursor storage root: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("cursor storage root is not a directory: %s", clean)
	}
	return AgentStorageRoot(clean), nil
}
