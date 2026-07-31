package submission_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestLegacyUnaryRetirementSmoke_RuntimeSubmitPathsStayBatchOnly proves every
// legacy runtime submit ingress path records canonical FACTORY_REQUEST_BATCH work
// requests in the public Factory Event history.
func TestLegacyUnaryRetirementSmoke_RuntimeSubmitPathsStayBatchOnly(t *testing.T) {
	if testing.Short() {
		t.Skip("slow legacy unary retirement boundary smoke")
	}

	t.Run("direct_POST_and_idempotent_PUT", func(t *testing.T) {
		assertLegacyUnaryDirectSubmitAndPut(t)
	})

	t.Run("startup_work_file_batch", func(t *testing.T) {
		assertLegacyUnaryStartupWorkFileBatch(t)
	})

	t.Run("file_watcher_non_batch_JSON", func(t *testing.T) {
		assertLegacyUnaryFileWatcherBatchConversion(t)
	})

	t.Run("cron_internal_time_work", func(t *testing.T) {
		assertLegacyUnaryCronSubmitPath(t)
	})
}

func assertLegacyUnaryDirectSubmitAndPut(t *testing.T) {
	t.Helper()

	factoryDir := support.ScaffoldFactory(t, simplePipelineFactoryConfig())
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:     factoryDir,
		UseMockWorkers: true,
	})
	defer server.Stop(t)

	submitted := support.SubmitDefaultSessionWork(t, server.URL(), factoryapi.SubmitWorkRequest{
		Name:         stringPtr("direct-post-canonical-submit"),
		WorkTypeName: "task",
		Payload:      map[string]string{"title": "direct post canonical submit"},
	})
	if submitted.TraceId == "" {
		t.Fatal("POST /work returned an empty trace ID")
	}
	support.WaitForTerminalStatus(t, server.URL(), 10*time.Second)

	workTypeName := "task"
	workID := "work-retired-unary-put"
	request := factoryapi.WorkRequest{
		RequestId: "request-retired-unary-put",
		Type:      factoryapi.WorkRequestTypeFactoryRequestBatch,
		Works: &[]factoryapi.Work{{
			Name:         "idempotent-put",
			WorkId:       stringPtr(workID),
			WorkTypeName: &workTypeName,
			Payload:      map[string]string{"title": "idempotent put canonical submit"},
		}},
	}
	first := support.UpsertDefaultSessionWorkRequest(t, server.URL(), request)
	retry := support.UpsertDefaultSessionWorkRequest(t, server.URL(), request)
	if retry.TraceId != first.TraceId {
		t.Fatalf("idempotent PUT trace_id changed: first=%q retry=%q", first.TraceId, retry.TraceId)
	}
	waitForWorkIDsComplete(t, server.URL(), []string{workID}, 10*time.Second)
	support.AssertSingleWorkRequestEvent(t, server.GetFactoryEvents(t), request.RequestId, workID, "task")
}

func assertLegacyUnaryStartupWorkFileBatch(t *testing.T) {
	t.Helper()

	factoryDir := support.ScaffoldFactory(t, simplePipelineFactoryConfig())
	workFile := filepath.Join(factoryDir, "startup-work.json")
	support.WriteWorkRequestFile(t, workFile, work.SubmitRequest{
		RequestID:  "request-retired-unary-work-file",
		Name:       "startup-file",
		WorkID:     "work-retired-unary-work-file",
		WorkTypeID: "task",
		Payload:    []byte(`{"title":"startup file canonical submit"}`),
	})

	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
		Args:                      []string{"--work", workFile},
	})
	defer server.Stop(t)

	waitForWorkIDsComplete(t, server.URL(), []string{"work-retired-unary-work-file"}, 10*time.Second)
	support.AssertSingleWorkRequestEvent(
		t,
		server.GetFactoryEvents(t),
		"request-retired-unary-work-file",
		"work-retired-unary-work-file",
		"task",
	)
}

