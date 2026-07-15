package runtimetests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/config/configinit"
	"github.com/portpowered/infinite-you/pkg/config/namedfactorypath"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestRetiredSurfaceResidue_ReadCurrentFactoryPointerRejectsLegacyEncodedSegment(t *testing.T) {
	rootDir := t.TempDir()
	if err := os.MkdirAll(rootDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	encodedSegment, err := namedfactorypath.LegacyLayoutSegment("@you/goal")
	if err != nil {
		t.Fatalf("LegacyLayoutSegment(@you/goal): %v", err)
	}
	pointerPath := filepath.Join(rootDir, interfaces.CurrentFactoryPointerFile)
	if err := os.WriteFile(pointerPath, []byte(encodedSegment+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(current pointer): %v", err)
	}

	if _, err := ReadCurrentFactoryPointer(rootDir); err == nil {
		t.Fatal("expected legacy encoded current-factory pointer to be rejected")
	}
}

func TestRetiredSurfaceResidue_ReadCurrentFactoryPointerAcceptsCanonicalScopedName(t *testing.T) {
	rootDir := t.TempDir()
	if _, err := PersistNamedFactory(rootDir, "@you/goal", namedFactoryPayload(t, "goal")); err != nil {
		t.Fatalf("PersistNamedFactory(@you/goal): %v", err)
	}
	if err := WriteCurrentFactoryPointer(rootDir, "@you/goal"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer: %v", err)
	}

	current, err := ReadCurrentFactoryPointer(rootDir)
	if err != nil {
		t.Fatalf("ReadCurrentFactoryPointer: %v", err)
	}
	if current != "@you/goal" {
		t.Fatalf("current = %q, want canonical scoped display name @you/goal", current)
	}
	if strings.Contains(current, "%2F") {
		t.Fatalf("current = %q, must not use percent-encoded layout segments", current)
	}
}

func TestRetiredSurfaceResidue_ConfigInitLeavesLegacyEncodedSiblingUntouched(t *testing.T) {
	homeDir := t.TempDir()
	namedFactoriesRoot := filepath.Join(homeDir, ".you-agent-factory", "you-agent-factories")
	encodedDir := seedLegacyEncodedGoalFactoryForResidueTest(t, namedFactoriesRoot)
	beforeSnapshot := snapshotDirectoryContents(t, encodedDir)

	result, err := configinit.Init(homeDir)
	if err != nil {
		t.Fatalf("configinit.Init: %v", err)
	}
	if len(result.PackagedFactories) == 0 {
		t.Fatal("expected packaged factories to be ensured during init")
	}

	assertDirectorySnapshotUnchanged(t, encodedDir, beforeSnapshot)
	for _, factory := range result.PackagedFactories {
		if factory.Name == "@you/goal" && strings.Contains(factory.FactoryDir, "%2F") {
			t.Fatalf("packaged factory dir = %q, must not use percent-encoded scoped leaf names", factory.FactoryDir)
		}
	}
}

func seedLegacyEncodedGoalFactoryForResidueTest(t *testing.T, factoriesRoot string) string {
	t.Helper()

	segment, err := namedfactorypath.LegacyLayoutSegment("@you/goal")
	if err != nil {
		t.Fatalf("LegacyLayoutSegment(@you/goal): %v", err)
	}
	encodedDir := filepath.Join(factoriesRoot, segment)
	if err := os.MkdirAll(encodedDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(encoded legacy dir): %v", err)
	}
	markerPath := filepath.Join(encodedDir, legacyEncodedGoalMarkerFile)
	if err := os.WriteFile(markerPath, []byte("do-not-touch-legacy-encoded-goal\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(marker): %v", err)
	}
	return encodedDir
}
