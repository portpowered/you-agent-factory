package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// NamedFactoryListEntry describes one persisted named factory under a factory root.
type NamedFactoryListEntry struct {
	Name           string `json:"name"`
	FactoryDir     string `json:"factoryDirectory"`
	Current        bool   `json:"current"`
}

// ListNamedFactories discovers persisted named factories by scanning rootDir for
// subdirectories that contain a valid factory.json layout.
func ListNamedFactories(rootDir string) ([]NamedFactoryListEntry, error) {
	if strings.TrimSpace(rootDir) == "" {
		return nil, fmt.Errorf("factory root is required")
	}

	info, err := os.Stat(rootDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("factory root %s does not exist: %w", rootDir, err)
		}
		return nil, fmt.Errorf("stat factory root %s: %w", rootDir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("factory root %s is not a directory", rootDir)
	}

	currentName, err := readCurrentFactoryPointerForList(rootDir)
	if err != nil {
		return nil, err
	}

	children, err := os.ReadDir(rootDir)
	if err != nil {
		return nil, fmt.Errorf("read factory root %s: %w", rootDir, err)
	}

	entries := make([]NamedFactoryListEntry, 0, len(children))
	for _, child := range children {
		if !child.IsDir() {
			continue
		}
		name := child.Name()
		if name == interfaces.InputsDir || name == interfaces.WorkersDir || name == interfaces.WorkstationsDir {
			continue
		}
		factoryDir := filepath.Join(rootDir, name)
		if err := requireFactoryConfig(factoryDir); err != nil {
			continue
		}
		entries = append(entries, NamedFactoryListEntry{
			Name:       name,
			FactoryDir: factoryDir,
			Current:    currentName != "" && name == currentName,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})
	return entries, nil
}

func readCurrentFactoryPointerForList(rootDir string) (string, error) {
	name, err := ReadCurrentFactoryPointer(rootDir)
	if err == nil {
		return name, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	return "", fmt.Errorf("read current factory pointer: %w", err)
}
