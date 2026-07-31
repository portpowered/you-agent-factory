package submission_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/internal/testutil"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	batchInputsWorkType    = "task"
	batchInputsAltWorkType = "review"

	batchInputsInlineRequestID = "work-batch-inputs-inline"
	batchInputsInlineWorkName  = "inline-shape-task"

	batchInputsFileRequestID = "work-batch-inputs-file"
	batchInputsFileWorkName  = "file-shape-task"

	batchInputsStdinRequestID = "work-batch-inputs-stdin"
	batchInputsStdinWorkName  = "stdin-shape-task"

	batchInputsDefaultTypeRequestID = "work-batch-inputs-default-type"
	batchInputsDefaultTypeWorkName  = "default-type-task"

	batchInputsExplicitTypeRequestID = "work-batch-inputs-explicit-type"
	batchInputsExplicitTypeWorkName  = "explicit-type-review"

	batchInputsUnknownTypeRequestID = "work-batch-inputs-unknown-type"
	batchInputsUnknownTypeWorkName  = "unknown-type-task"
	batchInputsUnknownWorkType      = "nonexistent-type"

	batchInputsMixedBatchRequestID  = "work-batch-inputs-mixed-batch"
	batchInputsMixedValidWorkName   = "mixed-valid-task"
	batchInputsMixedInvalidWorkName = "mixed-invalid-task"

	batchIngressRegressionRequestID = "request-http-batch-ingress-regression"
	batchIngressRegressionWorkID    = "work-http-batch-ingress-regression"
	batchIngressRegressionTraceID   = "trace-http-batch-ingress-regression"
)

// TestWorkBatchAcceptsInlineFileAndStdinShapes proves the public Work Request
// batch ingress accepts the same canonical FACTORY_REQUEST_BATCH document when
// provided inline, via a filesystem path, or via stdin, and that each ingress
// path yields customer-visible accept outcomes for the submitted works.
func TestWorkBatchAcceptsInlineFileAndStdinShapes(t *testing.T) {
	factoryDir := support.ScaffoldFactory(t, batchInputsFactoryConfig())
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:     factoryDir,
		UseMockWorkers: true,
	})
	defer server.Stop(t)

	baseURL := server.URL()
	processHarness := newRootProcessHarness(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	canonicalBatch := func(requestID, workName string) string {
		return fmt.Sprintf(
			`{"requestId":%q,"type":"FACTORY_REQUEST_BATCH","works":[{"name":%q,"workTypeName":%q,"payload":{"title":"Batch inputs shape parity"}}]}`,
			requestID,
			workName,
			batchInputsWorkType,
		)
	}

	t.Run("inline", func(t *testing.T) {
		batchJSON := canonicalBatch(batchInputsInlineRequestID, batchInputsInlineWorkName)
		output, err := runYouSubmitBatch(ctx, processHarness, factoryDir, baseURL, batchJSON, nil)
		if err != nil {
			t.Fatalf("you submit batch inline: %v\noutput:\n%s", err, output)
		}
		submitted := decodeBatchSubmitJSON(t, output)
		assertBatchSubmitAcknowledgment(t, output, batchInputsInlineRequestID, batchInputsInlineWorkName)
		assertBatchWorkListedAfterSubmit(t, baseURL, batchInputsInlineWorkName, submitted.Works[0].WorkID)
	})

	t.Run("file", func(t *testing.T) {
		batchJSON := canonicalBatch(batchInputsFileRequestID, batchInputsFileWorkName)
		batchPath := filepath.Join(t.TempDir(), "batch-inputs-shape.json")
		if err := os.WriteFile(batchPath, []byte(batchJSON), 0o600); err != nil {
			t.Fatalf("write batch file: %v", err)
		}

		output, err := runYouSubmitBatch(ctx, processHarness, factoryDir, baseURL, batchPath, nil)
		if err != nil {
			t.Fatalf("you submit batch file: %v\noutput:\n%s", err, output)
		}
		submitted := decodeBatchSubmitJSON(t, output)
		assertBatchSubmitAcknowledgment(t, output, batchInputsFileRequestID, batchInputsFileWorkName)
		assertBatchWorkListedAfterSubmit(t, baseURL, batchInputsFileWorkName, submitted.Works[0].WorkID)
	})

	t.Run("stdin", func(t *testing.T) {
		batchJSON := canonicalBatch(batchInputsStdinRequestID, batchInputsStdinWorkName)
		output, err := runYouSubmitBatch(ctx, processHarness, factoryDir, baseURL, "-", strings.NewReader(batchJSON))
		if err != nil {
			t.Fatalf("you submit batch stdin: %v\noutput:\n%s", err, output)
		}
		submitted := decodeBatchSubmitJSON(t, output)
		assertBatchSubmitAcknowledgment(t, output, batchInputsStdinRequestID, batchInputsStdinWorkName)
		assertBatchWorkListedAfterSubmit(t, baseURL, batchInputsStdinWorkName, submitted.Works[0].WorkID)
	})
}

