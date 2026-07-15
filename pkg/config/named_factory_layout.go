package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/portpowered/infinite-you/pkg/config/defaultpaths"
	"github.com/portpowered/infinite-you/pkg/config/namedfactorypath"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
)

// NamedFactoryListEntry describes one persisted named factory under a factory root.
type NamedFactoryListEntry struct {
	Name       string `json:"name"`
	FactoryDir string `json:"factoryDirectory"`
	Current    bool   `json:"current"`
}

const (
	defaultNamedFactoryHomeDir     = ".you-agent-factory"
	defaultProjectNamedFactoryRoot = "factory"
	scopedNamedFactoryPrefix       = "@"
)

// NamedFactoryPathSegments returns the validated hierarchical path segments for
// a canonical named-factory display name.
func NamedFactoryPathSegments(name string) ([]string, error) {
	segments, err := namedfactorypath.PathSegments(name)
	if err != nil {
		return nil, wrapInvalidNamedFactoryName(name, err)
	}
	return segments, nil
}

// NamedFactoryNameFromPathSegments reconstructs the canonical named-factory
// display name from validated hierarchical path segments.
func NamedFactoryNameFromPathSegments(segments []string) (string, error) {
	name, err := namedfactorypath.NameFromPathSegments(segments)
	if err != nil {
		return "", wrapInvalidNamedFactoryName(strings.Join(segments, "/"), err)
	}
	return name, nil
}

// MapNamedFactoryDir maps a canonical named-factory display name to its
// hierarchical on-disk directory under factoriesRoot.
func MapNamedFactoryDir(factoriesRoot, name string) (string, error) {
	if strings.TrimSpace(factoriesRoot) == "" {
		return "", fmt.Errorf("factory root is required")
	}
	dir, err := namedfactorypath.MapDir(factoriesRoot, name)
	if err != nil {
		return "", wrapInvalidNamedFactoryName(name, err)
	}
	return dir, nil
}

