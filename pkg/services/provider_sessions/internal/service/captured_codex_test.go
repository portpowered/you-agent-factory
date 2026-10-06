package service

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/events"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type capturedCodexFake struct {
	catalog map[string]recordings.WorkerCapturedCatalogPage
	pages   map[string]recordings.WorkerCapturedActivityPage
	failure error
	calls   int
}

func (f *capturedCodexFake) ListWorkerSessionCaptures(ctx context.Context, req recordings.WorkerCapturedCatalogRequest) (recordings.WorkerCapturedCatalogPage, error) {
	f.calls++
	if req.Limit != capturedPageLimit || !req.RequireCompleteMembership {
		panic("unbounded catalog request")
	}
	if err := ctx.Err(); err != nil {
		return recordings.WorkerCapturedCatalogPage{}, err
	}
	return f.catalog[req.NextToken], f.failure
}

func (f *capturedCodexFake) ReadWorkerCapturedActivity(ctx context.Context, req recordings.WorkerCapturedActivityRequest) (recordings.WorkerCapturedActivityPage, error) {
	f.calls++
	if req.Limit != capturedPageLimit || req.BoundPayload {
		panic("wrong complete-history request")
	}
	if err := ctx.Err(); err != nil {
		return recordings.WorkerCapturedActivityPage{}, err
	}
	return f.pages[req.NextToken], f.failure
}

func (*capturedCodexFake) LookupWorkerSessionCapture(context.Context, string) (recordings.WorkerSessionCatalogEntry, error) {
	panic("tuple lookup must enumerate every association")
}

func codexCaptureFixture(t *testing.T, scope string) (*capturedCodexFake, providers.SessionRef) {
	t.Helper()
	ref := providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "captured-ref"}
	page := recordings.WorkerCapturedActivityPage{
		Catalog:    recordings.WorkerSessionCatalogEntry{WorkerSessionID: "worker", FactorySessionID: scope, RecordingID: "recording", RecordingGenerationID: "generation", CommittedPosition: 6},
		Health:     recordings.WorkerRecordingStatusComplete,
		Terminal:   &recordings.WorkerRecordingTerminal{Position: 6, Phase: workers.PhaseCompleted, Status: "COMPLETED"},
		TokenUsage: &workers.UsagePayload{InputTokens: 10, CachedInputTokens: 3, OutputTokens: 4, TotalTokens: 14},
	}
	opening := workers.SessionPayload{WorkerSessionID: "worker", FactorySessionID: scope, AttemptID: "attempt"}
	terminal := opening
	terminal.Status = "COMPLETED"
	terminal.Continuation = &workers.SessionContinuation{Provider: "codex", Kind: ref.Kind, ID: ref.ID}
	payloads := []any{opening,
		workers.MessagePayload{Role: "assistant", ContentBlocks: []workers.ContentBlock{{Kind: workers.ContentBlockText, Text: "old"}}},
		workers.MessageDeltaPayload{ContentBlockKind: workers.ContentBlockText, TextDelta: " delta"},
		workers.MessagePayload{Role: "assistant", ContentBlocks: []workers.ContentBlock{{Kind: workers.ContentBlockText, Text: "final answer"}}},
		workers.ToolPayload{ToolCallID: "call", ToolName: "read", ArgumentsSummary: json.RawMessage(`{"path":"public"}`), ResultSummary: json.RawMessage(`"result"`), Status: "completed"},
		terminal,
	}
	kinds := []workers.Kind{workers.KindSession, workers.KindMessage, workers.KindMessage, workers.KindMessage, workers.KindTool, workers.KindSession}
	for i, payload := range payloads {
		phase := workers.PhaseCompleted
		if i == 0 {
			phase = workers.PhaseStarted
		}
		if i == 2 {
			phase = workers.PhaseDelta
		}
		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		draft := workers.Draft{Kind: kinds[i], Phase: phase, DispatchID: "attempt", TurnID: "turn", ItemID: "message", Payload: body}
		if i == 4 {
			draft.ItemID = "tool"
		}
		data, err := json.Marshal(draft)
		if err != nil {
			t.Fatal(err)
		}
		stamp := time.Date(2026, 10, 6, 14, 0, i, 0, time.UTC)
		page.Records = append(page.Records, recordings.WorkerCapturedRecord{
			Record: events.Record{ID: events.RecordID{Topic: "captured-worker", Position: events.AggregateSequence(i + 1)}, SourceID: "provider", Payload: data}, CapturedAt: &stamp,
		})
	}
	page.Opening = page.Records[0].Record
	item := recordings.WorkerCapturedCatalogItem{Catalog: page.Catalog, Opening: page.Opening, Terminal: page.Terminal, MetadataRecords: []events.Record{page.Records[5].Record}, Health: page.Health}
	return &capturedCodexFake{
		catalog: map[string]recordings.WorkerCapturedCatalogPage{"": {Items: []recordings.WorkerCapturedCatalogItem{item}, GenerationID: "catalog-generation"}},
		pages:   map[string]recordings.WorkerCapturedActivityPage{"": page},
	}, ref
}

