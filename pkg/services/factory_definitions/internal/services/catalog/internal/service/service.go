package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	catalog "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog"
	internalnamedfactories "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog/namedfactories"
)

// Service is the private nested catalog implementation behind the CTR-DEF
// root catalog slice.
type Service struct {
	paths      internalnamedfactories.PathResolver
	fileSystem internalnamedfactories.FileSystem
}

var _ catalog.Service = (*Service)(nil)

// New constructs the catalog implementation from exact injected ports.
func New(
	paths internalnamedfactories.PathResolver,
	fileSystem internalnamedfactories.FileSystem,
) *Service {
	if paths == nil || fileSystem == nil {
		return nil
	}
	return &Service{paths: paths, fileSystem: fileSystem}
}

func (s *Service) ListNamedFactories(
	_ context.Context,
	request factorydefinitions.ListNamedFactoriesRequest,
) (factorydefinitions.ListNamedFactoriesResult, error) {
	if err := s.requirePorts(); err != nil {
		return factorydefinitions.ListNamedFactoriesResult{}, err
	}
	entries, err := internalnamedfactories.List(s.paths, s.fileSystem, request.RootDir)
	if err != nil {
		return factorydefinitions.ListNamedFactoriesResult{}, err
	}
	return factorydefinitions.ListNamedFactoriesResult{Entries: entries}, nil
}

func (s *Service) GetNamedFactory(
	_ context.Context,
	request factorydefinitions.GetNamedFactoryRequest,
) (factorydefinitions.GetNamedFactoryResult, error) {
	if err := s.requirePorts(); err != nil {
		return factorydefinitions.GetNamedFactoryResult{}, err
	}
	factoryDir, err := internalnamedfactories.Resolve(
		s.paths,
		s.fileSystem,
		request.RootDir,
		request.Name,
	)
	if err != nil {
		return factorydefinitions.GetNamedFactoryResult{}, err
	}
	canonical, err := canonicalName(request.Name)
	if err != nil {
		return factorydefinitions.GetNamedFactoryResult{}, err
	}
	currentName, err := readOptionalCurrent(s.paths, request.RootDir)
	if err != nil {
		return factorydefinitions.GetNamedFactoryResult{}, err
	}
	return factorydefinitions.GetNamedFactoryResult{
		Entry: factorydefinitions.NamedFactoryListEntry{
			Name:       canonical,
			FactoryDir: factoryDir,
			Current:    canonical == currentName,
		},
	}, nil
}

func (s *Service) ResolveNamedFactory(
	_ context.Context,
	request factorydefinitions.ResolveNamedFactoryRequest,
) (factorydefinitions.ResolveNamedFactoryResult, error) {
	if err := s.requirePorts(); err != nil {
		return factorydefinitions.ResolveNamedFactoryResult{}, err
	}
	resolution, err := internalnamedfactories.ResolveAcrossRoots(
		s.paths,
		request.ProjectRoot,
		request.GlobalRoot,
		request.Name,
	)
	if err != nil {
		return factorydefinitions.ResolveNamedFactoryResult{}, err
	}
	if resolution == nil {
		return factorydefinitions.ResolveNamedFactoryResult{}, factorydefinitions.ErrNamedFactoryNotFound
	}
	return factorydefinitions.ResolveNamedFactoryResult{Resolution: *resolution}, nil
}

func (s *Service) DeleteNamedFactory(
	_ context.Context,
	request factorydefinitions.DeleteNamedFactoryRequest,
) (factorydefinitions.DeleteNamedFactoryResult, error) {
	if err := s.requirePorts(); err != nil {
		return factorydefinitions.DeleteNamedFactoryResult{}, err
	}
	canonical, err := canonicalName(request.Name)
	if err != nil {
		return factorydefinitions.DeleteNamedFactoryResult{}, err
	}
	factoryDir, err := internalnamedfactories.Resolve(
		s.paths,
		s.fileSystem,
		request.RootDir,
		canonical,
	)
	if err != nil {
		return factorydefinitions.DeleteNamedFactoryResult{}, err
	}
	if err := internalnamedfactories.Delete(
		s.paths,
		s.fileSystem,
		request.RootDir,
		canonical,
	); err != nil {
		return factorydefinitions.DeleteNamedFactoryResult{}, err
	}
	return factorydefinitions.DeleteNamedFactoryResult{
		Name:       canonical,
		FactoryDir: factoryDir,
	}, nil
}

