package root_test

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
)

// TestBuildProcessWiresRecordingsNeutralReplayGraph proves root.BuildProcess
// composes the Recordings graph that includes neutral replay without
// constructing Recordings implementation packages from pkg/root.
func TestBuildProcessWiresRecordingsNeutralReplayGraph(t *testing.T) {
	t.Parallel()

	if _, err := root.BuildProcess(context.Background(), serviceedges.Edges{}); err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
}
