package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunRejectsNonProvidersDurableProviderEffectOwner(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/services/factory_runtime/providereffect/contract.go", `package providereffect

import "context"

// Deliberate fixture: a non-Providers package claims durable ownership of the
// provider inference effect port.
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
		"prohibited durable provider-effect ownership",
		"pkg/services/factory_runtime/providereffect",
		"canonical owner: " + providersLeafEffectContractPackage,
		"Providers Execution leaf",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("run() stderr = %q, want substring %q", got, want)
		}
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

func TestRunRejectsEdgesRedefiningOrAliasingProvidersLeafEffectContract(t *testing.T) {
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
