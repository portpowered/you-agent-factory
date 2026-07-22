package apisurface_test

import (
	"encoding/json"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/transports/mapping"
)

func TestFactoryPreviewResultFromPreview_MapsResolvedWorkflowSource(t *testing.T) {
	preview := factory.WorkflowPreview{SourceResolution: factory.WorkflowSourceResolution{
		RequestKind: factory.WorkflowSourceKindWorkflowName,
		Found:       true,
		SourceRef:   ".claude/workflows/review.js",
		SourceHash:  "sha256:review",
	}}

	result := apisurface.FactoryPreviewResultFromPreview(preview)
	if !result.SourceResolution.Found ||
		result.SourceResolution.SourceRef == nil || *result.SourceResolution.SourceRef != preview.SourceResolution.SourceRef ||
		result.SourceResolution.SourceHash == nil || *result.SourceResolution.SourceHash != preview.SourceResolution.SourceHash {
		t.Fatalf("result = %#v, want detached source resolution %#v", result, preview.SourceResolution)
	}
}

func TestFactoryPreviewResultFromPreview_MapsMissingWorkflowDiagnostic(t *testing.T) {
	preview := factory.WorkflowPreview{SourceResolution: factory.WorkflowSourceResolution{
		RequestKind: factory.WorkflowSourceKindWorkflowName,
		Diagnostics: []factory.WorkflowSourceDiagnostic{{
			Code:    factory.WorkflowSourceCodeNotFound,
			Message: "scripted source was not found",
		}},
	}}

	result := apisurface.FactoryPreviewResultFromPreview(preview)
	if result.SourceResolution.Found {
		t.Fatalf("result = %#v, want not found", result.SourceResolution)
	}
	if result.SourceResolution.Diagnostics == nil || len(*result.SourceResolution.Diagnostics) == 0 {
		t.Fatal("expected source resolution diagnostics")
	}
	if (*result.SourceResolution.Diagnostics)[0].Code != factory.WorkflowSourceCodeNotFound {
		t.Fatalf("diagnostic code = %q, want %q", (*result.SourceResolution.Diagnostics)[0].Code, factory.WorkflowSourceCodeNotFound)
	}
}

func TestWorkflowSessionResultMappingPreservesDomainProjection(t *testing.T) {
	timestamp := time.Date(2026, 7, 16, 3, 15, 0, 0, time.UTC)
	checkpointRefs := []interfaces.FactorySessionJavaScriptCheckpointEventRef{{
		ID: "checkpoint-1", Timestamp: &timestamp,
		ArtifactRef: &interfaces.FactoryArtifactRef{
			ID: "artifact-checkpoint-1", Kind: "CHECKPOINT", Visibility: "INTERNAL_CHECKPOINT",
		},
	}}
	resultArtifact := &interfaces.FactoryArtifactRef{
		ID: "artifact-result-1", Kind: "FINAL_RESULT", Visibility: "PUBLIC",
	}

	live := apisurface.WorkflowSessionLiveResultToAPI(factory.LiveSessionResult{
		SessionID:         "session-js-1",
		Status:            interfaces.RuntimeStatusFinished,
		CheckpointRefs:    checkpointRefs,
		ResultArtifactRef: resultArtifact,
	})
	assertLiveWorkflowResult(t, live, timestamp)
	partial := apisurface.WorkflowSessionPartialResultToAPI(factory.PartialSessionResult{
		SessionID:                "session-js-1",
		Phase:                    "review",
		CheckpointRefs:           checkpointRefs,
		PartialResultArtifactRef: resultArtifact,
	})
	if partial.SessionId != "session-js-1" || partial.Phase != "review" {
		t.Fatalf("partial result = %#v", partial)
	}
	if partial.CheckpointRefs == nil || len(*partial.CheckpointRefs) != 1 || (*partial.CheckpointRefs)[0].Id != "checkpoint-1" {
		t.Fatalf("partial checkpoint refs = %#v", partial.CheckpointRefs)
	}
	if partial.PartialResultArtifactRef == nil || partial.PartialResultArtifactRef.Id != "artifact-result-1" {
		t.Fatalf("partial artifact ref = %#v", partial.PartialResultArtifactRef)
	}
	assertDurableWorkflowResult(t, factory.SessionResult{
		SessionID:    "session-js-1",
		ResultStatus: factory.ResultStatusFinal,
		PrimaryResult: []work.WorkContentPart{{
			Type: work.WorkContentPartTypeJSON,
			JSON: json.RawMessage(`{"ok":true}`),
		}},
		ArtifactRefs: []interfaces.FactoryArtifactRef{*resultArtifact},
	}, factory.SessionResultUpdatedPayload{
		ResultStatus: interfaces.FactorySessionResultStatusFinal,
		ResultSummary: []work.WorkContentPart{{
			Type: work.WorkContentPartTypeJSON,
			JSON: json.RawMessage(`{"ok":true}`),
		}},
	})
}

