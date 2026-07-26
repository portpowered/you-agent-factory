package work_test

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
)

// peerWorkRootConsumer is a peer-shaped characterization consumer (Factory
// Sessions / Workers style) that depends only on the Work root package contract
// surface. It must not import work/service, work/materialize, Factory Runtime,
// Petri, or other peer implementation packages to call or assert published
// slices.
type peerWorkRootConsumer struct {
	root work.Service
}

func (c peerWorkRootConsumer) admit(
	ctx context.Context,
	sessionID string,
	request work.WorkRequest,
) (work.WorkRequestSubmitResult, error) {
	return c.root.SubmitWorkRequestForSession(ctx, sessionID, request)
}

func (c peerWorkRootConsumer) stageAndMaterialize(
	ctx context.Context,
	stage work.StageContentRequest,
	contentURL string,
) (work.StageContentResult, string, work.ContentCleanup, error) {
	staged, err := c.root.StageContent(ctx, stage)
	if err != nil {
		return work.StageContentResult{}, "", nil, err
	}
	if _, err := c.root.PrepareContent(ctx, []work.StagedSubmissionItem{{
		ItemType:      stage.ItemType,
		StagedFileRef: staged.StagedFileRef,
		FileName:      staged.FileName,
		MediaType:     staged.MediaType,
	}}); err != nil {
		return work.StageContentResult{}, "", nil, err
	}
	if _, err := c.root.ResolveContent(ctx, staged.StagedFileRef); err != nil {
		return work.StageContentResult{}, "", nil, err
	}
	if err := c.root.CleanupContent(ctx, staged.StagedFileRef); err != nil {
		return work.StageContentResult{}, "", nil, err
	}
	path, cleanup, err := c.root.MaterializeContentURL(ctx, contentURL)
	return staged, path, cleanup, err
}

func (c peerWorkRootConsumer) inspectAndMove(
	ctx context.Context,
	sessionID string,
	workID string,
	listOpts work.ListOptions,
	toState string,
	requestID string,
) (work.ListResult, work.ReadModel, work.OperatorMoveResult, work.ReadModel, error) {
	listed, err := c.root.ListWork(ctx, sessionID, listOpts)
	if err != nil {
		return work.ListResult{}, work.ReadModel{}, work.OperatorMoveResult{}, work.ReadModel{}, err
	}
	got, err := c.root.GetWork(ctx, sessionID, workID)
	if err != nil {
		return work.ListResult{}, work.ReadModel{}, work.OperatorMoveResult{}, work.ReadModel{}, err
	}
	moved, err := c.root.MoveWorkForSession(ctx, sessionID, workID, toState, requestID)
	if err != nil {
		return work.ListResult{}, work.ReadModel{}, work.OperatorMoveResult{}, work.ReadModel{}, err
	}
	readAfter, err := c.root.MoveWorkAndRead(ctx, sessionID, workID, toState, requestID+"-read")
	return listed, got, moved, readAfter, err
}

func (c peerWorkRootConsumer) prepareAndSelectPrimary(
	ctx context.Context,
	prepare work.InvocationInputPreparationRequest,
	selectInput work.PrimaryResultSelectionInput,
) (work.PreparedInvocationInput, work.PrimaryResultSelection, error) {
	prepared, err := c.root.PrepareInvocationInput(ctx, prepare)
	if err != nil {
		return work.PreparedInvocationInput{}, work.PrimaryResultSelection{}, err
	}
	selected, err := c.root.ResolvePrimaryResult(ctx, selectInput)
	return prepared, selected, err
}