// TestWorkBatchSelectsDefaultAndExplicitWorkTypes proves public Work Request
// batch ingress materializes the Factory default work type when a batch work
// entry omits workTypeName and honors an explicit workTypeName when provided.
func TestWorkBatchSelectsDefaultAndExplicitWorkTypes(t *testing.T) {
	factoryDir := support.ScaffoldFactory(t, batchWorkTypeSelectionFactoryConfig())
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:     factoryDir,
		UseMockWorkers: true,
	})
	defer server.Stop(t)

	baseURL := server.URL()
	processHarness := newRootProcessHarness(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	t.Run("default", func(t *testing.T) {
		batchJSON := fmt.Sprintf(
			`{"requestId":%q,"type":"FACTORY_REQUEST_BATCH","works":[{"name":%q,"payload":{"title":"Default work type selection"}}]}`,
			batchInputsDefaultTypeRequestID,
			batchInputsDefaultTypeWorkName,
		)
		output, err := runYouSubmitBatch(ctx, processHarness, factoryDir, baseURL, batchJSON, nil)
		if err != nil {
			t.Fatalf("you submit batch default work type: %v\noutput:\n%s", err, output)
		}
		submitted := decodeBatchSubmitJSON(t, output)
		assertBatchWorkListedWithWorkType(
			t,
			baseURL,
			batchInputsDefaultTypeWorkName,
			submitted.Works[0].WorkID,
			batchInputsWorkType,
		)
	})

	t.Run("explicit", func(t *testing.T) {
		batchJSON := fmt.Sprintf(
			`{"requestId":%q,"type":"FACTORY_REQUEST_BATCH","works":[{"name":%q,"workTypeName":%q,"payload":{"title":"Explicit work type selection"}}]}`,
			batchInputsExplicitTypeRequestID,
			batchInputsExplicitTypeWorkName,
			batchInputsAltWorkType,
		)
		output, err := runYouSubmitBatch(ctx, processHarness, factoryDir, baseURL, batchJSON, nil)
		if err != nil {
			t.Fatalf("you submit batch explicit work type: %v\noutput:\n%s", err, output)
		}
		submitted := decodeBatchSubmitJSON(t, output)
		assertBatchWorkListedWithWorkType(
			t,
			baseURL,
			batchInputsExplicitTypeWorkName,
			submitted.Works[0].WorkID,
			batchInputsAltWorkType,
		)
	})
}

