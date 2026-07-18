package runtime_api

import (
	"bytes"
	"context"
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

	"github.com/portpowered/infinite-you/pkg/factory"
	modelprovider "github.com/portpowered/infinite-you/pkg/models/provider"
	"github.com/portpowered/infinite-you/pkg/service"
	submitcli "github.com/portpowered/infinite-you/pkg/transports/cli/submit"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/internal/functionalevidence"
)

func TestGeneratedAPIIntegrationSmoke_OpenAPIGeneratedServerAndLiveRuntimeStayAligned(t *testing.T) {
	support.SkipLongFunctional(t, "slow generated API and live runtime alignment smoke")

	dir := support.ScaffoldFactory(t, simplePipelineConfig())
	host, err := support.StartRootRunFunctionalHost(context.Background(), support.RootRunFunctionalHostConfig{
		FactoryRoot: dir,
		SystemRoot:  t.TempDir(),
	})
	if err != nil {
		t.Fatalf("StartRootRunFunctionalHost() error = %v", err)
	}
	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, shutdownErr := host.Shutdown(shutdownCtx); shutdownErr != nil {
			t.Errorf("Shutdown() error = %v", shutdownErr)
		}
	})

	stream := openRootRunFactoryEventHTTPStream(t, host)
	requireFunctionalEventStreamPrelude(t, stream)

	traceID := submitGeneratedWork(t, host.Endpoint(), factoryapi.SubmitWorkRequest{
		Name:         "generated-api-integration-smoke",
		WorkTypeName: "task",
		Payload:      map[string]string{"title": "generated API integration smoke"},
	})
	if traceID == "" {
		t.Fatal("POST /work returned an empty trace_id")
	}
	assertTerminalDispatchForTrace(t, stream, traceID)

	work := getGeneratedJSON[factoryapi.ListWorkResponse](t, support.DefaultSessionWorkURL(host.Endpoint(), "/work"))
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

	statusRead, err := host.REST().GetStatus(context.Background())
	if err != nil {
		t.Fatalf("generated GetStatus() error = %v", err)
	}
	if statusRead.StatusCode() != http.StatusOK || statusRead.JSON200 == nil {
		t.Fatalf("generated GetStatus() response = %#v, want typed 200", statusRead)
	}
	if statusRead.JSON200.TotalTokens != 1 {
		t.Fatalf("generated GET /status total_tokens = %d, want 1", statusRead.JSON200.TotalTokens)
	}
	if statusRead.JSON200.Categories.Terminal != 1 {
		t.Fatalf("generated GET /status terminal count = %d, want 1", statusRead.JSON200.Categories.Terminal)
	}
	functionalevidence.Covers(
		t,
		"rest/getEventsBySessionId",
		"rest/getStatusBySessionId",
		"rest/listWorkBySessionId",
		"rest/submitWorkBySessionId",
	)
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
	resp, err := http.Post(support.DefaultSessionWorkURL(server.URL(), "/work"), "application/json", bytes.NewReader(body))
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
	resp, err := http.Post(support.DefaultSessionWorkURL(server.URL(), "/work"), "application/json", bytes.NewReader(body))
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
	resp, err := http.Post(support.DefaultSessionWorkURL(server.URL(), "/work"), "application/json", bytes.NewReader(body))
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

func TestGeneratedAPIIntegrationSmoke_SubmitWorkContentAcceptsCanonicalParts(t *testing.T) {
	dir := support.ScaffoldFactory(t, simplePipelineConfig())
	server := startFunctionalServer(t, dir, true, factory.WithServiceMode())

	req := map[string]any{
		"name":         "generated-api-content-text",
		"workTypeName": "task",
		"content": []map[string]any{
			{"type": "text", "text": "Alpha "},
			{"type": "text", "text": "Beta"},
		},
	}
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal generated submit request: %v", err)
	}
	resp, err := http.Post(support.DefaultSessionWorkURL(server.URL(), "/work"), "application/json", bytes.NewReader(body))
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
	item := requireGeneratedWorkByTrace(t, work, submitted.TraceId)
	content := item.Content
	if content == nil || len(*content) != 2 {
		t.Fatalf("GET /work content = %#v, want two ordered canonical content parts", content)
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
		t.Fatalf("GET /work content parts = %#v, want ordered text content Alpha / Beta", content)
	}
}