func (s *Service) GetCurrentFactoryPointer(
	_ context.Context,
	request factorydefinitions.GetCurrentFactoryPointerRequest,
) (factorydefinitions.GetCurrentFactoryPointerResult, error) {
	if err := s.requirePorts(); err != nil {
		return factorydefinitions.GetCurrentFactoryPointerResult{}, err
	}
	name, err := s.paths.ReadCurrentPointer(request.RootDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return factorydefinitions.GetCurrentFactoryPointerResult{}, factorydefinitions.ErrCurrentFactoryNotFound
		}
		return factorydefinitions.GetCurrentFactoryPointerResult{}, err
	}
	factoryDir, err := s.paths.ResolveExistingDir(request.RootDir, name)
	if err != nil {
		if errors.Is(err, factorydefinitions.ErrNamedFactoryNotFound) {
			return factorydefinitions.GetCurrentFactoryPointerResult{}, factorydefinitions.ErrCurrentFactoryNotFound
		}
		return factorydefinitions.GetCurrentFactoryPointerResult{}, err
	}
	return factorydefinitions.GetCurrentFactoryPointerResult{
		Name:       name,
		FactoryDir: factoryDir,
	}, nil
}

func (s *Service) SetCurrentFactoryPointer(
	_ context.Context,
	request factorydefinitions.SetCurrentFactoryPointerRequest,
) (factorydefinitions.SetCurrentFactoryPointerResult, error) {
	if err := s.requirePorts(); err != nil {
		return factorydefinitions.SetCurrentFactoryPointerResult{}, err
	}
	canonical, err := canonicalName(request.Name)
	if err != nil {
		return factorydefinitions.SetCurrentFactoryPointerResult{}, err
	}
	if _, err := internalnamedfactories.Resolve(
		s.paths,
		s.fileSystem,
		request.RootDir,
		canonical,
	); err != nil {
		return factorydefinitions.SetCurrentFactoryPointerResult{}, err
	}
	if err := internalnamedfactories.WriteCurrentPointer(s.paths, request.RootDir, canonical); err != nil {
		return factorydefinitions.SetCurrentFactoryPointerResult{}, err
	}
	return factorydefinitions.SetCurrentFactoryPointerResult{Name: canonical}, nil
}

func (s *Service) ClearCurrentFactoryPointer(
	_ context.Context,
	request factorydefinitions.ClearCurrentFactoryPointerRequest,
) (factorydefinitions.ClearCurrentFactoryPointerResult, error) {
	if err := s.requirePorts(); err != nil {
		return factorydefinitions.ClearCurrentFactoryPointerResult{}, err
	}
	rootDir := strings.TrimSpace(request.RootDir)
	if rootDir == "" {
		return factorydefinitions.ClearCurrentFactoryPointerResult{}, fmt.Errorf("factory root is required")
	}
	pointerPath := filepath.Join(rootDir, factorydefinitions.CurrentFactoryPointerFile)
	if err := s.fileSystem.RemoveAll(pointerPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return factorydefinitions.ClearCurrentFactoryPointerResult{}, fmt.Errorf(
			"clear current factory pointer %s: %w",
			pointerPath,
			err,
		)
	}
	return factorydefinitions.ClearCurrentFactoryPointerResult{RootDir: rootDir}, nil
}

func (s *Service) requirePorts() error {
	if s == nil || s.paths == nil {
		return fmt.Errorf("named Factory path resolver is required")
	}
	if s.fileSystem == nil {
		return fmt.Errorf("named Factory catalog filesystem is required")
	}
	return nil
}

func canonicalName(name string) (string, error) {
	segments, err := internalnamedfactories.PathSegments(name)
	if err != nil {
		return "", err
	}
	return internalnamedfactories.NameFromPathSegments(segments)
}

func readOptionalCurrent(
	paths internalnamedfactories.PathResolver,
	rootDir string,
) (string, error) {
	name, err := paths.ReadCurrentPointer(rootDir)
	if err == nil {
		return name, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	return "", fmt.Errorf("read current factory pointer: %w", err)
}