// TestWorkBatchRejectsUnknownTypeWithoutPartialMutation proves public Work Request
// batch ingress rejects batches that name an unknown work type with a
// customer-visible failure and leaves durable Work unchanged for that request.
func TestWorkBatchRejectsUnknownTypeWithoutPartialMutation(t *testing.T) {
	factoryDir := support.ScaffoldFactory(t, batchWorkTypeSelectionFactoryConfig())
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:     factoryDir,
		UseMockWorkers: true,
	})
	defer server.Stop(t)

	baseURL := server.URL()
	processHarness := newRootProcessHarness(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	baselineListed := support.ListDefaultSessionWork(t, baseURL)
	baselineCount := len(baselineListed.Results)

	t.Run("unknown_type", func(t *testing.T) {
		batchJSON := fmt.Sprintf(
			`{"requestId":%q,"type":"FACTORY_REQUEST_BATCH","works":[{"name":%q,"workTypeName":%q,"payload":{"title":"Unknown work type rejection"}}]}`,
			batchInputsUnknownTypeRequestID,
			batchInputsUnknownTypeWorkName,
			batchInputsUnknownWorkType,
		)
		output, err := runYouSubmitBatch(ctx, processHarness, factoryDir, baseURL, batchJSON, nil)
		assertBatchSubmitRejected(t, output, err, batchInputsUnknownTypeRequestID)
		assertWorkNotListedByName(t, baseURL, batchInputsUnknownTypeWorkName)
		assertListedWorkCount(t, baseURL, baselineCount)
	})

	t.Run("mixed_batch_no_partial_submit", func(t *testing.T) {
		batchJSON := fmt.Sprintf(
			`{"requestId":%q,"type":"FACTORY_REQUEST_BATCH","works":[{"name":%q,"workTypeName":%q,"payload":{"title":"Valid work in mixed batch"}},{"name":%q,"workTypeName":%q,"payload":{"title":"Invalid work in mixed batch"}}]}`,
			batchInputsMixedBatchRequestID,
			batchInputsMixedValidWorkName,
			batchInputsWorkType,
			batchInputsMixedInvalidWorkName,
			batchInputsUnknownWorkType,
		)
		output, err := runYouSubmitBatch(ctx, processHarness, factoryDir, baseURL, batchJSON, nil)
		assertBatchSubmitRejected(t, output, err, batchInputsMixedBatchRequestID)
		assertWorkNotListedByName(t, baseURL, batchInputsMixedValidWorkName)
		assertWorkNotListedByName(t, baseURL, batchInputsMixedInvalidWorkName)
		assertListedWorkCount(t, baseURL, baselineCount)
	})
}

// TestBlockedDispatchConcurrentBatchIngressRegression proves accepted batch ingress
// stays HTTP-observable (WORK_REQUEST plus Work list/get) while an unrelated
// dispatch remains blocked, and same-request-ID replay stays idempotent.
func TestBlockedDispatchConcurrentBatchIngressRegression(t *testing.T) {
	factoryDir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "simple_pipeline"))
	dispatchRelease := make(chan struct{})
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		WaitForServiceModeRuntime: true,
		ProviderOverride:          support.BlockingInferenceProvider(dispatchRelease),
	})
	defer server.Stop(t)

	baseURL := server.URL()
	body, err := json.Marshal(factoryapi.SubmitWorkRequest{
		Name:         stringPtr("blocked-dispatch-for-ingress-regression"),
		WorkTypeName: batchInputsWorkType,
		Payload:      map[string]string{"title": "blocked dispatch for batch ingress regression"},
	})
	if err != nil {
		t.Fatalf("marshal POST /work request: %v", err)
	}
	submitted := postSubmitWork(t, baseURL, body)
	if submitted.TraceId == "" {
		t.Fatal("POST /work returned an empty trace ID")
	}
	waitForServiceModeSessionActive(t, baseURL, 10*time.Second)

	workTypeName := batchInputsWorkType
	batchRequest := factoryapi.WorkRequest{
		RequestId: batchIngressRegressionRequestID,
		Type:      factoryapi.WorkRequestTypeFactoryRequestBatch,
		Works: &[]factoryapi.Work{{
			Name:         "http-ingress-regression",
			WorkId:       stringPtr(batchIngressRegressionWorkID),
			WorkTypeName: &workTypeName,
			TraceId:      stringPtr(batchIngressRegressionTraceID),
			Payload:      map[string]string{"title": "concurrent batch ingress regression"},
		}},
	}

	first := support.UpsertDefaultSessionWorkRequest(t, baseURL, batchRequest)
	if first.RequestId != batchIngressRegressionRequestID {
		t.Fatalf("PUT /work-requests request_id = %q, want %q", first.RequestId, batchIngressRegressionRequestID)
	}
	if len(first.Works) != 1 || first.Works[0].WorkId != batchIngressRegressionWorkID {
		t.Fatalf(
			"PUT /work-requests works = %#v, want one work with id %q",
			first.Works,
			batchIngressRegressionWorkID,
		)
	}

	support.AssertSingleWorkRequestEvent(
		t,
		server.GetFactoryEvents(t),
		batchIngressRegressionRequestID,
		batchIngressRegressionWorkID,
		workTypeName,
	)
	assertBatchIngressWorkListAndGetVisible(t, baseURL, batchIngressRegressionWorkID)

	replayed := support.UpsertDefaultSessionWorkRequest(t, baseURL, batchRequest)
	if replayed.RequestId != first.RequestId || replayed.TraceId != first.TraceId {
		t.Fatalf("idempotent PUT identity changed: first=%#v replay=%#v", first, replayed)
	}
	support.AssertSingleWorkRequestEvent(
		t,
		server.GetFactoryEvents(t),
		batchIngressRegressionRequestID,
		batchIngressRegressionWorkID,
		workTypeName,
	)

	select {
	case <-dispatchRelease:
		t.Fatal("blocked dispatch released before ingress regression assertions finished")
	default:
	}

	close(dispatchRelease)
}

