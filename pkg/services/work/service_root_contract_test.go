package work_test

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
)

// rootServiceFake is a peer-shaped Work root Service that uses only Work-owned
// request, result, value, and typed-error contracts.
type rootServiceFake struct {
	submitResult work.WorkRequestSubmitResult
	submitErr    error
	moveResult   work.OperatorMoveResult
	moveErr      error
	listResult   work.ListResult
	listErr      error
	getResult    work.ReadModel
	getErr       error
	movedRead    work.ReadModel
	movedReadErr error

	stageResult      work.StageContentResult
	stageErr         error
	prepareResult    []work.WorkContentPart
	prepareErr       error
	resolveResult    work.ResolvedStagedContent
	resolveErr       error
	cleanupErr       error
	materializePath  string
	materializeClean work.ContentCleanup
	materializeErr   error

	prepareInvocationResult work.PreparedInvocationInput
	prepareInvocationErr    error
	primaryResult           work.PrimaryResultSelection
	primaryResultErr        error

	lastSessionID      string
	lastRequest        work.WorkRequest
	lastListOpts       work.ListOptions
	lastWorkID         string
	lastStateName      string
	lastRequestID      string
	lastStageRequest   work.StageContentRequest
	lastPrepareItems   []work.StagedSubmissionItem
	lastStagedRef      string
	lastContentURL     string
	lastInvocationReq  work.InvocationInputPreparationRequest
	lastPrimaryInput   work.PrimaryResultSelectionInput
	cleanupCalled      bool
	materializeCleaned bool
}

func (f *rootServiceFake) SubmitWorkRequestForSession(
	_ context.Context,
	sessionID string,
	request work.WorkRequest,
) (work.WorkRequestSubmitResult, error) {
	f.lastSessionID = sessionID
	f.lastRequest = request
	return f.submitResult, f.submitErr
}

func (f *rootServiceFake) MoveWorkForSession(
	_ context.Context,
	sessionID string,
	workID string,
	stateName string,
	requestID string,
) (work.OperatorMoveResult, error) {
	f.lastSessionID = sessionID
	f.lastWorkID = workID
	f.lastStateName = stateName
	f.lastRequestID = requestID
	return f.moveResult, f.moveErr
}

func (f *rootServiceFake) ListWork(
	_ context.Context,
	sessionID string,
	options work.ListOptions,
) (work.ListResult, error) {
	f.lastSessionID = sessionID
	f.lastListOpts = options
	return f.listResult, f.listErr
}

func (f *rootServiceFake) GetWork(
	_ context.Context,
	sessionID string,
	id string,
) (work.ReadModel, error) {
	f.lastSessionID = sessionID
	f.lastWorkID = id
	return f.getResult, f.getErr
}

func (f *rootServiceFake) MoveWorkAndRead(
	_ context.Context,
	sessionID string,
	id string,
	stateName string,
	requestID string,
) (work.ReadModel, error) {
	f.lastSessionID = sessionID
	f.lastWorkID = id
	f.lastStateName = stateName
	f.lastRequestID = requestID
	return f.movedRead, f.movedReadErr
}

func (f *rootServiceFake) StageContent(
	_ context.Context,
	request work.StageContentRequest,
) (work.StageContentResult, error) {
	f.lastStageRequest = request
	return f.stageResult, f.stageErr
}

func (f *rootServiceFake) PrepareContent(
	_ context.Context,
	items []work.StagedSubmissionItem,
) ([]work.WorkContentPart, error) {
	f.lastPrepareItems = append([]work.StagedSubmissionItem(nil), items...)
	return f.prepareResult, f.prepareErr
}

func (f *rootServiceFake) ResolveContent(
	_ context.Context,
	ref string,
) (work.ResolvedStagedContent, error) {
	f.lastStagedRef = ref
	return f.resolveResult, f.resolveErr
}

func (f *rootServiceFake) CleanupContent(_ context.Context, ref string) error {
	f.lastStagedRef = ref
	f.cleanupCalled = true
	return f.cleanupErr
}