func newSealRootServiceFake(stdin string) *rootServiceFake {
	return &rootServiceFake{
		submitResult: work.WorkRequestSubmitResult{
			RequestID: "seal-request-1",
			TraceID:   "seal-trace-1",
			Accepted:  true,
			Works: []work.WorkRequestSubmittedWork{{
				Name: "seal-work", WorkTypeName: "story", WorkID: "seal-work-1",
			}},
		},
		stageResult: work.StageContentResult{
			StagedFileRef: "submit-work-stage:v1:seal-ref",
			FileName:      "seal.png",
			MediaType:     "image/png",
			URL:           "file:///tmp/seal.png",
		},
		prepareResult: []work.WorkContentPart{{
			Type: work.WorkContentPartTypeImage, URL: "file:///tmp/seal.png", ContentType: "image/png",
		}},
		resolveResult:   work.ResolvedStagedContent{Path: "/tmp/seal.png", URL: "file:///tmp/seal.png"},
		materializePath: "/tmp/materialized/seal.png",
		listResult: work.ListResult{
			Results: []work.ReadModel{{
				CursorID: "seal-work-1", Name: "seal-work", WorkID: "seal-work-1",
				WorkTypeName: "story", State: &work.State{Name: "review", Type: work.StateTypeProcessing},
			}},
			MaxResults: work.DefaultListMaxResults,
		},
		getResult: work.ReadModel{
			CursorID: "seal-work-1", Name: "seal-work", WorkID: "seal-work-1",
			WorkTypeName: "story", State: &work.State{Name: "review", Type: work.StateTypeProcessing},
		},
		moveResult: work.OperatorMoveResult{WorkID: "seal-work-1", FromState: "draft", ToState: "done"},
		movedRead: work.ReadModel{
			CursorID: "seal-work-1", Name: "seal-work", WorkID: "seal-work-1",
			WorkTypeName: "story", State: &work.State{Name: "done", Type: work.StateTypeTerminal},
		},
		prepareInvocationResult: work.PreparedInvocationInput{
			Source:        work.InputSourceStdinText,
			ResolvedInput: &work.ResolvedInput{Source: work.InputSourceStdinText, Text: stdin},
		},
		primaryResult: work.PrimaryResultSelection{
			RequestID: "seal-request-1", Policy: work.ReturnPolicySubmittedWorkTerminal,
			WorkID: "seal-work-1", WorkTypeName: "story", WorkName: "seal-work",
			TerminalState: "story:done",
			PrimaryResult: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "sealed primary"}},
		},
	}
}

func TestServiceRootContract_SealAllPublishedSlicesThroughSingularService(t *testing.T) {
	stdin := "peer seal stdin"
	var root work.Service = newSealRootServiceFake(stdin)
	peer := peerWorkRootConsumer{root: root}
	ctx := context.Background()

	admitted, err := peer.admit(ctx, "session-seal", work.WorkRequest{
		RequestID: "seal-request-1",
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		Works:     []work.Work{{Name: "seal-work", WorkTypeID: "story", State: "draft"}},
	})
	if err != nil {
		t.Fatalf("admission slice: %v", err)
	}
	if !admitted.Accepted || admitted.RequestID != "seal-request-1" {
		t.Fatalf("admission result = %#v, want accepted seal-request-1", admitted)
	}

	assertSealContentSlice(t, peer, ctx)
	assertSealStateAccessSlice(t, peer, ctx)
	assertSealInvocationSlice(t, peer, ctx, stdin)
}

func assertSealContentSlice(t *testing.T, peer peerWorkRootConsumer, ctx context.Context) {
	t.Helper()
	staged, localPath, cleanup, err := peer.stageAndMaterialize(
		ctx,
		work.StageContentRequest{
			ItemType: "image", FileName: "seal.png", MediaType: "image/png", Content: []byte("png"),
		},
		"file:///fixtures/seal.png",
	)
	if err != nil {
		t.Fatalf("content slice: %v", err)
	}
	if staged.StagedFileRef == "" || localPath == "" || cleanup == nil {
		t.Fatalf("content outcomes = (%#v, %q, %v)", staged, localPath, cleanup)
	}
	cleanup()
}

func assertSealStateAccessSlice(t *testing.T, peer peerWorkRootConsumer, ctx context.Context) {
	t.Helper()
	listed, got, moved, readAfter, err := peer.inspectAndMove(
		ctx, "session-seal", "seal-work-1", work.ListOptions{WorkTypeName: "story"}, "done", "seal-move-1",
	)
	if err != nil {
		t.Fatalf("state-access slice: %v", err)
	}
	if len(listed.Results) != 1 || got.WorkID != "seal-work-1" {
		t.Fatalf("state-access list/get = (%#v, %#v)", listed, got)
	}
	if moved.ToState != "done" || readAfter.State == nil || readAfter.State.Type != work.StateTypeTerminal {
		t.Fatalf("state-access move = (%#v, %#v)", moved, readAfter)
	}
}

func assertSealInvocationSlice(t *testing.T, peer peerWorkRootConsumer, ctx context.Context, stdin string) {
	t.Helper()
	prepared, primary, err := peer.prepareAndSelectPrimary(
		ctx,
		work.InvocationInputPreparationRequest{Arguments: []string{"-"}, StdinText: &stdin},
		work.PrimaryResultSelectionInput{
			RequestID: "seal-request-1",
			InvocationReturn: &work.InvocationReturnConfig{
				Policy: work.ReturnPolicySubmittedWorkTerminal,
			},
			WorldState: work.InvocationWorldState{},
		},
	)
	if err != nil {
		t.Fatalf("invocation/return-policy slice: %v", err)
	}
	if prepared.ResolvedInput == nil || prepared.ResolvedInput.Text != stdin {
		t.Fatalf("prepared input = %#v, want sealed stdin", prepared)
	}
	if primary.Policy != work.ReturnPolicySubmittedWorkTerminal || len(primary.PrimaryResult) != 1 {
		t.Fatalf("primary result = %#v, want submitted-terminal content", primary)
	}
}