func TestCodexCapturedProjectsSnapshotsToolsUsageAndCaptureTimes(t *testing.T) {
	t.Parallel()
	for _, scope := range []string{"", "factory-session"} {
		t.Run(scope, func(t *testing.T) {
			t.Parallel()
			fake, ref := codexCaptureFixture(t, scope)
			s := capturedCodex{reader: fake}
			got, err := s.Details(t.Context(), ref)
			if err != nil {
				t.Fatal(err)
			}
			assertCodexCapturedTranscript(t, got, ref, fake.pages[""].Records[1].CapturedAt)
			assertCodexCapturedParse(t, got)
			*got.Transcript[0].Text = "caller mutation"
			*got.Parse.TokenUsage.InputTokens = 100
			again, err := s.Details(t.Context(), ref)
			if err != nil || *again.Transcript[0].Text != "final answer" || *again.Parse.TokenUsage.InputTokens != 10 {
				t.Fatal("result aliases stored input")
			}
		})
	}
}

func assertCodexCapturedTranscript(t *testing.T, got providersessions.Detail, ref providers.SessionRef, stamp *time.Time) {
	t.Helper()
	if got.ProviderSession.ID != ref.ID || len(got.Transcript) != 3 || *got.Transcript[0].Text != "final answer" ||
		got.Transcript[1].Type != providersessions.TranscriptToolCall || *got.Transcript[1].Name != "read" ||
		got.Transcript[2].Type != providersessions.TranscriptToolOutput || *got.Transcript[2].Output != "result" {
		t.Fatalf("detail = %+v", got)
	}
	for i, entry := range got.Transcript {
		if entry.Order != i+1 || entry.LineNumber != nil || entry.EncryptedContent != nil {
			t.Fatalf("entry facts = %+v", entry)
		}
	}
	if !got.Transcript[0].Timestamp.Equal(*stamp) {
		t.Fatal("lost first capture time")
	}
}

func assertCodexCapturedParse(t *testing.T, got providersessions.Detail) {
	t.Helper()
	if got.Source != (providersessions.SourceMetadata{}) || got.Parse.LineCount != 0 || got.Parse.MalformedLineCount != 0 || len(got.Parse.Turns) != 0 || len(got.Parse.CumulativeInputTokens) != 0 {
		t.Fatal("invented native source facts")
	}
	if got.Parse.TokenUsage == nil || *got.Parse.TokenUsage.InputTokens != 10 || *got.Parse.TokenUsage.CachedInputTokens != 3 || *got.Parse.TokenUsage.TotalTokens != 14 || len(got.Parse.FunctionCalls) != 2 {
		t.Fatalf("parse = %+v", got.Parse)
	}
}

