package submit

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
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

// resolveBatchInput selects batch JSON bytes from --file, file path, stdin, or inline JSON.
// When --file is set it wins over positional arguments and ignores unrelated stdin.
func resolveBatchInput(cfg BatchConfig) (batchResolvedInput, error) {
	if fileFlag := strings.TrimSpace(cfg.FileFlag); fileFlag != "" {
		return resolveBatchFileFlag(cfg, fileFlag)
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
			return batchResolvedInput{
				data:   []byte(arg),
				source: batchSourceInline,
				label:  "inline JSON",
			}, nil
		}
		if cfg.FileSystem == nil {
			return batchResolvedInput{}, fmt.Errorf("batch input file system is required")
		}
		if _, err := cfg.FileSystem.Stat(arg); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return batchResolvedInput{}, fmt.Errorf("batch file not found: %s", arg)
			}
			return batchResolvedInput{}, fmt.Errorf("batch file %s: %w", arg, err)
		}
		data, err := cfg.FileSystem.ReadFile(arg)
		if err != nil {
			return batchResolvedInput{}, fmt.Errorf("read %s: %w", arg, err)
		}
		return batchResolvedInput{data: data, source: batchSourceFile, label: arg}, nil
	default:
		return batchResolvedInput{}, fmt.Errorf("batch accepts at most one positional argument")
	}
}

func resolveBatchFileFlag(cfg BatchConfig, path string) (batchResolvedInput, error) {
	if path == "-" {
		data, err := readBatchStdin(cfg)
		if err != nil {
			return batchResolvedInput{}, err
		}
		return batchResolvedInput{data: data, source: batchSourceStdin, label: "stdin"}, nil
	}
	if cfg.FileSystem == nil {
		return batchResolvedInput{}, fmt.Errorf("batch input file system is required")
	}
	if _, err := cfg.FileSystem.Stat(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return batchResolvedInput{}, fmt.Errorf("batch file not found: %s", path)
		}
		return batchResolvedInput{}, fmt.Errorf("batch file %s: %w", path, err)
	}
	data, err := cfg.FileSystem.ReadFile(path)
	if err != nil {
		return batchResolvedInput{}, fmt.Errorf("read %s: %w", path, err)
	}
	return batchResolvedInput{data: data, source: batchSourceFile, label: path}, nil
}

func readBatchStdin(cfg BatchConfig) ([]byte, error) {
	stdin := cfg.Stdin
	if stdin == nil {
		return nil, fmt.Errorf("read batch stdin: process stdin reader is required")
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
	return false
}

func looksLikeInlineJSON(arg string) bool {
	trimmed := strings.TrimLeft(arg, " \t\r\n")
	return trimmed != "" && trimmed[0] == '{'
}
