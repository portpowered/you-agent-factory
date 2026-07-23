package factorydefinitions

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
)

// FactoryConfigPathSource is the policy-free file metadata effect used by the
// Factory Definitions owner to resolve an explicitly selected config file.
type FactoryConfigPathSource interface {
	Stat(string) (fs.FileInfo, error)
}

// FactoryConfigParser expands canonical Factory Definition bytes.
type FactoryConfigParser func([]byte) (*FactoryConfig, error)

type FactoryConfigRootResolver func(string) (string, error)
type FactoryConfigFileLoader func(string) (*FactoryConfig, error)

func NewFactoryConfigRootResolver(source FactoryConfigPathSource) FactoryConfigRootResolver {
	return func(path string) (string, error) {
		trimmed := strings.TrimSpace(path)
		if trimmed == "" {
			return "", fmt.Errorf("factory config path is required")
		}
		if source == nil {
			return "", fmt.Errorf("Factory Definitions config path source is required")
		}
		resolved, err := filepath.Abs(trimmed)
		if err != nil {
			return "", fmt.Errorf("resolve factory config path %s: %w", trimmed, err)
		}
		info, err := source.Stat(resolved)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return "", fmt.Errorf("factory config file not found: %s", trimmed)
			}
			return "", fmt.Errorf("find factory config file %s: %w", trimmed, err)
		}
		if info.IsDir() {
			return resolved, nil
		}
		if !info.Mode().IsRegular() {
			return "", fmt.Errorf("factory config path must be a file or directory: %s", trimmed)
		}
		return filepath.Dir(resolved), nil
	}
}

func NewFactoryConfigFileLoader(
	loadSource AuthoredFactorySourceLoader,
	parse FactoryConfigParser,
) FactoryConfigFileLoader {
	return func(path string) (*FactoryConfig, error) {
		trimmed := strings.TrimSpace(path)
		if trimmed == "" {
			return nil, fmt.Errorf("factory config path is required")
		}
		if loadSource == nil {
			return nil, fmt.Errorf("Factory Definitions authored source loader is required")
		}
		if parse == nil {
			return nil, fmt.Errorf("Factory Definitions config parser is required")
		}
		source, err := loadSource(trimmed)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil, fmt.Errorf("factory config file not found: %s", trimmed)
			}
			return nil, fmt.Errorf("read factory config file %s: %w", trimmed, err)
		}
		cfg, err := parse(source.Data)
		if err != nil {
			return nil, fmt.Errorf(
				"parse factory config %s (%s): %w",
				source.Path,
				source.Format,
				err,
			)
		}
		return cfg, nil
	}
}
