package api

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

func TestSubmitWork(t *testing.T) {
	mf := &testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}}
	srv := newTestServer(mf)

	rec := submitWorkRequest(t, srv, `{"name":"draft-prd","workTypeName":"prd","traceId":"test-trace-1","payload":{"title":"Draft PRD"}}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	resp := decodeJSONResponse[factoryapi.SubmitWorkResponse](t, rec)
	if resp.TraceId != "test-trace-1" {
		t.Errorf("expected trace_id test-trace-1, got %s", resp.TraceId)
	}
	if len(mf.WorkRequests) != 1 {
		t.Fatalf("expected 1 work request, got %d", len(mf.WorkRequests))
	}
	if mf.WorkRequests[0].Type != interfaces.WorkRequestTypeFactoryRequestBatch {
		t.Fatalf("work request type = %q, want FACTORY_REQUEST_BATCH", mf.WorkRequests[0].Type)
	}
	if len(mf.Submitted) != 1 {
		t.Fatalf("expected 1 submitted request, got %d", len(mf.Submitted))
	}
	if mf.Submitted[0].WorkTypeID != "prd" {
		t.Errorf("expected work type name prd, got %s", mf.Submitted[0].WorkTypeID)
	}
	if string(mf.Submitted[0].Payload) != `{"title":"Draft PRD"}` {
		t.Errorf("payload = %s, want JSON object payload", string(mf.Submitted[0].Payload))
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity this contract-heavy submit boundary test keeps the full canonical request and response shape in one reviewer-readable flow.
func TestSubmitWork_AcceptsCanonicalContent(t *testing.T) {
	mf := &testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}}
	srv := newTestServer(mf)

	rec := submitWorkRequest(t, srv, `{"name":"ui-review","workTypeName":"prd","content":[{"type":"text","text":"Review this UI."},{"type":"image","file":"fixtures/ui.png"}]}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(mf.Submitted) != 1 {
		t.Fatalf("submitted count = %d, want 1", len(mf.Submitted))
	}
	if len(mf.WorkRequests) != 1 || len(mf.WorkRequests[0].Works) != 1 {
		t.Fatalf("work requests = %#v, want one submitted work request", mf.WorkRequests)
	}
	if string(mf.Submitted[0].Payload) != "Review this UI." {
		t.Fatalf("payload = %q, want legacy text fallback", mf.Submitted[0].Payload)
	}
	if len(mf.Submitted[0].Content) != 2 {
		t.Fatalf("content count = %d, want 2", len(mf.Submitted[0].Content))
	}
	if mf.Submitted[0].Content[0].Type != interfaces.WorkContentPartTypeText || mf.Submitted[0].Content[0].Text != "Review this UI." {
		t.Fatalf("submitted content[0] = %#v, want canonical text content", mf.Submitted[0].Content[0])
	}
	if mf.Submitted[0].Content[1].Type != interfaces.WorkContentPartTypeImage || mf.Submitted[0].Content[1].File != "fixtures/ui.png" {
		t.Fatalf("submitted content[1] = %#v, want canonical image content", mf.Submitted[0].Content[1])
	}
	if len(mf.WorkRequests[0].Works[0].Content) != 2 {
		t.Fatalf("submitted work request content count = %d, want 2", len(mf.WorkRequests[0].Works[0].Content))
	}
	if mf.WorkRequests[0].Works[0].Content[0].Type != interfaces.WorkContentPartTypeText || mf.WorkRequests[0].Works[0].Content[0].Text != "Review this UI." {
		t.Fatalf("submitted work request content[0] = %#v, want canonical text content", mf.WorkRequests[0].Works[0].Content[0])
	}
	if mf.WorkRequests[0].Works[0].Content[1].Type != interfaces.WorkContentPartTypeImage || mf.WorkRequests[0].Works[0].Content[1].File != "fixtures/ui.png" {
		t.Fatalf("submitted work request content[1] = %#v, want canonical image content", mf.WorkRequests[0].Works[0].Content[1])
	}
}

