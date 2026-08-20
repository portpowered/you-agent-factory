package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunClassifiesOwnershipBoundaryEdgesBySourceClass(t *testing.T) {
	const peerImport = `import engine "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/engine"`

	tests := []struct {
		name    string
		files   map[string]string
		wantErr bool
		counts  string
		class   string
	}{
		{
			name: "production",
			files: map[string]string{
				"pkg/services/factory_sessions/doc.go":                "package factory_sessions\n",
				"pkg/services/factory_runtime/doc.go":                 "package factory_runtime\n",
				"pkg/services/factory_runtime/internal/engine/doc.go": "package engine\n",
				"pkg/services/factory_sessions/consume_peer.go":       "package factory_sessions\n" + peerImport + "\n",
			},
			wantErr: true,
			counts:  "dependency violation counts: production=1 test-only=0",
			class:   "[class=production]",
		},
		{
			name: "test-only",
			files: map[string]string{
				"pkg/services/factory_sessions/doc.go":                "package factory_sessions\n",
				"pkg/services/factory_runtime/doc.go":                 "package factory_runtime\n",
				"pkg/services/factory_runtime/internal/engine/doc.go": "package engine\n",
				"pkg/services/factory_sessions/consume_peer_test.go":  "package factory_sessions\n" + peerImport + "\n",
			},
			counts: "dependency violation counts: production=0 test-only=1",
			class:  "[class=test-only]",
		},
		{
			name: "mixed",
			files: map[string]string{
				"pkg/services/factory_sessions/doc.go":                "package factory_sessions\n",
				"pkg/services/factory_runtime/doc.go":                 "package factory_runtime\n",
				"pkg/services/factory_runtime/internal/engine/doc.go": "package engine\n",
				"pkg/services/factory_sessions/consume_peer.go":       "package factory_sessions\n" + peerImport + "\n",
				"pkg/services/factory_sessions/consume_peer_test.go":  "package factory_sessions\n" + peerImport + "\n",
			},
			wantErr: true,
			counts:  "dependency violation counts: production=1 test-only=1",
			class:   "[class=test-only]",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := fixtureRepository(t, test.files)
			var stdout, stderr bytes.Buffer
			err := run(config{root: root}, &stdout, &stderr)
			if (err != nil) != test.wantErr {
				t.Fatalf("run() error = %v, wantErr=%t; stdout=%q stderr=%q", err, test.wantErr, stdout.String(), stderr.String())
			}
			output := stdout.String() + stderr.String()
			if !strings.Contains(output, test.counts) {
				t.Fatalf("run() output = %q, want counts %q", output, test.counts)
			}
			if !strings.Contains(output, test.class) {
				t.Fatalf("run() output = %q, want source class %q", output, test.class)
			}
			if test.name == "test-only" && strings.Contains(stderr.String(), "new violation") {
				t.Fatalf("test-only finding entered blocking stderr path: %q", stderr.String())
			}
		})
	}
}

func TestOwnershipBoundaryDiscoveryRetainsTestOnlyEdgesInScopedAndPeerScans(t *testing.T) {
	root := fixtureRepository(t, map[string]string{
		"pkg/services/factory_sessions/doc.go":                "package factory_sessions\n",
		"pkg/services/factory_runtime/doc.go":                 "package factory_runtime\n",
		"pkg/services/factory_runtime/internal/engine/doc.go": "package engine\n",
		"pkg/initializer/application/initializer.go":          "package application\nimport _ \"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/engine\"\n",
		"pkg/initializer/application/initializer_test.go":     "package application\nimport _ \"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/engine\"\n",
		"pkg/services/factory_sessions/consume_peer.go":       "package factory_sessions\nimport _ \"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/engine\"\n",
		"pkg/services/factory_sessions/consume_peer_test.go":  "package factory_sessions\nimport _ \"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/engine\"\n",
	})

	findings := scanFixture(t, root)
	want := map[string]bool{
		"pkg/initializer/application/initializer.go|production":        false,
		"pkg/initializer/application/initializer_test.go|test-only":    false,
		"pkg/services/factory_sessions/consume_peer.go|production":     false,
		"pkg/services/factory_sessions/consume_peer_test.go|test-only": false,
	}
	for _, item := range findings {
		key := item.FilePath + "|" + string(effectiveBoundarySourceClass(item.class, item.FilePath))
		if _, ok := want[key]; ok {
			want[key] = true
		}
	}
	for key, found := range want {
		if !found {
			t.Errorf("scan findings missing classified edge %q: %#v", key, findings)
		}
	}
}

