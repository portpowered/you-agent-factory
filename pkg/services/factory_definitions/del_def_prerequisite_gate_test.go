package factorydefinitions_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// DEL-DEF story 001 confirms prerequisite packets are Factory-complete before
// leased deletion of emptied transitional top-level packages begins. Each
// subtest asserts one observable tree invariant left by INV-DEF-TOPLEVEL,
// IMP-DEF repair, CLN-DEF-CONTRACTS, CUT-DEF-SES, CUT-DEF-RUN, BOOT-DEF, and
// CLN-DEF-FOLD-TOPLEVEL. BOOT-DEF behavioral proofs live in
// pkg/services/system_initialization/initialize_definitions_root_boundary_test.go;
// CUT-DEF-RUN and CLN-DEF-FOLD import guards live in sibling boundary tests.

func TestDelDefPrerequisiteGate_AllPrerequisitePacketsFactoryComplete(t *testing.T) {
	t.Parallel()

	serviceRoot := definitionsServiceRootDir(t)

	t.Run("CLN-DEF-CONTRACTS_retired_mega_barrel", func(t *testing.T) {
		t.Parallel()
		_, err := os.Stat(filepath.Join(serviceRoot, "contracts"))
		if !os.IsNotExist(err) {
			t.Fatalf("contracts mega-barrel must be deleted before DEL-DEF; stat contracts/ = %v", err)
		}
	})

	t.Run("IMP-DEF_repair_subservices_present", func(t *testing.T) {
		t.Parallel()
		wantSubservices := []string{
			"catalog",
			"authoring_layout",
			"compilation",
			"validation",
			"snapshots_portability",
			"distribution",
			"invocation_policy",
		}
		subservicesRoot := filepath.Join(serviceRoot, "internal", "services")
		entries, err := os.ReadDir(subservicesRoot)
		if err != nil {
			t.Fatalf("ReadDir(%q) = %v", subservicesRoot, err)
		}
		var gotSubservices []string
		for _, entry := range entries {
			if entry.IsDir() {
				gotSubservices = append(gotSubservices, entry.Name())
			}
		}
		slices.Sort(gotSubservices)
		slices.Sort(wantSubservices)
		if !slices.Equal(gotSubservices, wantSubservices) {
			t.Fatalf("internal/services directories = %v, want %v", gotSubservices, wantSubservices)
		}
	})

	t.Run("CUT-DEF-SES_activation_gateway_published", func(t *testing.T) {
		t.Parallel()
		_, err := os.Stat(filepath.Join(serviceRoot, "activation_contract.go"))
		if err != nil {
			t.Fatalf("activation_contract.go must exist at Definitions root after CUT-DEF-SES: %v", err)
		}
	})

	t.Run("INV-DEF-TOPLEVEL_canonical_root_directories", func(t *testing.T) {
		t.Parallel()
		wantRootDirs := []string{"internal", "transports", "wire"}
		entries, err := os.ReadDir(serviceRoot)
		if err != nil {
			t.Fatalf("ReadDir(%q) = %v", serviceRoot, err)
		}
		var gotRootDirs []string
		for _, entry := range entries {
			if entry.IsDir() {
				gotRootDirs = append(gotRootDirs, entry.Name())
			}
		}
		for _, want := range wantRootDirs {
			if !slices.Contains(gotRootDirs, want) {
				t.Fatalf("service root directories = %v, missing canonical %q", gotRootDirs, want)
			}
		}
	})

	t.Run("DEL-DEF-002_deleted_transitional_packages_absent", func(t *testing.T) {
		t.Parallel()
		deletedTransitional := []string{
			"authoredlayout",
			"portableconfig",
			"loading",
			"namedfactories",
			"runtimeconfig",
		}
		for _, relativeDir := range deletedTransitional {
			_, err := os.Stat(filepath.Join(serviceRoot, relativeDir))
			if !os.IsNotExist(err) {
				t.Fatalf("transitional package %s must be deleted by DEL-DEF story 002; stat = %v", relativeDir, err)
			}
		}
	})

	t.Run("DEL-DEF-002_service_shim_held_under_root_wire_lease", func(t *testing.T) {
		t.Parallel()
		info, err := os.Stat(filepath.Join(serviceRoot, "service"))
		if err != nil {
			t.Fatalf("service compile shim must remain while root pkg/wire holds Automations lease; stat = %v", err)
		}
		if !info.IsDir() {
			t.Fatalf("service path must remain a directory while root pkg/wire holds Automations lease")
		}
	})

	t.Run("CLN-DEF-FOLD-TOPLEVEL_wire_construction_bridge", func(t *testing.T) {
		t.Parallel()
		_, err := os.Stat(filepath.Join(serviceRoot, "wire", "wire.go"))
		if err != nil {
			t.Fatalf("wire/wire.go must construct Definitions after fold; stat = %v", err)
		}
	})
}

func definitionsServiceRootDir(t *testing.T) string {
	t.Helper()

	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() = %v", err)
	}
	if strings.HasSuffix(filepath.ToSlash(root), "/pkg/services/factory_definitions") {
		return root
	}
	t.Fatalf("unexpected test working directory %q; run from pkg/services/factory_definitions", root)
	return ""
}
