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

func TestMaterializationServiceNilBinderReturnsNil(t *testing.T) {
	t.Parallel()
	if got := MaterializationService(nil); got != nil {
		t.Fatalf("MaterializationService(nil) = %#v, want nil", got)
	}
}

func TestAdmissionContentServiceNilBinderReturnsNil(t *testing.T) {
	t.Parallel()
	if got := AdmissionContentService(nil, mustRequestPreparationService(t)); got != nil {
		t.Fatalf("AdmissionContentService(nil, prep) = %#v, want nil", got)
	}
	if got := AdmissionContentService(&recordingPeerContentStaging{}, nil); got != nil {
		t.Fatalf("AdmissionContentService(staging, nil) = %#v, want nil", got)
	}
}

func TestAdmissionContentServiceDelegatesContentLifecycle(t *testing.T) {
	t.Parallel()

	staging := &recordingPeerContentStagingWithLifecycle{}
	preparation := mustRequestPreparationService(t)
	service := AdmissionContentService(staging, preparation)
	ctx := context.Background()

	prepared, err := service.PrepareContent(ctx, []StagedSubmissionItem{{
		ItemType: "image", StagedFileRef: "peer-stage-ref", FileName: "peer.png", MediaType: "image/png",
	}})
	if err != nil || len(prepared) != 1 || prepared[0].URL != "file:///peer.png" {
		t.Fatalf("PrepareContent = (%#v, %v)", prepared, err)
	}

	resolved, err := service.ResolveContent(ctx, "peer-stage-ref")
	if err != nil || resolved.Path != "/tmp/peer.png" {
		t.Fatalf("ResolveContent = (%#v, %v)", resolved, err)
	}

	if err := service.CleanupContent(ctx, "peer-stage-ref"); err != nil || !staging.cleaned {
		t.Fatalf("CleanupContent = %v, cleaned = %v", err, staging.cleaned)
	}

	_, _, err = service.MaterializeContentURL(ctx, "file:///peer.png")
	if err == nil || !strings.Contains(err.Error(), "does not support content materialization") {
		t.Fatalf("MaterializeContentURL error = %v, want materialization unsupported", err)
	}
}

func TestMaterializationServiceRejectsUnsupportedSlices(t *testing.T) {
	t.Parallel()

	service := MaterializationService(ContentMaterializeFunc(func(context.Context, string) (string, ContentCleanup, error) {
		return "", nil, nil
	}))
	ctx := context.Background()

	cases := []struct {
		name string
		call func() error
		want string
	}{
		{
			name: "admission",
			call: func() error {
				_, err := service.SubmitWorkRequestForSession(ctx, "session-1", WorkRequest{})
				return err
			},
			want: "does not support admission",
		},
		{
			name: "content stage",
			call: func() error {
				_, err := service.StageContent(ctx, StageContentRequest{})
				return err
			},
			want: "does not support content staging",
		},
		{
			name: "invocation input",
			call: func() error {
				_, err := service.PrepareInvocationInput(ctx, InvocationInputPreparationRequest{})
				return err
			},
			want: "does not support invocation policy",
		},
		{
			name: "primary result",
			call: func() error {
				_, err := service.ResolvePrimaryResult(ctx, PrimaryResultSelectionInput{})
				return err
			},
			want: "does not support invocation policy",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := tc.call(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

type recordingPeerContentStagingWithLifecycle struct {
	recordingPeerContentStaging
	cleaned bool
}

func (s *recordingPeerContentStagingWithLifecycle) PrepareContent(
	_ context.Context,
	items []StagedSubmissionItem,
) ([]WorkContentPart, error) {
	if len(items) != 1 {
		return nil, nil
	}
	return []WorkContentPart{{
		Type: WorkContentPartTypeImage,
		URL:  "file:///peer.png",
	}}, nil
}

func (s *recordingPeerContentStagingWithLifecycle) ResolveContent(
	_ context.Context,
	ref string,
) (ResolvedStagedContent, error) {
	return ResolvedStagedContent{Path: "/tmp/peer.png", URL: "file:///" + ref}, nil
}

func (s *recordingPeerContentStagingWithLifecycle) CleanupContent(_ context.Context, _ string) error {
	s.cleaned = true
	return nil
}