func waitForServiceModeSessionActive(t *testing.T, baseURL string, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		session := support.GetDefaultSession(t, baseURL)
		if session.Runtime.Progress.FactoryState == "RUNNING" &&
			session.Runtime.Progress.InFlightCount > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	session := support.GetDefaultSession(t, baseURL)
	t.Fatalf("timed out waiting for active service-mode session: %#v", session.Runtime)
}

func assertBatchIngressWorkListAndGetVisible(t *testing.T, baseURL, workID string) {
	t.Helper()

	listed := support.ListDefaultSessionWork(t, baseURL)
	if !batchIngressWorkListingContainsID(listed, workID) {
		t.Fatalf(
			"work %q missing from public Work list before blocked dispatch completed; listed=%#v",
			workID,
			listed.Results,
		)
	}

	endpoint := support.DefaultSessionWorkURL(baseURL, "/work/"+workID)
	response, err := http.Get(endpoint)
	if err != nil {
		t.Fatalf("GET %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET %s status = %d, want 200 while blocked dispatch continues", endpoint, response.StatusCode)
	}

	work := support.GetJSON[factoryapi.Work](t, endpoint)
	if support.StringPointerValue(work.WorkId) != workID {
		t.Fatalf("GET /work/%s workId = %q, want %q", workID, support.StringPointerValue(work.WorkId), workID)
	}
}

func batchIngressWorkListingContainsID(listed factoryapi.ListWorkResponse, workID string) bool {
	for _, item := range listed.Results {
		if support.StringPointerValue(item.WorkId) == workID {
			return true
		}
	}
	return false
}

// TestWorkBatchDependencyOrderingNormalizesRuntimeWork proves batch upsert with
// DEPENDS_ON relations dispatches dependent work only after prerequisite terminal
// outcomes and preserves canonical work type names in public projections.
func TestWorkBatchDependencyOrderingNormalizesRuntimeWork(t *testing.T) {
	factoryDir := support.ScaffoldFactory(t, competingPipelineFactoryConfig())
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
	})
	defer server.Stop(t)

	stream := support.OpenFactoryEventStreamAt(
		t,
		support.DefaultSessionEventsURL(server.URL()),
	)
	_ = stream.NextEvent(5 * time.Second) // RUN_REQUEST
	_ = stream.NextEvent(5 * time.Second) // INITIAL_STRUCTURE_REQUEST

	const (
		firstWorkID  = "work-batch-dependency-first"
		secondWorkID = "work-batch-dependency-second"
	)
	requiredState := "complete"
	workTypeName := batchInputsWorkType
	request := factoryapi.WorkRequest{
		RequestId: "request-batch-dependency",
		Type:      factoryapi.WorkRequestTypeFactoryRequestBatch,
		Works: &[]factoryapi.Work{
			{
				Name:         "first",
				WorkId:       stringPtr(firstWorkID),
				WorkTypeName: &workTypeName,
				Payload:      map[string]string{"step": "first"},
			},
			{
				Name:         "second",
				WorkId:       stringPtr(secondWorkID),
				WorkTypeName: &workTypeName,
				Payload:      map[string]string{"step": "second"},
			},
		},
		Relations: &[]factoryapi.Relation{{
			Type:           factoryapi.RelationTypeDependsOn,
			SourceWorkName: "second",
			TargetWorkName: "first",
			RequiredState:  &requiredState,
		}},
	}

	response := support.UpsertDefaultSessionWorkRequest(t, server.URL(), request)
	if response.RequestId != request.RequestId || response.TraceId == "" || len(response.Works) != 2 {
		t.Fatalf("PUT /work-requests response = %#v, want request id, trace id, and two works", response)
	}
	wantWorks := []factoryapi.UpsertWorkRequestSubmittedWork{
		{Name: "first", WorkTypeName: workTypeName, WorkId: firstWorkID},
		{Name: "second", WorkTypeName: workTypeName, WorkId: secondWorkID},
	}
	if !reflect.DeepEqual(response.Works, wantWorks) {
		t.Fatalf("PUT /work-requests works = %#v, want %#v", response.Works, wantWorks)
	}
	if replayed := support.UpsertDefaultSessionWorkRequest(t, server.URL(), request); !reflect.DeepEqual(replayed, response) {
		t.Fatalf("replayed PUT /work-requests response = %#v, want original %#v", replayed, response)
	}

	items := waitForWorkIDsComplete(t, server.URL(), []string{firstWorkID, secondWorkID}, 10*time.Second)
	for _, item := range items {
		if support.StringPointerValue(item.WorkTypeName) != workTypeName {
			t.Fatalf(
				"batch work %s workTypeName = %q, want %q",
				support.StringPointerValue(item.WorkId),
				support.StringPointerValue(item.WorkTypeName),
				workTypeName,
			)
		}
	}
	assertPublicBatchDurableOutcomes(t, server.URL(), firstWorkID, secondWorkID)
	assertPublicBatchDependencyAndIdempotency(t, stream, request.RequestId, firstWorkID, secondWorkID)
}

