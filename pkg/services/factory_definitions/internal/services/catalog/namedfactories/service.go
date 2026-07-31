package namedfactories

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	namedfactorypath "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog/namedpaths"
)

type PathResolver interface {
	ResolveExistingDir(string, string) (string, error)
	RequireDefinitionDir(string) error
	ResolveCurrentDir(string) (string, error)
	ReadCurrentPointer(string) (string, error)
	WriteCurrentPointer(string, string) error
}

type FileSystem interface {
	Stat(string) (os.FileInfo, error)
	ReadDir(string) ([]os.DirEntry, error)
	RemoveAll(string) error
}

func ValidateName(name string) error {
	_, err := canonicalName(name)
	return err
}

func PathSegments(name string) ([]string, error) {
	segments, err := namedfactorypath.PathSegments(name)
	if err != nil {
		return nil, &invalidNameError{name: strings.TrimSpace(name), err: err}
	}
	return segments, nil
}

func NameFromPathSegments(segments []string) (string, error) {
	name, err := namedfactorypath.NameFromPathSegments(segments)
	if err != nil {
		return "", &invalidNameError{
			name: strings.Join(segments, "/"),
			err:  err,
		}
	}
	return name, nil
}

func MapDir(rootDir, name string) (string, error) {
	if strings.TrimSpace(rootDir) == "" {
		return "", fmt.Errorf("factory root is required")
	}
	canonical, err := canonicalName(name)
	if err != nil {
		return "", err
	}
	return namedfactorypath.MapDir(rootDir, canonical)
}

func Resolve(paths PathResolver, fileSystem FileSystem, rootDir, name string) (string, error) {
	if paths == nil {
		return "", fmt.Errorf("named Factory path resolver is required")
	}
	if fileSystem == nil {
		return "", fmt.Errorf("named Factory catalog filesystem is required")
	}
	canonical, err := canonicalName(name)
	if err != nil {
		return "", err
	}
	factoryDir, err := namedfactorypath.MapDir(rootDir, canonical)
	if err != nil {
		return "", err
	}
	err = paths.RequireDefinitionDir(factoryDir)
	if err == nil {
		return factoryDir, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		if info, statErr := fileSystem.Stat(factoryDir); statErr == nil && info.IsDir() {
			return "", fmt.Errorf(
				"resolve named factory %q in root %s: existing target could not be loaded: %w",
				canonical,
				rootDir,
				err,
			)
		} else if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
			return "", fmt.Errorf("stat named factory %q directory: %w", canonical, statErr)
		}
		return "", fmt.Errorf(
			"resolve named factory %q in root %s: %w",
			canonical,
			rootDir,
			&notFoundError{name: canonical},
		)
	}
	return "", err
}

func ResolveCurrent(paths PathResolver, rootDir string) (string, error) {
	if paths == nil {
		return "", fmt.Errorf("named Factory path resolver is required")
	}
	factoryDir, err := paths.ResolveCurrentDir(rootDir)
	if err == nil {
		return factoryDir, nil
	}
	if errors.Is(err, namedfactorypath.ErrLayoutNotFound) {
		return "", fmt.Errorf(
			"resolve current factory in %s: %w",
			rootDir,
			factorydefinitions.ErrFactoryLayoutNotFound,
		)
	}
	if errors.Is(err, namedfactorypath.ErrInvalidName) {
		return "", &invalidNameError{name: "", err: err}
	}
	if errors.Is(err, namedfactorypath.ErrNotFound) {
		return "", fmt.Errorf("%w: %w", factorydefinitions.ErrNamedFactoryNotFound, err)
	}
	return "", err
}

func ReadCurrentPointer(paths PathResolver, rootDir string) (string, error) {
	if paths == nil {
		return "", fmt.Errorf("named Factory path resolver is required")
	}
	name, err := paths.ReadCurrentPointer(rootDir)
	if err != nil && errors.Is(err, namedfactorypath.ErrInvalidName) {
		return "", &invalidNameError{name: "", err: err}
	}
	return name, err
}

func WriteCurrentPointer(paths PathResolver, rootDir, name string) error {
	if paths == nil {
		return fmt.Errorf("named Factory path resolver is required")
	}
	canonical, err := canonicalName(name)
	if err != nil {
		return err
	}
	return paths.WriteCurrentPointer(rootDir, canonical)
}

