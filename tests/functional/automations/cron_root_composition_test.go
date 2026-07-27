package automations

import (
	"encoding/json"
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

func TestAutomationsCronJitterProducesStableSubmissionTiming(t *testing.T) {
	start := time.Date(2026, time.April, 18, 12, 30, 0, 0, time.UTC)
	fakeClock := clockwork.NewFakeClockAt(start)

	observedSubmissions := make(chan work.FactorySubmissionRecord, 8)
	dir := support.ScaffoldFactory(t, cronJitterFactoryConfig())
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

	record := waitForCronSubmission(t, observedSubmissions, "jittered-task", start, 5*time.Second)
	assertCronJitterPayload(t, record, "jittered-task", start, 5*time.Second)
}

func TestAutomationsCronSkipsMalformedWorkstationAndFiresValid(t *testing.T) {
	start := time.Date(2026, time.April, 18, 12, 30, 0, 0, time.UTC)
	fakeClock := clockwork.NewFakeClockAt(start)

	observedSubmissions := make(chan work.FactorySubmissionRecord, 8)
	dir := support.ScaffoldFactory(t, cronMixedValidityFactoryConfig())
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

	record := waitForCronSubmission(t, observedSubmissions, "valid-task", start, 5*time.Second)
	if record.Request.Tags[interfaces.TimeWorkTagKeyCronWorkstation] != "valid-task" {
		t.Fatalf("cron submission workstation = %q, want valid-task", record.Request.Tags[interfaces.TimeWorkTagKeyCronWorkstation])
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

func cronJitterFactoryConfig() map[string]any {
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
			"name":     "jittered-task",
			"behavior": "CRON",
			"worker":   "cron-worker",
			"cron": map[string]any{
				"schedule":       "* * * * *",
				"triggerAtStart": true,
				"jitter":         "5s",
				"expiryWindow":   "10s",
			},
			"outputs": []map[string]string{{"workType": "task", "state": "init"}},
		}},
	}
}

func cronMixedValidityFactoryConfig() map[string]any {
	return map[string]any{
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
			},
		}},
		"workers": []map[string]string{{"name": "cron-worker"}},
		"workstations": []map[string]any{
			{
				"name":     "invalid-task",
				"behavior": "CRON",
				"worker":   "cron-worker",
				"cron": map[string]any{
					"schedule":       "not-a-cron",
					"triggerAtStart": true,
				},
				"outputs": []map[string]string{{"workType": "task", "state": "init"}},
			},
			{
				"name":     "valid-task",
				"behavior": "CRON",
				"worker":   "cron-worker",
				"cron": map[string]any{
					"schedule":       "* * * * *",
					"triggerAtStart": true,
					"expiryWindow":   "10s",
				},
				"outputs": []map[string]string{{"workType": "task", "state": "init"}},
			},
		},
	}
}

func assertCronJitterPayload(
	t *testing.T,
	record work.FactorySubmissionRecord,
	workstation string,
	nominalAt time.Time,
	maxJitter time.Duration,
) {
	t.Helper()

	var payload map[string]string
	if err := json.Unmarshal(record.Request.Payload, &payload); err != nil {
		t.Fatalf("cron submission payload is not JSON: %v", err)
	}
	if payload["cron_workstation"] != workstation {
		t.Fatalf("cron submission payload workstation = %q, want %q", payload["cron_workstation"], workstation)
	}

	nominal, err := time.Parse(time.RFC3339Nano, payload["nominal_at"])
	if err != nil {
		t.Fatalf("cron submission nominal_at = %q: %v", payload["nominal_at"], err)
	}
	if !nominal.Equal(nominalAt.UTC()) {
		t.Fatalf("cron submission nominal_at = %s, want %s", nominal, nominalAt.UTC())
	}

	jitter, err := time.ParseDuration(payload["jitter"])
	if err != nil {
		t.Fatalf("cron submission jitter = %q: %v", payload["jitter"], err)
	}
	if jitter < 0 || jitter > maxJitter {
		t.Fatalf("cron submission jitter = %s, want inclusive [0, %s]", jitter, maxJitter)
	}

	dueAt, err := time.Parse(time.RFC3339Nano, payload["due_at"])
	if err != nil {
		t.Fatalf("cron submission due_at = %q: %v", payload["due_at"], err)
	}
	if !dueAt.Equal(nominal.Add(jitter)) {
		t.Fatalf("cron submission due_at = %s, want nominal+jitter=%s", dueAt, nominal.Add(jitter))
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
