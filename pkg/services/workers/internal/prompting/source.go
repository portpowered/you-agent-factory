package prompting

import (
	"fmt"
	"strings"
)

// PromptSourceFileSystem is the narrow read boundary used to refresh an
// authored prompt for one dispatch.
type PromptSourceFileSystem interface {
	ReadFile(string) ([]byte, error)
}

// ResolveAuthoredPromptSource reads one fixed authored prompt source. Body
// sources are AGENTS.md files whose frontmatter is discarded; template files
// are returned verbatim. The caller owns the source identity and decides when
// this operation runs.
func ResolveAuthoredPromptSource(
	fileSystem PromptSourceFileSystem,
	path string,
	bodySource bool,
) (string, error) {
	if fileSystem == nil {
		return "", fmt.Errorf("prompt source filesystem is required")
	}
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("prompt source path is required")
	}
	data, err := fileSystem.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read prompt source %s: %w", path, err)
	}
	if !bodySource {
		return string(data), nil
	}
	return authoredPromptBody(data, path)
}

func authoredPromptBody(data []byte, path string) (string, error) {
	content := string(data)
	rest := content
	closingDelimiter := "\n---\n"
	closingLength := len(closingDelimiter)
	switch {
	case strings.HasPrefix(content, "---\n"):
		rest = content[len("---\n"):]
	case strings.HasPrefix(content, "---\r\n"):
		rest = content[len("---\r\n"):]
		closingDelimiter = "\r\n---\r\n"
		closingLength = len(closingDelimiter)
	default:
		return content, nil
	}

	index := strings.Index(rest, closingDelimiter)
	if index < 0 {
		if strings.HasSuffix(strings.TrimSpace(rest), "---") {
			return "", nil
		}
		return "", fmt.Errorf(
			"parse authored prompt source %s: missing closing frontmatter delimiter",
			path,
		)
	}
	return strings.TrimSpace(rest[index+closingLength:]), nil
}
