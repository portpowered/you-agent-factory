package runtime_api

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	submitcli "github.com/portpowered/infinite-you/pkg/cli/submit"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestGeneratedAPIIntegrationSmoke_OpenAPIGeneratedServerAndLiveRuntimeStayAligned(t *testing.T) {
	support.SkipLongFunctional(t, "slow generated API and live runtime alignment smoke")

	dir := support.ScaffoldFactory(t, simplePipelineConfig())
	server := startFunctionalServer(t, dir, true, factory.WithServiceMode())

	traceID := submitGeneratedWork(t, server.URL(), factoryapi.SubmitWorkRequest{
		Name:         "generated-api-integration-smoke",
		WorkTypeName: "task",
		Payload:      map[string]string{"title": "generated API integration smoke"},
	})
	if traceID == "" {
		t.Fatal("POST /work returned an empty trace_id")
	}

	work := waitForGeneratedWorkComplete(t, server.URL(), traceID, 10*time.Second)
	if len(work.Results) != 1 {
		t.Fatalf("GET /work result count = %d, want 1", len(work.Results))
	}
	item := work.Results[0]
	if stringPointerValue(item.TraceId) != traceID {
		t.Fatalf("GET /work trace_id = %q, want %q", stringPointerValue(item.TraceId), traceID)
	}
	if stringPointerValue(item.WorkTypeName) != "task" {
		t.Fatalf("GET /work work type = %q, want task", stringPointerValue(item.WorkTypeName))
	}
	if item.Name != "generated-api-integration-smoke" {
		t.Fatalf("GET /work name = %q, want generated-api-integration-smoke", item.Name)
	}
	if generatedWorkStateName(item.State) != "complete" || generatedWorkStateType(item.State) != factoryapi.WorkStateTypeTERMINAL {
		t.Fatalf("GET /work state = %#v, want complete/TERMINAL", item.State)
	}

	statusRead := getGeneratedJSON[factoryapi.StatusResponse](t, server.URL()+"/status")
	if statusRead.TotalTokens != 1 {
		t.Fatalf("GET /status total_tokens = %d, want 1", statusRead.TotalTokens)
	}
	if statusRead.Categories.Terminal != 1 {
		t.Fatalf("GET /status terminal count = %d, want 1", statusRead.Categories.Terminal)
	}

	assertGeneratedEventsStreamHasCanonicalHistory(t, server.URL())
}

func TestGeneratedAPIIntegrationSmoke_SubmitWorkItemsAcceptHeaderOnlyStructuredSubmission(t *testing.T) {
	dir := support.ScaffoldFactory(t, simplePipelineConfig())
	server := startFunctionalServer(t, dir, true, factory.WithServiceMode())

	req := map[string]any{
		"name":         "generated-api-header-only",
		"workTypeName": "task",
		"items":        []map[string]any{},
	}
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal generated submit request: %v", err)
	}
	resp, err := http.Post(server.URL()+"/work", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /work: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST /work status = %d, want 201: %s", resp.StatusCode, string(payload))
	}
	var submitted factoryapi.SubmitWorkResponse
	if err := json.NewDecoder(resp.Body).Decode(&submitted); err != nil {
		t.Fatalf("decode submit response: %v", err)
	}
	if submitted.TraceId == "" {
		t.Fatalf("submit response trace_id is empty, want non-empty trace identifier")
	}

	workList := waitForGeneratedWorkComplete(t, server.URL(), submitted.TraceId, 10*time.Second)
	if len(workList.Results) != 1 {
		t.Fatalf("GET /work result count = %d, want 1", len(workList.Results))
	}
	work := workList.Results[0]
	if work.Name != "generated-api-header-only" || stringPointerValue(work.WorkTypeName) != "task" {
		t.Fatalf("GET /work = %#v, want header-only name and work type", work)
	}
	if work.Content != nil && len(*work.Content) != 0 {
		t.Fatalf("GET /work content = %#v, want empty structured content", work.Content)
	}
}

