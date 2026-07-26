package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/service"
)

func TestCatalogIsolatesFailuresClaimsInvalidEffectiveNamesAndIsDeterministic(t *testing.T) {
	t.Parallel()

	projectRoot := "/project"
	globalRoot := "/global"
	projectUnreadable := candidate("unreadable", projectRoot, "secret-project")
	projectUnreadable.Canonical = nil
	projectUnreadable.Failure = factorydefinitions.EffectiveFactoryCatalogDiagnosticUnreadable
	discovery := source{
		roots: map[string][]factorydefinitions.EffectiveFactoryCatalogCandidate{
			projectRoot: {
				projectUnreadable,
				{
					Name:      "malformed",
					Canonical: []byte(`{"secret":"do-not-expose"`),
				},
				{
					Name:      "../private-name",
					Canonical: []byte(`{"token":"do-not-expose"}`),
				},
				candidate("valid-project", projectRoot, "project"),
			},
			globalRoot: {
				candidate("malformed", globalRoot, "shadowed-global"),
				candidate("unreadable", globalRoot, "shadowed-global"),
				candidate("valid-global", globalRoot, "global"),
			},
		},
		packaged: []factorydefinitions.EffectiveFactoryCatalogCandidate{
			{Name: "bad-package", Canonical: []byte(`not-json-with-secret`)},
			packagedCandidate("valid-package", "packaged"),
		},
	}.discovery()
	catalog, err := factoryservice.NewEffectiveCatalog(
		discovery,
		func(
			ctx context.Context,
			candidate factorydefinitions.EffectiveFactoryCatalogCandidate,
		) (*factorydefinitions.FactoryConfig, error) {
			if candidate.Name == "malformed" {
				return nil, errors.New("filesystem says token=do-not-expose")
			}
			return normalize(ctx, candidate)
		},
	)
	if err != nil {
		t.Fatalf("new effective catalog: %v", err)
	}

	first := list(t, catalog, projectRoot, globalRoot)
	second := list(t, catalog, projectRoot, globalRoot)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("repeated results differ:\nfirst:  %#v\nsecond: %#v", first, second)
	}
	if got, want := names(first.Entries), []string{
		"valid-global",
		"valid-package",
		"valid-project",
	}; !slices.Equal(got, want) {
		t.Fatalf("valid names = %v, want %v", got, want)
	}
	wantDiagnostics := []factorydefinitions.EffectiveFactoryCatalogDiagnostic{
		{
			Code:    factorydefinitions.EffectiveFactoryCatalogDiagnosticInvalidName,
			Source:  factorydefinitions.EffectiveFactoryCatalogSourceProjectLocal,
			Message: "Factory entry has an invalid canonical name",
		},
		{
			Code:    factorydefinitions.EffectiveFactoryCatalogDiagnosticMalformed,
			Source:  factorydefinitions.EffectiveFactoryCatalogSourceProjectLocal,
			Name:    "malformed",
			Message: "Factory definition is malformed",
		},
		{
			Code:    factorydefinitions.EffectiveFactoryCatalogDiagnosticUnreadable,
			Source:  factorydefinitions.EffectiveFactoryCatalogSourceProjectLocal,
			Name:    "unreadable",
			Message: "Factory definition could not be read",
		},
		{
			Code:    factorydefinitions.EffectiveFactoryCatalogDiagnosticMalformed,
			Source:  factorydefinitions.EffectiveFactoryCatalogSourcePackaged,
			Name:    "bad-package",
			Message: "Factory definition is malformed",
		},
	}
	if !reflect.DeepEqual(first.Diagnostics, wantDiagnostics) {
		t.Fatalf("diagnostics = %#v, want %#v", first.Diagnostics, wantDiagnostics)
	}
	assertSensitiveSafeDiagnostics(t, first.Diagnostics)
}

func assertSensitiveSafeDiagnostics(
	t *testing.T,
	diagnostics []factorydefinitions.EffectiveFactoryCatalogDiagnostic,
) {
	t.Helper()
	encoded, err := json.Marshal(diagnostics)
	if err != nil {
		t.Fatalf("encode diagnostics: %v", err)
	}
	for _, sensitive := range []string{
		"do-not-expose",
		"private-name",
		"shadowed-global",
		"secret-project",
	} {
		if strings.Contains(string(encoded), sensitive) {
			t.Fatalf("diagnostics expose sensitive value %q: %s", sensitive, encoded)
		}
	}
}

func TestCatalogCancellationIsAtomicAcrossDiscoveryAndMerge(t *testing.T) {
	t.Parallel()

	for _, test := range cancellationCases() {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			catalog, err := factoryservice.NewEffectiveCatalog(
				test.discovery(cancel),
				test.normalizer(cancel),
			)
			if err != nil {
				t.Fatalf("new effective catalog: %v", err)
			}
			result, err := catalog(ctx, factorydefinitions.ListEffectiveFactoriesRequest{
				ProjectRoot: "/project",
				GlobalRoot:  "/global",
			})
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("error = %v, want context.Canceled", err)
			}
			if !reflect.DeepEqual(
				result,
				factorydefinitions.ListEffectiveFactoriesResult{},
			) {
				t.Fatalf("canceled result = %#v, want empty atomic result", result)
			}
		})
	}
}

