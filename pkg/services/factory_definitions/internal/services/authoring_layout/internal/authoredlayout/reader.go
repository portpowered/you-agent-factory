package authoredlayout

import (
	"errors"
	"fmt"
	factoryeffects "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/effects"
	"io/fs"
	"path/filepath"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// WorkerParser and WorkstationParser decode authored AGENTS.md bytes without
// owning their filesystem source.
type WorkerParser func([]byte, string) (*factorydefinitions.FactoryWorkerConfig, error)
type WorkstationParser func([]byte, string) (*factorydefinitions.FactoryWorkstationConfig, error)
type BodyParser func([]byte, string) (string, error)

// Reader owns filesystem reads for the split Factory Definition layout.
// Representation parsing remains an injected transport adapter selected by
// Wire.
type Reader struct {
	parseWorker      WorkerParser
	parseWorkstation WorkstationParser
	parseBody        BodyParser
	fileSystem       factoryeffects.AuthoredLayoutReaderFileSystem
}

// NewReader constructs a split-layout reader from pure representation
// adapters.
func NewReader(
	parseWorker WorkerParser,
	parseWorkstation WorkstationParser,
	parseBody BodyParser,
	fileSystem factoryeffects.AuthoredLayoutReaderFileSystem,
) *Reader {
	return &Reader{
		parseWorker:      parseWorker,
		parseWorkstation: parseWorkstation,
		parseBody:        parseBody,
		fileSystem:       fileSystem,
	}
}

// LoadWorkerConfig reads and parses one Worker AGENTS.md.
func (r *Reader) LoadWorkerConfig(
	dir string,
) (*factorydefinitions.FactoryWorkerConfig, error) {
	if r == nil || r.parseWorker == nil {
		return nil, fmt.Errorf("Factory Definitions Worker parser is required")
	}
	if r.fileSystem == nil {
		return nil, fmt.Errorf("Factory Definitions authored-layout reader filesystem is required")
	}
	path := filepath.Join(dir, factorydefinitions.FactoryAgentsFileName)
	data, err := r.fileSystem.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load worker config from %s: %w", dir, err)
	}
	config, err := r.parseWorker(data, path)
	if err != nil {
		return nil, fmt.Errorf("load worker config from %s: %w", dir, err)
	}
	return config, nil
}

// LoadWorkstationConfig reads and parses one Workstation AGENTS.md and resolves
// its optional prompt-file contents.
func (r *Reader) LoadWorkstationConfig(
	dir string,
) (*factorydefinitions.FactoryWorkstationConfig, error) {
	if r == nil || r.parseWorkstation == nil {
		return nil, fmt.Errorf("Factory Definitions Workstation parser is required")
	}
	if r.fileSystem == nil {
		return nil, fmt.Errorf("Factory Definitions authored-layout reader filesystem is required")
	}
	path := filepath.Join(dir, factorydefinitions.FactoryAgentsFileName)
	data, err := r.fileSystem.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load workstation config from %s: %w", dir, err)
	}
	config, err := r.parseWorkstation(data, path)
	if err != nil {
		return nil, fmt.Errorf("load workstation config from %s: %w", dir, err)
	}
	if config.PromptFile != "" {
		config.PromptTemplate, err = r.LoadWorkstationPromptTemplate(
			dir,
			config.PromptFile,
		)
		if err != nil {
			return nil, err
		}
	}
	return config, nil
}

// LoadWorkerBody reads a Worker AGENTS.md body if the file exists.
func (r *Reader) LoadWorkerBody(dir string) (string, bool, error) {
	if r == nil || r.parseBody == nil {
		return "", false, fmt.Errorf("Factory Definitions AGENTS.md body parser is required")
	}
	if r.fileSystem == nil {
		return "", false, fmt.Errorf("Factory Definitions authored-layout reader filesystem is required")
	}
	path := filepath.Join(dir, factorydefinitions.FactoryAgentsFileName)
	data, err := r.fileSystem.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("load worker body from %s: %w", dir, err)
	}
	body, err := r.parseBody(data, path)
	if err != nil {
		return "", false, fmt.Errorf("load worker body from %s: %w", dir, err)
	}
	return body, true, nil
}

// LoadWorkstationBody reads a body-only Workstation AGENTS.md. Authored files
// with frontmatter are definitions, not body-only prompt overrides.
func (r *Reader) LoadWorkstationBody(dir string) (string, bool, error) {
	if r == nil || r.fileSystem == nil {
		return "", false, fmt.Errorf("Factory Definitions authored-layout reader filesystem is required")
	}
	path := filepath.Join(dir, factorydefinitions.FactoryAgentsFileName)
	data, err := r.fileSystem.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("load workstation body from %s: %w", dir, err)
	}
	content := string(data)
	if strings.HasPrefix(content, "---\n") ||
		strings.HasPrefix(content, "---\r\n") {
		return "", false, nil
	}
	return content, true, nil
}

// LoadWorkstationPromptTemplate reads one prompt referenced by a Workstation.
func (r *Reader) LoadWorkstationPromptTemplate(
	dir string,
	promptFile string,
) (string, error) {
	if r == nil || r.fileSystem == nil {
		return "", fmt.Errorf("Factory Definitions authored-layout reader filesystem is required")
	}
	path := filepath.Join(dir, promptFile)
	data, err := r.fileSystem.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("load prompt file %s: %w", path, err)
	}
	return string(data), nil
}

// SplitRuntimeEntityDirExists reports whether one split-layout entity
// directory exists.
func (r *Reader) SplitRuntimeEntityDirExists(dir string) bool {
	if r == nil || r.fileSystem == nil {
		return false
	}
	info, err := r.fileSystem.Stat(dir)
	return err == nil && info.IsDir()
}
