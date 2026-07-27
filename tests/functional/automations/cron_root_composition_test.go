package automations

import (
	"sync"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestBuildProcessRemainsCronInertBeforeRuntimeLifecycle(t *testing.T) {
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

func TestAutomationsCronActivatesThroughRuntimeLifecycle(t *testing.T) {
	start := time.Date(2026, time.April, 18, 12, 30, 0, 0, time.UTC)
	fakeClock := clockwork.NewFakeClockAt(start)

	observedSubmissions := make(chan work.FactorySubmissionRecord, 8)
	dir := support.ScaffoldFactory(t, cronCompositionFactoryConfig())
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{
			Clock: fakeClock,
			SubmissionRecorder: func(record work.FactorySubmissionRecord) {
				select {
				case observedSubmissions <- record:
				default:
					t.Fatalf("cron submission channel overflow")
				}
			},
		},
	})
	t.Cleanup(func() { server.Stop(t) })

	record := waitForCronSubmission(t, observedSubmissions, "scheduled-task", start, 5*time.Second)
	if record.Request.WorkTypeID != interfaces.SystemTimeWorkTypeID {
		t.Fatalf("cron submission work type = %q, want %q", record.Request.WorkTypeID, interfaces.SystemTimeWorkTypeID)
	}
}

func cronCompositionFactoryConfig() map[string]any {
	return map[string]any{
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
			},
		}},
		"workers": []map[string]string{{"name": "cron-worker"}},
		"workstations": []map[string]any{{
			"name":     "scheduled-task",
			"behavior": "CRON",
			"worker":   "cron-worker",
			"cron": map[string]any{
				"schedule":       "* * * * *",
				"triggerAtStart": true,
				"expiryWindow":   "10s",
			},
			"outputs": []map[string]string{{"workType": "task", "state": "init"}},
		}},
	}
}

func waitForCronSubmission(
	t *testing.T,
	submissions <-chan work.FactorySubmissionRecord,
	workstation string,
	nominalAt time.Time,
	timeout time.Duration,
) work.FactorySubmissionRecord {
	t.Helper()

	wantNominalAt := nominalAt.UTC().Format(time.RFC3339Nano)
	deadline := time.After(timeout)
	for {
		select {
		case record := <-submissions:
			if record.Request.Tags[interfaces.TimeWorkTagKeyCronWorkstation] != workstation {
				continue
			}
			if got := record.Request.Tags[interfaces.TimeWorkTagKeyNominalAt]; got != wantNominalAt {
				t.Fatalf("cron submission nominal_at = %q, want %q", got, wantNominalAt)
			}
			return record
		case <-deadline:
			t.Fatalf("timed out waiting for cron submission from %q at %s", workstation, wantNominalAt)
		}
	}
}