func (f *rootServiceFake) MaterializeContentURL(
	_ context.Context,
	rawURL string,
) (string, work.ContentCleanup, error) {
	f.lastContentURL = rawURL
	cleanup := f.materializeClean
	if cleanup == nil && f.materializeErr == nil {
		cleanup = func() { f.materializeCleaned = true }
	}
	return f.materializePath, cleanup, f.materializeErr
}

func (f *rootServiceFake) PrepareInvocationInput(
	_ context.Context,
	request work.InvocationInputPreparationRequest,
) (work.PreparedInvocationInput, error) {
	f.lastInvocationReq = request
	return f.prepareInvocationResult, f.prepareInvocationErr
}

func (f *rootServiceFake) ResolvePrimaryResult(
	_ context.Context,
	input work.PrimaryResultSelectionInput,
) (work.PrimaryResultSelection, error) {
	f.lastPrimaryInput = input
	return f.primaryResult, f.primaryResultErr
}

var _ work.Service = (*rootServiceFake)(nil)

func newRootSeamExerciseFake() *rootServiceFake {
	return &rootServiceFake{
		submitResult: work.WorkRequestSubmitResult{
			RequestID: "request-1",
			TraceID:   "trace-1",
			Accepted:  true,
			Works: []work.WorkRequestSubmittedWork{{
				Name: "story-1", WorkTypeName: "story", WorkID: "work-1",
			}},
		},
		moveResult: work.OperatorMoveResult{
			WorkID: "work-1", WorkTypeID: "story", FromState: "draft", ToState: "review",
		},
		listResult: work.ListResult{
			Results: []work.ReadModel{{
				CursorID: "work-1", Name: "story-1", WorkID: "work-1",
				WorkTypeName: "story", State: &work.State{Name: "review", Type: work.StateTypeProcessing},
			}},
			MaxResults: work.DefaultListMaxResults,
		},
		getResult: work.ReadModel{
			CursorID: "work-1", Name: "story-1", WorkID: "work-1",
			WorkTypeName: "story", State: &work.State{Name: "review", Type: work.StateTypeProcessing},
		},
		movedRead: work.ReadModel{
			CursorID: "work-1", Name: "story-1", WorkID: "work-1",
			WorkTypeName: "story", State: &work.State{Name: "done", Type: work.StateTypeTerminal},
		},
	}
}

func TestServiceRootContract_FakeImplementsAndExercisesSeam(t *testing.T) {
	fake := newRootSeamExerciseFake()
	// Peers consume only the singular root Service seam.
	var service work.Service = fake
	ctx := context.Background()

	submit, err := service.SubmitWorkRequestForSession(ctx, "session-1", work.WorkRequest{
		RequestID: "request-1",
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		Works:     []work.Work{{Name: "story-1", WorkTypeID: "story", State: "draft"}},
	})
	if err != nil {
		t.Fatalf("SubmitWorkRequestForSession: %v", err)
	}
	if !submit.Accepted || submit.RequestID != "request-1" || len(submit.Works) != 1 {
		t.Fatalf("submit result = %#v, want accepted request-1 with one work", submit)
	}
	if fake.lastSessionID != "session-1" || fake.lastRequest.RequestID != "request-1" {
		t.Fatalf("submit routed = (%q, %q)", fake.lastSessionID, fake.lastRequest.RequestID)
	}

	move, err := service.MoveWorkForSession(ctx, "session-1", "work-1", "review", "move-1")
	if err != nil {
		t.Fatalf("MoveWorkForSession: %v", err)
	}
	if move.WorkID != "work-1" || move.ToState != "review" {
		t.Fatalf("move result = %#v, want work-1 -> review", move)
	}

	listed, err := service.ListWork(ctx, "session-1", work.ListOptions{WorkTypeName: "story"})
	if err != nil {
		t.Fatalf("ListWork: %v", err)
	}
	if len(listed.Results) != 1 || listed.Results[0].WorkID != "work-1" {
		t.Fatalf("list result = %#v, want one work-1 entry", listed)
	}
	if fake.lastListOpts.WorkTypeName != "story" {
		t.Fatalf("list options = %#v, want workTypeName=story", fake.lastListOpts)
	}

	got, err := service.GetWork(ctx, "session-1", "work-1")
	if err != nil {
		t.Fatalf("GetWork: %v", err)
	}
	if got.WorkID != "work-1" || got.State == nil || got.State.Name != "review" {
		t.Fatalf("get result = %#v, want work-1 in review", got)
	}

	moved, err := service.MoveWorkAndRead(ctx, "session-1", "work-1", "done", "move-2")
	if err != nil {
		t.Fatalf("MoveWorkAndRead: %v", err)
	}
	if moved.WorkID != "work-1" || moved.State == nil || moved.State.Type != work.StateTypeTerminal {
		t.Fatalf("move-and-read result = %#v, want terminal work-1", moved)
	}
}

