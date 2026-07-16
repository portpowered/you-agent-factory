package runtime

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
)

func TestServiceMode_SubmissionWhilePaused_BuffersUntilResume(t *testing.T) {
	h := startServiceModeRunHarness(t,
		factory.WithNet(buildSimpleNet()),
		factory.WithServiceMode(),
		factory.WithInlineDispatch(),
		factory.WithWorkerExecutor("mock", &passExecutor{}),
		factory.WithLogger(logging.NoopLogger{}),
	)
	defer h.stop()

	h.pauseAndWait()
	submitPausedBufferTask(t, h.Factory, "request-runtime-paused-submit-001", "trace-runtime-paused-submit")
	assertPausedSubmissionNotApplied(t, h.Factory)

	h.resumeAndWait()
	waitForWorkAtPlace(t, h.Factory, "task:done", time.Second)
	assertTaskDoneOnce(t, h.Factory)
}
