package support

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

// RecordReplayFixture creates a deterministic recorded artifact for functional
// replay scenarios using only scripted test collaborators.
func RecordReplayFixture(t *testing.T) string {
	t.Helper()

	dir := testutil.CopyFixtureDir(t, LegacyFixtureDir(t, "service_simple"))
	artifactPath := filepath.Join(t.TempDir(), "service-simple.replay.json")
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkTypeID: "task",
		WorkID:     "replay-event-stream-work",
		TraceID:    "replay-event-stream-trace",
		Payload:    []byte(`{"title":"replay event stream"}`),
	})

	harness := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(testutil.NewMockProvider(
			interfaces.InferenceResponse{Content: "Step one done. COMPLETE"},
			interfaces.InferenceResponse{Content: "Step two done. COMPLETE"},
		)),
		testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithRecordPath(artifactPath),
	)
	harness.RunUntilComplete(t, 10*time.Second)
	return artifactPath
}