func TestOwnershipBoundaryTestOnlyFindingsDoNotUseProductionBaseline(t *testing.T) {
	const (
		productionPath = "pkg/services/factory_sessions/consume_peer.go"
		testPath       = "pkg/services/factory_sessions/consume_peer_test.go"
		target         = modulePath + "/pkg/services/factory_runtime/internal/engine"
	)
	root := fixtureRepository(t, map[string]string{
		"pkg/services/factory_sessions/doc.go":                "package factory_sessions\n",
		"pkg/services/factory_runtime/doc.go":                 "package factory_runtime\n",
		"pkg/services/factory_runtime/internal/engine/doc.go": "package engine\n",
		productionPath: "package factory_sessions\nimport _ \"" + target + "\"\n",
		testPath:       "package factory_sessions\nimport _ \"" + target + "\"\n",
	})
	writeBaseline(t, root, baselineEntryFor(rulePeerServiceImplementation, productionPath, target))

	var stdout, stderr bytes.Buffer
	if err := run(config{root: root}, &stdout, &stderr); err != nil {
		t.Fatalf("run with recorded production edge and test-only edge: %v; stdout=%s stderr=%s", err, stdout.String(), stderr.String())
	}
	output := stdout.String() + stderr.String()
	if !strings.Contains(output, "test-only observation") ||
		!strings.Contains(output, "dependency violation counts: production=0 test-only=1") {
		t.Fatalf("output = %q, want visible non-blocking test-only edge and zero unrecorded production count", output)
	}
	if !strings.Contains(stdout.String(), "1 deletion-only baseline") {
		t.Fatalf("stdout = %q, want active production baseline", stdout.String())
	}
}

func TestOwnershipBoundaryCreateBaselineIgnoresTestOnlyFindings(t *testing.T) {
	root := fixtureRepository(t, map[string]string{
		"pkg/services/factory_sessions/doc.go":                "package factory_sessions\n",
		"pkg/services/factory_runtime/doc.go":                 "package factory_runtime\n",
		"pkg/services/factory_runtime/internal/engine/doc.go": "package engine\n",
		"pkg/services/factory_sessions/consume_peer_test.go":  "package factory_sessions\nimport _ \"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/engine\"\n",
	})

	var stdout, stderr bytes.Buffer
	err := run(config{root: root, createBaseline: true}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "absence records zero debt") {
		t.Fatalf("create baseline with only test findings = %v, want zero production debt refusal", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, baselineFile)); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("test-only baseline exists or stat failed unexpectedly: %v", statErr)
	}
}

func TestOwnershipBoundaryKeysIncludeSourceClass(t *testing.T) {
	production := findingKey(finding{
		Rule: rulePeerServiceImplementation, FilePath: "pkg/services/factory_sessions/edge.go",
		Target: "peer", class: productionSourceClass,
	})
	testOnly := findingKey(finding{
		Rule: rulePeerServiceImplementation, FilePath: "pkg/services/factory_sessions/edge.go",
		Target: "peer", class: testOnlySourceClass,
	})
	if production == testOnly {
		t.Fatalf("finding keys collapsed source classes: %q", production)
	}

	productionEntry := entryKey(baselineEntry{
		Rule: rulePeerServiceImplementation, FilePath: "pkg/services/factory_sessions/edge.go",
		Target: "peer", Class: string(productionSourceClass),
	})
	testOnlyEntry := entryKey(baselineEntry{
		Rule: rulePeerServiceImplementation, FilePath: "pkg/services/factory_sessions/edge.go",
		Target: "peer", Class: string(testOnlySourceClass),
	})
	if productionEntry == testOnlyEntry {
		t.Fatalf("baseline keys collapsed source classes: %q", productionEntry)
	}
}
