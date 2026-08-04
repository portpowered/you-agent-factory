package root_test

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
)

// TestBuildProcessWiresRecordingsNeutralReplayGraph proves root.BuildProcess
// composes the Recordings graph that includes neutral replay without
// constructing Recordings implementation packages from pkg/root.
func TestBuildProcessWiresRecordingsNeutralReplayGraph(t *testing.T) {
	t.Parallel()

	replayReads := 0
	if _, err := root.BuildProcess(context.Background(), serviceedges.Edges{
		FactorySessionReplayRecordingReader: func(string) ([]byte, error) {
			replayReads++
			return nil, errors.New("replay input must remain inert during process construction")
		},
	}); err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	if replayReads != 0 {
		t.Fatalf("BuildProcess() read replay input %d times, want zero before runtime opening", replayReads)
	}
}