func TestCodexCapturedEnumeratesAllAssociationsAndRejectsAmbiguity(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"paged", "duplicate", "distinct worker", "distinct journal", "changed catalog", "missing", "catalog generation", "catalog cursor cycle"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fake, ref := codexCaptureFixture(t, "")
			first := fake.catalog[""]
			second := first
			second.Items = append([]recordings.WorkerCapturedCatalogItem(nil), first.Items...)
			first.NextToken = "next"
			want := error(nil)
			switch name {
			case "paged":
				first.Items[0].MetadataRecords = nil
			case "distinct worker":
				second.Items[0].Catalog.WorkerSessionID = "other"
				want = providersessions.ErrAmbiguousSessionFile
			case "distinct journal":
				second.Items[0].Catalog.RecordingID = "other"
				want = providersessions.ErrAmbiguousSessionFile
			case "changed catalog":
				second.Items[0].Catalog.CommittedPosition++
				want = providersessions.ErrSessionStorageUnavailable
			case "missing":
				first.Items, second.Items = nil, nil
				first.NextToken = ""
				want = providersessions.ErrSessionNotFound
			case "catalog generation":
				second.GenerationID = "changed"
				want = providersessions.ErrSessionStorageUnavailable
			case "catalog cursor cycle":
				second.NextToken = "next"
				want = providersessions.ErrSessionStorageUnavailable
			}
			fake.catalog[""], fake.catalog["next"] = first, second
			_, err := (&capturedCodex{reader: fake}).Details(t.Context(), ref)
			if !errors.Is(err, want) {
				t.Fatalf("error = %v, want %v", err, want)
			}
		})
	}
}

func TestCodexCapturedRejectsDamagedOrChangingActivity(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"paged", "incomplete", "degraded", "no terminal", "gap", "truncated", "short", "topic", "generation", "head", "opening", "terminal", "usage", "malformed", "cursor cycle", "empty page", "negative usage"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fake, ref := codexCaptureFixture(t, "factory")
			first, second := fake.pages[""], fake.pages[""]
			first.Records, second.Records = first.Records[:3], second.Records[3:]
			first.NextToken = "next"
			want := error(providersessions.ErrSessionStorageUnavailable)
			switch name {
			case "paged":
				want = nil
			case "incomplete":
				first.Health = recordings.WorkerRecordingStatusIncomplete
			case "degraded":
				second.Health = recordings.WorkerRecordingStatusDegraded
			case "no terminal":
				first.Terminal = nil
			case "gap":
				second.Records = second.Records[1:]
			case "truncated":
				second.Records[0].Truncated = true
			case "short":
				second.Records = second.Records[:2]
			case "topic":
				second.Records[0].Record.ID.Topic = "foreign"
			default:
				mutateCapturedCodexHead(name, &first, &second)
			}
			fake.pages[""], fake.pages["next"] = first, second
			_, err := (&capturedCodex{reader: fake}).Details(t.Context(), ref)
			if !errors.Is(err, want) {
				t.Fatalf("error = %v, want %v", err, want)
			}
		})
	}
}

func mutateCapturedCodexHead(name string, first, second *recordings.WorkerCapturedActivityPage) {
	switch name {
	case "generation":
		second.Catalog.RecordingGenerationID = "changed"
	case "head":
		second.Catalog.CommittedPosition++
	case "opening":
		second.Opening.SourceID = "changed"
	case "terminal":
		terminal := *second.Terminal
		terminal.Status = "FAILED"
		second.Terminal = &terminal
	case "usage":
		second.TokenUsage = &workers.UsagePayload{InputTokens: 50}
	case "malformed":
		second.Records[0].Record.Payload = []byte(`{broken`)
	case "cursor cycle":
		second.NextToken = "next"
	case "empty page":
		first.Records = nil
	case "negative usage":
		first.TokenUsage.InputTokens = -1
	}
}