type sealTypedFailureCase struct {
	name string
	want error
	call func(peerWorkRootConsumer) error
}

func sealTypedFailureCases(ctx context.Context) []sealTypedFailureCase {
	cases := sealAdmissionAndContentFailureCases(ctx)
	return append(cases, sealStateAndInvocationFailureCases(ctx)...)
}

func sealAdmissionAndContentFailureCases(ctx context.Context) []sealTypedFailureCase {
	return []sealTypedFailureCase{
		{
			name: "admission invalid",
			want: work.ErrInvalidWorkRequest,
			call: func(peer peerWorkRootConsumer) error {
				_, err := peer.admit(ctx, "session-seal", work.WorkRequest{RequestID: "bad"})
				return err
			},
		},
		{
			name: "admission conflict",
			want: work.ErrWorkRequestConflict,
			call: func(peer peerWorkRootConsumer) error {
				_, err := peer.admit(ctx, "session-seal", work.WorkRequest{RequestID: "dup"})
				return err
			},
		},
		{
			name: "admission rejected",
			want: work.ErrWorkRequestRejected,
			call: func(peer peerWorkRootConsumer) error {
				_, err := peer.admit(ctx, "session-seal", work.WorkRequest{RequestID: "rejected"})
				return err
			},
		},
		{
			name: "content invalid staged ref",
			want: work.ErrInvalidStagedContentRef,
			call: func(peer peerWorkRootConsumer) error {
				_, err := peer.root.ResolveContent(ctx, "not-a-ref")
				return err
			},
		},
		{
			name: "content expired staged ref",
			want: work.ErrStagedContentExpired,
			call: func(peer peerWorkRootConsumer) error {
				_, err := peer.root.ResolveContent(ctx, "submit-work-stage:v1:expired")
				return err
			},
		},
		{
			name: "content unsafe URL",
			want: work.ErrUnsafeContentURL,
			call: func(peer peerWorkRootConsumer) error {
				_, _, err := peer.root.MaterializeContentURL(ctx, "http://127.0.0.1/secret")
				return err
			},
		},
		{
			name: "content inaccessible URL",
			want: work.ErrContentURLInaccessible,
			call: func(peer peerWorkRootConsumer) error {
				_, _, err := peer.root.MaterializeContentURL(ctx, "https://example.invalid/missing")
				return err
			},
		},
	}
}

func sealStateAndInvocationFailureCases(ctx context.Context) []sealTypedFailureCase {
	return []sealTypedFailureCase{
		{
			name: "state-access missing work",
			want: work.ErrWorkNotFound,
			call: func(peer peerWorkRootConsumer) error {
				_, err := peer.root.GetWork(ctx, "session-seal", "missing")
				return err
			},
		},
		{
			name: "state-access already-applied move",
			want: work.ErrMoveWorkRequestAlreadyApplied,
			call: func(peer peerWorkRootConsumer) error {
				_, err := peer.root.MoveWorkForSession(ctx, "session-seal", "work-1", "done", "dup")
				return err
			},
		},
		{
			name: "invocation invalid input",
			want: work.ErrInvalidInvocationInput,
			call: func(peer peerWorkRootConsumer) error {
				_, err := peer.root.PrepareInvocationInput(ctx, work.InvocationInputPreparationRequest{
					Arguments: []string{""},
				})
				return err
			},
		},
		{
			name: "return-policy unsupported",
			want: work.ErrUnsupportedReturnPolicy,
			call: func(peer peerWorkRootConsumer) error {
				_, err := peer.root.ResolvePrimaryResult(ctx, work.PrimaryResultSelectionInput{
					RequestID: "bad-policy",
					InvocationReturn: &work.InvocationReturnConfig{
						Policy: "NOT_A_POLICY",
					},
				})
				return err
			},
		},
	}
}

func TestServiceRootContract_SealTypedFailuresPerPublishedSlice(t *testing.T) {
	ctx := context.Background()
	for _, tc := range sealTypedFailureCases(ctx) {
		t.Run(tc.name, func(t *testing.T) {
			fake := &rootServiceFake{
				submitErr:            tc.want,
				resolveErr:           tc.want,
				materializeErr:       tc.want,
				getErr:               tc.want,
				moveErr:              tc.want,
				prepareInvocationErr: tc.want,
				primaryResultErr:     tc.want,
			}
			peer := peerWorkRootConsumer{root: fake}
			if err := tc.call(peer); !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
		})
	}
}