func TestGeneratedAPIIntegrationSmoke_BatchUpsertAcceptsWorksContent(t *testing.T) {
	dir := support.ScaffoldFactory(t, simplePipelineConfig())
	server := startFunctionalServer(t, dir, true, factory.WithServiceMode())

	workID := "work-generated-api-batch-content"
	requestID := "request-generated-api-batch-content"
	body, err := json.Marshal(map[string]any{
		"requestId": requestID,
		"type":      "FACTORY_REQUEST_BATCH",
		"works": []map[string]any{{
			"name":         "content-batch-work",
			"workId":       workID,
			"workTypeName": "task",
			"content": []map[string]any{
				{"type": "text", "text": "Batch canonical content."},
				{"type": "text", "text": "Second batch part."},
			},
		}},
	})
	if err != nil {
		t.Fatalf("marshal generated batch request: %v", err)
	}
	endpoint := support.DefaultSessionWorkURL(server.URL(), "/work-requests/"+url.PathEscape(requestID))
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
	var upserted factoryapi.UpsertWorkRequestResponse
	if err := json.NewDecoder(resp.Body).Decode(&upserted); err != nil {
		t.Fatalf("decode generated work request response: %v", err)
	}
	if upserted.RequestId != requestID || upserted.TraceId == "" {
		t.Fatalf("PUT /work-requests response = %#v, want request id and trace id", upserted)
	}
	if len(upserted.Works) != 1 || upserted.Works[0].WorkId != workID {
		t.Fatalf("PUT /work-requests works = %#v, want one accepted work with id %q", upserted.Works, workID)
	}

	items := waitForGeneratedWorkIDsComplete(t, server.URL(), []string{workID}, 10*time.Second)
	content := items[0].Content
	if content == nil || len(*content) != 2 {
		t.Fatalf("GET /work content = %#v, want two ordered batch content parts", content)
	}
	firstPart, err := (*content)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("decode first batch content part: %v", err)
	}
	secondPart, err := (*content)[1].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("decode second batch content part: %v", err)
	}
	if firstPart.Text != "Batch canonical content." || secondPart.Text != "Second batch part." {
		t.Fatalf("GET /work batch content = %#v, want ordered batch text parts", content)
	}
	functionalevidence.Covers(t, "rest/upsertWorkRequestBySessionId")
}

func TestGeneratedAPIIntegrationSmoke_SubmitWorkItemsAcceptMixedTextAndImageSubmissionOnSupportedRunner(t *testing.T) {
	dir := support.ScaffoldFactory(t, simplePipelineConfig())
	support.WriteAgentConfig(t, dir, "worker-a", support.BuildModelWorkerConfig(modelprovider.Codex, "gpt-5-codex"))

	server := startFunctionalServerWithConfig(
		t,
		dir,
		false,
		func(cfg *service.FactoryServiceConfig) {
			support.ConfigureWorkerCommands(t, cfg, support.NewStaticSuccessCommandRunner("Done. COMPLETE"), nil)
		},
		factory.WithServiceMode(),
	)
	stagedImageRef, stagedImageURL := stageGeneratedSubmitWorkFile(t, server.URL(), "image", "review.png", "image/png", []byte("png-bytes"))

	traceID := submitGeneratedWork(t, server.URL(), factoryapi.SubmitWorkRequest{
		Name:         "generated-api-items-mixed",
		WorkTypeName: "task",
		Items: &[]factoryapi.SubmitWorkItem{
			mustSubmitWorkTextItem(t, "Review this screenshot."),
			mustSubmitWorkImageItem(t, stagedImageRef, stagedImageURL, "review.png", "image/png"),
		},
	})

	work := waitForGeneratedWorkComplete(t, server.URL(), traceID, 10*time.Second)
	item := requireGeneratedWorkByTrace(t, work, traceID)
	content := item.Content
	if content == nil || len(*content) != 1 {
		t.Fatalf("GET /work content = %#v, want one accepted response content part", content)
	}
	textPart, err := (*content)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("decode projected text content: %v", err)
	}
	if textPart.Text != "Done. COMPLETE" {
		t.Fatalf("projected text part = %#v, want worker response content", textPart)
	}
	if textPart.Text == "Review this screenshot." {
		t.Fatalf("terminal work echoed submitted request text instead of response content")
	}
}

