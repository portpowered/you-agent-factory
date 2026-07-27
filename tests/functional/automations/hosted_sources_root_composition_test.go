package automations

import (
	"sync"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestBuildProcessRemainsHostedSourcesInertBeforeRuntimeLifecycle(t *testing.T) {
	t.Parallel()

	var submissionsMu sync.Mutex
	var submissions []work.FactorySubmissionRecord
	recorder := func(record work.FactorySubmissionRecord) {
		submissionsMu.Lock()
		submissions = append(submissions, record)
		submissionsMu.Unlock()
	}

	_ = support.BuildProcess(t, serviceedges.Edges{
		SubmissionRecorder: recorder,
	})

	submissionsMu.Lock()
	defer submissionsMu.Unlock()
	if len(submissions) != 0 {
		t.Fatalf(
			"BuildProcess() submitted %d Work records, want zero before runtime lifecycle",
			len(submissions),
		)
	}
}
