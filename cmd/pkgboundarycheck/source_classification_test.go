package main

import (
	"bytes"
	"strconv"
	"strings"
	"testing"
)

func TestRunClassifiesPackageBoundaryEdgesBySourceClass(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		files         map[string]string
		wantErr       bool
		wantError     string
		wantErrorText string
		wantCounts    string
	}{
		{
			name: "production",
			files: map[string]string{
				"pkg/services/work/composition.go": "package work\nimport _ \"github.com/portpowered/infinite-you/pkg/wire\"\n",
			},
			wantErr:       true,
			wantError:     "found 1 package-boundary violation(s)",
			wantErrorText: "prohibited application composition import: pkg/services/work (pkg/services/work/composition.go) [class=production]",
			wantCounts:    "dependency violation counts: production=1 test-only=0",
		},
		{
			name: "test-only",
			files: map[string]string{
				"pkg/services/work/composition_test.go": "package work_test\nimport _ \"github.com/portpowered/infinite-you/pkg/wire\"\n",
			},
			wantErrorText: "prohibited application composition import: pkg/services/work (pkg/services/work/composition_test.go) [class=test-only]",
			wantCounts:    "dependency violation counts: production=0 test-only=1",
		},
		{
			name: "mixed",
			files: map[string]string{
				"pkg/services/work/composition.go":      "package work\nimport _ \"github.com/portpowered/infinite-you/pkg/wire\"\n",
				"pkg/services/work/composition_test.go": "package work_test\nimport _ \"github.com/portpowered/infinite-you/pkg/wire\"\n",
			},
			wantErr:       true,
			wantError:     "found 1 package-boundary violation(s)",
			wantErrorText: "dependency violation counts: production=1 test-only=1",
			wantCounts:    "dependency violation counts: production=1 test-only=1",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			repoRoot := t.TempDir()
			for filePath, source := range test.files {
				writeGoSourceFile(t, repoRoot, filePath, source)
			}
			var stdout, stderr bytes.Buffer
			err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &stdout, &stderr)
			if test.wantErr != (err != nil) {
				t.Fatalf("run() error = %v, wantErr=%t; stdout=%q stderr=%q", err, test.wantErr, stdout.String(), stderr.String())
			}
			if test.wantError != "" && (err == nil || !strings.Contains(err.Error(), test.wantError)) {
				t.Fatalf("run() error = %v, want substring %q", err, test.wantError)
			}
			output := stdout.String() + stderr.String()
			if !strings.Contains(output, test.wantErrorText) {
				t.Fatalf("run() output = %q, want %q", output, test.wantErrorText)
			}
			if !strings.Contains(output, test.wantCounts) {
				t.Fatalf("run() output = %q, want counts %q", output, test.wantCounts)
			}
		})
	}
}

func TestPackageBoundaryImportScansRetainTestOnlyEdges(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoImportFile(t, repoRoot, "pkg/transports/work.go", "work", "github.com/portpowered/infinite-you/pkg/services/work/internal/engine")
	writeGoImportFile(t, repoRoot, "pkg/transports/work_test.go", "work_test", "github.com/portpowered/infinite-you/pkg/services/work/internal/engine")
	writeGoImportFile(t, repoRoot, "pkg/services/work/peer.go", "work", "github.com/portpowered/infinite-you/pkg/services/models/internal/catalog")
	writeGoImportFile(t, repoRoot, "pkg/services/work/peer_test.go", "work_test", "github.com/portpowered/infinite-you/pkg/services/models/internal/catalog")

	converged, err := scanConvergedServiceSubpackageImports(repoRoot)
	if err != nil {
		t.Fatalf("scanConvergedServiceSubpackageImports() error = %v", err)
	}
	if got := countSourceClasses(converged, func(finding transportServiceImplementationFinding) boundarySourceClass { return finding.class }, func(finding transportServiceImplementationFinding) string { return finding.filePath }); got != "production=1 test-only=1" {
		t.Fatalf("converged classes = %s, want production=1 test-only=1", got)
	}

	peer, err := scanPeerServiceImports(repoRoot)
	if err != nil {
		t.Fatalf("scanPeerServiceImports() error = %v", err)
	}
	if got := countSourceClasses(peer, func(finding peerServiceImportFinding) boundarySourceClass { return finding.class }, func(finding peerServiceImportFinding) string { return finding.filePath }); got != "production=1 test-only=1" {
		t.Fatalf("peer classes = %s, want production=1 test-only=1", got)
	}
}

