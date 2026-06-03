package cursorstorage

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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
func DefaultAgentStorageRoot() (AgentStorageRoot, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home directory: %w", err)
	}
	switch runtime.GOOS {
	case "linux":
		for _, candidate := range []string{
			filepath.Join(home, ".config", "cursor", "chats"),
			filepath.Join(home, ".cursor", "chats"),
		} {
			if info, statErr := os.Stat(candidate); statErr == nil && info.IsDir() {
				return AgentStorageRoot(candidate), nil
			}
		}
		return AgentStorageRoot(filepath.Join(home, ".cursor", "chats")), nil
	case "darwin":
		// cursor-agent CLI storage is Linux-only in upstream; macOS uses empty default until configured.
		return AgentStorageRoot(""), nil
	default:
		return "", fmt.Errorf("unsupported OS for cursor-agent storage: %s", runtime.GOOS)
	}
}

// NormalizeAgentStorageRoot cleans and validates a configured root directory.
func NormalizeAgentStorageRoot(root string) (AgentStorageRoot, error) {
	trimmed := filepath.Clean(strings.TrimSpace(root))
	if trimmed == "" || trimmed == "." {
		defaultRoot, err := DefaultAgentStorageRoot()
		if err != nil {
			return "", err
		}
		return defaultRoot, nil
	}
	clean, err := filepath.Abs(trimmed)
	if err != nil {
		return "", fmt.Errorf("resolve cursor storage root: %w", err)
	}
	info, err := os.Stat(clean)
	if err != nil {
		return "", fmt.Errorf("stat cursor storage root: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("cursor storage root is not a directory: %s", clean)
	}
	return AgentStorageRoot(clean), nil
}
