package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClassifierValidationMatrixEmitsExactLaneSets(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		paths []string
		areas string
		lanes []string
	}{
		{"factory only", []string{"factory/workstations/process.yaml"}, "factory-content", nil},
		{"reference docs", []string{"docs/reference/factory.md"}, "documentation-reference", []string{laneDocsReference}},
		{"docs README", []string{"docs/README.md"}, "documentation-reference", []string{laneDocsReference}},
		{"root README", []string{"README.md"}, "readme", []string{laneReadme}},
		{"frontend", []string{"ui/src/App.tsx"}, "frontend", []string{laneFrontend}},
		{"backend or CLI", []string{"cmd/factory/main.go"}, "backend", []string{laneBackend, laneBackendConformance, laneUIBackendIntegration}},
		{"generated packaged Factory", []string{"packages/packaged-factories/generated/factories/tts/factory.json"}, "packaged-factories-package", []string{laneBackend, laneBackendConformance, lanePackagedFactoriesPackage}},
		{"built-in Models catalog", []string{"pkg/services/models/catalog_contract.go"}, "backend", []string{laneBackend, laneBackendConformance, laneUIBackendIntegration}},
		{"backend registry", []string{"pkg/services/models/internal/backendregistry/registry.go"}, "backend", []string{laneBackend, laneBackendConformance, laneUIBackendIntegration}},
		{"default backend manifest", []string{"pkg/services/models/internal/artifacts/default-manifest.json"}, "backend", []string{laneBackend, laneBackendConformance, laneUIBackendIntegration}},
		{"release-built command wiring", []string{"cmd/omnivoice-llamacpp/main.go"}, "backend", []string{laneBackend, laneBackendConformance, laneUIBackendIntegration}},
		{"API contract", []string{"api/openapi-main.yaml"}, "api-contract", []string{laneFrontend, laneBackend, laneUIBackendIntegration, laneAPIPackage}},
		{"contracts check", []string{"cmd/contractscheck/repository_mutation_test.go"}, "api-contract", []string{laneFrontend, laneBackend, laneUIBackendIntegration, laneAPIPackage}},
		{"API package", []string{"packages/api/package.json"}, "api-package", []string{laneFrontend, laneBackend, laneUIBackendIntegration, laneAPIPackage}},
		{"Packaged Factories package", []string{"packages/packaged-factories/package.json"}, "packaged-factories-package", []string{laneBackend, lanePackagedFactoriesPackage}},
		{"Model Providers package", []string{"packages/model-providers/package.json"}, "model-providers-package", []string{laneBackend, laneModelProvidersPackage}},
		{"mixed frontend and backend", []string{"ui/src/App.tsx", "pkg/root/build.go"}, "backend,frontend", []string{laneFrontend, laneBackend, laneUIBackendIntegration}},
		{"conservative full verification", []string{".github/workflows/ci.yml", "Makefile", "go.mod", "go.sum", "unknown-root-file"}, "ci-tooling,unknown", allLaneNames},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := classifyPaths(tc.paths)
			if got := strings.Join(result.Areas, ","); got != tc.areas {
				t.Fatalf("areas = %q, want %q", got, tc.areas)
			}
			assertLaneSet(t, result, tc.lanes...)
		})
	}
}

func TestClassifyPathsConservativelyRunsEverything(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		paths []string
	}{
		{name: "empty input", paths: nil},
		{name: "empty slice", paths: []string{}},
		{name: "workflow", paths: []string{".github/workflows/ci.yml"}},
		{name: "CI script", paths: []string{"scripts/ci/check.sh"}},
		{name: "Makefile", paths: []string{"Makefile"}},
		{name: "go.mod", paths: []string{"go.mod"}},
		{name: "go.sum", paths: []string{"go.sum"}},
		{name: "unknown path", paths: []string{"unknown-root-file"}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := classifyPaths(tc.paths)
			if result.Classification != "full" {
				t.Fatalf("classification = %q, want full", result.Classification)
			}
			assertLaneSet(t, result, allLaneNames...)
		})
	}
}

func TestFactoryContentIsNeutralWhenMixedWithOwnedPath(t *testing.T) {
	t.Parallel()
	result := classifyPaths([]string{
		`factory\workstations\process.yaml`,
		"./pkg/root/build.go",
		"pkg/root/build.go",
	})

	assertLaneSet(t, result, laneBackend, laneUIBackendIntegration)
	if got := strings.Join(result.ChangedPaths, ","); got != "factory/workstations/process.yaml,pkg/root/build.go" {
		t.Fatalf("ChangedPaths = %q, want normalized unique paths", got)
	}
}

func TestClassifierDecisionsAreIndependentOfPathOrderAndDuplicates(t *testing.T) {
	t.Parallel()
	want := classifyPaths([]string{
		"ui/src/App.tsx",
		"api/openapi-main.yaml",
		"pkg/root/build.go",
	})
	got := classifyPaths([]string{
		`pkg\root\build.go`,
		"api/openapi-main.yaml",
		"./ui/src/App.tsx",
		"pkg/root/build.go",
	})

	if got.Classification != want.Classification ||
		strings.Join(got.Areas, "|") != strings.Join(want.Areas, "|") ||
		strings.Join(got.ChangedPaths, "|") != strings.Join(want.ChangedPaths, "|") {
		t.Fatalf("classification changed with input order or duplicates: got %#v, want %#v", got, want)
	}
	assertLaneSet(t, got, laneFrontend, laneBackend, laneUIBackendIntegration, laneAPIPackage)
}