func TestGeneratedAPIIntegrationSmoke_SubmitWorkItemsRejectMixedTextAndImageSubmissionOnUnsupportedRunner(t *testing.T) {
	dir := support.ScaffoldFactory(t, simplePipelineConfig())
	support.WriteAgentConfig(t, dir, "worker-a", support.BuildModelWorkerConfig(modelprovider.Gemini, "gemini-1.5-pro"))
	host, err := support.StartRootRunFunctionalHost(context.Background(), support.RootRunFunctionalHostConfig{
		FactoryRoot: dir,
		SystemRoot:  t.TempDir(),
	})
	if err != nil {
		t.Fatalf("StartRootRunFunctionalHost() error = %v", err)
	}
	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, shutdownErr := host.Shutdown(shutdownCtx); shutdownErr != nil {
			t.Errorf("Shutdown() error = %v", shutdownErr)
		}
	})

	stream := openRootRunFactoryEventHTTPStream(t, host)
	requireFunctionalEventStreamPrelude(t, stream)
	stagedImageRef, stagedImageURL := stageGeneratedSubmitWorkFile(t, host.Endpoint(), "image", "review.png", "image/png", []byte("png-bytes"))

	traceID := submitGeneratedWork(t, host.Endpoint(), factoryapi.SubmitWorkRequest{
		Name:         "generated-api-items-unsupported-mixed",
		WorkTypeName: "task",
		Items: &[]factoryapi.SubmitWorkItem{
			mustSubmitWorkTextItem(t, "Review this screenshot."),
			mustSubmitWorkImageItem(t, stagedImageRef, stagedImageURL, "review.png", "image/png"),
		},
	})

	terminalEvent := terminalDispatchForTrace(t, stream, traceID)
	terminalPayload, err := terminalEvent.Payload.AsDispatchResponseEventPayload()
	if err != nil {
		t.Fatalf("decode terminal DISPATCH_RESPONSE payload: %v", err)
	}
	if terminalPayload.Outcome != factoryapi.WorkOutcomeFailed {
		t.Fatalf("terminal DISPATCH_RESPONSE outcome = %q, want FAILED", terminalPayload.Outcome)
	}
	const wantFailure = "image input is not supported by the gemini runner in v1"
	if terminalPayload.Error == nil || !strings.Contains(*terminalPayload.Error, wantFailure) {
		t.Fatalf("terminal DISPATCH_RESPONSE error = %#v, want substring %q", terminalPayload.Error, wantFailure)
	}

	work := getGeneratedJSON[factoryapi.ListWorkResponse](t, support.DefaultSessionWorkURL(host.Endpoint(), "/work"))
	item := requireGeneratedWorkByTrace(t, work, traceID)
	if generatedWorkStateName(item.State) != "failed" || generatedWorkStateType(item.State) != factoryapi.WorkStateTypeFAILED {
		t.Fatalf("GET /work state = %#v, want failed/FAILED work state", item.State)
	}
	if generatedWorkStateName(item.State) == "complete" {
		t.Fatalf("GET /work state = %#v, must not expose successful completion for rejected image input", item.State)
	}
}

