package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"

	factoryroot "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	compilationservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation"
)

// Service is the private nested compilation implementation behind the CTR-DEF
// root compile slice.
type Service struct {
	loadCanonical      factoryroot.CanonicalFactoryJSONLoader
	loadFromFactoryDir factoryroot.LoadedFactoryLoader
	encodeFactory      factoryroot.FactoryConfigJSONEncoder
}

var _ compilationservice.Service = (*Service)(nil)

// New constructs the compilation implementation from exact injected ports.
func New(
	loadCanonical factoryroot.CanonicalFactoryJSONLoader,
	loadFromFactoryDir factoryroot.LoadedFactoryLoader,
	encodeFactory factoryroot.FactoryConfigJSONEncoder,
) *Service {
	return &Service{
		loadCanonical:      loadCanonical,
		loadFromFactoryDir: loadFromFactoryDir,
		encodeFactory:      encodeFactory,
	}
}

// LoadCanonicalFactorySource preserves the injected canonical loader's arguments,
// result and error for existing validation consumers.
func (s *Service) LoadCanonicalFactorySource(
	payload []byte,
	workstationLoader factoryroot.WorkstationLoader,
) (factoryroot.MutableLoadedFactorySource, error) {
	return s.loadCanonical(payload, workstationLoader)
}

// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
func (s *Service) CompileEffectiveFactorySource(
	ctx context.Context,
	request factoryroot.CompileEffectiveFactorySourceRequest,
) (factoryroot.CompileEffectiveFactorySourceResult, error) {
	if err := ctx.Err(); err != nil {
		return factoryroot.CompileEffectiveFactorySourceResult{}, err
	}

	canonical := bytes.TrimSpace(request.Canonical)
	factoryDir := strings.TrimSpace(request.FactoryDir)
	if len(canonical) == 0 && factoryDir == "" {
		return factoryroot.CompileEffectiveFactorySourceResult{}, factoryroot.ErrInvalidAuthoredFactorySource
	}
	if len(canonical) > 0 && bytes.Equal(canonical, []byte("{")) {
		return factoryroot.CompileEffectiveFactorySourceResult{}, factoryroot.ErrInvalidAuthoredFactorySource
	}
	if len(canonical) > 0 && strings.Contains(string(canonical), `"$unresolved"`) {
		return factoryroot.CompileEffectiveFactorySourceResult{}, factoryroot.ErrUnresolvedDefinitionReference
	}

	var (
		loaded factoryroot.MutableLoadedFactorySource
		err    error
	)
	switch {
	case len(canonical) > 0:
		loaded, err = s.loadCanonical(canonical, nil)
	case factoryDir != "":
		loaded, err = s.loadFromFactoryDir(factoryDir, nil)
	}
	if err != nil {
		return factoryroot.CompileEffectiveFactorySourceResult{}, mapCompileError(err)
	}
	if loaded == nil || loaded.FactoryConfig() == nil {
		return factoryroot.CompileEffectiveFactorySourceResult{}, factoryroot.ErrInvalidAuthoredFactorySource
	}

	encoded, err := s.encodeFactory(loaded.FactoryConfig())
	if err != nil {
		return factoryroot.CompileEffectiveFactorySourceResult{}, factoryroot.ErrInvalidAuthoredFactorySource
	}
	contentIdentity := string(bytes.TrimSpace(encoded))
	if contentIdentity == "" {
		return factoryroot.CompileEffectiveFactorySourceResult{}, factoryroot.ErrInvalidAuthoredFactorySource
	}

	effectiveFactoryDir := factoryDir
	if effectiveFactoryDir == "" {
		effectiveFactoryDir = loaded.FactoryDir()
	}
	runtimeBaseDir := loaded.RuntimeBaseDir()
	if runtimeBaseDir == "" {
		runtimeBaseDir = effectiveFactoryDir
	}

	return factoryroot.CompileEffectiveFactorySourceResult{
		Effective: factoryroot.EffectiveFactorySource{
			FactoryDir:      effectiveFactoryDir,
			RuntimeBaseDir:  runtimeBaseDir,
			ContentIdentity: contentIdentity,
		},
	}, nil
}

func mapCompileError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, factoryroot.ErrUnresolvedDefinitionReference) {
		return err
	}
	if strings.Contains(err.Error(), "$unresolved") {
		return factoryroot.ErrUnresolvedDefinitionReference
	}
	if errors.Is(err, factoryroot.ErrInvalidNamedFactory) {
		return factoryroot.ErrInvalidAuthoredFactorySource
	}
	return fmt.Errorf("%w: %w", factoryroot.ErrInvalidAuthoredFactorySource, err)
}