// pkgmaintcheck:ignore-cyclomatic-complexity service-ownership migration preserves this decision flow; simplify branches and remove this exemption.
func List(paths PathResolver, fileSystem FileSystem, rootDir string) ([]factorydefinitions.NamedFactoryListEntry, error) {
	if paths == nil {
		return nil, fmt.Errorf("named Factory path resolver is required")
	}
	if fileSystem == nil {
		return nil, fmt.Errorf("named Factory catalog filesystem is required")
	}
	if err := validateRoot(fileSystem, rootDir); err != nil {
		return nil, err
	}
	currentName, err := readOptionalCurrent(paths, rootDir)
	if err != nil {
		return nil, err
	}
	children, err := fileSystem.ReadDir(rootDir)
	if err != nil {
		return nil, fmt.Errorf("read factory root %s: %w", rootDir, err)
	}

	entries := make([]factorydefinitions.NamedFactoryListEntry, 0, len(children))
	seen := make(map[string]struct{}, len(children))
	appendEntry := func(name, factoryDir string) {
		if _, exists := seen[name]; exists {
			return
		}
		seen[name] = struct{}{}
		entries = append(entries, factorydefinitions.NamedFactoryListEntry{
			Name:       name,
			FactoryDir: factoryDir,
			Current:    name == currentName,
		})
	}

	for _, child := range children {
		if !child.IsDir() || skipRootChild(child.Name()) {
			continue
		}
		childDir := filepath.Join(rootDir, child.Name())
		if isFactoryDir(fileSystem, childDir) {
			name, canonicalErr := namedfactorypath.NameFromPathSegments([]string{child.Name()})
			if canonicalErr == nil {
				appendEntry(name, childDir)
			}
			continue
		}
		if !strings.HasPrefix(child.Name(), "@") {
			continue
		}
		scopedChildren, readErr := fileSystem.ReadDir(childDir)
		if readErr != nil {
			continue
		}
		for _, scopedChild := range scopedChildren {
			if !scopedChild.IsDir() || isStagingDir(scopedChild.Name()) {
				continue
			}
			factoryDir := filepath.Join(childDir, scopedChild.Name())
			if !isFactoryDir(fileSystem, factoryDir) {
				continue
			}
			name, canonicalErr := namedfactorypath.NameFromPathSegments(
				[]string{child.Name(), scopedChild.Name()},
			)
			if canonicalErr == nil {
				appendEntry(name, factoryDir)
			}
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})
	return entries, nil
}

func Delete(paths PathResolver, fileSystem FileSystem, rootDir, name string) error {
	if paths == nil {
		return fmt.Errorf("named Factory path resolver is required")
	}
	if fileSystem == nil {
		return fmt.Errorf("named Factory catalog filesystem is required")
	}
	if strings.TrimSpace(rootDir) == "" {
		return fmt.Errorf("factory root is required")
	}
	if err := namedfactorypath.ValidateName(name); err != nil {
		return err
	}
	segments, err := namedfactorypath.PathSegments(name)
	if err != nil {
		return err
	}
	canonicalName, err := namedfactorypath.NameFromPathSegments(segments)
	if err != nil {
		return err
	}
	factoryDir, err := paths.ResolveExistingDir(rootDir, canonicalName)
	if err != nil {
		if errors.Is(err, namedfactorypath.ErrNotFound) {
			return &notFoundError{name: canonicalName}
		}
		return err
	}

	current, err := paths.ReadCurrentPointer(rootDir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read current factory pointer: %w", err)
	}
	if current == canonicalName {
		return fmt.Errorf(
			"delete factory %q: %w: switch .current-factory to another factory first",
			canonicalName,
			factorydefinitions.ErrNamedFactoryIsCurrent,
		)
	}
	if err := fileSystem.RemoveAll(factoryDir); err != nil {
		return fmt.Errorf("delete factory %q: %w", canonicalName, err)
	}
	return nil
}

