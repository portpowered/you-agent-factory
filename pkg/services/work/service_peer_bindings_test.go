package work

import (
	"context"
	"strings"
	"testing"
)

func TestMaterializationServiceDelegatesMaterializeContentURL(t *testing.T) {
	t.Parallel()

	materialized := ""
	service := MaterializationService(ContentMaterializeFunc(func(_ context.Context, rawURL string) (string, ContentCleanup, error) {
		materialized = rawURL
		return "/tmp/materialized.png", func() {}, nil
	}))
	if service == nil {
		t.Fatal("MaterializationService() = nil, want service")
	}

	path, cleanup, err := service.MaterializeContentURL(context.Background(), "file:///fixtures/peer.png")
	if err != nil || path != "/tmp/materialized.png" || cleanup == nil || materialized != "file:///fixtures/peer.png" {
		t.Fatalf("MaterializeContentURL = (%q, %v, %v, %q)", path, cleanup, err, materialized)
	}
	cleanup()

	_, err = service.PrepareWorkRequest(context.Background(), WorkRequestPreparation{})
	if err == nil || !strings.Contains(err.Error(), "does not support admission prep") {
		t.Fatalf("PrepareWorkRequest error = %v, want admission prep unsupported", err)
	}
}

func TestAdmissionContentServiceDelegatesStagingAndPreparation(t *testing.T) {
	t.Parallel()

	staging := &recordingPeerContentStaging{}
	preparation := mustRequestPreparationService(t)
	service := AdmissionContentService(staging, preparation)
	if service == nil {
		t.Fatal("AdmissionContentService() = nil, want service")
	}
	ctx := context.Background()

	staged, err := service.StageContent(ctx, StageContentRequest{
		ItemType: "image", FileName: "peer.png", MediaType: "image/png", Content: []byte("png"),
	})
	if err != nil || staged.StagedFileRef != "peer-stage-ref" {
		t.Fatalf("StageContent = (%#v, %v)", staged, err)
	}

	prepared, err := service.PrepareWorkRequest(ctx, WorkRequestPreparation{
		Request: WorkRequest{
			RequestID: "request-peer-1",
			Type:      WorkRequestTypeFactoryRequestBatch,
			Works:     []Work{{Name: "draft", WorkTypeID: "task"}},
		},
	})
	if err != nil || prepared.RequestID != "request-peer-1" {
		t.Fatalf("PrepareWorkRequest = (%#v, %v)", prepared, err)
	}

	_, err = service.SubmitWorkRequestForSession(ctx, "session-1", prepared)
	if err == nil || !strings.Contains(err.Error(), "does not support admission") {
		t.Fatalf("SubmitWorkRequestForSession error = %v, want admission unsupported", err)
	}
}

type recordingPeerContentStaging struct{}

func (recordingPeerContentStaging) StageContent(
	_ context.Context,
	request StageContentRequest,
) (StageContentResult, error) {
	return StageContentResult{
		StagedFileRef: "peer-stage-ref",
		FileName:      request.FileName,
		MediaType:     request.MediaType,
	}, nil
}

func (recordingPeerContentStaging) PrepareContent(context.Context, []StagedSubmissionItem) ([]WorkContentPart, error) {
	return nil, nil
}

func (recordingPeerContentStaging) ResolveContent(context.Context, string) (ResolvedStagedContent, error) {
	return ResolvedStagedContent{}, nil
}

func (recordingPeerContentStaging) CleanupContent(context.Context, string) error {
	return nil
}
