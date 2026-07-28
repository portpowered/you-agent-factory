package runtime_api

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
)

// TestWorkRootPolicySlicesRejectUnsupportedOperations proves published Work root
// policy/materialization/admission binders fail closed on slices they do not own.
// The functional lane measures coverage for pkg/services/work when these tests run.
func TestWorkRootPolicySlicesRejectUnsupportedOperations(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	policy := work.NewInvocationPolicyService()
	materialization := work.MaterializationService(work.ContentMaterializeFunc(func(context.Context, string) (string, work.ContentCleanup, error) {
		return "/tmp/materialized", func() {}, nil
	}))
	admissionPrep, err := work.NewRequestPreparationService(work.NewContentPreparation())
	if err != nil {
		t.Fatalf("NewRequestPreparationService: %v", err)
	}
	admission := work.AdmissionContentService(
		&functionalPeerContentStaging{},
		admissionPrep,
	)

	for _, tc := range []struct {
		name    string
		service work.Service
		call    func(work.Service) error
		want    string
	}{
		{
			name:    "policy admission",
			service: policy,
			call: func(service work.Service) error {
				_, err := service.SubmitWorkRequestForSession(ctx, "session-1", work.WorkRequest{})
				return err
			},
			want: "does not support admission",
		},
		{
			name:    "policy content staging",
			service: policy,
			call: func(service work.Service) error {
				_, err := service.StageContent(ctx, work.StageContentRequest{})
				return err
			},
			want: "does not support content staging",
		},
		{
			name:    "materialization admission prep",
			service: materialization,
			call: func(service work.Service) error {
				_, err := service.PrepareWorkRequest(ctx, work.WorkRequestPreparation{})
				return err
			},
			want: "does not support admission prep",
		},
		{
			name:    "materialization invocation policy",
			service: materialization,
			call: func(service work.Service) error {
				_, err := service.ResolvePrimaryResult(ctx, work.PrimaryResultSelectionInput{})
				return err
			},
			want: "does not support invocation policy",
		},
		{
			name:    "admission materialization",
			service: admission,
			call: func(service work.Service) error {
				_, _, err := service.MaterializeContentURL(ctx, "file:///peer.png")
				return err
			},
			want: "does not support content materialization",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := tc.call(tc.service); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestWorkRootPolicyServicePrepareInvocationInputRejectsWhitespaceOnlyText(t *testing.T) {
	t.Parallel()

	_, err := work.NewInvocationPolicyService().PrepareInvocationInput(context.Background(), work.InvocationInputPreparationRequest{
		CompatibilityContent: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "   "}},
	})
	if err == nil || !errors.Is(err, work.ErrInvalidInvocationInput) {
		t.Fatalf("error = %v, want ErrInvalidInvocationInput", err)
	}
}

type functionalPeerContentStaging struct{}

func (functionalPeerContentStaging) StageContent(
	_ context.Context,
	request work.StageContentRequest,
) (work.StageContentResult, error) {
	return work.StageContentResult{
		StagedFileRef: "functional-stage-ref",
		FileName:      request.FileName,
		MediaType:     request.MediaType,
	}, nil
}

func (functionalPeerContentStaging) PrepareContent(context.Context, []work.StagedSubmissionItem) ([]work.WorkContentPart, error) {
	return nil, nil
}

func (functionalPeerContentStaging) ResolveContent(context.Context, string) (work.ResolvedStagedContent, error) {
	return work.ResolvedStagedContent{}, nil
}

func (functionalPeerContentStaging) CleanupContent(context.Context, string) error {
	return nil
}