func TestSubmitWork_AcceptsStructuredItems(t *testing.T) {
	mf := &testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}}
	srv := newTestServer(mf)

	staged := stageSubmitWorkTestFile(t, srv, "image", "ui.png", "image/png", []byte("png-bytes"))

	rec := submitWorkRequest(t, srv, `{"name":"ui-review","workTypeName":"prd","items":[{"type":"text","text":"Review this UI."},{"type":"image","stagedFileRef":"`+staged.StagedFileRef+`","fileName":"ui.png","mediaType":"image/png"}]}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(mf.Submitted) != 1 {
		t.Fatalf("submitted count = %d, want 1", len(mf.Submitted))
	}
	if string(mf.Submitted[0].Payload) != "Review this UI." {
		t.Fatalf("payload = %q, want legacy text fallback", mf.Submitted[0].Payload)
	}
	if len(mf.Submitted[0].Content) != 2 {
		t.Fatalf("content count = %d, want 2", len(mf.Submitted[0].Content))
	}
	if mf.Submitted[0].Content[0].Type != interfaces.WorkContentPartTypeText || mf.Submitted[0].Content[0].Text != "Review this UI." {
		t.Fatalf("submitted content[0] = %#v, want canonical text content", mf.Submitted[0].Content[0])
	}
	if mf.Submitted[0].Content[1].Type != interfaces.WorkContentPartTypeImage || mf.Submitted[0].Content[1].ContentType != "image/png" {
		t.Fatalf("submitted content[1] = %#v, want canonical staged image content", mf.Submitted[0].Content[1])
	}
	stagedContent, err := os.ReadFile(mf.Submitted[0].Content[1].File)
	if err != nil {
		t.Fatalf("read submitted staged file: %v", err)
	}
	if string(stagedContent) != "png-bytes" {
		t.Fatalf("submitted staged file content = %q, want png-bytes", stagedContent)
	}
	if mf.Submitted[0].Content[1].Metadata[submitWorkItemTypeMetadataKey] != "image" || mf.Submitted[0].Content[1].Metadata[submitWorkFileNameMetadataKey] != "ui.png" {
		t.Fatalf("submitted content[1].metadata = %#v, want item type and file name metadata", mf.Submitted[0].Content[1].Metadata)
	}
}

func TestSubmitWork_AcceptsUppercaseAndExtendedCanonicalContent(t *testing.T) {
	mf := &testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}}
	srv := newTestServer(mf)

	rec := submitWorkRequest(t, srv, `{
		"name":"tts-request",
		"workTypeName":"prd",
		"content":[
			{"type":"TEXT","text":"Synthesize this","label":"prompt"},
			{"type":"AUDIO","file":"artifacts/output.wav","contentType":"audio/wav","artifactId":"artifact-audio-1","metadata":{"voice":"alloy"}},
			{"type":"JSON","json":{"voice":"alloy","speed":1}}
		]
	}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	assertUppercaseExtendedCanonicalSubmission(t, mf)
}

func assertUppercaseExtendedCanonicalSubmission(t *testing.T, mf *testutil.MockFactory) {
	t.Helper()
	if len(mf.Submitted) != 1 || len(mf.Submitted[0].Content) != 3 {
		t.Fatalf("submitted content = %#v, want 3 canonical parts", mf.Submitted)
	}
	assertUppercaseExtendedTextPart(t, mf.Submitted[0].Content[0])
	assertUppercaseExtendedAudioPart(t, mf.Submitted[0].Content[1])
	assertUppercaseExtendedJSONPart(t, mf.Submitted[0].Content[2])
	if string(mf.Submitted[0].Payload) != "Synthesize this" {
		t.Fatalf("payload = %q, want legacy text projection from uppercase TEXT part", mf.Submitted[0].Payload)
	}
}

func assertUppercaseExtendedTextPart(t *testing.T, part interfaces.WorkContentPart) {
	t.Helper()
	if part.Type != interfaces.WorkContentPartTypeText || part.Label != "prompt" {
		t.Fatalf("submitted content[0] = %#v, want normalized text part with label", part)
	}
}