type cancellationCase struct {
	name       string
	discovery  func(context.CancelFunc) factorydefinitions.EffectiveFactoryCatalogDiscovery
	normalizer func(context.CancelFunc) factorydefinitions.EffectiveFactoryDefinitionNormalizer
}

func cancellationCases() []cancellationCase {
	return []cancellationCase{
		{name: "project discovery", discovery: cancelProjectDiscovery, normalizer: standardNormalizer},
		{name: "global discovery", discovery: cancelGlobalDiscovery, normalizer: standardNormalizer},
		{name: "packaged discovery", discovery: cancelPackagedDiscovery, normalizer: standardNormalizer},
		{name: "merge normalization", discovery: mergeCancellationDiscovery, normalizer: cancelNormalizer},
	}
}

func standardNormalizer(
	context.CancelFunc,
) factorydefinitions.EffectiveFactoryDefinitionNormalizer {
	return normalize
}

func cancelProjectDiscovery(
	cancel context.CancelFunc,
) factorydefinitions.EffectiveFactoryCatalogDiscovery {
	return factorydefinitions.EffectiveFactoryCatalogDiscovery{
		ListRoot: func(context.Context, string) (
			[]factorydefinitions.EffectiveFactoryCatalogCandidate,
			error,
		) {
			cancel()
			return partialCandidates(), errors.New("source error must not beat cancellation")
		},
		ListPackaged: unexpectedPackagedDiscovery,
	}
}

func cancelGlobalDiscovery(
	cancel context.CancelFunc,
) factorydefinitions.EffectiveFactoryCatalogDiscovery {
	rootCalls := 0
	return factorydefinitions.EffectiveFactoryCatalogDiscovery{
		ListRoot: func(context.Context, string) (
			[]factorydefinitions.EffectiveFactoryCatalogCandidate,
			error,
		) {
			rootCalls++
			if rootCalls == 2 {
				cancel()
				return partialCandidates(), errors.New("source error must not beat cancellation")
			}
			return partialCandidates(), nil
		},
		ListPackaged: unexpectedPackagedDiscovery,
	}
}

func cancelPackagedDiscovery(
	cancel context.CancelFunc,
) factorydefinitions.EffectiveFactoryCatalogDiscovery {
	return factorydefinitions.EffectiveFactoryCatalogDiscovery{
		ListRoot: func(context.Context, string) (
			[]factorydefinitions.EffectiveFactoryCatalogCandidate,
			error,
		) {
			return nil, nil
		},
		ListPackaged: func(context.Context) (
			[]factorydefinitions.EffectiveFactoryCatalogCandidate,
			error,
		) {
			cancel()
			return partialCandidates(), errors.New("source error must not beat cancellation")
		},
	}
}

func unexpectedPackagedDiscovery(context.Context) (
	[]factorydefinitions.EffectiveFactoryCatalogCandidate,
	error,
) {
	return nil, errors.New("packaged discovery called after cancellation")
}

func partialCandidates() []factorydefinitions.EffectiveFactoryCatalogCandidate {
	return []factorydefinitions.EffectiveFactoryCatalogCandidate{
		packagedCandidate("partial", "partial"),
	}
}

func mergeCancellationDiscovery(
	context.CancelFunc,
) factorydefinitions.EffectiveFactoryCatalogDiscovery {
	return source{
		roots: map[string][]factorydefinitions.EffectiveFactoryCatalogCandidate{
			"/project": {candidate("partial", "/project", "partial")},
		},
	}.discovery()
}

func cancelNormalizer(
	cancel context.CancelFunc,
) factorydefinitions.EffectiveFactoryDefinitionNormalizer {
	return func(
		ctx context.Context,
		candidate factorydefinitions.EffectiveFactoryCatalogCandidate,
	) (*factorydefinitions.FactoryConfig, error) {
		cancel()
		return normalize(ctx, candidate)
	}
}

func TestCatalogPreCancellationSkipsEveryDiscoverySource(t *testing.T) {
	t.Parallel()

	calls := 0
	discovery := factorydefinitions.EffectiveFactoryCatalogDiscovery{
		ListRoot: func(context.Context, string) (
			[]factorydefinitions.EffectiveFactoryCatalogCandidate,
			error,
		) {
			calls++
			return nil, nil
		},
		ListPackaged: func(context.Context) (
			[]factorydefinitions.EffectiveFactoryCatalogCandidate,
			error,
		) {
			calls++
			return nil, nil
		},
	}
	catalog := newCatalog(t, discovery)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := catalog(ctx, factorydefinitions.ListEffectiveFactoriesRequest{
		ProjectRoot: "/project",
		GlobalRoot:  "/global",
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if calls != 0 {
		t.Fatalf("discovery calls = %d, want zero", calls)
	}
	if !reflect.DeepEqual(result, factorydefinitions.ListEffectiveFactoriesResult{}) {
		t.Fatalf("pre-canceled result = %#v, want empty atomic result", result)
	}
}