func TestGeneratedAPIIntegrationSmoke_SubmitWorkItemsRejectEmptyStructuredSubmission(t *testing.T) {
	dir := support.ScaffoldFactory(t, simplePipelineConfig())
	server := startFunctionalServer(t, dir, true, factory.WithServiceMode())

	req := map[string]any{
		"name":         "generated-api-empty-items",
		"workTypeName": "task",
		"items": []map[string]any{
			{"type": "text", "text": "   "},
		},
	}
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal generated submit request: %v", err)
	}
	resp, err := http.Post(server.URL()+"/work", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /work: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST /work status = %d, want 400: %s", resp.StatusCode, string(payload))
	}
}

func TestGeneratedAPIIntegrationSmoke_SubmitWorkItemsAcceptOrderedTextSubmission(t *testing.T) {
	dir := support.ScaffoldFactory(t, simplePipelineConfig())
	server := startFunctionalServer(t, dir, true, factory.WithServiceMode())

	req := map[string]any{
		"name":         "generated-api-items-text",
		"workTypeName": "task",
		"items": []map[string]any{
			{"type": "text", "text": "Alpha "},
			{"type": "text", "text": "Beta"},
		},
	}
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal generated submit request: %v", err)
	}
	resp, err := http.Post(server.URL()+"/work", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /work: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST /work status = %d, want 201: %s", resp.StatusCode, string(payload))
	}
	var submitted factoryapi.SubmitWorkResponse
	if err := json.NewDecoder(resp.Body).Decode(&submitted); err != nil {
		t.Fatalf("decode submit response: %v", err)
	}

	work := waitForGeneratedWorkComplete(t, server.URL(), submitted.TraceId, 10*time.Second)
	if len(work.Results) != 1 {
		t.Fatalf("GET /work result count = %d, want 1", len(work.Results))
	}
	content := work.Results[0].Content
	if content == nil || len(*content) != 2 {
		t.Fatalf("GET /work content = %#v, want two ordered text content parts", content)
	}
	firstPart, err := (*content)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("decode first projected text content: %v", err)
	}
	secondPart, err := (*content)[1].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("decode second projected text content: %v", err)
	}
	if firstPart.Text != "Alpha " || secondPart.Text != "Beta" {
		t.Fatalf("GET /work content parts = %#v, want ordered text items Alpha / Beta", content)
	}
}

func TestGeneratedAPIIntegrationSmoke_SubmitWorkItemsAcceptMixedTextAndImageSubmissionOnSupportedRunner(t *testing.T) {
	dir := support.ScaffoldFactory(t, simplePipelineConfig())
	support.WriteAgentConfig(t, dir, "worker-a", support.BuildModelWorkerConfig(interfaces.ModelProviderCodex, "gpt-5-codex"))

	server := startFunctionalServerWithConfig(
		t,
		dir,
		false,
		func(cfg *service.FactoryServiceConfig) {
			cfg.ProviderCommandRunnerOverride = support.NewStaticSuccessCommandRunner("Done. COMPLETE")
		},
		factory.WithServiceMode(),
	)
	stagedImageRef := stageGeneratedSubmitWorkFile(t, server.URL(), "image", "review.png", "image/png", []byte("png-bytes"))

	traceID := submitGeneratedWork(t, server.URL(), factoryapi.SubmitWorkRequest{
		Name:         "generated-api-items-mixed",
		WorkTypeName: "task",
		Items: &[]factoryapi.SubmitWorkItem{
			mustSubmitWorkTextItem(t, "Review this screenshot."),
			mustSubmitWorkImageItem(t, stagedImageRef, "review.png", "image/png"),
		},
	})

	work := waitForGeneratedWorkComplete(t, server.URL(), traceID, 10*time.Second)
	item := requireGeneratedWorkByTrace(t, work, traceID)
	content := item.Content
	if content == nil || len(*content) != 2 {
		t.Fatalf("GET /work content = %#v, want ordered text and image content parts", content)
	}
	textPart, err := (*content)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("decode projected text content: %v", err)
	}
	imagePart, err := (*content)[1].AsWorkImageContentPart()
	if err != nil {
		t.Fatalf("decode projected image content: %v", err)
	}
	if textPart.Text != "Review this screenshot." {
		t.Fatalf("projected text part = %#v, want authored text", textPart)
	}
	if stringPointerValue(imagePart.ContentType) != "image/png" {
		t.Fatalf("projected image part = %#v, want staged image reference and media type", imagePart)
	}
	imageContent, err := os.ReadFile(imagePart.File)
	if err != nil {
		t.Fatalf("read staged image content: %v", err)
	}
	if string(imageContent) != "png-bytes" {
		t.Fatalf("staged image content = %q, want png-bytes", imageContent)
	}
}