func TestGeneratedAPIIntegrationSmoke_SubmitWorkItemsRejectForgedStructuredFileReference(t *testing.T) {
	dir := support.ScaffoldFactory(t, simplePipelineConfig())
	server := startFunctionalServer(t, dir, true, factory.WithServiceMode())

	req := factoryapi.SubmitWorkRequest{
		Name:         "generated-api-forged-staged-ref",
		WorkTypeName: "task",
		Items: &[]factoryapi.SubmitWorkItem{
			mustSubmitWorkImageItem(t, "staged://forged-review.png", "file://forged-review.png", "review.png", "image/png"),
		},
	}
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal forged staged-ref request: %v", err)
	}
	resp, err := http.Post(support.DefaultSessionWorkURL(server.URL(), "/work"), "application/json", bytes.NewReader(body))
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

func submitGeneratedWork(t *testing.T, baseURL string, req factoryapi.SubmitWorkRequest) string {
	t.Helper()
	if req.Name == "" {
		req.Name = "generated-api-submit"
	}
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal generated submit request: %v", err)
	}
	resp, err := http.Post(support.DefaultSessionWorkURL(baseURL, "/work"), "application/json", bytes.NewReader(body))
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
) (stagedFileRef string, contentURL string) {
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
	resp, err := http.Post(support.DefaultSessionWorkURL(baseURL, "/work/staged-files"), "application/json", bytes.NewReader(body))
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
	return out.StagedFileRef, string(out.Url)
}

func putGeneratedWorkRequest(t *testing.T, baseURL string, requestID string, req factoryapi.WorkRequest) factoryapi.UpsertWorkRequestResponse {
	t.Helper()
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal generated work request: %v", err)
	}
	endpoint := support.DefaultSessionWorkURL(baseURL, "/work-requests/"+url.PathEscape(requestID))
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
		work := getGeneratedJSON[factoryapi.ListWorkResponse](t, support.DefaultSessionWorkURL(baseURL, "/work"))
		for _, item := range work.Results {
			if stringPointerValue(item.TraceId) == traceID && generatedWorkPlaceID(item) == placeID {
				return work
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	work := getGeneratedJSON[factoryapi.ListWorkResponse](t, support.DefaultSessionWorkURL(baseURL, "/work"))
	t.Fatalf("timed out waiting for trace %q at %s; last work response: %#v", traceID, placeID, work)
	return factoryapi.ListWorkResponse{}
}

func waitForGeneratedWorkTypeComplete(t *testing.T, baseURL string, workType string, timeout time.Duration) factoryapi.Work {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		work := getGeneratedJSON[factoryapi.ListWorkResponse](t, support.DefaultSessionWorkURL(baseURL, "/work"))
		for _, item := range work.Results {
			if stringPointerValue(item.WorkTypeName) == workType && generatedWorkStateName(item.State) == "complete" {
				return item
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	work := getGeneratedJSON[factoryapi.ListWorkResponse](t, support.DefaultSessionWorkURL(baseURL, "/work"))
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
		work := getGeneratedJSON[factoryapi.ListWorkResponse](t, support.DefaultSessionWorkURL(baseURL, "/work"))
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
	work := getGeneratedJSON[factoryapi.ListWorkResponse](t, support.DefaultSessionWorkURL(baseURL, "/work"))
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

func mustSubmitWorkImageItem(t *testing.T, stagedFileRef string, contentURL string, fileName string, mediaType string) factoryapi.SubmitWorkItem {
	t.Helper()

	var item factoryapi.SubmitWorkItem
	if err := item.FromSubmitWorkImageItem(factoryapi.SubmitWorkImageItem{
		Type:          factoryapi.SubmitWorkItemTypeImage,
		StagedFileRef: stagedFileRef,
		Url:           factoryapi.SubmitWorkContentURLProperty(contentURL),
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
