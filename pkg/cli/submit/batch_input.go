package submit

import (
	"fmt"
	"os"
	"strings"
)

const (
	batchSourceFile   = "file"
	batchSourceStdin  = "stdin"
	batchSourceInline = "inline"
)

// resolveBatchFileInput selects a filesystem batch path for story-004 file ingress.
// Stdin, inline JSON, and --file are implemented in later stories.
func resolveBatchFileInput(cfg BatchConfig) (path string, err error) {
	if strings.TrimSpace(cfg.FileFlag) != "" {
		return "", fmt.Errorf("batch --file is not yet supported; use a positional file path")
	}
	switch len(cfg.Args) {
	case 0:
		return "", fmt.Errorf("batch input required: provide a file path, pipe JSON, or pass inline JSON (see you submit batch --help)")
	case 1:
		arg := strings.TrimSpace(cfg.Args[0])
		if arg == "-" {
			return "", fmt.Errorf("batch stdin input is not yet supported; use a file path")
		}
		if looksLikeInlineJSON(arg) {
			return "", fmt.Errorf("batch inline JSON is not yet supported; use a file path")
		}
		if _, err := os.Stat(arg); err != nil {
			if os.IsNotExist(err) {
				return "", fmt.Errorf("batch file not found: %s", arg)
			}
			return "", fmt.Errorf("batch file %s: %w", arg, err)
		}
		return arg, nil
	default:
		return "", fmt.Errorf("batch accepts at most one positional argument")
	}
}

func looksLikeInlineJSON(arg string) bool {
	trimmed := strings.TrimLeft(arg, " \t\r\n")
	return trimmed != "" && trimmed[0] == '{'
}