func TestGeneratedAPIIntegrationSmoke_SubmitWorkItemsRejectMixedTextAndImageSubmissionOnUnsupportedRunner(t *testing.T) {
	dir := support.ScaffoldFactory(t, simplePipelineConfig())
	support.WriteAgentConfig(t, dir, "worker-a", support.BuildModelWorkerConfig(interfaces.ModelProviderGemini, "gemini-1.5-pro"))
	runner := support.NewRecordingCommandRunner("unused")

	server := startFunctionalServerWithConfig(
		t,
		dir,
		false,
		func(cfg *service.FactoryServiceConfig) {
			cfg.ProviderCommandRunnerOverride = runner
		},
		factory.WithServiceMode(),
	)
	stagedImageRef := stageGeneratedSubmitWorkFile(t, server.URL(), "image", "review.png", "image/png", []byte("png-bytes"))

	traceID := submitGeneratedWork(t, server.URL(), factoryapi.SubmitWorkRequest{
		Name:         "generated-api-items-unsupported-mixed",
		WorkTypeName: "task",
		Items: &[]factoryapi.SubmitWorkItem{
			mustSubmitWorkTextItem(t, "Review this screenshot."),
			mustSubmitWorkImageItem(t, stagedImageRef, "review.png", "image/png"),
		},
	})

	work := waitForGeneratedWorkAtPlace(t, server.URL(), traceID, "task:failed", 10*time.Second)
	item := requireGeneratedWorkByTrace(t, work, traceID)
	if generatedWorkStateName(item.State) != "failed" {
		t.Fatalf("GET /work state = %#v, want failed work state", item.State)
	}
	if runner.CallCount() != 0 {
		t.Fatalf("provider command runner calls = %d, want 0 because capability rejection should happen before subprocess launch", runner.CallCount())
	}

	workID := stringPointerValue(item.WorkId)
	snapshot := server.GetEngineStateSnapshot(t)
	for _, token := range snapshot.Marking.TokensInPlace("task:failed") {
		if token.Color.WorkID != workID {
			continue
		}
		if token.History.LastError == "" {
			t.Fatalf("failed token history = %#v, want last error evidence", token.History)
		}
		const want = "image input is not supported by the gemini runner in v1"
		if !strings.Contains(token.History.LastError, want) {
			t.Fatalf("failed token last error = %q, want substring %q", token.History.LastError, want)
		}
		return
	}

	t.Fatalf("failed token for work %q not found in task:failed", workID)
}

func TestGeneratedAPIIntegrationSmoke_SubmitWorkItemsRejectForgedStructuredFileReference(t *testing.T) {
	dir := support.ScaffoldFactory(t, simplePipelineConfig())
	server := startFunctionalServer(t, dir, true, factory.WithServiceMode())

	req := factoryapi.SubmitWorkRequest{
		Name:         "generated-api-forged-staged-ref",
		WorkTypeName: "task",
		Items: &[]factoryapi.SubmitWorkItem{
			mustSubmitWorkImageItem(t, "staged://forged-review.png", "review.png", "image/png"),
		},
	}
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal forged staged-ref request: %v", err)
	}
	resp, err := http.Post(server.URL()+"/work", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /work with forged staged ref: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST /work status = %d, want 400: %s", resp.StatusCode, string(payload))
	}
}

