package service

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type historyCatalogFake struct {
	capturedActivityFake
	items []recordings.WorkerCapturedCatalogItem
	err   error
}

func (f *historyCatalogFake) ListWorkerSessionCaptures(_ context.Context, req recordings.WorkerCapturedCatalogRequest) (recordings.WorkerCapturedCatalogPage, error) {
	if req.NextToken == "second" {
		return recordings.WorkerCapturedCatalogPage{Items: f.items[1:]}, f.err
	}
	if len(f.items) > 1 {
		return recordings.WorkerCapturedCatalogPage{Items: f.items[:1], NextToken: "second"}, f.err
	}
	return recordings.WorkerCapturedCatalogPage{Items: f.items}, f.err
}

func historyCapture(t *testing.T, id, factory, attempt string, ended bool) recordings.WorkerCapturedCatalogItem {
	t.Helper()
	openingPayload, err := json.Marshal(workers.SessionPayload{
		WorkerSessionID: id, FactorySessionID: factory, AttemptID: attempt, WorkIDs: []string{"work"},
	})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(workers.Draft{Kind: workers.KindSession, Phase: workers.PhaseStarted, Payload: openingPayload})
	if err != nil {
		t.Fatal(err)
	}
	origin := "direct"
	if factory != "" {
		origin = "factory"
	}
	item := recordings.WorkerCapturedCatalogItem{
		Catalog: recordings.WorkerSessionCatalogEntry{WorkerSessionID: id, FactorySessionID: factory, Origin: origin},
		Opening: events.Record{Payload: payload}, Health: recordings.WorkerRecordingStatusIncomplete, OwnerLost: true,
	}
	if ended {
		item.Terminal = &recordings.WorkerRecordingTerminal{Status: "COMPLETED"}
		item.Health = recordings.WorkerRecordingStatusComplete
	}
	return item
}

func TestArchivedHistoryMergesOwnedAndDurableScopedIdentities(t *testing.T) {
	t.Parallel()
	r := newObservationRegistry(nil, nil)
	addActiveHistoryFixture(r, "live", true)
	activeAttempt := r.observations["live"].attemptID
	fake := &historyCatalogFake{items: []recordings.WorkerCapturedCatalogItem{
		historyCapture(t, "live", "", activeAttempt, false),
		historyCapture(t, "live", "other-factory", "other-attempt", true),
		historyCapture(t, "ended", "", "attempt", true),
		historyCapture(t, "lost", "", "attempt", false),
	}}
	r.logs = &LogReader{reader: fake, logger: logging.NoopLogger{}}
	for _, tc := range []struct {
		history workersessions.ObservationHistory
		want    []string
	}{
		{workersessions.ObservationHistoryAll, []string{"ended", "live", "live", "lost"}},
		{workersessions.ObservationHistoryArchived, []string{"ended", "live", "lost"}},
	} {
		page, err := r.ListWorkerSessionObservations(t.Context(), workersessions.ListWorkerSessionObservationsRequest{History: tc.history})
		if err != nil {
			t.Fatal(err)
		}
		ids := make([]string, 0, len(page.Observations))
		for _, o := range page.Observations {
			ids = append(ids, o.WorkerSessionID)
		}
		if !reflect.DeepEqual(ids, tc.want) {
			t.Fatalf("%s: %v want %v", tc.history, ids, tc.want)
		}
		lost := page.Observations[len(page.Observations)-1]
		if lost.State != workersessions.StateFailed || lost.Failure.Kind != workersessions.FailureCauseProcessGone || lost.RecordingHealth != recordings.WorkerRecordingStatusIncomplete || lost.EndedAt != nil || lost.ConfirmationState != workersessions.ConfirmationStateUnconfirmed {
			t.Fatalf("owner loss invented completion: %+v", lost)
		}
	}
}