func assertUppercaseExtendedAudioPart(t *testing.T, part interfaces.WorkContentPart) {
	t.Helper()
	if part.Type != interfaces.WorkContentPartTypeAudio || part.File != "artifacts/output.wav" || part.ContentType != "audio/wav" || part.ArtifactID != "artifact-audio-1" {
		t.Fatalf("submitted content[1] = %#v, want canonical audio content", part)
	}
	audioMetadata, _ := json.Marshal(part.Metadata)
	if string(audioMetadata) != `{"voice":"alloy"}` {
		t.Fatalf("audio metadata = %s, want voice metadata", audioMetadata)
	}
}

func assertUppercaseExtendedJSONPart(t *testing.T, part interfaces.WorkContentPart) {
	t.Helper()
	jsonValue := map[string]any{}
	if err := json.Unmarshal(part.JSON, &jsonValue); err != nil {
		t.Fatalf("decode json content: %v", err)
	}
	if part.Type != interfaces.WorkContentPartTypeJSON || jsonValue["voice"] != "alloy" || jsonValue["speed"] != float64(1) {
		t.Fatalf("submitted content[2] = %#v, want canonical json content", part)
	}
}

func TestSubmitWork_RejectsConflictingContentAndPayload(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}})
	rec := submitWorkRequest(t, srv, `{"name":"conflicting-content","workTypeName":"prd","content":[{"type":"text","text":"canonical"}],"payload":"different"}`)
	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", `work_request: works[0] ("conflicting-content") has invalid content/payload: payload conflicts with explicit content`)
}

func TestSubmitWork_RejectsStructuredItemsCombinedWithPayload(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}})
	rec := submitWorkRequest(t, srv, `{"name":"conflicting-items","workTypeName":"prd","items":[{"type":"text","text":"canonical"}],"payload":"different"}`)
	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "items cannot be combined with payload")
}