func TestServiceRootContract_TypedFailuresRemainDistinguishable(t *testing.T) {
	fake := &rootServiceFake{
		getErr:  work.ErrWorkNotFound,
		moveErr: work.ErrMoveWorkRequestAlreadyApplied,
	}
	var service work.Service = fake
	ctx := context.Background()

	_, err := service.GetWork(ctx, "session-1", "missing")
	if !errors.Is(err, work.ErrWorkNotFound) {
		t.Fatalf("GetWork error = %v, want ErrWorkNotFound", err)
	}

	_, err = service.MoveWorkForSession(ctx, "session-1", "work-1", "done", "dup-move")
	if !errors.Is(err, work.ErrMoveWorkRequestAlreadyApplied) {
		t.Fatalf("MoveWorkForSession error = %v, want ErrMoveWorkRequestAlreadyApplied", err)
	}
}

func TestServiceRootContract_AdmissionSliceSuccess(t *testing.T) {
	fake := &rootServiceFake{
		submitResult: work.WorkRequestSubmitResult{
			RequestID: "request-admit-1",
			TraceID:   "trace-admit-1",
			Accepted:  true,
			Works: []work.WorkRequestSubmittedWork{{
				Name:         "story-admit",
				WorkTypeName: "story",
				WorkID:       "work-admit-1",
			}},
		},
	}
	var service work.Service = fake
	ctx := context.Background()

	request := work.WorkRequest{
		RequestID: "request-admit-1",
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{{
			Name:       "story-admit",
			WorkTypeID: "story",
			State:      "draft",
			Payload:    map[string]any{"title": "admit"},
		}},
	}
	result, err := service.SubmitWorkRequestForSession(ctx, "session-admit", request)
	if err != nil {
		t.Fatalf("SubmitWorkRequestForSession: %v", err)
	}
	if !result.Accepted || result.RequestID != "request-admit-1" {
		t.Fatalf("admission result = %#v, want accepted request-admit-1", result)
	}
	if len(result.Works) != 1 || result.Works[0].WorkID != "work-admit-1" {
		t.Fatalf("admission works = %#v, want work-admit-1", result.Works)
	}
	if fake.lastSessionID != "session-admit" || fake.lastRequest.RequestID != "request-admit-1" {
		t.Fatalf("admission routed = (%q, %q)", fake.lastSessionID, fake.lastRequest.RequestID)
	}
	if len(fake.lastRequest.Works) != 1 || fake.lastRequest.Works[0].Name != "story-admit" {
		t.Fatalf("admission payload = %#v, want story-admit work", fake.lastRequest.Works)
	}
}

