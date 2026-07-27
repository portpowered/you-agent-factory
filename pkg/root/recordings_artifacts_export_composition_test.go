package root_test

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
)

// TestBuildProcessWiresRecordingsArtifactExportGraph proves root.BuildProcess
// composes the Recordings graph that includes artifacts close/export/read without
// constructing Recordings implementation packages from pkg/root.
func TestBuildProcessWiresRecordingsArtifactExportGraph(t *testing.T) {
	t.Parallel()

	if _, err := root.BuildProcess(context.Background(), serviceedges.Edges{}); err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
}
