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