func TestCodexCapturedValidationCancellationAndSafeFailures(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"path id", "kind", "provider", "canceled", "storage"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fake, ref := codexCaptureFixture(t, "")
			ctx := t.Context()
			want := providersessions.ErrInvalidIdentifier
			switch name {
			case "path id":
				ref.ID = "../private"
			case "kind":
				ref.Kind = "other"
				want = providersessions.ErrUnsupportedKind
			case "provider":
				ref.Provider = providers.IDCursor
				want = providersessions.ErrUnsupportedProvider
			case "canceled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
				want = providersessions.ErrOperationCanceled
			case "storage":
				fake.failure = errors.New("secret profile /private/root")
				want = providersessions.ErrSessionStorageUnavailable
			}
			_, err := (&capturedCodex{reader: fake}).Details(ctx, ref)
			if !errors.Is(err, want) || strings.Contains(err.Error(), "private") {
				t.Fatalf("unsafe/unexpected error: %v", err)
			}
			if name != "storage" && fake.calls != 0 {
				t.Fatal("read before validation/cancellation")
			}
		})
	}
}

func TestCodexCapturedCompleteEmptyAndAbsentOrZeroUsage(t *testing.T) {
	t.Parallel()
	for _, usage := range []*workers.UsagePayload{nil, {}} {
		t.Run(string(mustJSON(t, usage)), func(t *testing.T) {
			t.Parallel()
			fake, ref := codexCaptureFixture(t, "")
			page := fake.pages[""]
			page.Records = []recordings.WorkerCapturedRecord{page.Records[0], page.Records[5]}
			page.Records[1].Record.ID.Position = 2
			page.Catalog.CommittedPosition, page.Terminal.Position = 2, 2
			page.TokenUsage = usage
			item := fake.catalog[""].Items[0]
			item.Catalog, item.Terminal, item.MetadataRecords = page.Catalog, page.Terminal, []events.Record{page.Records[1].Record}
			fake.catalog[""] = recordings.WorkerCapturedCatalogPage{Items: []recordings.WorkerCapturedCatalogItem{item}}
			fake.pages[""] = page
			got, err := (&capturedCodex{reader: fake}).Details(t.Context(), ref)
			if err != nil || !reflect.DeepEqual(got.Transcript, []providersessions.TranscriptEntry{}) || got.Parse.ParseErrors == nil || got.Parse.FunctionCalls == nil {
				t.Fatalf("empty = %+v, %v", got, err)
			}
			if (got.Parse.TokenUsage == nil) != (usage == nil) {
				t.Fatal("lost absence versus explicit zero usage")
			}
		})
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestCapturedCodexPeerMethodsShareProjectionAndCancellation(t *testing.T) {
	t.Parallel()
	fake, ref := codexCaptureFixture(t, "factory-session")
	service := inspectionService{codex: capturedCodex{reader: fake}}
	detail, err := service.Details("codex", ref.Kind, ref.ID)
	if err != nil {
		t.Fatal(err)
	}
	inspected, err := service.Inspect(providersessions.InspectRequest{Session: ref})
	if err != nil || inspected.Session != ref || inspected.Source != detail.Source {
		t.Fatalf("Inspect = %+v, %v", inspected, err)
	}
	projected, err := service.Project(providersessions.ProjectRequest{Session: ref})
	if err != nil || projected.Session != ref || !reflect.DeepEqual(projected.Detail, detail) {
		t.Fatalf("Project = %+v, %v", projected, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = service.Project(providersessions.ProjectRequest{Context: ctx, Session: ref})
	if !errors.Is(err, providersessions.ErrOperationCanceled) {
		t.Fatalf("canceled Project = %v", err)
	}
	_, err = service.Inspect(providersessions.InspectRequest{Context: ctx, Session: ref})
	if !errors.Is(err, providersessions.ErrOperationCanceled) {
		t.Fatalf("canceled Inspect = %v", err)
	}
}