func TestSubmitWork_AcceptsHeaderOnlyStructuredSubmitWork(t *testing.T) {
	// Dashboard submit-work sends name, workTypeName, and items: [] when optional text
	// inputs are blank. Header/type-only submissions carry no structured content.
	mf := &testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}}
	srv := newTestServer(mf)
	rec := submitWorkRequest(t, srv, `{"name":"header-only-request","workTypeName":"prd","items":[]}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("header-only structured submit-work: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	resp := decodeJSONResponse[factoryapi.SubmitWorkResponse](t, rec)
	if resp.TraceId == "" {
		t.Fatalf("expected non-empty trace_id, got %q", resp.TraceId)
	}
	if len(mf.Submitted) != 1 {
		t.Fatalf("submitted count = %d, want 1", len(mf.Submitted))
	}
	if mf.Submitted[0].Name != "header-only-request" {
		t.Fatalf("submitted name = %q, want header-only-request", mf.Submitted[0].Name)
	}
	if mf.Submitted[0].WorkTypeID != "prd" {
		t.Fatalf("submitted work type = %q, want prd", mf.Submitted[0].WorkTypeID)
	}
	if len(mf.Submitted[0].Content) != 0 {
		t.Fatalf("submitted content count = %d, want empty structured content", len(mf.Submitted[0].Content))
	}
	if len(mf.WorkRequests) != 1 || len(mf.WorkRequests[0].Works) != 1 {
		t.Fatalf("work requests = %#v, want one submitted work request", mf.WorkRequests)
	}
	if len(mf.WorkRequests[0].Works[0].Content) != 0 {
		t.Fatalf("submitted work request content count = %d, want empty structured content", len(mf.WorkRequests[0].Works[0].Content))
	}
}

func TestSubmitWork_RejectsBlankOnlyStructuredItems(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}})
	rec := submitWorkRequest(t, srv, `{"name":"blank-items","workTypeName":"prd","items":[{"type":"text","text":"   \t"}]}`)
	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "items must contain at least one non-empty item")
}

func TestSubmitWork_RejectsStructuredFileItemWithoutStagedReference(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}})
	rec := submitWorkRequest(t, srv, `{"name":"missing-staged-ref","workTypeName":"prd","items":[{"type":"document","stagedFileRef":"","fileName":"spec.pdf","mediaType":"application/pdf"}]}`)
	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "items[0].stagedFileRef must be a non-empty string")
}

func TestSubmitWork_RejectsForgedStructuredFileReference(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}})

	rec := submitWorkRequest(t, srv, `{"name":"forged-staged-ref","workTypeName":"prd","items":[{"type":"image","stagedFileRef":"staged://forged-ui.png","fileName":"ui.png","mediaType":"image/png"}]}`)
	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "items[0]: stagedFileRef must be a backend-issued staged file reference")
}

func TestStageSubmitWorkFile(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{})

	response := stageSubmitWorkTestFile(t, srv, "image", "ui.png", "image/png", []byte("png-bytes"))
	if response.FileName != "ui.png" || response.MediaType != "image/png" {
		t.Fatalf("stage response = %#v, want identifying metadata", response)
	}
	if response.StagedFileRef == "" {
		t.Fatalf("stagedFileRef must be non-empty")
	}
	stagedPath, err := resolveSubmitWorkStagedFileRef(response.StagedFileRef)
	if err != nil {
		t.Fatalf("resolve staged file ref: %v", err)
	}
	stagedContent, err := os.ReadFile(stagedPath)
	if err != nil {
		t.Fatalf("read staged file: %v", err)
	}
	if string(stagedContent) != "png-bytes" {
		t.Fatalf("staged file content = %q, want png-bytes", stagedContent)
	}
}

func stageSubmitWorkTestFile(
	t *testing.T,
	srv *Server,
	itemType string,
	fileName string,
	mediaType string,
	content []byte,
) factoryapi.StageSubmitWorkFileResponse {
	t.Helper()

	rec := submitWorkStageFileRequest(t, srv, "/work/staged-files", `{
		"itemType":"`+itemType+`",
		"fileName":"`+fileName+`",
		"mediaType":"`+mediaType+`",
		"contentBase64":"`+base64.StdEncoding.EncodeToString(content)+`"
	}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	return decodeJSONResponse[factoryapi.StageSubmitWorkFileResponse](t, rec)
}

func TestStageSubmitWorkFile_RejectsTextItemType(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{})

	rec := submitWorkStageFileRequest(t, srv, "/work/staged-files", `{
		"itemType":"text",
		"fileName":"notes.txt",
		"mediaType":"text/plain",
		"contentBase64":"`+base64.StdEncoding.EncodeToString([]byte("text"))+`"
	}`)
	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "itemType must be one of image, video, audio, or document")
}

func TestStageSubmitWorkFileBySessionId_NotFound(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{})

	rec := submitWorkStageFileRequest(t, srv, "/factory-sessions/session-missing/work/staged-files", `{
		"itemType":"document",
		"fileName":"spec.pdf",
		"mediaType":"application/pdf",
		"contentBase64":"`+base64.StdEncoding.EncodeToString([]byte("pdf"))+`"
	}`)
	assertJSONError(t, rec, http.StatusNotFound, "NOT_FOUND", "factory session not found")
}

func TestSubmitWork_RejectsInvalidContentPartShape(t *testing.T) {
	mf := &testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}}
	srv := newTestServer(mf)
	rec := submitWorkRequest(t, srv, `{"workTypeName":"prd","content":[{"type":"image","text":"wrong-field"}]}`)
	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "content[0].text is not supported")
	if len(mf.Submitted) != 0 || len(mf.WorkRequests) != 0 {
		t.Fatalf("submissions = workRequests:%d submitted:%d, want 0/0", len(mf.WorkRequests), len(mf.Submitted))
	}
}

