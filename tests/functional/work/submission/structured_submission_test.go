package submission_test

import (
	"encoding/json"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestStructuredSubmissionSimplePipeline preserves the simple-pipeline
// structured submission witnesses on one serialized Factory fixture. Every
// case uses a distinct name or trace, so prior completed Work cannot satisfy a
// later case's public projection assertions.
func TestStructuredSubmissionSimplePipeline(t *testing.T) {
	factoryDir := support.ScaffoldFactory(t, submissionInputPreservingFactoryConfig())
	configureSubmissionCodexWorkers(t, factoryDir, "worker-a")
	server := support.StartFunctionalAPIServer(t, submissionServerConfig(factoryDir, submissionInputPreservingProviderRunner()))
	defer server.Stop(t)

	t.Run("TestAPIPOSTSubmitAndQueryWork", func(t *testing.T) {
		if testing.Short() {
			t.Skip("slow config-driven REST submit/query functional test")
		}
		assertAPIPOSTSubmitAndQueryWork(t, server)
	})
	t.Run("TestAPISubmitWorkAcceptsHeaderOnlyStructuredSubmission", func(t *testing.T) {
		assertAPISubmitWorkAcceptsHeaderOnlyStructuredSubmission(t, server)
	})
	t.Run("TestCLIWorkTypeNameReachesLiveAPIHandler", func(t *testing.T) {
		if testing.Short() {
			t.Skip("slow CLI submit functional test")
		}
		assertCLIWorkTypeNameReachesLiveAPIHandler(t, server, factoryDir)
	})
	t.Run("TestAPISubmitWorkRejectsEmptyStructuredSubmission", func(t *testing.T) {
		assertAPISubmitWorkRejectsEmptyStructuredSubmission(t, server)
	})
	t.Run("TestAPISubmitWorkAcceptsOrderedTextSubmission", func(t *testing.T) {
		assertAPISubmitWorkAcceptsOrderedTextSubmission(t, server)
	})
	t.Run("TestAPISubmitWorkAcceptsCanonicalContentParts", func(t *testing.T) {
		assertAPISubmitWorkAcceptsCanonicalContentParts(t, server)
	})
	t.Run("TestAPISubmitWorkRejectsForgedStructuredFileReference", func(t *testing.T) {
		assertAPISubmitWorkRejectsForgedStructuredFileReference(t, server)
	})
}

// assertAPISubmitWorkAcceptsHeaderOnlyStructuredSubmission proves the public
// HTTP submit surface accepts a header-only structured submission and projects
// empty customer-visible content after completion.
func assertAPISubmitWorkAcceptsHeaderOnlyStructuredSubmission(
	t *testing.T,
	server *support.FunctionalAPIServer,
) {
	body, err := json.Marshal(map[string]any{
		"name":         "submission-header-only",
		"workTypeName": "task",
		"items":        []map[string]any{},
	})
	if err != nil {
		t.Fatalf("marshal submit request: %v", err)
	}
	submitted := postSubmitWork(t, server.URL(), body)
	if submitted.TraceId == "" {
		t.Fatalf("submit response traceId is empty, want customer-visible trace identity")
	}

	listed := waitForWorkByTraceComplete(t, server.URL(), submitted.TraceId, 10*time.Second)
	work := requireWorkByTrace(t, listed, submitted.TraceId)
	if work.Name != "submission-header-only" ||
		support.StringPointerValue(work.WorkTypeName) != "task" {
		t.Fatalf("GET /work = %#v, want header-only name and work type", work)
	}
	if work.Content != nil && len(*work.Content) != 0 {
		t.Fatalf("GET /work content = %#v, want empty structured content", work.Content)
	}
}

// assertAPISubmitWorkRejectsEmptyStructuredSubmission proves empty structured
// submit items return HTTP 400 through the public submit surface.
func assertAPISubmitWorkRejectsEmptyStructuredSubmission(
	t *testing.T,
	server *support.FunctionalAPIServer,
) {
	body, err := json.Marshal(map[string]any{
		"name":         "submission-empty-items",
		"workTypeName": "task",
		"items": []map[string]any{
			{"type": "text", "text": "   "},
		},
	})
	if err != nil {
		t.Fatalf("marshal submit request: %v", err)
	}
	postSubmitWorkExpectStatus(t, server.URL(), body, 400)
}

// assertAPISubmitWorkAcceptsOrderedTextSubmission proves ordered structured text
// items are preserved in the customer-visible Work content projection.
func assertAPISubmitWorkAcceptsOrderedTextSubmission(
	t *testing.T,
	server *support.FunctionalAPIServer,
) {
	body, err := json.Marshal(map[string]any{
		"name":         "submission-items-text",
		"workTypeName": "task",
		"items": []map[string]any{
			{"type": "text", "text": "Alpha "},
			{"type": "text", "text": "Beta"},
		},
	})
	if err != nil {
		t.Fatalf("marshal submit request: %v", err)
	}
	submitted := postSubmitWork(t, server.URL(), body)

	listed := waitForWorkByTraceComplete(t, server.URL(), submitted.TraceId, 10*time.Second)
	work := requireWorkByTrace(t, listed, submitted.TraceId)
	content := work.Content
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

// assertAPISubmitWorkAcceptsCanonicalContentParts proves canonical content
// parts on POST /work are preserved in the customer-visible Work projection.
func assertAPISubmitWorkAcceptsCanonicalContentParts(
	t *testing.T,
	server *support.FunctionalAPIServer,
) {
	body, err := json.Marshal(map[string]any{
		"name":         "submission-content-text",
		"workTypeName": "task",
		"content": []map[string]any{
			{"type": "text", "text": "Alpha "},
			{"type": "text", "text": "Beta"},
		},
	})
	if err != nil {
		t.Fatalf("marshal submit request: %v", err)
	}
	submitted := postSubmitWork(t, server.URL(), body)

	listed := waitForWorkByTraceComplete(t, server.URL(), submitted.TraceId, 10*time.Second)
	work := requireWorkByTrace(t, listed, submitted.TraceId)
	content := work.Content
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

// TestAPISubmitWorkAcceptsMixedTextAndImageOnSupportedRunner proves mixed text
// and image structured submissions complete on a capability-supported runner.
func TestAPISubmitWorkAcceptsMixedTextAndImageOnSupportedRunner(t *testing.T) {
	factoryDir := support.ScaffoldFactory(t, simplePipelineFactoryConfig())
	support.WriteAgentConfig(
		t,
		factoryDir,
		"worker-a",
		support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"),
	)
	edges := serviceedges.Edges{}
	support.ConfigureWorkerCommands(
		t,
		&edges,
		support.NewStaticSuccessCommandRunner("Done. COMPLETE"),
		nil,
	)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		WaitForServiceModeRuntime: true,
		Edges:                     edges,
	})
	defer server.Stop(t)

	staged := stageSubmitWorkFile(
		t,
		server.URL(),
		"image",
		"review.png",
		"image/png",
		[]byte("png-bytes"),
	)
	submitted := support.SubmitDefaultSessionWork(t, server.URL(), factoryapi.SubmitWorkRequest{
		Name:         stringPtr("submission-items-mixed"),
		WorkTypeName: "task",
		Items: &[]factoryapi.SubmitWorkItem{
			mustSubmitWorkTextItem(t, "Review this screenshot."),
			mustSubmitWorkImageItem(t, staged.StagedFileRef, string(staged.Url), "review.png", "image/png"),
		},
	})

	listed := waitForWorkByTraceComplete(t, server.URL(), submitted.TraceId, 10*time.Second)
	work := requireWorkByTrace(t, listed, submitted.TraceId)
	content := work.Content
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

// TestAPISubmitWorkRejectsMixedTextAndImageOnUnsupportedRunner proves mixed text
// and image structured submissions fail before provider launch on unsupported runners.
func TestAPISubmitWorkRejectsMixedTextAndImageOnUnsupportedRunner(t *testing.T) {
	factoryDir := support.ScaffoldFactory(t, simplePipelineFactoryConfig())
	support.WriteAgentConfig(
		t,
		factoryDir,
		"worker-a",
		support.BuildModelWorkerConfig(modelprovider.ProviderClaude, "claude-test-model"),
	)
	runner := support.NewRecordingCommandRunner("unused")
	edges := serviceedges.Edges{}
	support.ConfigureWorkerCommands(t, &edges, runner, nil)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		WaitForServiceModeRuntime: true,
		Edges:                     edges,
	})
	defer server.Stop(t)

	staged := stageSubmitWorkFile(
		t,
		server.URL(),
		"image",
		"review.png",
		"image/png",
		[]byte("png-bytes"),
	)
	submitted := support.SubmitDefaultSessionWork(t, server.URL(), factoryapi.SubmitWorkRequest{
		Name:         stringPtr("submission-items-unsupported-mixed"),
		WorkTypeName: "task",
		Items: &[]factoryapi.SubmitWorkItem{
			mustSubmitWorkTextItem(t, "Review this screenshot."),
			mustSubmitWorkImageItem(t, staged.StagedFileRef, string(staged.Url), "review.png", "image/png"),
		},
	})

	listed := waitForWorkByTraceAtPlace(t, server.URL(), submitted.TraceId, "task:failed", 10*time.Second)
	work := requireWorkByTrace(t, listed, submitted.TraceId)
	if workStateName(work.State) != "failed" {
		t.Fatalf("GET /work state = %#v, want failed work state", work.State)
	}
	if runner.CallCount() != 0 {
		t.Fatalf(
			"provider command runner calls = %d, want 0 because capability rejection should happen before subprocess launch",
			runner.CallCount(),
		)
	}
}

// assertAPISubmitWorkRejectsForgedStructuredFileReference proves forged
// staged-file references are rejected with HTTP 400 through the public submit
// surface.
func assertAPISubmitWorkRejectsForgedStructuredFileReference(
	t *testing.T,
	server *support.FunctionalAPIServer,
) {
	body, err := json.Marshal(factoryapi.SubmitWorkRequest{
		Name:         stringPtr("submission-forged-staged-ref"),
		WorkTypeName: "task",
		Items: &[]factoryapi.SubmitWorkItem{
			mustSubmitWorkImageItem(
				t,
				"staged://forged-review.png",
				"file://forged-review.png",
				"review.png",
				"image/png",
			),
		},
	})
	if err != nil {
		t.Fatalf("marshal forged staged-ref request: %v", err)
	}
	postSubmitWorkExpectStatus(t, server.URL(), body, 400)
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

func mustSubmitWorkImageItem(
	t *testing.T,
	stagedFileRef string,
	contentURL string,
	fileName string,
	mediaType string,
) factoryapi.SubmitWorkItem {
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