func TestArchivedHistoryFiltersBeforeFrozenPages(t *testing.T) {
	t.Parallel()
	r := newObservationRegistry(nil, nil)
	addActiveHistoryFixture(r, "active", true)
	fake := &historyCatalogFake{items: []recordings.WorkerCapturedCatalogItem{
		historyCapture(t, "active", "", r.observations["active"].attemptID, true),
		historyCapture(t, "a", "factory", "attempt", true),
		historyCapture(t, "b", "factory", "attempt", true),
		historyCapture(t, "direct", "", "attempt", true),
	}}
	r.logs = &LogReader{reader: fake, logger: logging.NoopLogger{}}
	req := workersessions.ListWorkerSessionObservationsRequest{History: workersessions.ObservationHistoryArchived,
		Scope: workersessions.ObservationScopeFactory, FactorySessionID: "factory", States: []workersessions.State{workersessions.StateCompleted}, MaxResults: 1}
	first, err := r.ListWorkerSessionObservations(t.Context(), req)
	if err != nil || len(first.Observations) != 1 || first.Observations[0].WorkerSessionID != "a" || first.NextToken == "" {
		t.Fatalf("first: %+v %v", first, err)
	}
	fake.items = nil
	req.NextToken = first.NextToken
	second, err := r.ListWorkerSessionObservations(t.Context(), req)
	if err != nil || len(second.Observations) != 1 || second.Observations[0].WorkerSessionID != "b" || second.NextToken != "" {
		t.Fatalf("second: %+v %v", second, err)
	}
	req.History = workersessions.ObservationHistoryAll
	if _, err := r.ListWorkerSessionObservations(t.Context(), req); !errors.Is(err, workersessions.ErrInvalidObservationPagination) {
		t.Fatalf("cross-history cursor: %v", err)
	}
}

func TestArchivedHistoryExcludesOwnersBeforeStateFiltering(t *testing.T) {
	t.Parallel()
	r := newObservationRegistry(nil, nil)
	addActiveHistoryFixture(r, "active", true)
	item := historyCapture(t, "active", "", r.observations["active"].attemptID, true)
	r.logs = &LogReader{reader: &historyCatalogFake{items: []recordings.WorkerCapturedCatalogItem{item}}, logger: logging.NoopLogger{}}
	page, err := r.ListWorkerSessionObservations(t.Context(), workersessions.ListWorkerSessionObservationsRequest{
		History: workersessions.ObservationHistoryArchived, States: []workersessions.State{workersessions.StateCompleted},
	})
	if err != nil || len(page.Observations) != 0 {
		t.Fatalf("state filter restored an owned capture as archived: %+v %v", page, err)
	}
}

func TestArchivedHistoryCannotInferLossOrChooseDuplicateCapture(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"unknown-owner", "duplicate", "private-storage-error", "wrong-opening"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			r := newObservationRegistry(nil, nil)
			item := historyCapture(t, "worker", "", "attempt", false)
			fake := &historyCatalogFake{items: []recordings.WorkerCapturedCatalogItem{item}}
			switch kind {
			case "unknown-owner":
				fake.items[0].OwnerLost = false
			case "duplicate":
				fake.items = append(fake.items, item)
			case "private-storage-error":
				fake.err = errors.New("private path and contents")
			case "wrong-opening":
				fake.items[0].Catalog.WorkerSessionID = "other"
			}
			r.logs = &LogReader{reader: fake, logger: logging.NoopLogger{}}
			page, err := r.ListWorkerSessionObservations(t.Context(), workersessions.ListWorkerSessionObservationsRequest{History: workersessions.ObservationHistoryArchived})
			if !errors.Is(err, workersessions.ErrObservationProjectionUnavailable) || len(page.Observations) != 0 {
				t.Fatalf("unsafe selection: %+v %v", page, err)
			}
		})
	}
}

func TestArchivedHistoryUsesBoundedCommittedUsageAndTerminalConfirmation(t *testing.T) {
	t.Parallel()
	for _, captured := range []bool{false, true} {
		t.Run(map[bool]string{false: "uncaptured-terminal", true: "captured-terminal"}[captured], func(t *testing.T) {
			t.Parallel()
			item := historyCapture(t, "worker", "", "attempt", true)
			item.Catalog.CommittedPosition = 2
			item.Terminal.Position = 3
			want := workersessions.ConfirmationStateUnconfirmed
			if captured {
				item.Terminal.Position = 2
				want = workersessions.ConfirmationStateConfirmed
			}
			item.MetadataRecords = []events.Record{{Payload: []byte(`{"kind":"USAGE","phase":"UPDATED","payload":{"inputTokens":0,"totalTokens":12}}`)}}
			got, err := capturedHistoryIdentity(item, nil)
			if err != nil || got.ConfirmationState != want || got.TokenUsage == nil || got.TokenUsage.InputTokens == nil || *got.TokenUsage.InputTokens != 0 || got.TokenUsage.OutputTokens != nil || *got.TokenUsage.TotalTokens != 12 {
				t.Fatalf("committed facts: %+v %v", got, err)
			}
			if got.EndedAt != nil || got.Duration != nil {
				t.Fatalf("missing terminal time synthesized: %+v", got)
			}
		})
	}
}
