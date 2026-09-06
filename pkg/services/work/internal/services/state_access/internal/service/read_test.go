package service_test

import (
	"context"
	"encoding/base64"
	"errors"
	"reflect"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
	internalservice "github.com/portpowered/infinite-you/pkg/services/work/internal/services/state_access/internal/service"
)

func querySnapshot() work.ReadSnapshot {
	return work.ReadSnapshot{Items: []work.ReadModel{
		{CursorID: "tok-story", WorkID: "work-story", Name: "Review PRD", WorkTypeName: "story", TraceID: "trace-root", State: &work.State{Name: "review", Type: work.StateTypeProcessing}},
		{CursorID: "tok-bug", WorkID: "work-bug", Name: "Fix bug", WorkTypeName: "bug", CurrentChainingTraceID: "trace-chain-1", State: &work.State{Name: "init", Type: work.StateTypeInitial}},
		{CursorID: "tok-plan", WorkID: "work-plan", Name: "Plan feature", WorkTypeName: "story", TraceID: "trace-plan", State: &work.State{Name: "complete", Type: work.StateTypeTerminal}},
	}}
}

func TestListWorkReturnsDetachedReadModels(t *testing.T) {
	t.Parallel()

	adapter := &recordingSessionAdapter{snapshot: querySnapshot()}
	svc := internalservice.New(stubSessionResolver{adapter: adapter}, nil)
	ctx := context.Background()

	got, err := svc.ListWork(ctx, "session-1", work.ListOptions{WorkTypeName: "bug"})
	if err != nil {
		t.Fatalf("ListWork: %v", err)
	}
	if len(got.Results) != 1 || got.Results[0].WorkID != "work-bug" {
		t.Fatalf("ListWork = %#v, want one bug work item", got)
	}
	if got.Counts != nil {
		t.Fatalf("ListWork counts = %#v, want omitted when not requested", got.Counts)
	}
	got.Results[0].State.Name = "mutated"
	second, err := svc.ListWork(ctx, "session-1", work.ListOptions{WorkTypeName: "bug"})
	if err != nil || second.Results[0].State.Name != "init" {
		t.Fatalf("ListWork mutated source snapshot: %#v, %v", second, err)
	}
}

func TestListWorkHonorsPaginationNextToken(t *testing.T) {
	t.Parallel()

	adapter := &recordingSessionAdapter{snapshot: work.ReadSnapshot{Items: []work.ReadModel{
		{CursorID: "tok-active-1", WorkID: "work-active-1", Name: "Alpha first", WorkTypeName: "task", State: &work.State{Name: "review", Type: work.StateTypeProcessing}},
		{CursorID: "tok-active-2", WorkID: "work-active-2", Name: "Alpha second", WorkTypeName: "task", State: &work.State{Name: "review", Type: work.StateTypeProcessing}},
	}}}
	svc := internalservice.New(stubSessionResolver{adapter: adapter}, nil)
	ctx := context.Background()

	first, err := svc.ListWork(ctx, "session-1", work.ListOptions{Name: "alpha", MaxResults: 1})
	if err != nil || len(first.Results) != 1 || first.NextToken != base64.StdEncoding.EncodeToString([]byte("tok-active-1")) {
		t.Fatalf("first ListWork = %#v, %v", first, err)
	}
	second, err := svc.ListWork(ctx, "session-1", work.ListOptions{Name: "alpha", MaxResults: 1, NextToken: first.NextToken})
	if err != nil || len(second.Results) != 1 || second.Results[0].WorkID != "work-active-2" {
		t.Fatalf("second ListWork = %#v, %v", second, err)
	}
}

func TestListWorkDerivesSupersessionBeforeCountsAndPagination(t *testing.T) {
	t.Parallel()

	svc := internalservice.New(stubSessionResolver{adapter: &recordingSessionAdapter{
		snapshot: supersessionReadSnapshot(),
	}}, nil)
	assertDefaultSupersessionSelection(t, svc)
	assertIncludedSupersessionHistory(t, svc)
}