func ResolveAcrossRoots(
	paths PathResolver,
	projectRoot string,
	globalRoot string,
	name string,
) (*factorydefinitions.NamedFactoryResolution, error) {
	if paths == nil {
		return nil, fmt.Errorf("named Factory path resolver is required")
	}
	projectRoot = strings.TrimSpace(projectRoot)
	globalRoot = strings.TrimSpace(globalRoot)
	if projectRoot == "" {
		return nil, fmt.Errorf("project factory root is required")
	}
	if globalRoot == "" {
		return nil, fmt.Errorf("global factory root is required")
	}
	canonicalName, err := canonicalName(name)
	if err != nil {
		return nil, err
	}

	if factoryDir, found, resolveErr := resolveCandidate(paths, projectRoot, canonicalName); resolveErr != nil {
		return nil, resolveErr
	} else if found {
		precedence := factorydefinitions.NamedFactoryPrecedenceDecisionNone
		if _, globalFound, _ := resolveCandidate(paths, globalRoot, canonicalName); globalFound {
			precedence = factorydefinitions.NamedFactoryPrecedenceDecisionProjectOverGlobal
		}
		return &factorydefinitions.NamedFactoryResolution{
			Name:               canonicalName,
			FactoryDir:         factoryDir,
			Source:             factorydefinitions.NamedFactoryResolutionSourceProjectLocal,
			ProjectRoot:        projectRoot,
			GlobalRoot:         globalRoot,
			PrecedenceDecision: precedence,
		}, nil
	}
	if factoryDir, found, resolveErr := resolveCandidate(paths, globalRoot, canonicalName); resolveErr != nil {
		return nil, resolveErr
	} else if found {
		return &factorydefinitions.NamedFactoryResolution{
			Name:               canonicalName,
			FactoryDir:         factoryDir,
			Source:             factorydefinitions.NamedFactoryResolutionSourceGlobal,
			ProjectRoot:        projectRoot,
			GlobalRoot:         globalRoot,
			PrecedenceDecision: factorydefinitions.NamedFactoryPrecedenceDecisionNone,
		}, nil
	}
	return nil, fmt.Errorf(
		"resolve named factory %q in project root %s or global root %s: %w",
		canonicalName,
		projectRoot,
		globalRoot,
		&notFoundError{name: canonicalName},
	)
}

type notFoundError struct {
	name string
}

type invalidNameError struct {
	name string
	err  error
}

func (e *invalidNameError) Error() string {
	return fmt.Sprintf("invalid named factory name %q: %v", e.name, e.err)
}

func (e *invalidNameError) Unwrap() []error {
	return []error{factorydefinitions.ErrInvalidNamedFactoryName, e.err}
}

func (e *notFoundError) Error() string {
	return fmt.Sprintf("named factory %q not found", e.name)
}

func (e *notFoundError) Unwrap() []error {
	return []error{factorydefinitions.ErrNamedFactoryNotFound, os.ErrNotExist}
}

func canonicalName(name string) (string, error) {
	segments, err := namedfactorypath.PathSegments(name)
	if err != nil {
		return "", &invalidNameError{name: strings.TrimSpace(name), err: err}
	}
	canonical, err := namedfactorypath.NameFromPathSegments(segments)
	if err != nil {
		return "", &invalidNameError{name: strings.TrimSpace(name), err: err}
	}
	return canonical, nil
}

func resolveCandidate(paths PathResolver, rootDir, name string) (string, bool, error) {
	factoryDir, err := paths.ResolveExistingDir(rootDir, name)
	if err == nil {
		return factoryDir, true, nil
	}
	if errors.Is(err, namedfactorypath.ErrNotFound) {
		return "", false, nil
	}
	return "", false, err
}

func validateRoot(fileSystem FileSystem, rootDir string) error {
	if strings.TrimSpace(rootDir) == "" {
		return fmt.Errorf("factory root is required")
	}
	info, err := fileSystem.Stat(rootDir)
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

func readOptionalCurrent(paths PathResolver, rootDir string) (string, error) {
	name, err := paths.ReadCurrentPointer(rootDir)
	if err == nil {
		return name, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	return "", fmt.Errorf("read current factory pointer: %w", err)
}

func skipRootChild(name string) bool {
	return name == factorydefinitions.InputsDir ||
		name == factorydefinitions.WorkersDir ||
		name == factorydefinitions.WorkstationsDir ||
		isStagingDir(name) ||
		strings.Contains(name, "%2F")
}

func isStagingDir(name string) bool {
	return strings.HasPrefix(name, ".") && strings.Contains(name, ".staging-")
}

func isFactoryDir(fileSystem FileSystem, factoryDir string) bool {
	info, err := fileSystem.Stat(filepath.Join(factoryDir, factorydefinitions.FactoryConfigFile))
	return err == nil && info.Mode().IsRegular()
}
