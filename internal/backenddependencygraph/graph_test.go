package backenddependencygraph

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderDOTColorsPackageFamiliesAndServiceInternalImports(t *testing.T) {
	t.Parallel()

	packages := []Package{
		{
			ImportPath: "example.com/factory/pkg/config",
			Imports: []string{
				"example.com/factory/pkg/services/factory_definitions/contracts",
				"example.com/factory/pkg/services/factory_definitions/definition",
				"fmt",
			},
		},
		{
			ImportPath: "example.com/factory/pkg/services/factory_definitions/definition",
			Imports:    []string{"example.com/factory/pkg/services/factory_definitions/contracts"},
		},
		{
			ImportPath: "example.com/factory/pkg/initializer/application",
			Imports:    []string{"example.com/factory/pkg/services/factory_definitions/contracts"},
		},
		{
			ImportPath: "example.com/factory/pkg/transports/http",
			Imports:    []string{"example.com/factory/pkg/services/factory_definitions/contracts"},
		},
		{ImportPath: "example.com/factory/pkg/transports/cli"},
		{ImportPath: "example.com/factory/pkg/transports/mcp"},
		{ImportPath: "example.com/factory/pkg/transports/mapping"},
		{ImportPath: "example.com/factory/pkg/platform/clock"},
		{ImportPath: "example.com/factory/pkg/platform/logging"},
		{ImportPath: "example.com/factory/pkg/orchestrators/javascript/policy"},
		{ImportPath: "example.com/factory/pkg/orchestrators/petri"},
		{
			ImportPath: "example.com/factory/pkg/wire",
			Imports:    []string{"example.com/factory/pkg/services/factory_definitions/definition"},
		},
		{
			ImportPath:  "example.com/factory/tests/functional/smoke",
			TestImports: []string{"example.com/factory/pkg/services/factory_definitions"},
		},
		{ImportPath: "example.com/factory/tests/release/smoke"},
		{ImportPath: "example.com/factory/tests/stress/load"},
		{ImportPath: "example.com/factory/tests/adhoc/debug"},
		{ImportPath: "example.com/factory/tests/internal/testutil"},
		{
			ImportPath: "example.com/factory/cmd/factory",
			Imports:    []string{"example.com/factory/pkg/services/factory_definitions"},
		},
		{ImportPath: "example.com/factory/pkg/root"},
		{ImportPath: "example.com/factory/pkg/services/factory_definitions"},
		{ImportPath: "example.com/factory/pkg/services/factory_definitions/contracts"},
	}

	got := string(RenderDOT(packages, "example.com/factory"))
	wantFragments := []string{
		`subgraph cluster_commands {`,
		`label="Commands"; color="#a16207"; bgcolor="#fffbeb"; style="rounded"; penwidth=1.5;`,
		`subgraph cluster_tests_functional {`,
		`label="Tests: functional"; color="#be123c"; bgcolor="#fff1f2"; style="rounded"; penwidth=1.5;`,
		`subgraph cluster_tests_release {`,
		`label="Tests: release"; color="#be123c"; bgcolor="#fff1f2"; style="rounded"; penwidth=1.5;`,
		`subgraph cluster_tests_stress {`,
		`label="Tests: stress"; color="#be123c"; bgcolor="#fff1f2"; style="rounded"; penwidth=1.5;`,
		`subgraph cluster_tests_adhoc {`,
		`label="Tests: adhoc"; color="#be123c"; bgcolor="#fff1f2"; style="rounded"; penwidth=1.5;`,
		`subgraph cluster_tests_support {`,
		`label="Tests: support"; color="#be123c"; bgcolor="#fff1f2"; style="rounded"; penwidth=1.5;`,
		`subgraph cluster_components {`,
		`label="Components"; color="#7c3aed"; bgcolor="#f5f3ff"; style="rounded"; penwidth=1.5;`,
		`subgraph cluster_config {`,
		`label="Config"; color="#1d4ed8"; bgcolor="#eff6ff"; style="rounded"; penwidth=1.5;`,
		`subgraph cluster_initializer {`,
		`label="Initializer"; color="#c2410c"; bgcolor="#fff7ed"; style="rounded"; penwidth=1.5;`,
		`subgraph cluster_platform_clock {`,
		`label="Platform: clock"; color="#0f766e"; bgcolor="#f0fdfa"; style="rounded"; penwidth=1.5;`,
		`subgraph cluster_platform_logging {`,
		`label="Platform: logging"; color="#0f766e"; bgcolor="#f0fdfa"; style="rounded"; penwidth=1.5;`,
		`subgraph cluster_orchestrator_javascript {`,
		`label="Orchestrator: javascript"; color="#a21caf"; bgcolor="#fdf4ff"; style="rounded"; penwidth=1.5;`,
		`subgraph cluster_orchestrator_petri {`,
		`label="Orchestrator: petri"; color="#a21caf"; bgcolor="#fdf4ff"; style="rounded"; penwidth=1.5;`,
		`subgraph cluster_transport_cli {`,
		`label="Transport: cli"; color="#0369a1"; bgcolor="#f0f9ff"; style="rounded"; penwidth=1.5;`,
		`subgraph cluster_transport_rest {`,
		`label="Transport: rest"; color="#0369a1"; bgcolor="#f0f9ff"; style="rounded"; penwidth=1.5;`,
		`subgraph cluster_transport_mcp {`,
		`label="Transport: mcp"; color="#0369a1"; bgcolor="#f0f9ff"; style="rounded"; penwidth=1.5;`,
		`subgraph cluster_transport_mapping {`,
		`label="Transport: mapping"; color="#0369a1"; bgcolor="#f0f9ff"; style="rounded"; penwidth=1.5;`,
		`subgraph cluster_service_factory_definitions {`,
		`label="Service: factory_definitions"; color="#2563eb"; bgcolor="#ffffff"; style="rounded"; penwidth=1.5;`,
		`"example.com/factory/cmd/factory" [label="cmd/factory", fillcolor="#fef3c7"]`,
		`"example.com/factory/pkg/root" [label="pkg/root", fillcolor="#c4b5fd"]`,
		`"example.com/factory/pkg/config" [label="pkg/config", fillcolor="#bfdbfe"]`,
		`"example.com/factory/pkg/services/factory_definitions" [label="pkg/services/factory_definitions", fillcolor="#bfdbfe"]`,
		`"example.com/factory/pkg/services/factory_definitions/contracts" [label="pkg/services/factory_definitions/contracts", fillcolor="#bfdbfe"]`,
		`"example.com/factory/cmd/factory" -> "example.com/factory/pkg/services/factory_definitions" [color="#16a34a", penwidth=1.4]`,
		`"example.com/factory/pkg/config" -> "example.com/factory/pkg/services/factory_definitions/contracts" [color="#dc2626", penwidth=1.4]`,
		`"example.com/factory/pkg/config" -> "example.com/factory/pkg/services/factory_definitions/definition" [color="#dc2626", penwidth=1.4]`,
		`"example.com/factory/pkg/services/factory_definitions/definition" -> "example.com/factory/pkg/services/factory_definitions/contracts";`,
		`"example.com/factory/pkg/initializer/application" -> "example.com/factory/pkg/services/factory_definitions/contracts" [color="#dc2626", penwidth=1.4]`,
		`"example.com/factory/pkg/transports/http" -> "example.com/factory/pkg/services/factory_definitions/contracts" [color="#dc2626", penwidth=1.4]`,
		`"example.com/factory/pkg/wire" -> "example.com/factory/pkg/services/factory_definitions/definition" [color="#2563eb", penwidth=1.4]`,
		`"example.com/factory/tests/functional/smoke" -> "example.com/factory/pkg/services/factory_definitions" [color="#94a3b8", style="dashed", penwidth=1.0]`,
	}
	for _, fragment := range wantFragments {
		if !strings.Contains(got, fragment) {
			t.Errorf("RenderDOT() missing %q in:\n%s", fragment, got)
		}
	}
	if strings.Contains(got, `-> "fmt"`) {
		t.Fatalf("RenderDOT() included standard-library dependency in:\n%s", got)
	}
	if strings.Index(got, "cmd/factory") > strings.Index(got, "pkg/service") {
		t.Fatalf("RenderDOT() nodes are not sorted in:\n%s", got)
	}
	if strings.Index(got, "cluster_commands") > strings.Index(got, "cluster_tests_functional") ||
		strings.Index(got, "cluster_tests_functional") > strings.Index(got, "cluster_tests_release") ||
		strings.Index(got, "cluster_tests_release") > strings.Index(got, "cluster_tests_stress") ||
		strings.Index(got, "cluster_tests_stress") > strings.Index(got, "cluster_tests_adhoc") ||
		strings.Index(got, "cluster_tests_adhoc") > strings.Index(got, "cluster_components") ||
		strings.Index(got, "cluster_components") > strings.Index(got, "cluster_config") ||
		strings.Index(got, "cluster_config") > strings.Index(got, "cluster_platform_clock") ||
		strings.Index(got, "cluster_platform_clock") > strings.Index(got, "cluster_orchestrator_javascript") ||
		strings.Index(got, "cluster_orchestrator_javascript") > strings.Index(got, "cluster_transport_cli") ||
		strings.Index(got, "cluster_transport_cli") > strings.Index(got, "cluster_service_factory_definitions") {
		t.Fatalf("RenderDOT() clusters are not sorted by group in:\n%s", got)
	}
}

func TestWriteDOTCreatesParentDirectory(t *testing.T) {
	t.Parallel()

	outputPath := filepath.Join(t.TempDir(), "nested", "graph.dot")
	if err := WriteDOT(outputPath, []byte("digraph {}\n")); err != nil {
		t.Fatalf("WriteDOT() error = %v", err)
	}
	got, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(got) != "digraph {}\n" {
		t.Fatalf("WriteDOT() contents = %q", got)
	}
}