func namedFactoryStagingPrefix(name string) string {
	safe := strings.NewReplacer("/", "--", `\`, "--", "@", "").Replace(strings.TrimSpace(name))
	if safe == "" {
		safe = "factory"
	}
	return "." + safe + ".staging-"
}

// GlobalNamedFactoryRootForHome builds the customer-owned global named-factory
// root for a resolved home directory.
func GlobalNamedFactoryRootForHome(homeDir string) (string, error) {
	trimmed := strings.TrimSpace(homeDir)
	if trimmed == "" {
		return "", fmt.Errorf("user home directory is required")
	}
	return defaultpaths.NamedFactoriesRoot(trimmed), nil
}

// GlobalWorkflowRootForHome builds the customer-owned global workflow lookup root
// for a resolved home directory.
func GlobalWorkflowRootForHome(homeDir string) (string, error) {
	trimmed := strings.TrimSpace(homeDir)
	if trimmed == "" {
		return "", fmt.Errorf("user home directory is required")
	}
	return filepath.Join(trimmed, defaultNamedFactoryHomeDir, "workflows"), nil
}

// DefaultGlobalNamedFactoryRoot returns the default global named-factory root
// under the current user's home directory.
func DefaultGlobalNamedFactoryRoot() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home for named factories: %w", err)
	}
	return GlobalNamedFactoryRootForHome(homeDir)
}

// DefaultProjectNamedFactoryRoot returns the default project-local named
// factory root for a caller working directory.
func DefaultProjectNamedFactoryRoot(cwd string) (string, error) {
	trimmed := strings.TrimSpace(cwd)
	if trimmed == "" {
		return "", fmt.Errorf("working directory is required")
	}
	return filepath.Join(trimmed, defaultProjectNamedFactoryRoot), nil
}

// ListNamedFactories discovers persisted named factories by scanning rootDir for
// subdirectories that contain a valid factory.json layout.
func ListNamedFactories(rootDir string) ([]NamedFactoryListEntry, error) {
	if err := validateNamedFactoryListRoot(rootDir); err != nil {
		return nil, err
	}

	currentName, err := readCurrentFactoryPointerForList(rootDir)
	if err != nil {
		return nil, err
	}

	children, err := os.ReadDir(rootDir)
	if err != nil {
		return nil, fmt.Errorf("read factory root %s: %w", rootDir, err)
	}

	collector := newNamedFactoryListCollector(currentName, len(children))
	for _, child := range children {
		collectNamedFactoriesFromRootChild(rootDir, child, collector)
	}

	entries := collector.entries()
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})
	return entries, nil
}

func validateNamedFactoryListRoot(rootDir string) error {
	if strings.TrimSpace(rootDir) == "" {
		return fmt.Errorf("factory root is required")
	}
	info, err := os.Stat(rootDir)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("factory root %s does not exist: %w", rootDir, err)
		}
		return fmt.Errorf("stat factory root %s: %w", rootDir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("factory root %s is not a directory", rootDir)
	}
	return nil
}

func isReservedNamedFactoryListDir(name string) bool {
	return name == interfaces.InputsDir || name == interfaces.WorkersDir || name == interfaces.WorkstationsDir
}

func isNamedFactoryStagingDir(name string) bool {
	return strings.HasPrefix(name, ".") && strings.Contains(name, ".staging-")
}

func isLegacyEncodedNamedFactoryLeaf(name string) bool {
	return strings.Contains(name, "%2F")
}

func shouldSkipNamedFactoryListEntry(name string) bool {
	return isReservedNamedFactoryListDir(name) ||
		isNamedFactoryStagingDir(name) ||
		isLegacyEncodedNamedFactoryLeaf(name)
}

func collectNamedFactoriesFromRootChild(rootDir string, child os.DirEntry, collector *namedFactoryListCollector) {
	if !child.IsDir() {
		return
	}
	name := child.Name()
	if shouldSkipNamedFactoryListEntry(name) {
		return
	}
	factoryDir := filepath.Join(rootDir, name)
	if err := requireFactoryConfig(factoryDir); err == nil {
		displayName, err := canonicalNamedFactoryName(name)
		if err != nil {
			return
		}
		collector.append(displayName, factoryDir)
		return
	}
	collectScopedNamedFactories(factoryDir, name, collector)
}

func collectScopedNamedFactories(scopeDir, scopeName string, collector *namedFactoryListCollector) {
	if !strings.HasPrefix(scopeName, scopedNamedFactoryPrefix) {
		return
	}
	scopeChildren, err := os.ReadDir(scopeDir)
	if err != nil {
		return
	}
	for _, scopeChild := range scopeChildren {
		if !scopeChild.IsDir() || isNamedFactoryStagingDir(scopeChild.Name()) {
			continue
		}
		scopedFactoryDir := filepath.Join(scopeDir, scopeChild.Name())
		if err := requireFactoryConfig(scopedFactoryDir); err != nil {
			continue
		}
		displayName, err := NamedFactoryNameFromPathSegments([]string{scopeName, scopeChild.Name()})
		if err != nil {
			continue
		}
		collector.append(displayName, scopedFactoryDir)
	}
}

type namedFactoryListCollector struct {
	currentName string
	seen        map[string]struct{}
	items       []NamedFactoryListEntry
}

func newNamedFactoryListCollector(currentName string, capacity int) *namedFactoryListCollector {
	return &namedFactoryListCollector{
		currentName: currentName,
		seen:        make(map[string]struct{}, capacity),
		items:       make([]NamedFactoryListEntry, 0, capacity),
	}
}

func (c *namedFactoryListCollector) append(displayName, factoryDir string) {
	if displayName == "" || factoryDir == "" {
		return
	}
	if _, ok := c.seen[displayName]; ok {
		return
	}
	c.seen[displayName] = struct{}{}
	c.items = append(c.items, NamedFactoryListEntry{
		Name:       displayName,
		FactoryDir: factoryDir,
		Current:    c.currentName != "" && displayName == c.currentName,
	})
}

func (c *namedFactoryListCollector) entries() []NamedFactoryListEntry {
	return c.items
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

// ErrNamedFactoryIsCurrent reports that a named factory cannot be deleted
// because it is selected by .current-factory.
var ErrNamedFactoryIsCurrent = errors.New("cannot delete current factory")

// DeleteNamedFactory removes a persisted named factory directory under rootDir.
// It refuses to delete the factory referenced by .current-factory.
func DeleteNamedFactory(rootDir, name string) error {
	if strings.TrimSpace(rootDir) == "" {
		return fmt.Errorf("factory root is required")
	}

	canonicalName, err := canonicalNamedFactoryName(name)
	if err != nil {
		return err
	}

	factoryDir, err := ResolveNamedFactoryDir(rootDir, name)
	if err != nil {
		return err
	}

	current, err := ReadCurrentFactoryPointer(rootDir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read current factory pointer: %w", err)
	}
	if current == canonicalName {
		return fmt.Errorf(
			"delete factory %q: %w: switch .current-factory to another factory first",
			canonicalName,
			ErrNamedFactoryIsCurrent,
		)
	}

	if err := os.RemoveAll(factoryDir); err != nil {
		return fmt.Errorf("delete factory %q: %w", canonicalName, err)
	}
	return nil
}