func TestServiceRootContract_AdmissionTypedFailures(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{name: "invalid", err: work.ErrInvalidWorkRequest},
		{name: "conflict", err: work.ErrWorkRequestConflict},
		{name: "rejected", err: work.ErrWorkRequestRejected},
	}
	ctx := context.Background()
	request := work.WorkRequest{
		RequestID: "request-bad",
		Type:      work.WorkRequestTypeFactoryRequestBatch,
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &rootServiceFake{submitErr: tc.err}
			var service work.Service = fake

			_, err := service.SubmitWorkRequestForSession(ctx, "session-1", request)
			if !errors.Is(err, tc.err) {
				t.Fatalf("SubmitWorkRequestForSession error = %v, want %v", err, tc.err)
			}
		})
	}
}

func TestServiceRootContract_StateAccessSliceSuccess(t *testing.T) {
	fake := &rootServiceFake{
		listResult: work.ListResult{
			Results: []work.ReadModel{{
				CursorID:     "cursor-work-1",
				Name:         "story-1",
				WorkID:       "work-1",
				WorkTypeName: "story",
				State:        &work.State{Name: "review", Type: work.StateTypeProcessing},
				TraceID:      "trace-1",
			}},
			MaxResults: 10,
			NextToken:  "",
		},
		getResult: work.ReadModel{
			CursorID:     "cursor-work-1",
			Name:         "story-1",
			WorkID:       "work-1",
			WorkTypeName: "story",
			State:        &work.State{Name: "review", Type: work.StateTypeProcessing},
			TraceID:      "trace-1",
		},
		moveResult: work.OperatorMoveResult{
			WorkID:     "work-1",
			WorkTypeID: "story",
			FromState:  "draft",
			ToState:    "review",
		},
		movedRead: work.ReadModel{
			CursorID:     "cursor-work-1",
			Name:         "story-1",
			WorkID:       "work-1",
			WorkTypeName: "story",
			State:        &work.State{Name: "done", Type: work.StateTypeTerminal},
			TraceID:      "trace-1",
		},
	}
	var service work.Service = fake
	ctx := context.Background()

	listed, err := service.ListWork(ctx, "session-state", work.ListOptions{
		WorkTypeName: "story",
		StateName:    "review",
		MaxResults:   10,
	})
	if err != nil {
		t.Fatalf("ListWork: %v", err)
	}
	if len(listed.Results) != 1 {
		t.Fatalf("list result = %#v, want one detached ReadModel", listed)
	}
	assertDetachedReadModel(t, listed.Results[0], "work-1", "review", work.StateTypeProcessing)
	if fake.lastSessionID != "session-state" || fake.lastListOpts.WorkTypeName != "story" {
		t.Fatalf("list routed = (%q, %#v)", fake.lastSessionID, fake.lastListOpts)
	}

	got, err := service.GetWork(ctx, "session-state", "work-1")
	if err != nil {
		t.Fatalf("GetWork: %v", err)
	}
	assertDetachedReadModel(t, got, "work-1", "review", work.StateTypeProcessing)

	moved, err := service.MoveWorkForSession(ctx, "session-state", "work-1", "review", "move-state-1")
	if err != nil {
		t.Fatalf("MoveWorkForSession: %v", err)
	}
	if moved.WorkID != "work-1" || moved.FromState != "draft" || moved.ToState != "review" {
		t.Fatalf("move result = %#v, want detached work-1 draft->review", moved)
	}
	if fake.lastRequestID != "move-state-1" {
		t.Fatalf("move requestID = %q, want move-state-1", fake.lastRequestID)
	}

	readAfterMove, err := service.MoveWorkAndRead(ctx, "session-state", "work-1", "done", "move-state-2")
	if err != nil {
		t.Fatalf("MoveWorkAndRead: %v", err)
	}
	assertDetachedReadModel(t, readAfterMove, "work-1", "done", work.StateTypeTerminal)
	if fake.lastStateName != "done" || fake.lastRequestID != "move-state-2" {
		t.Fatalf("move-and-read routed = (%q, %q)", fake.lastStateName, fake.lastRequestID)
	}
}