func assertPublicBatchDurableOutcomes(t *testing.T, baseURL, firstWorkID, secondWorkID string) {
	t.Helper()
	listed := support.ListDefaultSessionWork(t, baseURL)
	counts := map[string]int{}
	for _, workItem := range listed.Results {
		workID := support.StringPointerValue(workItem.WorkId)
		if workID == firstWorkID || workID == secondWorkID {
			counts[workID]++
		}
	}
	if counts[firstWorkID] != 1 || counts[secondWorkID] != 1 {
		t.Fatalf("public durable batch work counts = %#v, want one outcome for each batch work", counts)
	}
}

func assertPublicBatchDependencyAndIdempotency(
	t *testing.T,
	stream *support.FactoryEventStream,
	requestID, firstWorkID, secondWorkID string,
) {
	t.Helper()

	requestEvents, relationEvents := 0, 0
	firstTerminalSequence, secondDispatchSequence := -1, -1
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		event := stream.NextEvent(time.Until(deadline))
		switch event.Type {
		case factoryapi.FactoryEventTypeWorkRequest:
			if support.StringPointerValue(event.Context.RequestId) == requestID {
				requestEvents++
				payload, err := event.Payload.AsWorkRequestEventPayload()
				if err != nil {
					t.Fatalf("decode public WORK_REQUEST event: %v", err)
				}
				if payload.Type != factoryapi.WorkRequestTypeFactoryRequestBatch ||
					len(support.FactoryWorksValue(payload.Works)) != 2 {
					t.Fatalf("public WORK_REQUEST payload = %#v, want two-work FACTORY_REQUEST_BATCH", payload)
				}
			}
		case factoryapi.FactoryEventTypeRelationshipChangeRequest:
			if support.StringPointerValue(event.Context.RequestId) == requestID {
				relationEvents++
				payload, err := event.Payload.AsRelationshipChangeRequestEventPayload()
				if err != nil {
					t.Fatalf("decode public RELATIONSHIP_CHANGE_REQUEST event: %v", err)
				}
				if payload.Relation.Type != factoryapi.RelationTypeDependsOn ||
					payload.Relation.SourceWorkName != "second" ||
					support.StringPointerValue(payload.Relation.TargetWorkId) != firstWorkID {
					t.Fatalf("public dependency relation = %#v, want second DEPENDS_ON first", payload.Relation)
				}
			}
		case factoryapi.FactoryEventTypeDispatchResponse:
			if payload, err := event.Payload.AsDispatchResponseEventPayload(); err == nil &&
				payload.Outcome == factoryapi.WorkOutcomeAccepted &&
				publicEventWorkIDsContain(event.Context.WorkIds, firstWorkID) {
				firstTerminalSequence = event.Context.Sequence
			}
		case factoryapi.FactoryEventTypeDispatchRequest:
			if payload, err := event.Payload.AsDispatchRequestEventPayload(); err == nil &&
				publicDispatchInputsContainWork(payload, secondWorkID) {
				secondDispatchSequence = event.Context.Sequence
			}
		}
		if requestEvents == 1 && relationEvents == 1 &&
			firstTerminalSequence >= 0 && secondDispatchSequence > firstTerminalSequence {
			return
		}
	}
	t.Fatalf(
		"public batch events = requests:%d relations:%d first-terminal:%d second-dispatch:%d; want one request, one relation, and dependency ordering",
		requestEvents,
		relationEvents,
		firstTerminalSequence,
		secondDispatchSequence,
	)
}