func TestGeneratedAPIIntegrationSmoke_CLIWorkTypeNameReachesLiveAPIHandler(t *testing.T) {
	support.SkipLongFunctional(t, "slow CLI submit generated API smoke")

	dir := support.ScaffoldFactory(t, simplePipelineConfig())
	server := startFunctionalServer(t, dir, true, factory.WithServiceMode())

	payloadPath := filepath.Join(t.TempDir(), "request.md")
	if err := os.WriteFile(payloadPath, []byte("ship name based CLI submit"), 0o644); err != nil {
		t.Fatalf("write CLI submit payload: %v", err)
	}

	if err := submitcli.Submit(submitcli.SubmitConfig{
		Name:         "  cli-live-api-name  ",
		WorkTypeName: "task",
		Payload:      payloadPath,
		Server:       functionalServerBase(t, server.URL()),
	}); err != nil {
		t.Fatalf("agent-factory submit --work-type-name: %v", err)
	}

	item := waitForGeneratedWorkTypeComplete(t, server.URL(), "task", 10*time.Second)
	if item.Name != "cli-live-api-name" {
		t.Fatalf("CLI-submitted work name = %q, want cli-live-api-name", item.Name)
	}
	if stringPointerValue(item.WorkTypeName) != "task" || generatedWorkStateName(item.State) != "complete" {
		t.Fatalf("CLI-submitted work = %#v, want task in complete state", item)
	}
}

func TestGeneratedAPIIntegrationSmoke_BatchWorkTypeNameNormalizesRuntimeWork(t *testing.T) {
	support.SkipLongFunctional(t, "slow batch generated API normalization smoke")

	dir := support.ScaffoldFactory(t, simplePipelineConfig())
	server := startFunctionalServer(t, dir, true, factory.WithServiceMode())

	firstWorkID := "work-generated-api-batch-first"
	secondWorkID := "work-generated-api-batch-second"
	requiredState := "complete"
	workTypeName := "task"
	request := factoryapi.WorkRequest{
		RequestId: "request-generated-api-batch",
		Type:      factoryapi.WorkRequestTypeFactoryRequestBatch,
		Works: &[]factoryapi.Work{
			{Name: "first", WorkId: &firstWorkID, WorkTypeName: &workTypeName, Payload: map[string]string{"step": "first"}},
			{Name: "second", WorkId: &secondWorkID, WorkTypeName: &workTypeName, Payload: map[string]string{"step": "second"}},
		},
		Relations: &[]factoryapi.Relation{{Type: factoryapi.RelationTypeDependsOn, SourceWorkName: "second", TargetWorkName: "first", RequiredState: &requiredState}},
	}

	resp := putGeneratedWorkRequest(t, server.URL(), request.RequestId, request)
	if resp.RequestId != request.RequestId || resp.TraceId == "" {
		t.Fatalf("PUT /work-requests response = %#v, want request id and trace id", resp)
	}

	items := waitForGeneratedWorkIDsComplete(t, server.URL(), []string{firstWorkID, secondWorkID}, 10*time.Second)
	for _, item := range items {
		if stringPointerValue(item.WorkTypeName) != "task" {
			t.Fatalf("generated batch work %s work type = %q, want task", stringPointerValue(item.WorkId), stringPointerValue(item.WorkTypeName))
		}
	}

	snapshot := server.GetEngineStateSnapshot(t)
	var firstSeen bool
	var secondSeen bool
	for _, token := range snapshot.Marking.Tokens {
		if token == nil {
			continue
		}
		switch token.Color.WorkID {
		case firstWorkID:
			firstSeen = true
		case secondWorkID:
			secondSeen = true
			if len(token.Color.Relations) != 1 {
				t.Fatalf("second runtime relations = %d, want 1", len(token.Color.Relations))
			}
			relation := token.Color.Relations[0]
			if relation.TargetWorkID != firstWorkID || relation.RequiredState != "complete" {
				t.Fatalf("second runtime relation = %#v, want dependency on first completion", relation)
			}
		}
	}
	if !firstSeen || !secondSeen {
		t.Fatalf("runtime snapshot missing generated API batch tokens: first=%v second=%v", firstSeen, secondSeen)
	}
}