func TestClassifierOwnershipBoundaries(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		path  string
		lanes []string
	}{
		{name: "service HTTP adapter is API-owned", path: "pkg/services/factory_sessions/transports/http/handler.go", lanes: []string{laneFrontend, laneBackend, laneUIBackendIntegration, laneAPIPackage}},
		{name: "generated client contract is API-owned", path: "ui/packages/client/src/generated/openapi.ts", lanes: []string{laneFrontend, laneBackend, laneUIBackendIntegration, laneAPIPackage}},
		{name: "UI inference is frontend-owned", path: "ui/src/features/timeline/state/timeline/replayWorldStateInference.test.ts", lanes: []string{laneFrontend}},
		{name: "provider inference is backend-owned", path: "tests/functional/workers/inference/selection_test.go", lanes: []string{laneBackend, laneUIBackendIntegration}},
		{name: "release script is backend-owned", path: "scripts/release/smoke-install.sh", lanes: []string{laneBackend, laneBackendConformance, laneUIBackendIntegration}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertLaneSet(t, classifyPaths([]string{tc.path}), tc.lanes...)
		})
	}
}

func TestBackendConformanceSelectionCoversNamedSurfaces(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		path string
		want bool
	}{
		{name: "generated packaged Factory", path: "packages/packaged-factories/generated/factories/tts/factory.json", want: true},
		{name: "built-in Models catalog", path: "pkg/services/models/catalog_contract.go", want: true},
		{name: "backend registry", path: "pkg/services/models/internal/backendregistry/registry.go", want: true},
		{name: "default manifest", path: "pkg/services/models/internal/artifacts/default-manifest.json", want: true},
		{name: "Makefile", path: "Makefile", want: true},
		{name: "release command wiring", path: "cmd/omnivoice-llamacpp/main.go", want: true},
		{name: "unrelated documentation", path: "docs/architecture/architecture.md", want: false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := classifyPaths([]string{tc.path})
			if got := result.Lanes[laneBackendConformance].ShouldRun; got != tc.want {
				t.Fatalf("Backend Conformance selected = %t, want %t for %s", got, tc.want, tc.path)
			}
		})
	}
}

func assertLaneSet(t *testing.T, result classificationResult, want ...string) {
	t.Helper()
	wantSet := make(map[string]bool, len(want))
	for _, lane := range want {
		wantSet[lane] = true
	}
	var got, expected []string
	for _, lane := range allLaneNames {
		if result.Lanes[lane].ShouldRun {
			got = append(got, lane)
		}
		if wantSet[lane] {
			expected = append(expected, lane)
		}
	}
	if strings.Join(got, "|") != strings.Join(expected, "|") {
		t.Fatalf("selected lanes = %q, want exact set %q", strings.Join(got, "|"), strings.Join(expected, "|"))
	}
}

func TestRunWritesNamedLaneOutputs(t *testing.T) {
	tempDir := t.TempDir()
	changed := filepath.Join(tempDir, "changed.txt")
	if err := os.WriteFile(changed, []byte("ui/src/App.tsx\npkg/root/build.go\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	output, summary := filepath.Join(tempDir, "output.txt"), filepath.Join(tempDir, "summary.md")
	t.Setenv("GITHUB_OUTPUT", output)
	t.Setenv("GITHUB_STEP_SUMMARY", summary)
	stdout := &bytes.Buffer{}
	if err := run(config{changedFilesPath: changed}, stdout, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	result := classifyPaths([]string{"ui/src/App.tsx", "pkg/root/build.go"})
	for _, lane := range allLaneNames {
		want := "lane_" + githubOutputKey(lane) + "=" + boolText(result.Lanes[lane].ShouldRun)
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("stdout does not contain %q: %q", want, stdout.String())
		}
	}
	contents, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"run_everything=false",
		"run_frontend=true",
		"frontend_command=make frontend-verification",
		"run_backend=true",
		"backend_command=make backend-verification",
		"run_backend_conformance=false",
		"backend_conformance_command=make test-backend-conformance",
		"run_ui_backend_integration=true",
		"ui_backend_integration_command=make ui-backend-integration",
		"api_package_command=make api-package-verify",
		"packaged_factories_package_command=make packaged-factory-package-verify",
		"model_providers_package_command=make model-provider-package-verify",
		"run_docs_reference=false",
	} {
		if !strings.Contains(string(contents), want) {
			t.Errorf("output does not contain %q", want)
		}
	}
	contents, err = os.ReadFile(summary)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contents), "### Verification policy") {
		t.Fatalf("summary missing policy: %q", contents)
	}
}

func TestRunWritesConservativeFallbackOutput(t *testing.T) {
	tempDir := t.TempDir()
	changed := filepath.Join(tempDir, "changed.txt")
	if err := os.WriteFile(changed, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(tempDir, "output.txt")
	t.Setenv("GITHUB_OUTPUT", output)
	if err := run(config{changedFilesPath: changed}, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contents), "run_everything=true") {
		t.Fatalf("output = %q, want conservative fallback", contents)
	}
	result := classifyPaths(nil)
	for _, lane := range allLaneNames {
		want := "run_" + githubOutputKey(lane) + "=" + boolText(result.Lanes[lane].ShouldRun)
		if !strings.Contains(string(contents), want) {
			t.Errorf("output does not contain %q", want)
		}
	}
}

func boolText(value bool) string {
	if value {
		return "true"
	}
	return "false"
}