func publicDispatchInputsContainWork(payload factoryapi.DispatchRequestEventPayload, workID string) bool {
	for _, input := range payload.Inputs {
		if input.WorkId == workID {
			return true
		}
	}
	return false
}

func publicEventWorkIDsContain(workIDs *[]string, want string) bool {
	if workIDs == nil {
		return false
	}
	for _, workID := range *workIDs {
		if workID == want {
			return true
		}
	}
	return false
}

type batchInputsSubmitJSON struct {
	RequestID string `json:"requestId"`
	TraceID   string `json:"traceId"`
	WorkCount int    `json:"workCount"`
	Works     []struct {
		Name   string `json:"name"`
		WorkID string `json:"workId"`
	} `json:"works"`
}

func batchWorkTypeStates() []map[string]any {
	return []map[string]any{
		{"name": "init", "type": "INITIAL"},
		{"name": "complete", "type": "TERMINAL"},
		{"name": "failed", "type": "FAILED"},
	}
}

func batchWorkTypeWorkstation(name, workType string) map[string]any {
	return map[string]any{
		"name":      name,
		"worker":    "mock-worker",
		"inputs":    []map[string]string{{"workType": workType, "state": "init"}},
		"outputs":   []map[string]string{{"workType": workType, "state": "complete"}},
		"onFailure": []map[string]string{{"workType": workType, "state": "failed"}},
	}
}

func batchInputsFactoryConfig() map[string]any {
	return map[string]any{
		"name": "work-batch-inputs",
		"workTypes": []map[string]any{
			{
				"name":   batchInputsWorkType,
				"states": batchWorkTypeStates(),
			},
		},
		"workers": []map[string]string{
			{"name": "mock-worker"},
		},
		"workstations": []map[string]any{
			batchWorkTypeWorkstation("process-task", batchInputsWorkType),
		},
	}
}

func batchWorkTypeSelectionFactoryConfig() map[string]any {
	return map[string]any{
		"name": "work-batch-work-type-selection",
		"workTypes": []map[string]any{
			{
				"name":             batchInputsWorkType,
				"handlingBehavior": []string{"DEFAULT"},
				"states":           batchWorkTypeStates(),
			},
			{
				"name":   batchInputsAltWorkType,
				"states": batchWorkTypeStates(),
			},
		},
		"workers": []map[string]string{
			{"name": "mock-worker"},
		},
		"workstations": []map[string]any{
			batchWorkTypeWorkstation("process-task", batchInputsWorkType),
			batchWorkTypeWorkstation("process-review", batchInputsAltWorkType),
		},
	}
}

func newRootProcessHarness(t *testing.T) *builtcliacceptance.Harness {
	t.Helper()
	return builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
}

func runYouSubmitBatch(
	ctx context.Context,
	processHarness *builtcliacceptance.Harness,
	workingDir string,
	serverURL string,
	batchSource string,
	stdin io.Reader,
) ([]byte, error) {
	args := []string{"--server", serverURL, "--json", "submit", "batch", batchSource}
	cmd := processHarness.CommandContext(ctx, args...)
	cmd.Dir = workingDir
	if stdin != nil {
		cmd.Stdin = stdin
	}
	return cmd.CombinedOutput()
}

