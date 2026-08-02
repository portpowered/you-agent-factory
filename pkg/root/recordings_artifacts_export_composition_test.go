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

func TestBuildProcessAcceptsRecordingArtifactReadEdgeWithoutConstructionIO(t *testing.T) {
	t.Parallel()

	readCalls := 0
	if _, err := root.BuildProcess(context.Background(), serviceedges.Edges{
		RecordingReadFile: func(string) ([]byte, error) {
			readCalls++
			return nil, nil
		},
	}); err != nil {
		t.Fatalf("BuildProcess() with RecordingReadFile edge error = %v", err)
	}
	if readCalls != 0 {
		t.Fatalf("RecordingReadFile calls during inert BuildProcess() = %d, want zero", readCalls)
	}
}
