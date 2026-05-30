package submit

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	batchSourceFile   = "file"
	batchSourceStdin  = "stdin"
	batchSourceInline = "inline"
)

type batchResolvedInput struct {
	data   []byte
	source string
	label  string
}

// resolveBatchInput selects batch JSON bytes from file path or stdin.
// Inline JSON and --file are implemented in later stories.
func resolveBatchInput(cfg BatchConfig) (batchResolvedInput, error) {
	if strings.TrimSpace(cfg.FileFlag) != "" {
		return batchResolvedInput{}, fmt.Errorf("batch --file is not yet supported; use a positional file path or stdin")
	}

	switch len(cfg.Args) {
	case 0:
		if stdinIsTTY(cfg) {
			return batchResolvedInput{}, fmt.Errorf("batch input required: provide a file path, pipe JSON, or pass inline JSON (see you submit batch --help)")
		}
		data, err := readBatchStdin(cfg)
		if err != nil {
			return batchResolvedInput{}, err
		}
		return batchResolvedInput{data: data, source: batchSourceStdin, label: "stdin"}, nil
	case 1:
		arg := strings.TrimSpace(cfg.Args[0])
		if arg == "-" {
			data, err := readBatchStdin(cfg)
			if err != nil {
				return batchResolvedInput{}, err
			}
			return batchResolvedInput{data: data, source: batchSourceStdin, label: "stdin"}, nil
		}
		if looksLikeInlineJSON(arg) {
			return batchResolvedInput{}, fmt.Errorf("batch inline JSON is not yet supported; use a file path or stdin")
		}
		if _, err := os.Stat(arg); err != nil {
			if os.IsNotExist(err) {
				return batchResolvedInput{}, fmt.Errorf("batch file not found: %s", arg)
			}
			return batchResolvedInput{}, fmt.Errorf("batch file %s: %w", arg, err)
		}
		data, err := os.ReadFile(arg)
		if err != nil {
			return batchResolvedInput{}, fmt.Errorf("read %s: %w", arg, err)
		}
		return batchResolvedInput{data: data, source: batchSourceFile, label: arg}, nil
	default:
		return batchResolvedInput{}, fmt.Errorf("batch accepts at most one positional argument")
	}
}

func readBatchStdin(cfg BatchConfig) ([]byte, error) {
	stdin := cfg.Stdin
	if stdin == nil {
		stdin = os.Stdin
	}
	data, err := io.ReadAll(stdin)
	if err != nil {
		return nil, fmt.Errorf("read batch stdin: %w", err)
	}
	if isEmptyBatchInput(data) {
		return nil, fmt.Errorf("batch stdin input is empty")
	}
	return data, nil
}

func isEmptyBatchInput(data []byte) bool {
	return len(bytes.TrimSpace(data)) == 0
}

func stdinIsTTY(cfg BatchConfig) bool {
	if cfg.StdinIsTTY != nil {
		return cfg.StdinIsTTY()
	}
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func looksLikeInlineJSON(arg string) bool {
	trimmed := strings.TrimLeft(arg, " \t\r\n")
	return trimmed != "" && trimmed[0] == '{'
}
