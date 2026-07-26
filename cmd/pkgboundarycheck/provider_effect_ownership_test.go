package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunAllowsProvidersExecutionLeafOwningProviderEffectContract(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, providersLeafEffectContractPackage+"/contract.go", `package inferencecontract

import "context"

// Deliberate fixture: the Providers Execution leaf owns the provider inference
// effect port named by the normative backend standard.
type Provider interface {
	Infer(context.Context, string) (string, error)
}
`)

	stderr := &bytes.Buffer{}
	if err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr); err != nil {
		t.Fatalf("run() error = %v, want Providers Execution leaf provider-effect ownership allowed; stderr=%q", err, stderr.String())
	}
}

func TestRunRejectsRenamedNonProvidersDurableProviderEffectOwner(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/services/factory_runtime/providereffect/contract.go", `package providereffect

import "context"

// Deliberate fixture: a non-Providers package claims durable ownership of the
// provider inference effect port under a different local type name.
type InferencePort interface {
	Infer(context.Context, string) (string, error)
}
`)

	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want non-Providers durable provider-effect ownership rejected")
	}
	got := stderr.String()
	for _, want := range []string{
		"prohibited durable provider-effect ownership",
		"pkg/services/factory_runtime/providereffect",
		"InferencePort",
		"canonical owner: " + providersLeafEffectContractPackage,
		"Providers Execution leaf",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("run() stderr = %q, want substring %q", got, want)
		}
	}
}

func TestRunAllowsUnrelatedProviderInterface(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/platform/credentials/provider.go", `package credentials

// Deliberate fixture: the common Provider name alone does not make this an
// inference/process effect port.
type Provider interface {
	Get(string) (string, error)
}
`)

	stderr := &bytes.Buffer{}
	if err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr); err != nil {
		t.Fatalf("run() error = %v, want unrelated Provider interface allowed; stderr=%q", err, stderr.String())
	}
}

func TestRunRejectsProvidersSubpackageDurableProviderEffectOwner(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/services/providers/catalog/contract.go", `package catalog

import "context"

// Deliberate fixture: only the Providers Execution leaf may declare this port.
type Provider interface {
	Infer(context.Context, string) (string, error)
}
`)

	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want non-Execution Providers provider-effect ownership rejected")
	}
	got := stderr.String()
	for _, want := range []string{
		"prohibited durable provider-effect ownership",
		"pkg/services/providers/catalog",
		"canonical owner: " + providersLeafEffectContractPackage,
		"Providers Execution leaf",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("run() stderr = %q, want substring %q", got, want)
		}
	}
}

func TestRunAllowsEdgesAggregatingProvidersLeafEffectContract(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, providersLeafEffectContractPackage+"/contract.go", `package inferencecontract

import "context"

type Provider interface {
	Infer(context.Context, string) (string, error)
}
`)
	writeGoSourceFile(t, repoRoot, "pkg/services/edges/definition.go", `package edges

import leaf "github.com/portpowered/infinite-you/`+providersLeafEffectContractPackage+`"

type Edges struct {
	ProviderOverride leaf.Provider
}
`)

	stderr := &bytes.Buffer{}
	if err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr); err != nil {
		t.Fatalf("run() error = %v, want edges aggregating Providers leaf effect contract allowed; stderr=%q", err, stderr.String())
	}
}

func TestRunRejectsEdgesTypeNameRedefiningProvidersLeafEffectContract(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name             string
		localDeclaration string
	}{
		{
			name:             "alias",
			localDeclaration: "type Edges = leaf.Provider",
		},
		{
			name:             "defined type",
			localDeclaration: "type Edges leaf.Provider",
		},
		{
			name: "embedded canonical interface",
			localDeclaration: `type Edges interface {
	leaf.Provider
}`,
		},
		{
			name: "declared provider effect method",
			localDeclaration: `type Edges interface {
	Infer(context.Context, string) (string, error)
}`,
		},
	}

	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			repoRoot := t.TempDir()
			writeGoSourceFile(t, repoRoot, providersLeafEffectContractPackage+"/contract.go", `package inferencecontract

import "context"

type Provider interface {
	Infer(context.Context, string) (string, error)
}
`)
			writeGoSourceFile(
				t,
				repoRoot,
				"pkg/services/edges/definition.go",
				"package edges\n\n"+
					"import (\n"+
					"\t\"context\"\n"+
					"\tleaf \"github.com/portpowered/infinite-you/"+providersLeafEffectContractPackage+"\"\n"+
					")\n\n"+
					"var _ context.Context\n"+
					"var _ leaf.Provider\n\n"+
					testCase.localDeclaration+"\n",
			)

			stderr := &bytes.Buffer{}
			err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
			if err == nil {
				t.Fatal("run() error = nil, want Edges provider-effect contract redefinition rejected")
			}
			got := stderr.String()
			for _, want := range []string{
				"prohibited provider-effect contract redefinition",
				"pkg/services/edges",
				"Edges",
				"aggregate the exact Providers leaf effect contract unchanged",
				"canonical owner: " + providersLeafEffectContractPackage,
			} {
				if !strings.Contains(got, want) {
					t.Fatalf("run() stderr = %q, want substring %q", got, want)
				}
			}
		})
	}
}

func TestRunRejectsEdgesAliasingProvidersLeafEffectContract(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, providersLeafEffectContractPackage+"/contract.go", `package inferencecontract

import "context"

type Provider interface {
	Infer(context.Context, string) (string, error)
}
`)
	writeGoSourceFile(t, repoRoot, "pkg/services/edges/definition.go", `package edges

import leaf "github.com/portpowered/infinite-you/`+providersLeafEffectContractPackage+`"

type Edges struct {
	ProviderOverride leaf.Provider
}

// Deliberate fixture: edges aliases the Providers leaf effect contract instead
// of aggregating the exact imported type unchanged.
type Provider = leaf.Provider
`)

	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want edges aliasing Providers leaf effect contract rejected")
	}
	got := stderr.String()
	for _, want := range []string{
		"prohibited provider-effect contract redefinition",
		"pkg/services/edges",
		"aggregate the exact Providers leaf effect contract unchanged",
		providersLeafEffectContractPackage,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("run() stderr = %q, want substring %q", got, want)
		}
	}
}

func TestRunRejectsEdgesRedefiningImportedProvidersLeafEffectContract(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name              string
		importDeclaration string
		aggregatedType    string
		localDeclaration  string
	}{
		{
			name:              "defined selector type",
			importDeclaration: `import leaf "github.com/portpowered/infinite-you/` + providersLeafEffectContractPackage + `"`,
			aggregatedType:    "leaf.Provider",
			localDeclaration:  "type LocalProvider leaf.Provider",
		},
		{
			name:              "embedded canonical interface",
			importDeclaration: `import leaf "github.com/portpowered/infinite-you/` + providersLeafEffectContractPackage + `"`,
			aggregatedType:    "leaf.Provider",
			localDeclaration: `type LocalProvider interface {
	leaf.Provider
}`,
		},
		{
			name:              "defined dot-imported type",
			importDeclaration: `import . "github.com/portpowered/infinite-you/` + providersLeafEffectContractPackage + `"`,
			aggregatedType:    "Provider",
			localDeclaration:  "type LocalProvider Provider",
		},
	}

	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			repoRoot := t.TempDir()
			writeGoSourceFile(t, repoRoot, providersLeafEffectContractPackage+"/contract.go", `package inferencecontract

import "context"

type Provider interface {
	Infer(context.Context, string) (string, error)
}
`)
			writeGoSourceFile(
				t,
				repoRoot,
				"pkg/services/edges/definition.go",
				"package edges\n\n"+
					testCase.importDeclaration+"\n\n"+
					"type Edges struct {\n\tProviderOverride "+testCase.aggregatedType+"\n}\n\n"+
					testCase.localDeclaration+"\n",
			)

			stderr := &bytes.Buffer{}
			err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
			if err == nil {
				t.Fatal("run() error = nil, want edges redefining Providers leaf effect contract rejected")
			}
			got := stderr.String()
			for _, want := range []string{
				"prohibited provider-effect contract redefinition",
				"pkg/services/edges",
				"LocalProvider",
				"aggregate the exact Providers leaf effect contract unchanged",
				"canonical owner: " + providersLeafEffectContractPackage,
			} {
				if !strings.Contains(got, want) {
					t.Fatalf("run() stderr = %q, want substring %q", got, want)
				}
			}
		})
	}
}

func TestRunAllowsEdgesAliasingUnrelatedProviderType(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/platform/credentials/provider.go", `package credentials

type Provider interface {
	Get(string) (string, error)
}
`)
	writeGoSourceFile(t, repoRoot, "pkg/services/edges/definition.go", `package edges

import credentials "github.com/portpowered/infinite-you/pkg/platform/credentials"

type Edges struct{}

// Deliberate fixture: selector spelling is insufficient; this alias does not
// resolve to the canonical Providers Execution leaf.
type Provider = credentials.Provider

// Deliberate fixtures: the defined and embedded forms are also allowed when
// their import does not resolve to the canonical Providers Execution leaf.
type LocalProvider credentials.Provider

type EmbeddedProvider interface {
	credentials.Provider
}
`)

	stderr := &bytes.Buffer{}
	if err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr); err != nil {
		t.Fatalf("run() error = %v, want unrelated Provider alias allowed; stderr=%q", err, stderr.String())
	}
}

func TestRunRejectsEdgesRedeclaringProviderEffectContractUnderAnotherName(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, providersLeafEffectContractPackage+"/contract.go", `package inferencecontract

import "context"

type Provider interface {
	Infer(context.Context, string) (string, error)
}
`)
	writeGoSourceFile(t, repoRoot, "pkg/services/edges/definition.go", `package edges

import "context"

// Deliberate fixture: a different local name must not hide that edges owns a
// newly declared provider inference effect contract.
type InferenceProvider interface {
	Infer(context.Context, string) (string, error)
}
`)

	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want edges provider-effect interface redeclaration rejected")
	}
	got := stderr.String()
	for _, want := range []string{
		"prohibited provider-effect contract redefinition",
		"pkg/services/edges",
		"InferenceProvider",
		"aggregate the exact Providers leaf effect contract unchanged",
		"canonical owner: " + providersLeafEffectContractPackage,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("run() stderr = %q, want substring %q", got, want)
		}
	}
}

func TestRunRejectsImportResolvedContextProviderEffectRedeclarations(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name                string
		packagePath         string
		contextImport       string
		contextType         string
		wantDiagnostic      string
		wantOwnershipPhrase string
	}{
		{
			name:                "edges redefinition",
			packagePath:         edgesPackagePath,
			contextImport:       `. "context"`,
			contextType:         "Context",
			wantDiagnostic:      "prohibited provider-effect contract redefinition",
			wantOwnershipPhrase: "aggregate the exact Providers leaf effect contract unchanged",
		},
		{
			name:                "non-Providers durable owner",
			packagePath:         "pkg/services/factory_runtime/providereffect",
			contextImport:       `. "context"`,
			contextType:         "Context",
			wantDiagnostic:      "prohibited durable provider-effect ownership",
			wantOwnershipPhrase: "Providers Execution leaf",
		},
		{
			name:                "renamed context import",
			packagePath:         "pkg/services/factory_runtime/renamedprovidereffect",
			contextImport:       `stdctx "context"`,
			contextType:         "stdctx.Context",
			wantDiagnostic:      "prohibited durable provider-effect ownership",
			wantOwnershipPhrase: "Providers Execution leaf",
		},
	}

	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			repoRoot := t.TempDir()
			writeGoSourceFile(t, repoRoot, testCase.packagePath+"/contract.go", `package providereffect

import `+testCase.contextImport+`

// Deliberate fixture: an alternate context import form must not hide a locally
// owned provider inference effect contract from the checker.
type InferenceProvider interface {
	Infer(`+testCase.contextType+`, string) (string, error)
}
`)

			stderr := &bytes.Buffer{}
			err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
			if err == nil {
				t.Fatal("run() error = nil, want import-resolved Context provider-effect redeclaration rejected")
			}
			got := stderr.String()
			for _, want := range []string{
				testCase.wantDiagnostic,
				testCase.packagePath,
				"InferenceProvider",
				testCase.wantOwnershipPhrase,
				"canonical owner: " + providersLeafEffectContractPackage,
			} {
				if !strings.Contains(got, want) {
					t.Fatalf("run() stderr = %q, want substring %q", got, want)
				}
			}
		})
	}
}

func TestRunAllowsWorkersProviderEffectMigrationDebtDeclaration(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, workersProviderEffectMigrationDebtPackage+"/contract.go", `package inferencecontract

import "context"

type Provider interface {
	Infer(context.Context, string) (string, error)
}
`)

	stderr := &bytes.Buffer{}
	if err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr); err != nil {
		t.Fatalf("run() error = %v, want Workers inferencecontract migration debt allowed; stderr=%q", err, stderr.String())
	}
}

func TestRunRejectsCompetingProviderCatalogAbstraction(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/services/factory_runtime/providercatalog/catalog.go", `package providercatalog

import "context"

// Deliberate fixture: invents a second Providers catalog beside the absorbed
// Standardized Providers enumeration/execution source of truth.
type Catalog interface {
	List(context.Context) ([]string, error)
}
`)

	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want competing provider catalog abstraction rejected")
	}
	got := stderr.String()
	for _, want := range []string{
		"prohibited competing provider catalog or execution abstraction",
		"pkg/services/factory_runtime/providercatalog",
		"enumeration and one-attempt execution share one Providers-owned source of truth",
		"Standardized Providers",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("run() stderr = %q, want substring %q", got, want)
		}
	}
}

func TestRunRejectsCompetingNestedProviderCatalogAbstraction(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/services/factory_runtime/provider/catalog/catalog.go", `package catalog

import "context"

// Deliberate fixture: nesting the second catalog below a provider directory
// must not bypass the shared Providers-owned source-of-truth rule.
type Catalog interface {
	List(context.Context) ([]string, error)
}
`)

	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want nested competing provider catalog abstraction rejected")
	}
	got := stderr.String()
	for _, want := range []string{
		"prohibited competing provider catalog or execution abstraction",
		"pkg/services/factory_runtime/provider/catalog",
		"enumeration and one-attempt execution share one Providers-owned source of truth",
		"Standardized Providers",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("run() stderr = %q, want substring %q", got, want)
		}
	}
}

func TestRunRejectsCompetingProviderConductorAbstraction(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/services/automations/providerconductor/conductor.go", `package providerconductor

import "context"

// Deliberate fixture: invents a parallel provider conductor beside the absorbed
// Standardized Providers model while the neutral-conductor lane is live.
type Conductor interface {
	Run(context.Context) error
}
`)

	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want competing provider conductor abstraction rejected")
	}
	got := stderr.String()
	for _, want := range []string{
		"prohibited competing provider catalog or execution abstraction",
		"pkg/services/automations/providerconductor",
		"enumeration and one-attempt execution share one Providers-owned source of truth",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("run() stderr = %q, want substring %q", got, want)
		}
	}
}

func TestRunAllowsAbsorbedWorkersProviderRegistryAndProvidersCatalog(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/services/workers/provider/registry/registry.go", `package registry

type Registry struct {
	IDs []string
}
`)
	writeGoSourceFile(t, repoRoot, "pkg/services/providers/catalog/catalog.go", `package catalog

import "context"

type Catalog interface {
	List(context.Context) ([]string, error)
}
`)

	stderr := &bytes.Buffer{}
	if err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr); err != nil {
		t.Fatalf("run() error = %v, want absorbed Workers registry and Providers catalog allowed; stderr=%q", err, stderr.String())
	}
}

func TestDurableOwnerDiagnosticNamesSharedCatalogExecutionSourceOfTruth(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/services/factory_runtime/providereffect/contract.go", `package providereffect

import "context"

type Provider interface {
	Infer(context.Context, string) (string, error)
}
`)

	stderr := &bytes.Buffer{}
	err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &bytes.Buffer{}, stderr)
	if err == nil {
		t.Fatal("run() error = nil, want non-Providers durable provider-effect ownership rejected")
	}
	got := stderr.String()
	for _, want := range []string{
		"enumeration and one-attempt execution share one Providers-owned source of truth",
		"do not invent a second Providers catalog, registry, conductor, or execution-contract family",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("run() stderr = %q, want substring %q", got, want)
		}
	}
}