func TestServiceRootContract_StateAccessTypedFailures(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name string
		call func(work.Service) error
		want error
	}{
		{
			name: "missing work on get",
			call: func(service work.Service) error {
				_, err := service.GetWork(ctx, "session-1", "missing")
				return err
			},
			want: work.ErrWorkNotFound,
		},
		{
			name: "missing work on move-and-read",
			call: func(service work.Service) error {
				_, err := service.MoveWorkAndRead(ctx, "session-1", "missing", "done", "move-missing")
				return err
			},
			want: work.ErrWorkNotFound,
		},
		{
			name: "already-applied move",
			call: func(service work.Service) error {
				_, err := service.MoveWorkForSession(ctx, "session-1", "work-1", "done", "dup-move")
				return err
			},
			want: work.ErrMoveWorkRequestAlreadyApplied,
		},
		{
			name: "already-applied move-and-read",
			call: func(service work.Service) error {
				_, err := service.MoveWorkAndRead(ctx, "session-1", "work-1", "done", "dup-move")
				return err
			},
			want: work.ErrMoveWorkRequestAlreadyApplied,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &rootServiceFake{
				getErr:       work.ErrWorkNotFound,
				movedReadErr: tc.want,
				moveErr:      work.ErrMoveWorkRequestAlreadyApplied,
			}
			var service work.Service = fake
			if err := tc.call(service); !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
		})
	}
}

func assertDetachedReadModel(
	t *testing.T,
	model work.ReadModel,
	wantWorkID string,
	wantStateName string,
	wantStateType string,
) {
	t.Helper()
	if model.WorkID != wantWorkID {
		t.Fatalf("ReadModel.WorkID = %q, want %q", model.WorkID, wantWorkID)
	}
	if model.Name == "" || model.WorkTypeName == "" {
		t.Fatalf("ReadModel missing customer identity fields: %#v", model)
	}
	if model.State == nil || model.State.Name != wantStateName || model.State.Type != wantStateType {
		t.Fatalf("ReadModel.State = %#v, want %s/%s", model.State, wantStateName, wantStateType)
	}
	// Peers consume customer-facing projection fields only; CursorID is the
	// Work-owned list cursor, not a Petri token/place/marking/topology handle.
	if model.CursorID == "" {
		t.Fatalf("ReadModel.CursorID empty; want opaque Work list cursor")
	}
}

func TestServiceRootContract_ContentStagingAndMaterializationSuccess(t *testing.T) {
	fake := &rootServiceFake{
		stageResult: work.StageContentResult{
			StagedFileRef: "submit-work-stage:v1:opaque-ref",
			FileName:      "photo.png",
			MediaType:     "image/png",
			URL:           "file:///tmp/staged/photo.png",
		},
		prepareResult: []work.WorkContentPart{{
			Type:        work.WorkContentPartTypeImage,
			URL:         "file:///tmp/staged/photo.png",
			ContentType: "image/png",
		}},
		resolveResult: work.ResolvedStagedContent{
			Path: "/tmp/staged/photo.png",
			URL:  "file:///tmp/staged/photo.png",
		},
		materializePath: "/tmp/materialized/photo.png",
	}
	var service work.Service = fake
	ctx := context.Background()

	staged, err := service.StageContent(ctx, work.StageContentRequest{
		ItemType:  "image",
		FileName:  "photo.png",
		MediaType: "image/png",
		Content:   []byte("png-bytes"),
	})
	if err != nil {
		t.Fatalf("StageContent: %v", err)
	}
	if staged.StagedFileRef == "" || staged.URL == "" {
		t.Fatalf("stage result = %#v, want opaque staged ref and URL", staged)
	}
	if fake.lastStageRequest.FileName != "photo.png" || len(fake.lastStageRequest.Content) == 0 {
		t.Fatalf("stage request = %#v, want photo.png payload", fake.lastStageRequest)
	}

	prepared, err := service.PrepareContent(ctx, []work.StagedSubmissionItem{{
		ItemType:      "image",
		StagedFileRef: staged.StagedFileRef,
		FileName:      "photo.png",
		MediaType:     "image/png",
	}})
	if err != nil {
		t.Fatalf("PrepareContent: %v", err)
	}
	if len(prepared) != 1 || prepared[0].URL == "" {
		t.Fatalf("prepare result = %#v, want one content part with URL", prepared)
	}

	resolved, err := service.ResolveContent(ctx, staged.StagedFileRef)
	if err != nil {
		t.Fatalf("ResolveContent: %v", err)
	}
	if resolved.Path == "" || resolved.URL == "" {
		t.Fatalf("resolve result = %#v, want local path and URL", resolved)
	}
	if fake.lastStagedRef != staged.StagedFileRef {
		t.Fatalf("resolve ref = %q, want staged ref", fake.lastStagedRef)
	}

	if err := service.CleanupContent(ctx, staged.StagedFileRef); err != nil {
		t.Fatalf("CleanupContent: %v", err)
	}
	if !fake.cleanupCalled {
		t.Fatal("CleanupContent was not routed through the root Service")
	}

	localPath, cleanup, err := service.MaterializeContentURL(ctx, "file:///fixtures/photo.png")
	if err != nil {
		t.Fatalf("MaterializeContentURL: %v", err)
	}
	if localPath == "" || cleanup == nil {
		t.Fatalf("materialize = (%q, %v), want local path and cleanup handle", localPath, cleanup)
	}
	if fake.lastContentURL != "file:///fixtures/photo.png" {
		t.Fatalf("materialize URL = %q, want file:///fixtures/photo.png", fake.lastContentURL)
	}
	cleanup()
	if !fake.materializeCleaned {
		t.Fatal("materialize cleanup handle did not run")
	}
}