func TestSubmitWork_RejectsInvalidExtendedContentMetadata(t *testing.T) {
	mf := &testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}}
	srv := newTestServer(mf)
	rec := submitWorkRequest(t, srv, `{"workTypeName":"prd","content":[{"type":"AUDIO","file":"voice.wav","metadata":"wrong"}]}`)
	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "content[0].metadata must be an object")
	if len(mf.Submitted) != 0 || len(mf.WorkRequests) != 0 {
		t.Fatalf("submissions = workRequests:%d submitted:%d, want 0/0", len(mf.WorkRequests), len(mf.Submitted))
	}
}

func TestSubmitWork_CurrentChainingTraceIDPreservesRuntimeBoundary(t *testing.T) {
	mf := &testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}}
	srv := newTestServer(mf)

	rec := submitWorkRequest(t, srv, `{"name":"chain-submit","workTypeName":"prd","currentChainingTraceId":"chain-submit-1","payload":{"title":"Draft PRD"}}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(mf.WorkRequests) != 1 || len(mf.WorkRequests[0].Works) != 1 {
		t.Fatalf("work requests = %#v, want one submitted work request", mf.WorkRequests)
	}
	if mf.WorkRequests[0].CurrentChainingTraceID != "chain-submit-1" || mf.WorkRequests[0].Works[0].CurrentChainingTraceID != "chain-submit-1" {
		t.Fatalf("current chaining trace IDs = %#v", mf.WorkRequests[0])
	}
	if len(mf.Submitted) != 1 {
		t.Fatalf("normalized submissions = %d, want 1", len(mf.Submitted))
	}
	if mf.Submitted[0].CurrentChainingTraceID != "chain-submit-1" || mf.Submitted[0].TraceID != "chain-submit-1" {
		t.Fatalf("normalized submission = %#v, want chaining trace preserved", mf.Submitted[0])
	}
}

func TestSubmitWork_CopiesTagMapBeforeRuntimeSubmission(t *testing.T) {
	mf := &testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}}
	srv := newTestServer(mf)

	rec := submitWorkRequest(t, srv, `{"name":"tag-copy","workTypeName":"prd","payload":{"title":"Draft PRD"},"tags":{"priority":"high","team":"api"}}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(mf.Submitted) != 1 {
		t.Fatalf("submitted count = %d, want 1", len(mf.Submitted))
	}
	if mf.Submitted[0].Tags["priority"] != "high" {
		t.Fatalf("submitted tags = %#v, want priority=high", mf.Submitted[0].Tags)
	}
	if mf.Submitted[0].Tags["team"] != "api" {
		t.Fatalf("submitted tags = %#v, want team=api", mf.Submitted[0].Tags)
	}
}

