package main

import (
	"testing"
)

func TestCrossOwnerPeerRuleAllowsPeerRootContractImports(t *testing.T) {
	root := fixtureRepository(t, map[string]string{
		"pkg/services/factory_sessions/doc.go": "package factory_sessions\n",
		"pkg/services/factory_runtime/doc.go":  "package factory_runtime\n",
		"pkg/services/workers/doc.go":          "package workers\n",
		"pkg/services/factory_sessions/consume_peer_root.go": `package factory_sessions
import (
  runtime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
  workers "github.com/portpowered/infinite-you/pkg/services/workers"
)
var (
  _ runtime.Service
  _ workers.Service
)`,
	})

	findings := scanFixture(t, root)
	for _, item := range findings {
		if item.Rule == rulePeerServiceImplementation {
			t.Fatalf("peer root contract import produced cross-owner violation: %#v", item)
		}
	}
}

func TestCrossOwnerPeerRuleAllowsSameOwnerNonRootImports(t *testing.T) {
	root := fixtureRepository(t, map[string]string{
		"pkg/services/factory_sessions/doc.go": "package factory_sessions\n",
		"pkg/services/factory_runtime/doc.go":  "package factory_runtime\n",
		"pkg/services/factory_sessions/internal/execution/local.go": `package execution
import local "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtime"
var _ local.Service`,
		"pkg/services/factory_sessions/wire/wire.go": `package wire
import execution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution"
func inject() { _ = execution.NewService() }`,
	})

	findings := scanFixture(t, root)
	for _, item := range findings {
		if item.Rule == rulePeerServiceImplementation {
			t.Fatalf("same-owner non-root import incorrectly fired peer rule: %#v", item)
		}
	}
}

func TestCrossOwnerPeerImportDecisionAllowsPeerRootAndSameOwner(t *testing.T) {
	root := fixtureRepository(t, map[string]string{
		"pkg/services/factory_sessions/doc.go": "package factory_sessions\n",
		"pkg/services/factory_runtime/doc.go":  "package factory_runtime\n",
	})
	inventory, err := loadOwnerInventory(root)
	if err != nil {
		t.Fatalf("loadOwnerInventory: %v", err)
	}

	cases := []struct {
		name          string
		importerOwner string
		importPath    string
		wantViolation bool
	}{
		{
			name:          "peer root allowed",
			importerOwner: "factory_sessions",
			importPath:    modulePath + "/pkg/services/factory_runtime",
			wantViolation: false,
		},
		{
			name:          "same-owner non-root allowed",
			importerOwner: "factory_sessions",
			importPath:    modulePath + "/pkg/services/factory_sessions/internal/execution",
			wantViolation: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := crossOwnerPeerImplementationImport(tc.importerOwner, tc.importPath, inventory)
			if got != tc.wantViolation {
				t.Fatalf(
					"crossOwnerPeerImplementationImport(%q, %q) = %v, want %v",
					tc.importerOwner,
					tc.importPath,
					got,
					tc.wantViolation,
				)
			}
		})
	}
}
