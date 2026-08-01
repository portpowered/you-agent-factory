package internal

import (
	"context"
	"fmt"
	"sort"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

type catalogSource struct {
	kind       factorydefinitions.EffectiveFactoryCatalogSource
	candidates []factorydefinitions.EffectiveFactoryCatalogCandidate
}

func (c effectiveCatalog) merge(
	ctx context.Context,
	sources ...catalogSource,
) (
	[]factorydefinitions.EffectiveFactoryCatalogEntry,
	[]factorydefinitions.EffectiveFactoryCatalogDiagnostic,
	error,
) {
	claimed := make(map[string]struct{})
	entries := make([]factorydefinitions.EffectiveFactoryCatalogEntry, 0)
	diagnostics := make([]factorydefinitions.EffectiveFactoryCatalogDiagnostic, 0)
	for _, source := range sources {
		candidates, sourceDiagnostics, err := canonicalCandidates(ctx, source)
		if err != nil {
			return nil, nil, err
		}
		diagnostics = append(diagnostics, sourceDiagnostics...)
		for _, candidate := range candidates {
			if err := ctx.Err(); err != nil {
				return nil, nil, err
			}
			if _, shadowed := claimed[candidate.Name]; shadowed {
				continue
			}
			claimed[candidate.Name] = struct{}{}
			entry, diagnostic, err := c.normalizeCandidate(ctx, source.kind, candidate)
			if err != nil {
				return nil, nil, err
			}
			if diagnostic != nil {
				diagnostics = append(diagnostics, *diagnostic)
			}
			if entry != nil {
				entries = append(entries, *entry)
			}
		}
	}
	sortCatalogResult(entries, diagnostics)
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	return entries, diagnostics, nil
}

func (c effectiveCatalog) normalizeCandidate(
	ctx context.Context,
	source factorydefinitions.EffectiveFactoryCatalogSource,
	candidate factorydefinitions.EffectiveFactoryCatalogCandidate,
) (
	*factorydefinitions.EffectiveFactoryCatalogEntry,
	*factorydefinitions.EffectiveFactoryCatalogDiagnostic,
	error,
) {
	if candidate.Failure != "" {
		diagnostic := catalogDiagnostic(candidate.Failure, source, candidate.Name)
		return nil, &diagnostic, nil
	}
	definition, err := c.normalize(ctx, cloneCandidate(candidate))
	if contextErr := ctx.Err(); contextErr != nil {
		return nil, nil, contextErr
	}
	if err != nil || definition == nil {
		diagnostic := catalogDiagnostic(
			factorydefinitions.EffectiveFactoryCatalogDiagnosticMalformed,
			source,
			candidate.Name,
		)
		return nil, &diagnostic, nil
	}
	entry, err := detachedEntry(candidate, definition)
	if contextErr := ctx.Err(); contextErr != nil {
		return nil, nil, contextErr
	}
	if err != nil {
		return nil, nil, fmt.Errorf("detach Factory %q: %w", candidate.Name, err)
	}
	return &entry, nil, nil
}

func canonicalCandidates(
	ctx context.Context,
	source catalogSource,
) (
	[]factorydefinitions.EffectiveFactoryCatalogCandidate,
	[]factorydefinitions.EffectiveFactoryCatalogDiagnostic,
	error,
) {
	candidates := make([]factorydefinitions.EffectiveFactoryCatalogCandidate, 0, len(source.candidates))
	diagnostics := make([]factorydefinitions.EffectiveFactoryCatalogDiagnostic, 0)
	for _, candidate := range source.candidates {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		segments, err := factorydefinitions.PathSegments(candidate.Name)
		if err != nil {
			diagnostics = append(diagnostics, invalidNameDiagnostic(source.kind))
			continue
		}
		name, err := factorydefinitions.NameFromPathSegments(segments)
		if err != nil {
			diagnostics = append(diagnostics, invalidNameDiagnostic(source.kind))
			continue
		}
		candidate.Name = name
		candidates = append(candidates, cloneCandidate(candidate))
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].Name < candidates[j].Name
	})
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	return candidates, diagnostics, nil
}

func invalidNameDiagnostic(
	source factorydefinitions.EffectiveFactoryCatalogSource,
) factorydefinitions.EffectiveFactoryCatalogDiagnostic {
	return catalogDiagnostic(
		factorydefinitions.EffectiveFactoryCatalogDiagnosticInvalidName,
		source,
		"",
	)
}

func catalogDiagnostic(
	code factorydefinitions.EffectiveFactoryCatalogDiagnosticCode,
	source factorydefinitions.EffectiveFactoryCatalogSource,
	name string,
) factorydefinitions.EffectiveFactoryCatalogDiagnostic {
	message := "Factory entry is invalid"
	switch code {
	case factorydefinitions.EffectiveFactoryCatalogDiagnosticInvalidName:
		message = "Factory entry has an invalid canonical name"
	case factorydefinitions.EffectiveFactoryCatalogDiagnosticUnreadable:
		message = "Factory definition could not be read"
	case factorydefinitions.EffectiveFactoryCatalogDiagnosticMalformed:
		message = "Factory definition is malformed"
	}
	return factorydefinitions.EffectiveFactoryCatalogDiagnostic{
		Code:    code,
		Source:  source,
		Name:    name,
		Message: message,
	}
}

func sortCatalogResult(
	entries []factorydefinitions.EffectiveFactoryCatalogEntry,
	diagnostics []factorydefinitions.EffectiveFactoryCatalogDiagnostic,
) {
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})
	sort.SliceStable(diagnostics, func(i, j int) bool {
		if diagnostics[i].Source != diagnostics[j].Source {
			return sourceRank(diagnostics[i].Source) < sourceRank(diagnostics[j].Source)
		}
		if diagnostics[i].Name != diagnostics[j].Name {
			return diagnostics[i].Name < diagnostics[j].Name
		}
		return diagnostics[i].Code < diagnostics[j].Code
	})
}

func sourceRank(source factorydefinitions.EffectiveFactoryCatalogSource) int {
	switch source {
	case factorydefinitions.EffectiveFactoryCatalogSourceProjectLocal:
		return 0
	case factorydefinitions.EffectiveFactoryCatalogSourceGlobal:
		return 1
	case factorydefinitions.EffectiveFactoryCatalogSourcePackaged:
		return 2
	default:
		return 3
	}
}
