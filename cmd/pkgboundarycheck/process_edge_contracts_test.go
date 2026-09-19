package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestScanProcessEdgeContractsAcceptsReviewedRepositoryShape(t *testing.T) {
	t.Parallel()

	findings, err := scanProcessEdgeContracts("../..")
	if err != nil {
		t.Fatalf("scanProcessEdgeContracts() error = %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("scanProcessEdgeContracts() findings = %#v, want none", findings)
	}
}

func TestScanProcessEdgeContractsRejectsModelsWireImportsAndUnexpectedTypes(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/services/edges/definition.go", `// Package edges is the process-edge aggregator for root.BuildProcess and pkg/wire.
// It is functional, not a service locator or Initializer bag, and exposes exact ports.
package edges

type Edges struct{}
`)
	writeGoSourceFile(t, repoRoot, "pkg/services/edges/models_effects.go", `package edges
import _ "github.com/portpowered/infinite-you/pkg/services/models/wire"
type EdgesExtra struct{}
`)
	writeGoSourceFile(t, repoRoot, "pkg/services/models/service_contract.go", `package models
type Service interface{}
`)

	findings, err := scanProcessEdgeContracts(repoRoot)
	if err != nil {
		t.Fatalf("scanProcessEdgeContracts() error = %v", err)
	}
	if len(findings) != 2 {
		t.Fatalf("findings = %#v, want import and type violations", findings)
	}
	var output bytes.Buffer
	writeProcessEdgeContractFindings(&output, findings)
	for _, want := range []string{
		"pkg/services/edges/models_effects.go",
		"github.com/portpowered/infinite-you/pkg/services/models/wire",
		"EdgesExtra",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("diagnostic = %q, want %q", output.String(), want)
		}
	}
}

func TestScanProcessEdgeContractsRejectsModelsRootInterfaceDrift(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoSourceFile(t, repoRoot, "pkg/services/edges/definition.go", `// Package edges is the process-edge aggregator for root.BuildProcess and pkg/wire.
// It is functional, not a service locator or Initializer bag, and exposes exact ports.
package edges

type Edges struct{}
`)
	writeGoSourceFile(t, repoRoot, "pkg/services/models/service_contract.go", `package models
type Service interface{}
type Other interface{}
`)

	findings, err := scanProcessEdgeContracts(repoRoot)
	if err != nil {
		t.Fatalf("scanProcessEdgeContracts() error = %v", err)
	}
	if len(findings) != 1 || findings[0].rule != modelsRootInterfaceRule {
		t.Fatalf("findings = %#v, want one Models interface finding", findings)
	}
	if !strings.Contains(findings[0].target, "Other") {
		t.Fatalf("finding target = %q, want interface drift", findings[0].target)
	}
}