func TestSubmitWork_MatchingTraceAliasesNormalizeAtBoundary(t *testing.T) {
	mf := &testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}}
	srv := newTestServer(mf)

	rec := submitWorkRequest(t, srv, `{"name":"chain-submit","workTypeName":"prd","currentChainingTraceId":"chain-submit-1","traceId":"chain-submit-1","payload":{"title":"Draft PRD"}}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(mf.Submitted) != 1 {
		t.Fatalf("normalized submissions = %d, want 1", len(mf.Submitted))
	}
	if mf.Submitted[0].CurrentChainingTraceID != "chain-submit-1" || mf.Submitted[0].TraceID != "chain-submit-1" {
		t.Fatalf("normalized submission = %#v, want matching aliases", mf.Submitted[0])
	}
}

func TestSubmitWork_ConflictingCurrentChainingTraceIDReturnsBadRequest(t *testing.T) {
	mf := &testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}}
	srv := newTestServer(mf)

	rec := submitWorkRequest(t, srv, `{"workTypeName":"prd","currentChainingTraceId":"chain-submit-1","traceId":"trace-submit-1","payload":{"title":"Draft PRD"}}`)
	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "currentChainingTraceId and traceId must match when both are provided")
	if len(mf.Submitted) != 0 {
		t.Fatalf("submitted count = %d, want 0", len(mf.Submitted))
	}
}

func TestSubmitWork_WorkTypeIDReturnsBadRequest(t *testing.T) {
	mf := &testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}}
	srv := newTestServer(mf)

	rec := submitWorkRequest(t, srv, `{"work_type_id":"legacy-task","traceId":"test-trace-legacy","payload":{"title":"Legacy"}}`)
	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "work_type_id is not supported; use workTypeName")
	if len(mf.Submitted) != 0 {
		t.Fatalf("submitted count = %d, want 0", len(mf.Submitted))
	}
}

func TestSubmitWork_TargetStateReturnsBadRequest(t *testing.T) {
	mf := &testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}}
	srv := newTestServer(mf)

	rec := submitWorkRequest(t, srv, `{"name":"draft","workTypeName":"prd","target_state":"queued","payload":{"title":"Draft PRD"}}`)
	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "target_state is not supported; use state")
	if len(mf.Submitted) != 0 {
		t.Fatalf("submitted count = %d, want 0", len(mf.Submitted))
	}
}

func TestSubmitWork_PreservesRuntimeRelations(t *testing.T) {
	mf := &testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}}
	srv := newTestServer(mf)

	rec := submitWorkRequest(t, srv, `{"name":"runtime-relations","workTypeName":"prd","payload":{"title":"Draft PRD"},"relations":[{"type":"DEPENDS_ON","targetWorkId":"review-work","requiredState":"complete"}]}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(mf.Submitted) != 1 || len(mf.Submitted[0].Relations) != 1 {
		t.Fatalf("submitted relations = %#v, want one", mf.Submitted)
	}
	relation := mf.Submitted[0].Relations[0]
	if relation.Type != interfaces.RelationDependsOn || relation.TargetWorkID != "review-work" || relation.RequiredState != "complete" {
		t.Fatalf("submitted relation = %#v, want dependency on review-work at complete", relation)
	}
}

func TestSubmitWork_WorkTypeNameWithWorkTypeIDReturnsBadRequest(t *testing.T) {
	mf := &testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}}
	srv := newTestServer(mf)

	rec := submitWorkRequest(t, srv, `{"workTypeName":"tasks","work_type_id":"legacy-task","payload":"fix lint"}`)
	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "work_type_id is not supported; use workTypeName")
	if len(mf.Submitted) != 0 {
		t.Fatalf("submitted count = %d, want 0", len(mf.Submitted))
	}
}

func TestSubmitWorkMissingWorkType(t *testing.T) {
	mf := &testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}}
	srv := newTestServer(mf)

	req := httptest.NewRequest(http.MethodPost, "/work", bytes.NewBufferString(`{"name":"missing-work-type","traceId":"test-trace-1"}`))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "workTypeName is required")
}

func TestSubmitWorkMissingName(t *testing.T) {
	mf := &testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}}
	srv := newTestServer(mf)

	req := httptest.NewRequest(http.MethodPost, "/work", bytes.NewBufferString(`{"workTypeName":"task","traceId":"test-trace-1","payload":{"title":"unnamed"}}`))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "name is required")
	if len(mf.Submitted) != 0 {
		t.Fatalf("submitted count = %d, want 0", len(mf.Submitted))
	}
}

func TestSubmitWorkBlankName(t *testing.T) {
	mf := &testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}}
	srv := newTestServer(mf)

	req := httptest.NewRequest(http.MethodPost, "/work", bytes.NewBufferString("{\"name\":\"   \\t\\n \",\"workTypeName\":\"task\",\"traceId\":\"test-trace-1\",\"payload\":{\"title\":\"blank\"}}"))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "name is required")
	if len(mf.Submitted) != 0 {
		t.Fatalf("submitted count = %d, want 0", len(mf.Submitted))
	}
}