func assertGeneratedEventsStreamHasCanonicalHistory(t *testing.T, baseURL string) {
	t.Helper()
	stream := openFactoryEventHTTPStream(t, baseURL+"/events")
	runRequest, initialStructure := requireFunctionalEventStreamPrelude(t, stream)
	assertFunctionalEventsUseCanonicalVocabulary(t, []factoryapi.FactoryEvent{runRequest, initialStructure},
		factoryapi.FactoryEventTypeRunRequest,
		factoryapi.FactoryEventTypeInitialStructureRequest,
	)
}

func submitGeneratedWork(t *testing.T, baseURL string, req factoryapi.SubmitWorkRequest) string {
	t.Helper()
	if req.Name == "" {
		req.Name = "generated-api-submit"
	}
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal generated submit request: %v", err)
	}
	resp, err := http.Post(baseURL+"/work", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /work: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /work status = %d, want 201", resp.StatusCode)
	}
	var out factoryapi.SubmitWorkResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode generated submit response: %v", err)
	}
	return out.TraceId
}

func stageGeneratedSubmitWorkFile(
	t *testing.T,
	baseURL string,
	itemType string,
	fileName string,
	mediaType string,
	content []byte,
) string {
	t.Helper()

	req := map[string]string{
		"itemType":      itemType,
		"fileName":      fileName,
		"mediaType":     mediaType,
		"contentBase64": base64.StdEncoding.EncodeToString(content),
	}
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal stage submit-work request: %v", err)
	}
	resp, err := http.Post(baseURL+"/work/staged-files", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /work/staged-files: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST /work/staged-files status = %d, want 201: %s", resp.StatusCode, string(payload))
	}
	var out factoryapi.StageSubmitWorkFileResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode staged-file response: %v", err)
	}
	return out.StagedFileRef
}

func putGeneratedWorkRequest(t *testing.T, baseURL string, requestID string, req factoryapi.WorkRequest) factoryapi.UpsertWorkRequestResponse {
	t.Helper()
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal generated work request: %v", err)
	}
	endpoint := baseURL + "/work-requests/" + url.PathEscape(requestID)
	httpReq, err := http.NewRequest(http.MethodPut, endpoint, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build PUT /work-requests request: %v", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		t.Fatalf("PUT /work-requests: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("PUT /work-requests status = %d, want 201: %s", resp.StatusCode, string(payload))
	}
	var out factoryapi.UpsertWorkRequestResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode generated work request response: %v", err)
	}
	return out
}

func getGeneratedJSON[T any](t *testing.T, endpoint string) T {
	t.Helper()
	resp, err := http.Get(endpoint)
	if err != nil {
		t.Fatalf("GET %s: %v", endpoint, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s status = %d, want 200", endpoint, resp.StatusCode)
	}
	var out T
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode %s as %T: %v", endpoint, out, err)
	}
	return out
}

func waitForGeneratedWorkComplete(t *testing.T, baseURL string, traceID string, timeout time.Duration) factoryapi.ListWorkResponse {
	t.Helper()
	return waitForGeneratedWorkAtPlace(t, baseURL, traceID, "task:complete", timeout)
}

