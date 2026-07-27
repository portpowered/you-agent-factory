package service

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

type effectiveCatalogSource struct {
	listRoot factorydefinitions.EffectiveFactoryRootListing
	read     factorydefinitions.EffectiveFactoryCandidateRead
	packaged []factorydefinitions.PackagedDefinition
}

// NewEffectiveCatalogDiscovery constructs read-only disk and published-package
// discovery.
func NewEffectiveCatalogDiscovery(
	listRoot factorydefinitions.EffectiveFactoryRootListing,
	read factorydefinitions.EffectiveFactoryCandidateRead,
	packaged []factorydefinitions.PackagedDefinition,
) (factorydefinitions.EffectiveFactoryCatalogDiscovery, error) {
	if listRoot == nil {
		return factorydefinitions.EffectiveFactoryCatalogDiscovery{}, fmt.Errorf(
			"persisted Factory catalog is required",
		)
	}
	if read == nil {
		return factorydefinitions.EffectiveFactoryCatalogDiscovery{}, fmt.Errorf(
			"Factory definition filesystem is required",
		)
	}
	cloned := make([]factorydefinitions.PackagedDefinition, len(packaged))
	for index, definition := range packaged {
		definition.JSON = append([]byte(nil), definition.JSON...)
		cloned[index] = definition
	}
	source := effectiveCatalogSource{listRoot: listRoot, read: read, packaged: cloned}
	return factorydefinitions.EffectiveFactoryCatalogDiscovery{
		ListRoot:     source.listRootCandidates,
		ListPackaged: source.listPackagedCandidates,
	}, nil
}

func (s effectiveCatalogSource) listRootCandidates(
	ctx context.Context,
	root string,
) ([]factorydefinitions.EffectiveFactoryCatalogCandidate, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	listed, err := s.listRoot(root)
	if contextErr := ctx.Err(); contextErr != nil {
		return nil, contextErr
	}
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return []factorydefinitions.EffectiveFactoryCatalogCandidate{}, nil
		}
		return nil, err
	}
	candidates := make([]factorydefinitions.EffectiveFactoryCatalogCandidate, 0, len(listed))
	for _, entry := range listed {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		canonical, err := s.read(filepath.Join(
			entry.FactoryDir,
			factorydefinitions.FactoryConfigFile,
		))
		if contextErr := ctx.Err(); contextErr != nil {
			return nil, contextErr
		}
		location := entry.FactoryDir
		candidate := factorydefinitions.EffectiveFactoryCatalogCandidate{
			Name:      entry.Name,
			Location:  &location,
			Canonical: append([]byte(nil), canonical...),
		}
		if err != nil {
			candidate.Canonical = nil
			candidate.Failure = factorydefinitions.EffectiveFactoryCatalogDiagnosticUnreadable
		}
		candidates = append(candidates, candidate)
	}
	return candidates, nil
}

func (s effectiveCatalogSource) listPackagedCandidates(
	ctx context.Context,
) ([]factorydefinitions.EffectiveFactoryCatalogCandidate, error) {
	candidates := make([]factorydefinitions.EffectiveFactoryCatalogCandidate, 0, len(s.packaged))
	for _, definition := range s.packaged {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		candidates = append(candidates, factorydefinitions.EffectiveFactoryCatalogCandidate{
			Name:      definition.Name,
			Canonical: append([]byte(nil), definition.JSON...),
		})
	}
	return candidates, nil
}
