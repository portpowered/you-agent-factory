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

func TestCrossOwnerPeerRuleRejectsPreviouslyUnlistedPeerImplementationPath(t *testing.T) {
	// brand_new_adapter is intentionally absent from any allowlist or private-root
	// catalog. Owner-derived classification alone must reject the peer import.
	const unlistedPeerPath = "/pkg/services/factory_definitions/brand_new_adapter"
	root := fixtureRepository(t, map[string]string{
		"pkg/services/factory_sessions/doc.go":    "package factory_sessions\n",
		"pkg/services/factory_definitions/doc.go": "package factory_definitions\n",
		"pkg/services/factory_definitions/brand_new_adapter/doc.go": "package brand_new_adapter\n",
		"pkg/services/factory_sessions/consume_unlisted.go": `package factory_sessions
import novelty "github.com/portpowered/infinite-you/pkg/services/factory_definitions/brand_new_adapter"
var _ = novelty.Adapter{}`,
	})

	findings := scanFixture(t, root)
	assertFinding(t, findings, rulePeerServiceImplementation, unlistedPeerPath)
	for _, item := range findings {
		if item.Rule != rulePeerServiceImplementation {
			continue
		}
		if !strings.Contains(item.Target, unlistedPeerPath) {
			continue
		}
		message := peerViolationMessage(item)
		if !strings.Contains(message, `importer owner "factory_sessions"`) ||
			!strings.Contains(message, `peer owner "factory_definitions"`) ||
			!strings.Contains(message, "pkg/services/factory_definitions") {
			t.Fatalf("unlisted peer path diagnostic incomplete: %q", message)
		}
		return
	}
	t.Fatal("expected previously-unlisted peer implementation path to be rejected")
}

func TestApprovedPeerExceptionsRemainExactPairwiseNotPrivateRootCatalog(t *testing.T) {
	approvedImporter := "pkg/services/edges"
	approvedImport := modulePath + "/pkg/services/workers/agypty"
	if !isApprovedPeerServiceContractImport(approvedImporter, approvedImport) {
		t.Fatalf("documented leaf-effect port missing from exact pairwise map")
	}

	// Sibling unlisted paths under the same peer must not inherit approval.
	unlistedSibling := modulePath + "/pkg/services/workers/agypty/never_listed_before"
	if isApprovedPeerServiceContractImport(approvedImporter, unlistedSibling) {
		t.Fatalf("pairwise exception incorrectly behaves like a private-root prefix allowlist")
	}
	unlistedImporter := "pkg/services/factory_sessions"
	if isApprovedPeerServiceContractImport(unlistedImporter, approvedImport) {
		t.Fatalf("pairwise exception incorrectly applies to an unlisted importer package")
	}

	for key := range approvedPeerServiceContractImports {
		packagePath, importPath, ok := strings.Cut(key, "\x00")
		if !ok || packagePath == "" || importPath == "" {
			t.Fatalf("approved peer exception key is not exact pairwise: %q", key)
		}
		if strings.ContainsAny(packagePath, "*?") || strings.ContainsAny(importPath, "*?") {
			t.Fatalf("approved peer exception must be exact, not wildcarded: %q", key)
		}
		if !strings.HasPrefix(importPath, modulePath+"/pkg/services/") {
			t.Fatalf("approved peer exception import is outside pkg/services: %q", key)
		}
	}
}

func TestPeerImportBaselineRemainsDeletionOnlyRatchet(t *testing.T) {
	const (
		importerPath = "pkg/services/factory_sessions/consume_peer_impl.go"
		target       = modulePath + "/pkg/services/factory_runtime/brand_new_adapter"
	)
	violatingSource := `package factory_sessions
import novelty "github.com/portpowered/infinite-you/pkg/services/factory_runtime/brand_new_adapter"
var _ = novelty.Adapter{}`
	clearedSource := "package factory_sessions\n"

	t.Run("active recorded peer debt passes without relocating packages", func(t *testing.T) {
		root := fixtureRepository(t, map[string]string{
			"pkg/services/factory_sessions/doc.go":                          "package factory_sessions\n",
			"pkg/services/factory_runtime/doc.go":                           "package factory_runtime\n",
			"pkg/services/factory_runtime/brand_new_adapter/doc.go":         "package brand_new_adapter\n",
			importerPath: violatingSource,
		})
		writeBaseline(t, root, baselineEntryFor(rulePeerServiceImplementation, importerPath, target))
		var stdout, stderr bytes.Buffer
		if err := run(config{root: root}, &stdout, &stderr); err != nil {
			t.Fatalf("run active peer baseline: %v; stderr=%s", err, stderr.String())
		}
		if !strings.Contains(stdout.String(), "1 deletion-only baseline") {
			t.Fatalf("stdout = %q, want active peer debt count", stdout.String())
		}
	})

	t.Run("clearing the import without relocating packages makes baseline stale", func(t *testing.T) {
		root := fixtureRepository(t, map[string]string{
			"pkg/services/factory_sessions/doc.go":                  "package factory_sessions\n",
			"pkg/services/factory_runtime/doc.go":                   "package factory_runtime\n",
			"pkg/services/factory_runtime/brand_new_adapter/doc.go": "package brand_new_adapter\n",
			importerPath: clearedSource,
		})
		writeBaseline(t, root, baselineEntryFor(rulePeerServiceImplementation, importerPath, target))
		var stdout, stderr bytes.Buffer
		err := run(config{root: root}, &stdout, &stderr)
		if err == nil || !strings.Contains(stderr.String(), "stale baseline") {
			t.Fatalf("run stale peer baseline err=%v stderr=%q", err, stderr.String())
		}
	})

	t.Run("new unlisted peer path is a new violation without allowlist edit", func(t *testing.T) {
		root := fixtureRepository(t, map[string]string{
			"pkg/services/factory_sessions/doc.go":                  "package factory_sessions\n",
			"pkg/services/factory_runtime/doc.go":                   "package factory_runtime\n",
			"pkg/services/factory_runtime/brand_new_adapter/doc.go": "package brand_new_adapter\n",
			importerPath: violatingSource,
		})
		var stdout, stderr bytes.Buffer
		err := run(config{root: root}, &stdout, &stderr)
		if err == nil || !strings.Contains(stderr.String(), "new violation") {
			t.Fatalf("run new peer violation err=%v stderr=%q", err, stderr.String())
		}
		if !strings.Contains(stderr.String(), "brand_new_adapter") {
			t.Fatalf("stderr missing previously-unlisted path: %q", stderr.String())
		}
	})
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