func waitForGeneratedWorkAtPlace(t *testing.T, baseURL string, traceID string, placeID string, timeout time.Duration) factoryapi.ListWorkResponse {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		work := getGeneratedJSON[factoryapi.ListWorkResponse](t, baseURL+"/work")
		for _, item := range work.Results {
			if stringPointerValue(item.TraceId) == traceID && generatedWorkPlaceID(item) == placeID {
				return work
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	work := getGeneratedJSON[factoryapi.ListWorkResponse](t, baseURL+"/work")
	t.Fatalf("timed out waiting for trace %q at %s; last work response: %#v", traceID, placeID, work)
	return factoryapi.ListWorkResponse{}
}

func waitForGeneratedWorkTypeComplete(t *testing.T, baseURL string, workType string, timeout time.Duration) factoryapi.Work {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		work := getGeneratedJSON[factoryapi.ListWorkResponse](t, baseURL+"/work")
		for _, item := range work.Results {
			if stringPointerValue(item.WorkTypeName) == workType && generatedWorkStateName(item.State) == "complete" {
				return item
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	work := getGeneratedJSON[factoryapi.ListWorkResponse](t, baseURL+"/work")
	t.Fatalf("timed out waiting for completed work type %q; last work response: %#v", workType, work)
	return factoryapi.Work{}
}

func waitForGeneratedWorkIDsComplete(t *testing.T, baseURL string, workIDs []string, timeout time.Duration) []factoryapi.Work {
	t.Helper()
	want := make(map[string]bool, len(workIDs))
	for _, workID := range workIDs {
		want[workID] = true
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		work := getGeneratedJSON[factoryapi.ListWorkResponse](t, baseURL+"/work")
		found := make(map[string]factoryapi.Work, len(want))
		for _, item := range work.Results {
			workID := stringPointerValue(item.WorkId)
			if want[workID] && generatedWorkStateName(item.State) == "complete" {
				found[workID] = item
			}
		}
		if len(found) == len(want) {
			items := make([]factoryapi.Work, 0, len(workIDs))
			for _, workID := range workIDs {
				items = append(items, found[workID])
			}
			return items
		}
		time.Sleep(100 * time.Millisecond)
	}
	work := getGeneratedJSON[factoryapi.ListWorkResponse](t, baseURL+"/work")
	t.Fatalf("timed out waiting for completed work IDs %v; last work response: %#v", workIDs, work)
	return nil
}

func requireGeneratedWorkByTrace(t *testing.T, work factoryapi.ListWorkResponse, traceID string) factoryapi.Work {
	t.Helper()
	for _, item := range work.Results {
		if stringPointerValue(item.TraceId) == traceID {
			return item
		}
	}
	t.Fatalf("trace %q missing from generated work response: %#v", traceID, work)
	return factoryapi.Work{}
}

func generatedWorkPlaceID(work factoryapi.Work) string {
	if work.State == nil {
		return stringPointerValue(work.WorkTypeName) + ":"
	}
	return stringPointerValue(work.WorkTypeName) + ":" + work.State.Name
}

func mustSubmitWorkTextItem(t *testing.T, text string) factoryapi.SubmitWorkItem {
	t.Helper()

	var item factoryapi.SubmitWorkItem
	if err := item.FromSubmitWorkTextItem(factoryapi.SubmitWorkTextItem{
		Type: factoryapi.SubmitWorkItemTypeText,
		Text: text,
	}); err != nil {
		t.Fatalf("encode submit-work text item: %v", err)
	}
	return item
}

func mustSubmitWorkImageItem(t *testing.T, stagedFileRef string, fileName string, mediaType string) factoryapi.SubmitWorkItem {
	t.Helper()

	var item factoryapi.SubmitWorkItem
	if err := item.FromSubmitWorkImageItem(factoryapi.SubmitWorkImageItem{
		Type:          factoryapi.SubmitWorkItemTypeImage,
		StagedFileRef: stagedFileRef,
		FileName:      fileName,
		MediaType:     mediaType,
	}); err != nil {
		t.Fatalf("encode submit-work image item: %v", err)
	}
	return item
}

func functionalServerBase(t *testing.T, rawURL string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse functional server URL %q: %v", rawURL, err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		t.Fatalf("functional server URL %q missing scheme or host", rawURL)
	}
	return strings.TrimSuffix(rawURL, "/")
}

func assertFunctionalEventsUseCanonicalVocabulary(t *testing.T, events []factoryapi.FactoryEvent, required ...factoryapi.FactoryEventType) {
	t.Helper()
	seen := make(map[factoryapi.FactoryEventType]int, len(events))
	for _, event := range events {
		seen[event.Type]++
		for _, retired := range retiredFunctionalFactoryEventTypes {
			if string(event.Type) == retired {
				t.Fatalf("event %s reintroduced retired public event type %q", event.Id, retired)
			}
		}
	}
	for _, eventType := range required {
		if seen[eventType] == 0 {
			t.Fatalf("events %v missing canonical event type %s", functionalEventTypes(events), eventType)
		}
	}
}