func TestSubmitWorkMarkdownPayload(t *testing.T) {
	mf := &testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}}
	srv := newTestServer(mf)

	rec := submitWorkRequest(t, srv, `{"name":"markdown-fix","workTypeName":"tasks","traceId":"trace-markdown","payload":"# Fix lint\n\nRun gofmt."}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(mf.Submitted) != 1 {
		t.Fatalf("expected 1 submitted request, got %d", len(mf.Submitted))
	}
	if mf.Submitted[0].WorkTypeID != "tasks" {
		t.Fatalf("WorkTypeID = %q, want tasks", mf.Submitted[0].WorkTypeID)
	}
	if string(mf.Submitted[0].Payload) != `"# Fix lint\n\nRun gofmt."` {
		t.Fatalf("payload = %s, want marshaled markdown string", string(mf.Submitted[0].Payload))
	}
}

func TestSubmitWorkInvalidPayload_ReturnsDocumentedBadRequest(t *testing.T) {
	mf := &testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}}
	srv := newTestServer(mf)

	req := httptest.NewRequest(http.MethodPost, "/work", bytes.NewBufferString(`{"workTypeName":`))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "invalid request payload")
}

func TestSubmitWorkUnknownWorkTypeReturnsBadRequest(t *testing.T) {
	mf := &testutil.MockFactory{
		Marking:   &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)},
		SubmitErr: errors.New(`work_request: works[0] ("unknown-work") references unknown work type "unknown"`),
	}
	srv := newTestServer(mf)

	rec := submitWorkRequest(t, srv, `{"name":"unknown-work","workTypeName":"unknown","payload":"fix lint"}`)
	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", `work_request: works[0] ("unknown-work") references unknown work type name "unknown"`)
}

func TestSubmitWorkAutoTraceID(t *testing.T) {
	mf := &testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}}
	srv := newTestServer(mf)

	req := httptest.NewRequest(http.MethodPost, "/work", bytes.NewBufferString(`{"name":"auto-trace","workTypeName":"prd"}`))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}
	resp := decodeJSONResponse[factoryapi.SubmitWorkResponse](t, rec)
	if resp.TraceId == "" {
		t.Error("expected auto-generated trace_id, got empty")
	}
}

func TestServer_APISurfaceSmokePreservesEmbeddedFactoryContract(t *testing.T) {
	eventTime := testSubmitSurfaceSmokeEventTime()
	liveEvents := make(chan factoryapi.FactoryEvent, 1)
	mf := newSubmitSurfaceSmokeFactory(t, eventTime, liveEvents)

	server := httptest.NewServer(newTestServer(mf).Handler())
	defer server.Close()

	assertSubmitSurfaceSmokeSubmitAndList(t, server.URL, mf)
	assertSubmitSurfaceSmokeCurrentFactory(t, server.URL)
	assertSubmitSurfaceSmokeEvents(t, server.URL)
}

func testSubmitSurfaceSmokeEventTime() time.Time {
	return time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC)
}

func newSubmitSurfaceSmokeFactory(t *testing.T, eventTime time.Time, liveEvents chan factoryapi.FactoryEvent) *testutil.MockFactory {
	t.Helper()

	currentFactoryID := "beta"
	return &testutil.MockFactory{
		Marking: &petri.MarkingSnapshot{
			Tokens: map[string]*interfaces.Token{
				"tok-api-surface-1": {
					ID:        "tok-api-surface-1",
					PlaceID:   "task:init",
					Color:     interfaces.TokenColor{WorkID: "work-api-surface-1", WorkTypeID: "task"},
					CreatedAt: eventTime,
					EnteredAt: eventTime,
					History: interfaces.TokenHistory{
						TotalVisits:         make(map[string]int),
						ConsecutiveFailures: make(map[string]int),
						PlaceVisits:         make(map[string]int),
					},
				},
			},
		},
		FactoryEventStream: &interfaces.FactoryEventStream{
			History: []factoryapi.FactoryEvent{
				testFactoryEvent(t, factoryapi.FactoryEventTypeWorkRequest, "factory-event/work-request/api-surface-history", factoryapi.FactoryEventContext{
					Tick:      1,
					EventTime: eventTime,
					RequestId: stringPointerForAPITest("request-api-surface"),
				}, factoryapi.WorkRequestEventPayload{Type: factoryapi.WorkRequestTypeFactoryRequestBatch}),
			},
			Events: liveEvents,
		},
		CurrentFactory: &factoryapi.Factory{Name: "beta", Id: &currentFactoryID},
	}
}

func assertSubmitSurfaceSmokeSubmitAndList(t *testing.T, serverURL string, mf *testutil.MockFactory) {
	t.Helper()

	submitResp, err := http.Post(serverURL+"/work", "application/json", bytes.NewBufferString(`{"name":"api-surface-smoke","workTypeName":"task","traceId":"trace-api-surface-smoke","payload":{"title":"API surface smoke"}}`))
	if err != nil {
		t.Fatalf("POST /work: %v", err)
	}
	defer submitResp.Body.Close()
	if submitResp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(submitResp.Body)
		t.Fatalf("POST /work status = %d, want 201: %s", submitResp.StatusCode, string(body))
	}

	var submitBody factoryapi.SubmitWorkResponse
	if err := json.NewDecoder(submitResp.Body).Decode(&submitBody); err != nil {
		t.Fatalf("decode submit response: %v", err)
	}
	if submitBody.TraceId != "trace-api-surface-smoke" {
		t.Fatalf("submit trace_id = %q, want trace-api-surface-smoke", submitBody.TraceId)
	}
	if len(mf.WorkRequests) != 1 {
		t.Fatalf("submitted work requests = %d, want 1", len(mf.WorkRequests))
	}

	listResp, err := http.Get(serverURL + "/work")
	if err != nil {
		t.Fatalf("GET /work: %v", err)
	}
	defer listResp.Body.Close()
	if listResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(listResp.Body)
		t.Fatalf("GET /work status = %d, want 200: %s", listResp.StatusCode, string(body))
	}
	var listBody factoryapi.ListWorkResponse
	if err := json.NewDecoder(listResp.Body).Decode(&listBody); err != nil {
		t.Fatalf("decode list work response: %v", err)
	}
	if len(listBody.Results) != 1 || stringValue(listBody.Results[0].WorkId) != "work-api-surface-1" {
		t.Fatalf("GET /work results = %#v, want work-api-surface-1", listBody.Results)
	}
	if mf.EngineStateSnapshotCalls == 0 {
		t.Fatal("expected GET /work to read engine state snapshot through the embedded API factory contract")
	}
}

func assertSubmitSurfaceSmokeCurrentFactory(t *testing.T, serverURL string) {
	t.Helper()

	currentResp, err := http.Get(serverURL + "/factory-sessions/~default/factory")
	if err != nil {
		t.Fatalf("GET /factory-sessions/~default/factory: %v", err)
	}
	defer currentResp.Body.Close()
	if currentResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(currentResp.Body)
		t.Fatalf("GET /factory-sessions/~default/factory status = %d, want 200: %s", currentResp.StatusCode, string(body))
	}
	var currentBody factoryapi.Factory
	if err := json.NewDecoder(currentResp.Body).Decode(&currentBody); err != nil {
		t.Fatalf("decode current factory response: %v", err)
	}
	if currentBody.Name != "beta" {
		t.Fatalf("current factory name = %q, want beta", currentBody.Name)
	}
}

func assertSubmitSurfaceSmokeEvents(t *testing.T, serverURL string) {
	t.Helper()

	eventsReq, err := http.NewRequest(http.MethodGet, serverURL+"/events", nil)
	if err != nil {
		t.Fatalf("new /events request: %v", err)
	}
	eventsResp, err := http.DefaultClient.Do(eventsReq)
	if err != nil {
		t.Fatalf("GET /events: %v", err)
	}
	defer eventsResp.Body.Close()
	if eventsResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(eventsResp.Body)
		t.Fatalf("GET /events status = %d, want 200: %s", eventsResp.StatusCode, string(body))
	}

	streamed := readSSEFactoryEvent(t, bufio.NewReader(eventsResp.Body))
	if streamed.Id != "factory-event/work-request/api-surface-history" {
		t.Fatalf("streamed event id = %q, want factory-event/work-request/api-surface-history", streamed.Id)
	}
}