func TestServiceRootContract_ContentTypedFailures(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name string
		call func(work.Service) error
		want error
	}{
		{
			name: "invalid staged ref",
			call: func(service work.Service) error {
				_, err := service.ResolveContent(ctx, "not-a-staged-ref")
				return err
			},
			want: work.ErrInvalidStagedContentRef,
		},
		{
			name: "expired staged ref",
			call: func(service work.Service) error {
				_, err := service.ResolveContent(ctx, "submit-work-stage:v1:expired")
				return err
			},
			want: work.ErrStagedContentExpired,
		},
		{
			name: "unsafe content URL",
			call: func(service work.Service) error {
				_, _, err := service.MaterializeContentURL(ctx, "http://127.0.0.1/secret")
				return err
			},
			want: work.ErrUnsafeContentURL,
		},
		{
			name: "remote content inaccessible",
			call: func(service work.Service) error {
				_, _, err := service.MaterializeContentURL(ctx, "https://example.invalid/missing.png")
				return err
			},
			want: work.ErrContentURLInaccessible,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &rootServiceFake{
				resolveErr:     tc.want,
				materializeErr: tc.want,
			}
			var service work.Service = fake
			if err := tc.call(service); !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestServiceRootContract_InvocationAndReturnPolicySliceSuccess(t *testing.T) {
	stdin := "ship from pipe"
	fake := &rootServiceFake{
		prepareInvocationResult: work.PreparedInvocationInput{
			Source: work.InputSourceStdinText,
			ResolvedInput: &work.ResolvedInput{
				Source: work.InputSourceStdinText,
				Text:   stdin,
			},
		},
		primaryResult: work.PrimaryResultSelection{
			RequestID:     "request-inv-1",
			Policy:        work.ReturnPolicySubmittedWorkTerminal,
			WorkID:        "work-terminal-1",
			WorkTypeName:  "task",
			WorkName:      "root",
			TerminalState: "task:complete",
			PrimaryResult: []work.WorkContentPart{{
				Type: work.WorkContentPartTypeText,
				Text: "terminal output",
			}},
		},
	}
	var service work.Service = fake
	ctx := context.Background()

	prepared, err := service.PrepareInvocationInput(ctx, work.InvocationInputPreparationRequest{
		Arguments: []string{"-"},
		StdinText: &stdin,
	})
	if err != nil {
		t.Fatalf("PrepareInvocationInput: %v", err)
	}
	if prepared.ResolvedInput == nil || prepared.ResolvedInput.Text != stdin {
		t.Fatalf("prepared input = %#v, want detached stdin text", prepared)
	}
	if len(fake.lastInvocationReq.Arguments) != 1 || fake.lastInvocationReq.Arguments[0] != "-" {
		t.Fatalf("prepare request = %#v, want dash argv", fake.lastInvocationReq)
	}

	submittedTerminal, err := service.ResolvePrimaryResult(ctx, work.PrimaryResultSelectionInput{
		RequestID: "request-inv-1",
		InvocationReturn: &work.InvocationReturnConfig{
			Policy: work.ReturnPolicySubmittedWorkTerminal,
		},
		WorldState: work.InvocationWorldState{},
	})
	if err != nil {
		t.Fatalf("ResolvePrimaryResult submitted-terminal: %v", err)
	}
	if submittedTerminal.Policy != work.ReturnPolicySubmittedWorkTerminal ||
		submittedTerminal.WorkID != "work-terminal-1" ||
		len(submittedTerminal.PrimaryResult) != 1 {
		t.Fatalf("submitted-terminal result = %#v, want detached primary content", submittedTerminal)
	}
	if fake.lastPrimaryInput.RequestID != "request-inv-1" {
		t.Fatalf("primary input requestID = %q, want request-inv-1", fake.lastPrimaryInput.RequestID)
	}

	fake.primaryResult = work.PrimaryResultSelection{
		RequestID:     "request-inv-2",
		Policy:        work.ReturnPolicyExplicit,
		WorkID:        "work-summary-1",
		WorkTypeName:  "summary",
		WorkName:      "summary",
		TerminalState: "summary:complete",
		PrimaryResult: []work.WorkContentPart{{
			Type: work.WorkContentPartTypeText,
			Text: "explicit summary",
		}},
	}
	explicit, err := service.ResolvePrimaryResult(ctx, work.PrimaryResultSelectionInput{
		RequestID: "request-inv-2",
		InvocationReturn: &work.InvocationReturnConfig{
			Policy:        work.ReturnPolicyExplicit,
			WorkTypeName:  "summary",
			TerminalState: "summary:complete",
		},
		WorldState: work.InvocationWorldState{},
	})
	if err != nil {
		t.Fatalf("ResolvePrimaryResult explicit: %v", err)
	}
	if explicit.Policy != work.ReturnPolicyExplicit ||
		explicit.WorkID != "work-summary-1" ||
		len(explicit.PrimaryResult) != 1 ||
		explicit.PrimaryResult[0].Text != "explicit summary" {
		t.Fatalf("explicit result = %#v, want configured summary primary content", explicit)
	}
}

func TestServiceRootContract_InvocationAndReturnPolicyTypedFailures(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name string
		call func(work.Service) error
		want error
	}{
		{
			name: "invalid invocation input",
			call: func(service work.Service) error {
				_, err := service.PrepareInvocationInput(ctx, work.InvocationInputPreparationRequest{
					Arguments: []string{""},
				})
				return err
			},
			want: work.ErrInvalidInvocationInput,
		},
		{
			name: "unsupported return policy",
			call: func(service work.Service) error {
				_, err := service.ResolvePrimaryResult(ctx, work.PrimaryResultSelectionInput{
					RequestID: "request-bad-policy",
					InvocationReturn: &work.InvocationReturnConfig{
						Policy: "NOT_A_POLICY",
					},
					WorldState: work.InvocationWorldState{},
				})
				return err
			},
			want: work.ErrUnsupportedReturnPolicy,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &rootServiceFake{
				prepareInvocationErr: work.ErrInvalidInvocationInput,
				primaryResultErr:     work.ErrUnsupportedReturnPolicy,
			}
			var service work.Service = fake
			if err := tc.call(service); !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
		})
	}
}