func assertLegacyUnaryFileWatcherBatchConversion(t *testing.T) {
	t.Helper()

	factoryDir := support.ScaffoldFactory(t, simplePipelineFactoryConfig())
	inputDir := filepath.Join(factoryDir, interfaces.InputsDir, "task", interfaces.DefaultChannelName)
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		t.Fatalf("create input dir: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(inputDir, "non-batch.json"),
		[]byte(`{"title":"raw JSON file input"}`),
		0o644,
	); err != nil {
		t.Fatalf("write non-batch seed: %v", err)
	}

	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:     factoryDir,
		UseMockWorkers: true,
	})
	defer server.Stop(t)

	waitForWorkTypeComplete(t, server.URL(), "task", 10*time.Second)
	support.AssertSingleWorkRequestEventByWorkName(t, server.GetFactoryEvents(t), "non-batch", "task")
}

func assertLegacyUnaryCronSubmitPath(t *testing.T) {
	t.Helper()

	start := time.Date(2026, time.April, 18, 12, 30, 0, 0, time.UTC)
	fakeClock := clockwork.NewFakeClockAt(start)
	factoryDir := support.ScaffoldFactory(t, retiredUnaryCronFactoryConfig("* * * * *"))
	observedSubmissions := make(chan work.FactorySubmissionRecord, 16)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{
			Clock: fakeClock,
			SubmissionRecorder: func(record work.FactorySubmissionRecord) {
				observedSubmissions <- record
			},
		},
	})
	defer server.Stop(t)

	waitForFakeClockWaiters(t, fakeClock, 1)
	nominalAt := start.Add(time.Minute)
	fakeClock.Advance(time.Minute)
	record := waitForCronSubmissionRecord(t, observedSubmissions, "poll-for-work", nominalAt, time.Second)
	if record.Source != "external-submit" {
		t.Fatalf("cron submission source = %q, want external-submit", record.Source)
	}
	if record.Request.WorkTypeID != interfaces.SystemTimeWorkTypeID {
		t.Fatalf("cron work type = %q, want %q", record.Request.WorkTypeID, interfaces.SystemTimeWorkTypeID)
	}

	assertWorkRequestEventIncludesWorkID(t, server.GetFactoryEvents(t), record.Request.WorkID, "poll-for-work")
}

func retiredUnaryCronFactoryConfig(schedule string) map[string]any {
	return map[string]any{
		"workTypes": []map[string]any{
			{
				"name": "task",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workers": []map[string]string{{"name": "cron-worker"}},
		"workstations": []map[string]any{
			{
				"name":     "poll-for-work",
				"behavior": "CRON",
				"worker":   "cron-worker",
				"cron":     map[string]any{"schedule": schedule, "expiryWindow": "10s"},
				"outputs":  []map[string]string{{"workType": "task", "state": "init"}},
			},
		},
	}
}

func waitForFakeClockWaiters(t *testing.T, fakeClock *clockwork.FakeClock, waiters int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := fakeClock.BlockUntilContext(ctx, waiters); err != nil {
		t.Fatalf("timed out waiting for %d fake-clock waiter(s): %v", waiters, err)
	}
}

func waitForCronSubmissionRecord(
	t *testing.T,
	submissions <-chan work.FactorySubmissionRecord,
	workstation string,
	nominalAt time.Time,
	timeout time.Duration,
) work.FactorySubmissionRecord {
	t.Helper()

	deadline := time.After(timeout)
	wantNominalAt := nominalAt.UTC().Format(time.RFC3339Nano)
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

func assertWorkRequestEventIncludesWorkID(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	workID, workstation string,
) {
	t.Helper()

	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeWorkRequest {
			continue
		}
		payload, err := event.Payload.AsWorkRequestEventPayload()
		if err != nil {
			t.Fatalf("decode WORK_REQUEST event %q: %v", event.Id, err)
		}
		for _, workItem := range support.FactoryWorksValue(payload.Works) {
			if support.StringPointerValue(workItem.WorkId) != workID {
				continue
			}
			if payload.Type != factoryapi.WorkRequestTypeFactoryRequestBatch {
				t.Fatalf("cron WORK_REQUEST type = %q, want FACTORY_REQUEST_BATCH", payload.Type)
			}
			return
		}
	}

	t.Fatalf("canonical history missing WORK_REQUEST for cron time work %q from %q", workID, workstation)
}