func supersessionReadSnapshot() work.ReadSnapshot {
	return work.ReadSnapshot{
		Items: []work.ReadModel{
			{CursorID: "tok-old", WorkID: "work-old", Name: "same name", State: &work.State{Type: work.StateTypeFailed}, FailureDetail: &work.FailureDetail{Reason: "SETUP_FAILED", Message: "old setup failure"}},
			{CursorID: "tok-new", WorkID: "work-new", Name: "same name", State: &work.State{Type: work.StateTypeProcessing}},
			{CursorID: "tok-fresh", WorkID: "work-fresh", Name: "fresh failure", State: &work.State{Type: work.StateTypeFailed}},
		},
		Admissions: []work.WorkAdmission{
			{WorkID: "work-old", Name: "same name", Order: 0},
			{WorkID: "work-new", Name: "same name", Order: 1},
			{WorkID: "work-fresh", Name: "fresh failure", Order: 2},
		},
	}
}

func assertDefaultSupersessionSelection(t *testing.T, svc *internalservice.Service) {
	t.Helper()

	defaultPage, err := svc.ListWork(context.Background(), "session-1", work.ListOptions{
		Counts: true, MaxResults: 1,
	})
	if err != nil {
		t.Fatalf("default ListWork: %v", err)
	}
	if len(defaultPage.Results) != 1 || defaultPage.Results[0].WorkID != "work-new" {
		t.Fatalf("default first page = %#v, want replacement only", defaultPage)
	}
	if defaultPage.Results[0].SupersededBy != "" || defaultPage.Counts == nil || defaultPage.Counts.Total != 2 {
		t.Fatalf("default first page annotation/counts = %#v, want current row and total 2", defaultPage)
	}
	if defaultPage.NextToken == "" {
		t.Fatalf("default first page next token = %q, want continuation after filtering", defaultPage.NextToken)
	}

	defaultSecondPage, err := svc.ListWork(context.Background(), "session-1", work.ListOptions{
		Counts: true, MaxResults: 1, NextToken: defaultPage.NextToken,
	})
	if err != nil {
		t.Fatalf("default second ListWork: %v", err)
	}
	if len(defaultSecondPage.Results) != 1 || defaultSecondPage.Results[0].WorkID != "work-fresh" || defaultSecondPage.NextToken != "" {
		t.Fatalf("default second page = %#v, want fresh failure and no continuation", defaultSecondPage)
	}
	if defaultSecondPage.Counts == nil || defaultSecondPage.Counts.Total != 2 {
		t.Fatalf("default second counts = %#v, want stable total 2", defaultSecondPage.Counts)
	}
}

func assertIncludedSupersessionHistory(t *testing.T, svc *internalservice.Service) {
	t.Helper()

	allHistory, err := svc.ListWork(context.Background(), "session-1", work.ListOptions{
		IncludeSuperseded: true, Counts: true,
	})
	if err != nil {
		t.Fatalf("include-superseded ListWork: %v", err)
	}
	if allHistory.Counts == nil || allHistory.Counts.Total != 3 || len(allHistory.Results) != 3 {
		t.Fatalf("include-superseded result = %#v, want all three rows", allHistory)
	}
	var old work.ReadModel
	for _, item := range allHistory.Results {
		if item.WorkID == "work-old" {
			old = item
		}
	}
	if old.SupersededBy != "work-new" {
		t.Fatalf("old history row SupersededBy = %q, want work-new", old.SupersededBy)
	}
	if old.FailureDetail != nil {
		t.Fatalf("old history row FailureDetail = %#v, want nil for superseded failed Work", old.FailureDetail)
	}

	got, err := svc.GetWork(context.Background(), "session-1", "work-old")
	if err != nil {
		t.Fatalf("GetWork(old): %v", err)
	}
	if got.SupersededBy != "work-new" {
		t.Fatalf("GetWork(old).SupersededBy = %q, want work-new", got.SupersededBy)
	}
	if got.FailureDetail != nil {
		t.Fatalf("GetWork(old).FailureDetail = %#v, want nil for superseded failed Work", got.FailureDetail)
	}
}

