package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestCrossOwnerPeerRuleRejectsPeerImplementationImport(t *testing.T) {
	root := fixtureRepository(t, map[string]string{
		"pkg/services/factory_sessions/doc.go": "package factory_sessions\n",
		"pkg/services/factory_runtime/doc.go":  "package factory_runtime\n",
		"pkg/services/factory_runtime/internal/engine/doc.go": "package engine\n",
		"pkg/services/factory_sessions/consume_peer_impl.go": `package factory_sessions
import engine "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/engine"
var _ = engine.Service{}`,
	})

	findings := scanFixture(t, root)
	assertFinding(
		t,
		findings,
		rulePeerServiceImplementation,
		"/pkg/services/factory_runtime/internal/engine",
	)
	for _, item := range findings {
		if item.Rule != rulePeerServiceImplementation {
			continue
		}
		message := peerViolationMessage(item)
		if !strings.Contains(message, `importer owner "factory_sessions"`) {
			t.Fatalf("diagnostic missing importer owner: %q", message)
		}
		if !strings.Contains(message, `peer owner "factory_runtime"`) {
			t.Fatalf("diagnostic missing peer owner: %q", message)
		}
		if !strings.Contains(message, "pkg/services/factory_runtime") ||
			!strings.Contains(message, "root contract") {
			t.Fatalf("diagnostic missing peer-root remediation: %q", message)
		}
		return
	}
	t.Fatal("expected peer implementation violation finding")
}

func TestCrossOwnerPeerRuleRejectsPeerNestedSubserviceImport(t *testing.T) {
	root := fixtureRepository(t, map[string]string{
		"pkg/services/factory_sessions/doc.go": "package factory_sessions\n",
		"pkg/services/workers/doc.go":          "package workers\n",
		"pkg/services/workers/services/hosted_logic/doc.go": "package hosted_logic\n",
		"pkg/services/factory_sessions/consume_nested.go": `package factory_sessions
import hosted "github.com/portpowered/infinite-you/pkg/services/workers/services/hosted_logic"
var _ = hosted.Service{}`,
	})

	findings := scanFixture(t, root)
	assertFinding(
		t,
		findings,
		rulePeerServiceImplementation,
		"/pkg/services/workers/services/hosted_logic",
	)
	for _, item := range findings {
		if item.Rule != rulePeerServiceImplementation {
			continue
		}
		message := peerViolationMessage(item)
		if !strings.Contains(message, `importer owner "factory_sessions"`) {
			t.Fatalf("diagnostic missing importer owner: %q", message)
		}
		if !strings.Contains(message, `peer owner "workers"`) {
			t.Fatalf("diagnostic missing peer owner: %q", message)
		}
		if !strings.Contains(message, "pkg/services/workers") ||
			!strings.Contains(message, "root contract") {
			t.Fatalf("diagnostic missing peer-root remediation: %q", message)
		}
		return
	}
	t.Fatal("expected peer nested-subservice violation finding")
}

func TestCrossOwnerPeerRuleKeepsExactLeafEffectPortExceptions(t *testing.T) {
	root := fixtureRepository(t, map[string]string{
		"pkg/services/edges/doc.go":    "package edges\n",
		"pkg/services/workers/doc.go":  "package workers\n",
		"pkg/services/workers/agypty/doc.go": "package agypty\n",
		"pkg/services/edges/leaf.go": `package edges
import agypty "github.com/portpowered/infinite-you/pkg/services/workers/agypty"
var _ = agypty.Service{}`,
	})

	findings := scanFixture(t, root)
	for _, item := range findings {
		if item.Rule == rulePeerServiceImplementation {
			t.Fatalf("approved leaf-effect port incorrectly rejected: %#v", item)
		}
	}
}

func TestPeerViolationMessageAppearsInRunOutput(t *testing.T) {
	root := fixtureRepository(t, map[string]string{
		"pkg/services/factory_sessions/doc.go": "package factory_sessions\n",
		"pkg/services/factory_runtime/doc.go":  "package factory_runtime\n",
		"pkg/services/factory_runtime/internal/engine/doc.go": "package engine\n",
		"pkg/services/factory_sessions/consume_peer_impl.go": `package factory_sessions
import engine "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/engine"
var _ = engine.Service{}`,
	})
	var stdout, stderr bytes.Buffer
	err := run(config{root: root}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected peer implementation import to fail run")
	}
	out := stderr.String()
	if !strings.Contains(out, `importer owner "factory_sessions"`) ||
		!strings.Contains(out, `peer owner "factory_runtime"`) ||
		!strings.Contains(out, "pkg/services/factory_runtime") {
		t.Fatalf("run stderr missing owner-named remediation: %q", out)
	}
}

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
		"pkg/services/workers/doc.go":          "package workers\n",
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
		{
			name:          "peer implementation rejected",
			importerOwner: "factory_sessions",
			importPath:    modulePath + "/pkg/services/factory_runtime/internal/engine",
			wantViolation: true,
		},
		{
			name:          "peer nested subservice rejected",
			importerOwner: "factory_sessions",
			importPath:    modulePath + "/pkg/services/workers/services/hosted_logic",
			wantViolation: true,
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
