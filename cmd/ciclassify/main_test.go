package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClassifyPathsRoutesOwnershipUnion(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		paths []string
		areas string
		run   []string
	}{
		{"factory only", []string{"factory/workstations/process.yaml"}, "factory-content", nil},
		{"reference docs", []string{"docs/reference/factory.md"}, "documentation-reference", []string{laneDocsReference}},
		{"other docs", []string{"docs/guides/factory.md"}, "documentation", nil},
		{"readme", []string{"README.md"}, "readme", []string{laneReadme}},
		{"frontend", []string{"ui/src/App.tsx"}, "frontend", []string{laneFrontend}},
		{"backend", []string{"cmd/factory/main.go"}, "backend", []string{laneBackend, laneUIBackendIntegration}},
		{"api", []string{"api/openapi-main.yaml"}, "api-contract", []string{laneFrontend, laneBackend, laneUIBackendIntegration, laneAPIPackage}},
		{"api package", []string{"packages/api/package.json"}, "api-package", []string{laneFrontend, laneBackend, laneUIBackendIntegration, laneAPIPackage}},
		{"api package script", []string{"scripts/api-package-contract.test.mjs"}, "api-package", []string{laneFrontend, laneBackend, laneUIBackendIntegration, laneAPIPackage}},
		{"packaged factories package", []string{"packages/packaged-factories/package.json"}, "packaged-factories-package", []string{laneBackend, lanePackagedFactoriesPackage}},
		{"model providers package", []string{"packages/model-providers/package.json"}, "model-providers-package", []string{laneBackend, laneModelProvidersPackage}},
		{"local inference", []string{"pkg/services/models/local/runtime.go"}, "local-inference", []string{laneBackend, laneLocalInference}},
		{"mixed", []string{"ui/src/App.tsx", "pkg/root/build.go"}, "backend,frontend", []string{laneFrontend, laneBackend, laneUIBackendIntegration}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := classifyPaths(tc.paths)
			if got := strings.Join(result.Areas, ","); got != tc.areas {
				t.Fatalf("areas = %q, want %q", got, tc.areas)
			}
			want := make(map[string]bool, len(tc.run))
			for _, lane := range tc.run {
				want[lane] = true
			}
			for _, lane := range allLaneNames {
				if result.Lanes[lane].ShouldRun != want[lane] {
					t.Errorf("%s = %t, want %t", lane, result.Lanes[lane].ShouldRun, want[lane])
				}
			}
		})
	}
}

func TestClassifyPathsConservativelyRunsEverything(t *testing.T) {
	t.Parallel()
	for _, paths := range [][]string{nil, {".github/workflows/ci.yml"}, {"Makefile"}, {"go.mod"}, {"unknown-root-file"}} {
		result := classifyPaths(paths)
		if result.Classification != "full" {
			t.Fatalf("classification = %q, want full", result.Classification)
		}
		for _, lane := range allLaneNames {
			if !result.Lanes[lane].ShouldRun {
				t.Errorf("%s was not selected", lane)
			}
		}
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
	if !strings.Contains(stdout.String(), "lane_ui_backend_integration=true") {
		t.Fatalf("stdout = %q", stdout.String())
	}
	contents, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"run_frontend=true", "run_backend=true", "run_ui_backend_integration=true", "run_docs_reference=false"} {
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
