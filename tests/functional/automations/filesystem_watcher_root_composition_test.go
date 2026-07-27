package automations

import (
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestBuildProcessRemainsFilesystemWatcherInertBeforeRuntimeLifecycle(t *testing.T) {
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
		t.Fatalf("BuildProcess() submitted %d Work records, want zero before runtime lifecycle", len(submissions))
	}
}

func TestAutomationsFilesystemWatcherPreseedsThroughRuntimeLifecycle(t *testing.T) {
	observedSubmissions := make(chan work.FactorySubmissionRecord, 8)
	dir := support.ScaffoldFactory(t, filesystemWatcherFactoryConfig())
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "preseed item"}`))

	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{
			SubmissionRecorder: func(record work.FactorySubmissionRecord) {
				select {
				case observedSubmissions <- record:
				default:
					t.Fatalf("filesystem watcher submission channel overflow")
				}
			},
		},
	})
	t.Cleanup(func() { server.Stop(t) })

	record := waitForFilesystemWatcherSubmission(t, observedSubmissions, "task", 5*time.Second)
	if record.Request.WorkTypeID != "task" {
		t.Fatalf("filesystem watcher submission work type = %q, want task", record.Request.WorkTypeID)
	}
}

func filesystemWatcherFactoryConfig() map[string]any {
	return map[string]any{
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
			},
		}},
		"workers": []map[string]string{{"name": "processor"}},
		"workstations": []map[string]any{{
			"name":   "process",
			"worker": "processor",
			"inputs": []map[string]string{{"workType": "task", "state": "init"}},
			"outputs": []map[string]string{
				{"workType": "task", "state": "complete"},
			},
		}},
	}
}

func waitForFilesystemWatcherSubmission(
	t *testing.T,
	submissions <-chan work.FactorySubmissionRecord,
	workType string,
	timeout time.Duration,
) work.FactorySubmissionRecord {
	t.Helper()

	deadline := time.After(timeout)
	for {
		select {
		case record := <-submissions:
			if record.Request.WorkTypeID != workType {
				continue
			}
			return record
		case <-deadline:
			t.Fatalf("timed out waiting for filesystem watcher submission for work type %q", workType)
		}
	}
}