func TestLiveAndReplaySnapshotsProduceIdenticalSupersessionSelection(t *testing.T) {
	t.Parallel()

	snapshot := work.ReadSnapshot{
		Items: []work.ReadModel{
			{CursorID: "tok-old", WorkID: "work-old", Name: "same name", State: &work.State{Type: work.StateTypeTerminal}},
			{CursorID: "tok-new", WorkID: "work-new", Name: "same name", State: &work.State{Type: work.StateTypeProcessing}},
		},
		Admissions: []work.WorkAdmission{
			{WorkID: "work-old", Name: "same name", Order: 0},
			{WorkID: "work-new", Name: "same name", Order: 1},
		},
	}
	live := internalservice.New(stubSessionResolver{
		adapter: &recordingSessionAdapter{snapshot: snapshot},
	}, nil)
	replay := internalservice.New(stubSessionResolver{}, &recordingSnapshotReader{snapshot: snapshot})
	options := work.ListOptions{Counts: true}

	liveResult, err := live.ListWork(context.Background(), "session-1", options)
	if err != nil {
		t.Fatalf("live ListWork: %v", err)
	}
	replayResult, err := replay.ListWork(context.Background(), "session-1", options)
	if err != nil {
		t.Fatalf("replay ListWork: %v", err)
	}
	if !reflect.DeepEqual(liveResult, replayResult) {
		t.Fatalf("live result = %#v, replay result = %#v, want identical projections", liveResult, replayResult)
	}
	if len(liveResult.Results) != 1 || liveResult.Results[0].WorkID != "work-new" || liveResult.Results[0].SupersededBy != "" {
		t.Fatalf("live/replay result = %#v, want only authoritative replacement", liveResult)
	}
	if liveResult.Counts == nil || liveResult.Counts.Total != 1 {
		t.Fatalf("live/replay counts = %#v, want one unsuperseded row", liveResult.Counts)
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing main-branch test complexity; split this scenario into focused helpers and remove this exemption.
func TestListWorkTerminalityCountsAndPaginationUseOneFilteredSelection(t *testing.T) {
	t.Parallel()

	adapter := &recordingSessionAdapter{snapshot: work.ReadSnapshot{Items: []work.ReadModel{
		{CursorID: "tok-story-terminal", WorkID: "work-story-terminal", WorkTypeName: "story", State: &work.State{Name: "complete", Type: work.StateTypeTerminal}},
		{CursorID: "tok-story-processing", WorkID: "work-story-processing", WorkTypeName: "story", State: &work.State{Name: "review", Type: work.StateTypeProcessing}},
		{CursorID: "tok-story-failed", WorkID: "work-story-failed", WorkTypeName: "story", State: &work.State{Name: "rejected", Type: work.StateTypeFailed}},
		{CursorID: "tok-story-initial", WorkID: "work-story-initial", WorkTypeName: "story", State: &work.State{Name: "init", Type: work.StateTypeInitial}},
		{CursorID: "tok-bug-terminal", WorkID: "work-bug-terminal", WorkTypeName: "bug", State: &work.State{Name: "complete", Type: work.StateTypeTerminal}},
		{CursorID: "tok-story-unknown", WorkID: "work-story-unknown", WorkTypeName: "story", State: &work.State{Name: "unknown", Type: "UNKNOWN"}},
		{CursorID: "tok-story-missing", WorkID: "work-story-missing", WorkTypeName: "story"},
	}}}
	svc := internalservice.New(stubSessionResolver{adapter: adapter}, nil)
	ctx := context.Background()
	options := work.ListOptions{WorkTypeName: "story", NonTerminal: true, Counts: true, MaxResults: 1}

	first, err := svc.ListWork(ctx, "session-1", options)
	if err != nil {
		t.Fatalf("first ListWork: %v", err)
	}
	if len(first.Results) != 1 || first.Results[0].WorkID != "work-story-initial" {
		t.Fatalf("first page = %#v, want initial story work", first)
	}
	if first.Counts == nil || first.Counts.Total != 2 {
		t.Fatalf("first counts = %#v, want total 2 before pagination", first.Counts)
	}
	if first.NextToken == "" {
		t.Fatalf("first page next token = %q, want continuation within filtered selection", first.NextToken)
	}

	second, err := svc.ListWork(ctx, "session-1", work.ListOptions{
		WorkTypeName: "story",
		NonTerminal:  true,
		Counts:       true,
		MaxResults:   1,
		NextToken:    first.NextToken,
	})
	if err != nil {
		t.Fatalf("second ListWork: %v", err)
	}
	if len(second.Results) != 1 || second.Results[0].WorkID != "work-story-processing" {
		t.Fatalf("second page = %#v, want processing story work", second)
	}
	if second.Counts == nil || second.Counts.Total != first.Counts.Total || second.NextToken != "" {
		t.Fatalf("second page counts/pagination = %#v, want stable total 2 and no continuation", second)
	}

	terminal, err := svc.ListWork(ctx, "session-1", work.ListOptions{
		WorkTypeName: "story",
		Terminal:     true,
		Counts:       true,
		MaxResults:   10,
	})
	if err != nil {
		t.Fatalf("terminal ListWork: %v", err)
	}
	if got := []string{terminal.Results[0].WorkID, terminal.Results[1].WorkID}; !reflect.DeepEqual(got, []string{"work-story-failed", "work-story-terminal"}) {
		t.Fatalf("terminal results = %v, want failed and terminal", got)
	}
	if terminal.Counts == nil || terminal.Counts.Total != 2 {
		t.Fatalf("terminal counts = %#v, want total 2", terminal.Counts)
	}

	combined, err := svc.ListWork(ctx, "session-1", work.ListOptions{
		StateName:    "review",
		WorkTypeName: "story",
		NonTerminal:  true,
		Counts:       true,
	})
	if err != nil {
		t.Fatalf("combined ListWork: %v", err)
	}
	if len(combined.Results) != 1 || combined.Results[0].WorkID != "work-story-processing" || combined.Counts == nil || combined.Counts.Total != 1 {
		t.Fatalf("combined result = %#v, want one processing story and total 1", combined)
	}

	zero, err := svc.ListWork(ctx, "session-1", work.ListOptions{
		WorkTypeName: "bug",
		NonTerminal:  true,
		Counts:       true,
	})
	if err != nil {
		t.Fatalf("zero-match ListWork: %v", err)
	}
	if len(zero.Results) != 0 || zero.Counts == nil || zero.Counts.Total != 0 {
		t.Fatalf("zero-match result = %#v, want empty page and total 0", zero)
	}
}

func TestListWorkRejectsContradictoryTerminalityBeforeReadingSnapshot(t *testing.T) {
	t.Parallel()

	adapter := &recordingSessionAdapter{snapshotErr: errors.New("snapshot must not be read")}
	svc := internalservice.New(stubSessionResolver{adapter: adapter}, nil)

	_, err := svc.ListWork(context.Background(), "session-1", work.ListOptions{Terminal: true, NonTerminal: true})
	if err == nil || err.Error() != "terminal and nonTerminal cannot both be selected" {
		t.Fatalf("ListWork() error = %v, want contradictory terminality validation", err)
	}
	var validation *work.ValidationError
	if !errors.As(err, &validation) || validation.Field != work.FilterTerminal {
		t.Fatalf("ListWork() error = %#v, want field %q validation", err, work.FilterTerminal)
	}
}

func TestGetWorkByCursorOrWorkIDAndNotFound(t *testing.T) {
	t.Parallel()

	adapter := &recordingSessionAdapter{snapshot: querySnapshot()}
	svc := internalservice.New(stubSessionResolver{adapter: adapter}, nil)
	ctx := context.Background()

	for _, id := range []string{"tok-story", "work-story"} {
		got, err := svc.GetWork(ctx, "session-1", id)
		if err != nil || got.WorkID != "work-story" {
			t.Fatalf("GetWork(%q) = %#v, %v", id, got, err)
		}
	}
	if _, err := svc.GetWork(ctx, "session-1", "missing"); !errors.Is(err, work.ErrWorkNotFound) {
		t.Fatalf("GetWork(missing) error = %v, want ErrWorkNotFound", err)
	}
}

func TestGetWorkReturnsAdmittedIdentityWhenTokenIsInFlight(t *testing.T) {
	t.Parallel()

	service := internalservice.New(stubSessionResolver{adapter: &recordingSessionAdapter{
		snapshot: work.ReadSnapshot{
			Admissions: []work.WorkAdmission{{WorkID: "work-in-flight", Name: "Deploy service"}},
		},
	}}, nil)

	got, err := service.GetWork(context.Background(), "session-1", "work-in-flight")
	if err != nil {
		t.Fatalf("GetWork(in-flight): %v", err)
	}
	if got.WorkID != "work-in-flight" || got.Name != "Deploy service" {
		t.Fatalf("GetWork(in-flight) = %#v, want admitted identity and name", got)
	}
}

func TestWorkReadConfirmationUsesOneWatermarkSamplePerResponse(t *testing.T) {
	t.Parallel()

	reader := &completedFlushSequenceReader{sequence: 5, available: true}
	snapshot := work.ReadSnapshot{
		StreamGenerationID: "generation-confirmation",
		Items: []work.ReadModel{
			{CursorID: "cursor-confirmed", WorkID: "work-confirmed", Name: "confirmed", CurrentStateSequence: 3, CurrentStateSequenceKnown: true},
			{CursorID: "cursor-pending", WorkID: "work-pending", Name: "pending", CurrentStateSequence: 8, CurrentStateSequenceKnown: true},
		},
	}
	svc := internalservice.New(
		stubSessionResolver{adapter: &recordingSessionAdapter{snapshot: snapshot}},
		nil,
		reader,
	)

	listed, err := svc.ListWork(context.Background(), "session-confirmation", work.ListOptions{})
	if err != nil {
		t.Fatalf("ListWork: %v", err)
	}
	if reader.calls != 1 || len(reader.generations) != 1 || reader.generations[0] != "generation-confirmation" {
		t.Fatalf("watermark samples = %d/%v, want one sample for the snapshot generation", reader.calls, reader.generations)
	}
	if listed.Results[0].ConfirmationState != work.ConfirmationStateConfirmed || listed.Results[1].ConfirmationState != work.ConfirmationStateUnconfirmed {
		t.Fatalf("confirmation states = %#v, want CONFIRMED then UNCONFIRMED", listed.Results)
	}

	shown, err := svc.GetWork(context.Background(), "session-confirmation", "work-pending")
	if err != nil {
		t.Fatalf("GetWork: %v", err)
	}
	if reader.calls != 2 || shown.ConfirmationState != work.ConfirmationStateUnconfirmed {
		t.Fatalf("show confirmation = %#v with %d samples, want UNCONFIRMED and one new sample", shown, reader.calls)
	}
}

func TestWorkReadConfirmationRemainsUnconfirmedUntilFlushCoversState(t *testing.T) {
	t.Parallel()

	reader := &completedFlushSequenceReader{sequence: 2, available: false}
	adapter := &recordingSessionAdapter{snapshot: work.ReadSnapshot{
		StreamGenerationID: "generation-reconcile",
		Items: []work.ReadModel{{
			CursorID:                  "cursor-reconcile",
			WorkID:                    "work-reconcile",
			Name:                      "reconcile",
			CurrentStateSequence:      4,
			CurrentStateSequenceKnown: true,
		}},
	}}
	svc := internalservice.New(stubSessionResolver{adapter: adapter}, nil, reader)

	before, err := svc.GetWork(context.Background(), "session-reconcile", "work-reconcile")
	if err != nil {
		t.Fatalf("GetWork before flush: %v", err)
	}
	if before.ConfirmationState != work.ConfirmationStateUnconfirmed {
		t.Fatalf("before flush confirmation = %q, want UNCONFIRMED", before.ConfirmationState)
	}

	reader.available = true
	reader.sequence = 4
	after, err := svc.GetWork(context.Background(), "session-reconcile", "work-reconcile")
	if err != nil {
		t.Fatalf("GetWork after flush: %v", err)
	}
	if after.ConfirmationState != work.ConfirmationStateConfirmed {
		t.Fatalf("after flush confirmation = %q, want CONFIRMED", after.ConfirmationState)
	}
}

func TestReplayWorkReadConfirmationUsesRecordedCursorAndWatermark(t *testing.T) {
	t.Parallel()

	reader := &completedFlushSequenceReader{sequence: 7, available: true}
	snapshotReader := &recordingSnapshotReader{snapshot: work.ReadSnapshot{
		StreamGenerationID: "generation-replay",
		Items: []work.ReadModel{{
			CursorID:                  "cursor-replay",
			WorkID:                    "work-replay",
			Name:                      "replayed",
			CurrentStateSequence:      7,
			CurrentStateSequenceKnown: true,
		}},
	}}
	svc := internalservice.New(stubSessionResolver{}, snapshotReader, reader)

	got, err := svc.GetWork(context.Background(), "session-replay", "work-replay")
	if err != nil {
		t.Fatalf("GetWork replay: %v", err)
	}
	if got.ConfirmationState != work.ConfirmationStateConfirmed {
		t.Fatalf("replay confirmation = %q, want CONFIRMED", got.ConfirmationState)
	}
	if reader.calls != 1 || len(reader.generations) != 1 || reader.generations[0] != "generation-replay" {
		t.Fatalf("replay watermark samples = %d/%v, want one generation-scoped sample", reader.calls, reader.generations)
	}
}

func TestMoveWorkAndReadReturnsDetachedPostMoveReadModel(t *testing.T) {
	t.Parallel()

	adapter := &recordingSessionAdapter{snapshot: work.ReadSnapshot{Items: []work.ReadModel{{
		CursorID: "tok-1",
		WorkID:   "work-1",
		Name:     "one",
		State:    &work.State{Name: "review", Type: work.StateTypeProcessing},
	}}}}
	svc := internalservice.New(stubSessionResolver{adapter: adapter}, nil)
	ctx := context.Background()

	read, err := svc.MoveWorkAndRead(ctx, "session-1", "work-1", "complete", "request-1")
	if err != nil {
		t.Fatalf("MoveWorkAndRead: %v", err)
	}
	if read.WorkID != "work-1" || adapter.movedID != "work-1" || adapter.requestID != "request-1" {
		t.Fatalf("MoveWorkAndRead = %#v, adapter move = (%q, %q)", read, adapter.movedID, adapter.requestID)
	}
	read.State.Name = "mutated"
	got, err := svc.GetWork(ctx, "session-1", "work-1")
	if err != nil || got.State.Name != "review" {
		t.Fatalf("detached read mutated source: %#v, %v", got, err)
	}
}

func TestReadSnapshotUsesSessionAdapterOnly(t *testing.T) {
	t.Parallel()

	adapter := &recordingSessionAdapter{snapshotErr: errors.New("snapshot unavailable")}
	svc := internalservice.New(stubSessionResolver{adapter: adapter}, nil)
	ctx := context.Background()

	_, err := svc.ListWork(ctx, "session-1", work.ListOptions{})
	if err == nil || err.Error() != "read Work snapshot: snapshot unavailable" {
		t.Fatalf("ListWork error = %v, want wrapped snapshot error", err)
	}
}

func TestReadSnapshotFallsBackToTheSnapshotReaderWhenSessionUnavailable(t *testing.T) {
	t.Parallel()

	svc := internalservice.New(
		stubSessionResolver{},
		&recordingSnapshotReader{snapshot: work.ReadSnapshot{Items: []work.ReadModel{{
			CursorID:     "work-rec",
			WorkID:       "work-rec",
			Name:         "Recorded work",
			WorkTypeName: "story",
			State:        &work.State{Name: "review", Type: work.StateTypeProcessing},
		}}}},
	)
	ctx := context.Background()

	got, err := svc.ListWork(ctx, "session-recordings", work.ListOptions{})
	if err != nil {
		t.Fatalf("ListWork: %v", err)
	}
	if len(got.Results) != 1 || got.Results[0].WorkID != "work-rec" {
		t.Fatalf("ListWork = %#v, want one recorded work item", got)
	}
}

type recordingSnapshotReader struct {
	snapshot work.ReadSnapshot
	err      error
}

type completedFlushSequenceReader struct {
	sequence    int64
	available   bool
	calls       int
	generations []string
}

func (reader *completedFlushSequenceReader) CompletedFlushSequence(generation string) (int64, bool) {
	reader.calls++
	reader.generations = append(reader.generations, generation)
	return reader.sequence, reader.available
}

func (a *recordingSnapshotReader) ReadWorkSnapshot(
	context.Context,
	string,
) (work.ReadSnapshot, error) {
	if a.err != nil {
		return work.ReadSnapshot{}, a.err
	}
	return a.snapshot, nil
}
