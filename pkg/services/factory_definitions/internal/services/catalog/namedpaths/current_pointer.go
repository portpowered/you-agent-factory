package namedpaths

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
)

type Resolver struct {
	fileSystem interface {
		ReadFile(string) ([]byte, error)
		Stat(string) (fs.FileInfo, error)
		MkdirAll(string, fs.FileMode) error
		WriteFile(string, []byte, fs.FileMode) error
	}
}

func New(fileSystem interface {
	ReadFile(string) ([]byte, error)
	Stat(string) (fs.FileInfo, error)
	MkdirAll(string, fs.FileMode) error
	WriteFile(string, []byte, fs.FileMode) error
}) (*Resolver, error) {
	if fileSystem == nil {
		return nil, fmt.Errorf("named Factory path filesystem is required")
	}
	return &Resolver{fileSystem: fileSystem}, nil
}

const currentFactoryPointerFile = ".current-factory"
const factoryConfigFile = "factory.json"

// ErrLayoutNotFound reports that a root contains neither a valid current
// pointer nor a directly authored Factory definition.
var ErrLayoutNotFound = errors.New("factory layout not found")

// ReadCurrentPointer returns the canonical named Factory selected at rootDir.
func (r *Resolver) ReadCurrentPointer(rootDir string) (string, error) {
	if r == nil || r.fileSystem == nil {
		return "", fmt.Errorf("named Factory path filesystem is required")
	}
	pointerPath := filepath.Join(rootDir, currentFactoryPointerFile)
	data, err := r.fileSystem.ReadFile(pointerPath)
	if err != nil {
		return "", err
	}
	name := strings.TrimSpace(string(data))
	if name == "" {
		return "", fmt.Errorf(
			"read current factory pointer %s: factory name is required for factory config layout",
			pointerPath,
		)
	}
	if err := ValidateName(name); err != nil {
		return "", fmt.Errorf(
			"read current factory pointer %s: %w",
			pointerPath,
			err,
		)
	}
	return name, nil
}

// ResolveCurrentDir resolves the selected named Factory, falling back to a
// direct factory.json rooted at rootDir.
func (r *Resolver) ResolveCurrentDir(rootDir string) (string, error) {
	if strings.TrimSpace(rootDir) == "" {
		return "", fmt.Errorf("factory root is required")
	}
	name, err := r.ReadCurrentPointer(rootDir)
	if err == nil {
		return r.ResolveExistingDir(rootDir, name)
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}

	configPath := filepath.Join(rootDir, factoryConfigFile)
	info, err := r.fileSystem.Stat(configPath)
	if err == nil && info.Mode().IsRegular() {
		return rootDir, nil
	}
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("find factory config %s: %w", configPath, err)
	}
	if err == nil {
		return "", fmt.Errorf("factory config %s is not a regular file", configPath)
	}
	return "", fmt.Errorf(
		"resolve current factory in %s: %w",
		rootDir,
		ErrLayoutNotFound,
	)
}

// WriteCurrentPointer persists the selected named Factory after verifying that
// its canonical definition directory exists.
func (r *Resolver) WriteCurrentPointer(rootDir, name string) error {
	if r == nil || r.fileSystem == nil {
		return fmt.Errorf("named Factory path filesystem is required")
	}
	if strings.TrimSpace(rootDir) == "" {
		return fmt.Errorf("factory root is required")
	}
	segments, err := PathSegments(name)
	if err != nil {
		return err
	}
	canonicalName, err := NameFromPathSegments(segments)
	if err != nil {
		return err
	}
	factoryDir, err := MapDir(rootDir, canonicalName)
	if err != nil {
		return err
	}
	configPath := filepath.Join(factoryDir, factoryConfigFile)
	if info, statErr := r.fileSystem.Stat(configPath); statErr != nil {
		return fmt.Errorf("set current factory %q: find factory config %s: %w", canonicalName, configPath, statErr)
	} else if info.IsDir() {
		return fmt.Errorf("set current factory %q: factory config %s is a directory", canonicalName, configPath)
	}
	if err := r.fileSystem.MkdirAll(rootDir, 0o755); err != nil {
		return fmt.Errorf("create factory root %s: %w", rootDir, err)
	}
	path := filepath.Join(rootDir, currentFactoryPointerFile)
	if err := r.fileSystem.WriteFile(path, []byte(canonicalName+"\n"), 0o644); err != nil {
		return fmt.Errorf("write current factory pointer %s: %w", path, err)
	}
	return nil
}