func decodeBatchSubmitJSON(t *testing.T, output []byte) batchInputsSubmitJSON {
	t.Helper()

	var submitted batchInputsSubmitJSON
	if err := json.Unmarshal(bytesTrimSpace(output), &submitted); err != nil {
		t.Fatalf("decode submit batch JSON: %v\noutput:\n%s", err, output)
	}
	if submitted.WorkCount != 1 || len(submitted.Works) != 1 || strings.TrimSpace(submitted.Works[0].WorkID) == "" {
		t.Fatalf("submit batch response missing accepted work identity: %#v", submitted)
	}
	if strings.TrimSpace(submitted.RequestID) == "" || strings.TrimSpace(submitted.TraceID) == "" {
		t.Fatalf("submit batch response missing request or trace identity: %#v", submitted)
	}
	return submitted
}

func assertBatchSubmitAcknowledgment(t *testing.T, output []byte, requestID, workName string) {
	t.Helper()

	text := string(output)
	for _, marker := range []string{
		`"requestId":` + jsonStringLiteral(requestID),
		`"traceId":`,
		`"workCount":1`,
		workName,
		batchInputsWorkType,
	} {
		if !strings.Contains(text, marker) {
			t.Fatalf("submit batch output missing %q:\n%s", marker, text)
		}
	}
}

func assertBatchWorkListedAfterSubmit(t *testing.T, baseURL, workName, workID string) {
	t.Helper()
	assertBatchWorkListedWithWorkType(t, baseURL, workName, workID, "")
}

func assertBatchWorkListedWithWorkType(t *testing.T, baseURL, workName, workID, workType string) {
	t.Helper()

	listed := support.ListDefaultSessionWork(t, baseURL)
	item, ok := findListedWorkByNameAndID(listed, workName, workID)
	if !ok {
		t.Fatalf(
			"public work list missing submitted work name=%q workId=%q: %#v",
			workName,
			workID,
			listed.Results,
		)
	}
	if workType == "" {
		return
	}
	if support.StringPointerValue(item.WorkTypeName) != workType {
		t.Fatalf(
			"public work list workTypeName = %q, want %q for name=%q workId=%q: %#v",
			support.StringPointerValue(item.WorkTypeName),
			workType,
			workName,
			workID,
			item,
		)
	}
}

func findListedWorkByNameAndID(listed factoryapi.ListWorkResponse, workName, workID string) (factoryapi.Work, bool) {
	for _, item := range listed.Results {
		if item.Name != workName {
			continue
		}
		if support.StringPointerValue(item.WorkId) == workID {
			return item, true
		}
	}
	return factoryapi.Work{}, false
}

func assertBatchSubmitRejected(t *testing.T, output []byte, err error, requestID string) {
	t.Helper()

	if err == nil {
		t.Fatalf("you submit batch unexpectedly succeeded:\n%s", output)
	}

	text := string(output)
	for _, marker := range []string{
		"batch submission failed (400)",
		"code=BAD_REQUEST",
		"family=BAD_REQUEST",
	} {
		if !strings.Contains(text, marker) {
			t.Fatalf("submit batch rejection output missing %q:\n%s", marker, text)
		}
	}

	for _, marker := range []string{
		`"requestId":` + jsonStringLiteral(requestID),
		"requestId: " + requestID,
		`"traceId":`,
		"traceId:",
		`"workCount":`,
		"work count:",
	} {
		if strings.Contains(text, marker) {
			t.Fatalf("submit batch rejection output must not contain success acknowledgment marker %q:\n%s", marker, text)
		}
	}
}

func assertWorkNotListedByName(t *testing.T, baseURL, workName string) {
	t.Helper()

	listed := support.ListDefaultSessionWork(t, baseURL)
	for _, item := range listed.Results {
		if item.Name == workName {
			t.Fatalf(
				"public work list unexpectedly contains rejected work name=%q: %#v",
				workName,
				item,
			)
		}
	}
}

func assertListedWorkCount(t *testing.T, baseURL string, want int) {
	t.Helper()

	listed := support.ListDefaultSessionWork(t, baseURL)
	if got := len(listed.Results); got != want {
		t.Fatalf("public work list count = %d, want %d after rejected batch: %#v", got, want, listed.Results)
	}
}

func bytesTrimSpace(raw []byte) []byte {
	return []byte(strings.TrimSpace(string(raw)))
}

func jsonStringLiteral(value string) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%q", value)
	}
	return string(encoded)
}
