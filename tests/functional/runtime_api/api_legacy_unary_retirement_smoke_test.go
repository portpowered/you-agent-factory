package runtime_api

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"go.uber.org/zap"
)

func TestLegacyUnaryRetirementSmoke_RuntimeSubmitPathsStayBatchOnly(t *testing.T) {
	skipSlowFunctionalSmokeInShort(t, "slow legacy unary retirement boundary smoke")

	t.Run("direct_POST_and_idempotent_PUT", func(t *testing.T) {
		dir := scaffoldFactory(t, simplePipelineConfig())
		server := startFunctionalServer(t, dir, true, factory.WithServiceMode())

		traceID := submitGeneratedWork(t, server.URL(), factoryapi.SubmitWorkRequest{
			WorkTypeName: "task",
			Payload:      map[string]string{"title": "direct post canonical submit"},
		})
		if traceID == "" {
			t.Fatal("POST /work returned an empty trace ID")
		}
		waitForGeneratedWorkComplete(t, server.URL(), traceID, 10*time.Second)

		workTypeName := "task"
		workID := "work-retired-unary-put"
		request := factoryapi.WorkRequest{
			RequestId: "request-retired-unary-put",
			Type:      factoryapi.WorkRequestTypeFactoryRequestBatch,
			Works: &[]factoryapi.Work{{
				Name:         "idempotent-put",
				WorkId:       &workID,
				WorkTypeName: &workTypeName,
				Payload:      map[string]string{"title": "idempotent put canonical submit"},
			}},
		}
		first := putGeneratedWorkRequest(t, server.URL(), request.RequestId, request)
		retry := putGeneratedWorkRequest(t, server.URL(), request.RequestId, request)
		if retry.TraceId != first.TraceId {
			t.Fatalf("idempotent PUT trace_id changed: first=%q retry=%q", first.TraceId, retry.TraceId)
		}
		waitForGeneratedWorkIDsComplete(t, server.URL(), []string{workID}, 10*time.Second)

		events, err := server.service.GetFactoryEvents(context.Background())
		if err != nil {
			t.Fatalf("GetFactoryEvents: %v", err)
		}
		support.AssertSingleWorkRequestEvent(t, events, request.RequestId, workID, "task")
	})

	t.Run("startup_work_file_batch", func(t *testing.T) {
		dir := scaffoldFactory(t, simplePipelineConfig())
		workFile := filepath.Join(dir, "startup-work.json")
		support.WriteWorkRequestFile(t, workFile, interfaces.SubmitRequest{
			RequestID:  "request-retired-unary-work-file",
			Name:       "startup-file",
			WorkID:     "work-retired-unary-work-file",
			WorkTypeID: "task",
			Payload:    []byte(`{"title":"startup file canonical submit"}`),
		})

		svc, err := service.BuildFactoryService(context.Background(), &service.FactoryServiceConfig{
			Dir:               dir,
			WorkFile:          workFile,
			MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
			Logger:            zap.NewNop(),
		})
		if err != nil {
			t.Fatalf("BuildFactoryService: %v", err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := svc.Run(ctx); err != nil {
			t.Fatalf("FactoryService.Run: %v", err)
		}

		events, err := svc.GetFactoryEvents(context.Background())
		if err != nil {
			t.Fatalf("GetFactoryEvents: %v", err)
		}
		support.AssertSingleWorkRequestEvent(t, events, "request-retired-unary-work-file", "work-retired-unary-work-file", "task")
	})

	t.Run("file_watcher_non_batch_JSON", func(t *testing.T) {
		dir := scaffoldFactory(t, simplePipelineConfig())
		inputDir := filepath.Join(dir, interfaces.InputsDir, "task", interfaces.DefaultChannelName)
		if err := os.MkdirAll(inputDir, 0o755); err != nil {
			t.Fatalf("create input dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(inputDir, "non-batch.json"), []byte(`{"title":"raw JSON file input"}`), 0o644); err != nil {
			t.Fatalf("write non-batch seed: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		svc, err := service.BuildFactoryService(ctx, &service.FactoryServiceConfig{
			Dir:               dir,
			MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
			Logger:            zap.NewNop(),
		})
		if err != nil {
			t.Fatalf("BuildFactoryService: %v", err)
		}
		if err := svc.Run(ctx); err != nil {
			t.Fatalf("FactoryService.Run: %v", err)
		}

		events, err := svc.GetFactoryEvents(context.Background())
		if err != nil {
			t.Fatalf("GetFactoryEvents: %v", err)
		}
		support.AssertSingleWorkRequestEventByWorkName(t, events, "non-batch", "task")
	})

	t.Run("cron_internal_time_work", func(t *testing.T) {
		start := time.Date(2026, time.April, 18, 12, 30, 0, 0, time.UTC)
		fakeClock := clockwork.NewFakeClockAt(start)
		dir := scaffoldFactory(t, retiredUnaryCronFactoryConfig("* * * * *"))
		observedSubmissions := make(chan interfaces.FactorySubmissionRecord, 16)
		server := startFunctionalServerWithConfig(t, dir, true, func(cfg *service.FactoryServiceConfig) {
			cfg.RuntimeMode = interfaces.RuntimeModeService
			cfg.Clock = fakeClock
		}, factory.WithSubmissionRecorder(func(record interfaces.FactorySubmissionRecord) {
			observedSubmissions <- record
		}))

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

		events, err := server.service.GetFactoryEvents(context.Background())
		if err != nil {
			t.Fatalf("GetFactoryEvents: %v", err)
		}
		assertWorkRequestEventIncludesWorkID(t, events, record.Request.WorkID, "poll-for-work")
	})
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
				"name":    "poll-for-work",
				"kind":    "cron",
				"worker":  "cron-worker",
				"cron":    map[string]any{"schedule": schedule, "expiryWindow": "10s"},
				"outputs": []map[string]string{{"workType": "task", "state": "init"}},
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
	submissions <-chan interfaces.FactorySubmissionRecord,
	workstation string,
	nominalAt time.Time,
	timeout time.Duration,
) interfaces.FactorySubmissionRecord {
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

func assertWorkRequestEventIncludesWorkID(t *testing.T, events []factoryapi.FactoryEvent, workID, workstation string) {
	t.Helper()

	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeWorkRequest {
			continue
		}
		payload, err := event.Payload.AsWorkRequestEventPayload()
		if err != nil {
			t.Fatalf("decode WORK_REQUEST event %q: %v", event.Id, err)
		}
		for _, work := range support.FactoryWorksValue(payload.Works) {
			if support.StringPointerValue(work.WorkId) != workID {
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