func TestPackageBoundaryDependencyScansRetainTestOnlyEdges(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeGoImportFile(t, repoRoot, "pkg/services/work/domain.go", "work", "github.com/portpowered/infinite-you/pkg/transports/mapping")
	writeGoImportFile(t, repoRoot, "pkg/services/work/domain_test.go", "work_test", "github.com/portpowered/infinite-you/pkg/transports/mapping")
	writeGoImportFile(t, repoRoot, "pkg/transports/runtime.go", "runtime", "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/runtime")
	writeGoImportFile(t, repoRoot, "pkg/transports/runtime_test.go", "runtime_test", "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/runtime")
	writeGoImportFile(t, repoRoot, "internal/testutil/support.go", "testutil", "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/service")
	writeGoImportFile(t, repoRoot, "internal/testutil/support_test.go", "testutil", "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/service")

	domain, err := scanDomainTransportImports(repoRoot, nil)
	if err != nil {
		t.Fatalf("scanDomainTransportImports() error = %v", err)
	}
	if got := countSourceClasses(domain, func(finding domainTransportImportFinding) boundarySourceClass { return finding.class }, func(finding domainTransportImportFinding) string { return finding.filePath }); got != "production=1 test-only=1" {
		t.Fatalf("domain transport classes = %s, want production=1 test-only=1", got)
	}

	transport, err := scanTransportServiceImplementationImports(repoRoot)
	if err != nil {
		t.Fatalf("scanTransportServiceImplementationImports() error = %v", err)
	}
	if got := countSourceClasses(transport, func(finding transportServiceImplementationFinding) boundarySourceClass { return finding.class }, func(finding transportServiceImplementationFinding) string { return finding.filePath }); got != "production=1 test-only=1" {
		t.Fatalf("transport implementation classes = %s, want production=1 test-only=1", got)
	}

	support, err := scanSupportServiceSubpackageImports(repoRoot)
	if err != nil {
		t.Fatalf("scanSupportServiceSubpackageImports() error = %v", err)
	}
	if got := countSourceClasses(support, func(finding supportServiceImportFinding) boundarySourceClass { return finding.class }, func(finding supportServiceImportFinding) string { return finding.filePath }); got != "production=1 test-only=1" {
		t.Fatalf("support service classes = %s, want production=1 test-only=1", got)
	}

	var stdout, stderr bytes.Buffer
	if err := run(config{root: repoRoot, packageRoot: defaultScanRoot}, &stdout, &stderr); err == nil {
		t.Fatal("run() error = nil, want all three dependency rules enforced")
	}
	for _, path := range []string{
		"pkg/services/work/domain_test.go",
		"pkg/transports/runtime_test.go",
		"internal/testutil/support_test.go",
	} {
		if !strings.Contains(stderr.String(), path+") [class=test-only]") {
			t.Fatalf("run() stderr = %q, want class-labeled test-only diagnostic for %s", stderr.String(), path)
		}
	}
}

func TestPackageBoundaryDependencyKeysIncludeSourceClass(t *testing.T) {
	t.Parallel()

	production := peerServiceImportKey("pkg/services/work/import.go", "github.com/portpowered/infinite-you/pkg/services/models/internal", productionSourceClass)
	testOnly := peerServiceImportKey("pkg/services/work/import.go", "github.com/portpowered/infinite-you/pkg/services/models/internal", testOnlySourceClass)
	if production == testOnly {
		t.Fatalf("peer-service keys collapsed source classes: %q", production)
	}

	production = serviceConstructionKey("pkg/services/work/import.go", "github.com/portpowered/infinite-you/pkg/services/models", "NewService", productionSourceClass)
	testOnly = serviceConstructionKey("pkg/services/work/import.go", "github.com/portpowered/infinite-you/pkg/services/models", "NewService", testOnlySourceClass)
	if production == testOnly {
		t.Fatalf("service-construction keys collapsed source classes: %q", production)
	}
}

func countSourceClasses[T any](findings []T, class func(T) boundarySourceClass, path func(T) string) string {
	production, testOnly := 0, 0
	for _, finding := range findings {
		switch effectiveBoundarySourceClass(class(finding), path(finding)) {
		case productionSourceClass:
			production++
		case testOnlySourceClass:
			testOnly++
		}
	}
	return "production=" + strconv.Itoa(production) + " test-only=" + strconv.Itoa(testOnly)
}
