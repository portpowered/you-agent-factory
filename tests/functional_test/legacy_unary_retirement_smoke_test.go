package functional_test

import (
	"context"
	"encoding/json"
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
	"github.com/portpowered/infinite-you/pkg/testutil"
	"go.uber.org/zap"
)

func TestLegacyUnaryRetirementSmoke_CanonicalSubmitPathsStayBatchOnly(t *testing.T) {
	skipSlowFunctionalSmokeInShort(t, "slow unary-retirement smoke")
	t.Run("direct_POST_and_idempotent_PUT", assertRetiredUnaryAPISmoke)
	t.Run("startup_work_file_batch", assertRetiredUnaryStartupWorkFileSmoke)
	t.Run("file_watcher_non_batch_JSON", assertRetiredUnaryFileWatcherSmoke)
	t.Run("replay_due_submission", assertRetiredUnaryReplaySmoke)
	t.Run("cron_internal_time_work", assertRetiredUnaryCronSmoke)
}

func assertRetiredUnaryAPISmoke(t *testing.T) {
	dir := scaffoldFactory(t, simplePipelineConfig())
	server := StartFunctionalServer(t, dir, true, factory.WithServiceMode())

	traceID := server.SubmitWork(t, "task", []byte(`{"title":"direct post canonical submit"}`))
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
	assertSingleWorkRequestEvent(t, events, request.RequestId, workID, "task")
}

func assertRetiredUnaryStartupWorkFileSmoke(t *testing.T) {
	dir := scaffoldFactory(t, simplePipelineConfig())
	workFile := filepath.Join(dir, "startup-work.json")
	request := interfaces.WorkRequest{
		RequestID: "request-retired-unary-work-file",
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		Works: []interfaces.Work{{
			Name:       "startup-file",
			WorkID:     "work-retired-unary-work-file",
			WorkTypeID: "task",
			Payload:    json.RawMessage(`{"title":"startup file canonical submit"}`),
		}},
	}
	data, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal startup work request: %v", err)
	}
	if err := os.WriteFile(workFile, data, 0o644); err != nil {
		t.Fatalf("write startup work request: %v", err)
	}

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
	assertSingleWorkRequestEvent(t, events, request.RequestID, "work-retired-unary-work-file", "task")
}

func assertRetiredUnaryFileWatcherSmoke(t *testing.T) {
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
	assertSingleWorkRequestEventByWorkName(t, events, "non-batch", "task")
}

func assertRetiredUnaryReplaySmoke(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "service_simple"))
	artifactPath := filepath.Join(t.TempDir(), "retired-unary-smoke.replay.json")
	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "step one COMPLETE"},
		interfaces.InferenceResponse{Content: "step two COMPLETE"},
	)
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithRecordPath(artifactPath),
	)
	request := interfaces.WorkRequest{
		RequestID: "request-retired-unary-replay",
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		Works: []interfaces.Work{{
			Name:       "replayed",
			WorkID:     "work-retired-unary-replay",
			WorkTypeID: "task",
			Payload:    json.RawMessage(`{"title":"record replay canonical submit"}`),
		}},
	}
	h.SubmitWorkRequest(context.Background(), request)
	h.RunUntilComplete(t, 10*time.Second)

	artifact := testutil.LoadReplayArtifact(t, artifactPath)
	assertReplayWorkRequestRecorded(t, artifact, request.RequestID, "external-submit", 1, 0)
	replayHarness := testutil.AssertReplaySucceeds(t, artifactPath, 10*time.Second)
	events, err := replayHarness.Service.GetFactoryEvents(context.Background())
	if err != nil {
		t.Fatalf("GetFactoryEvents after replay: %v", err)
	}
	assertSingleWorkRequestEvent(t, events, request.RequestID, "work-retired-unary-replay", "task")
}

func assertRetiredUnaryCronSmoke(t *testing.T) {
	start := time.Date(2026, time.April, 18, 12, 30, 0, 0, time.UTC)
	fakeClock := clockwork.NewFakeClockAt(start)
	dir := scaffoldFactory(t, cronSmokeFactoryConfig("* * * * *"))
	observedSubmissions := make(chan interfaces.FactorySubmissionRecord, 16)
	server := StartFunctionalServerWithConfig(t, dir, true, func(cfg *service.FactoryServiceConfig) {
		cfg.RuntimeMode = interfaces.RuntimeModeService
		cfg.Clock = fakeClock
	}, factory.WithSubmissionRecorder(func(record interfaces.FactorySubmissionRecord) {
		observedSubmissions <- record
	}))

	waitForFakeClockWaiters(t, fakeClock, 1)
	nominalAt := start.Add(time.Minute)
	fakeClock.Advance(time.Minute)
	record := waitForCronSubmissionFromWorkstation(t, observedSubmissions, "poll-for-work", nominalAt, time.Second)
	if record.Source != "external-submit" {
		t.Fatalf("cron submission source = %q, want external-submit", record.Source)
	}
	if record.Request.WorkTypeID != interfaces.SystemTimeWorkTypeID {
		t.Fatalf("cron work type = %q, want %q", record.Request.WorkTypeID, interfaces.SystemTimeWorkTypeID)
	}
	assertCronTimeWorkRetainedInCanonicalHistory(t, server, record.Request.WorkID, "poll-for-work")
}

func assertSingleWorkRequestEvent(t *testing.T, events []factoryapi.FactoryEvent, requestID, workID, workTypeName string) {
	t.Helper()

	var matches []factoryapi.WorkRequestEventPayload
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeWorkRequest || stringPointerValue(event.Context.RequestId) != requestID {
			continue
		}
		payload, err := event.Payload.AsWorkRequestEventPayload()
		if err != nil {
			t.Fatalf("decode WORK_REQUEST event %q: %v", event.Id, err)
		}
		matches = append(matches, payload)
	}
	if len(matches) != 1 {
		t.Fatalf("WORK_REQUEST events for %q = %d, want 1", requestID, len(matches))
	}
	assertWorkRequestPayloadContainsWork(t, matches[0], workID, workTypeName)
}

func assertSingleWorkRequestEventByWorkName(t *testing.T, events []factoryapi.FactoryEvent, workName, workTypeName string) {
	t.Helper()

	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeWorkRequest {
			continue
		}
		payload, err := event.Payload.AsWorkRequestEventPayload()
		if err != nil {
			t.Fatalf("decode WORK_REQUEST event %q: %v", event.Id, err)
		}
		for _, work := range factoryWorksValue(payload.Works) {
			if work.Name == workName && stringPointerValue(work.WorkTypeName) == workTypeName {
				return
			}
		}
	}
	t.Fatalf("missing WORK_REQUEST work item %q with work_type_name %q", workName, workTypeName)
}

func assertWorkRequestPayloadContainsWork(t *testing.T, payload factoryapi.WorkRequestEventPayload, workID, workTypeName string) {
	t.Helper()

	if payload.Type != factoryapi.WorkRequestTypeFactoryRequestBatch {
		t.Fatalf("WORK_REQUEST type = %q, want FACTORY_REQUEST_BATCH", payload.Type)
	}
	for _, work := range factoryWorksValue(payload.Works) {
		if stringPointerValue(work.WorkId) == workID {
			if stringPointerValue(work.WorkTypeName) != workTypeName {
				t.Fatalf("work %q work_type_name = %q, want %q", workID, stringPointerValue(work.WorkTypeName), workTypeName)
			}
			return
		}
	}
	t.Fatalf("WORK_REQUEST missing work_id %q: %#v", workID, factoryWorksValue(payload.Works))
}