func TestWorkflowArtifactsToAPIPreservesSessionProjection(t *testing.T) {
	capturedAt := time.Date(2026, 7, 16, 3, 20, 0, 0, time.UTC)
	auditMode := "FULL"
	contentHash := "sha256:artifact"
	label := "Review output"
	mimeType := "application/json"
	sizeBytes := int64(128)
	sourceDispatchID := "dispatch-1"
	summary := "Detached customer-visible result"
	paths, secrets, tokens := int32(1), int32(2), int32(3)

	mapped := apisurface.WorkflowArtifactsToAPI([]interfaces.FactoryArtifact{{
		AuditMode:   &auditMode,
		ContentHash: &contentHash,
		ID:          "artifact-1",
		Kind:        "CHILD_RESULT",
		Label:       &label,
		SizeBytes:   &sizeBytes,
		Summary:     &summary,
		Visibility:  "PUBLIC",
		CaptureMetadata: &interfaces.FactoryArtifactCaptureMetadata{
			CapturedAt: &capturedAt, MIMEType: &mimeType, SourceDispatchID: &sourceDispatchID,
		},
		RedactionCounts: &interfaces.FactoryArtifactRedactionCounts{
			Paths: &paths, Secrets: &secrets, Tokens: &tokens,
		},
	}})
	if mapped == nil || len(*mapped) != 1 {
		t.Fatalf("mapped artifacts = %#v, want one", mapped)
	}
	artifact := (*mapped)[0]
	assertWorkflowArtifactIdentity(t, artifact)
	assertWorkflowArtifactCaptureMetadata(t, artifact, capturedAt, sourceDispatchID, mimeType)
	assertWorkflowArtifactRedactionCounts(t, artifact, paths, secrets, tokens)
}

func assertWorkflowArtifactIdentity(t *testing.T, artifact factoryapi.FactoryArtifact) {
	t.Helper()
	if artifact.Id != "artifact-1" || artifact.Kind != factoryapi.FactoryArtifactKindCHILDRESULT ||
		artifact.Visibility != factoryapi.FactoryArtifactVisibilityPUBLIC || artifact.AuditMode == nil ||
		*artifact.AuditMode != factoryapi.FactoryArtifactAuditModeFULL {
		t.Fatalf("artifact identity = %#v", artifact)
	}
}

func assertWorkflowArtifactCaptureMetadata(
	t *testing.T,
	artifact factoryapi.FactoryArtifact,
	capturedAt time.Time,
	sourceDispatchID string,
	mimeType string,
) {
	t.Helper()
	if artifact.CaptureMetadata == nil || artifact.CaptureMetadata.CapturedAt == nil ||
		!artifact.CaptureMetadata.CapturedAt.Equal(capturedAt) || artifact.CaptureMetadata.SourceDispatchId == nil ||
		*artifact.CaptureMetadata.SourceDispatchId != sourceDispatchID || artifact.CaptureMetadata.MimeType == nil ||
		*artifact.CaptureMetadata.MimeType != mimeType {
		t.Fatalf("capture metadata = %#v", artifact.CaptureMetadata)
	}
}

func assertWorkflowArtifactRedactionCounts(
	t *testing.T,
	artifact factoryapi.FactoryArtifact,
	paths int32,
	secrets int32,
	tokens int32,
) {
	t.Helper()
	if artifact.RedactionCounts == nil || artifact.RedactionCounts.Paths == nil ||
		*artifact.RedactionCounts.Paths != paths || artifact.RedactionCounts.Secrets == nil ||
		*artifact.RedactionCounts.Secrets != secrets || artifact.RedactionCounts.Tokens == nil ||
		*artifact.RedactionCounts.Tokens != tokens {
		t.Fatalf("redaction counts = %#v", artifact.RedactionCounts)
	}
}

func assertLiveWorkflowResult(t *testing.T, live factoryapi.FactorySessionLiveResult, timestamp time.Time) {
	t.Helper()
	if live.Status != factoryapi.FactorySessionStatusFINISHED || live.CheckpointRefs == nil || len(*live.CheckpointRefs) != 1 {
		t.Fatalf("live result = %#v", live)
	}
	if got := (*live.CheckpointRefs)[0]; got.ArtifactRef == nil || got.ArtifactRef.Id != "artifact-checkpoint-1" || got.Timestamp == nil || !got.Timestamp.Equal(timestamp) {
		t.Fatalf("checkpoint ref = %#v", got)
	}
	if live.ResultArtifactRef == nil || live.ResultArtifactRef.Id != "artifact-result-1" {
		t.Fatalf("result artifact = %#v", live.ResultArtifactRef)
	}
}

func assertDurableWorkflowResult(
	t *testing.T,
	result factory.SessionResult,
	eventResult factory.SessionResultUpdatedPayload,
) {
	t.Helper()
	durable := apisurface.WorkflowSessionResultToAPI(result)
	if durable.ResultStatus != factoryapi.FactorySessionResultStatusFinal || durable.PrimaryResult == nil || len(*durable.PrimaryResult) != 1 {
		t.Fatalf("durable result = %#v", durable)
	}
	payload := apisurface.WorkflowSessionResultUpdatedPayloadToAPI(eventResult)
	if payload.ResultStatus != factoryapi.FactoryEventSessionResultStatusFinal || payload.ResultSummary == nil || len(*payload.ResultSummary) != 1 {
		t.Fatalf("event payload = %#v", payload)
	}
}
