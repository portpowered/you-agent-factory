package cli

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	workdomain "github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestWatchReducerAcceptsRefreshedModelRequestRetryWithoutProjectingWork(t *testing.T) {
	metadata, request, firstTransition := watchReconnectSetup(t)
	retryInitial := watchModelRequestEvent(t, "model-retry", 4, "model-before-refresh")
	retryRefreshed := watchModelRequestEvent(t, "model-retry", 4, "model-after-refresh")
	secondTransition := watchTransitionEvent(t, "move-2", 5, "work-1", "processing", "done", true)
	events := []factoryapi.FactoryEvent{metadata, request, firstTransition, retryInitial, retryRefreshed, retryInitial, secondTransition}

	reducer := newWatchReducer("session-retry")
	var transitions []WatchTransition
	var previousCursor *watchEventCursor
	for _, event := range events {
		transition, emit, _, err := reducer.Accept(event)
		if err != nil {
			t.Fatalf("Accept(%q) error = %v", event.Id, err)
		}
		if emit {
			transitions = append(transitions, transition)
		}
		cursor := reducer.Cursor()
		if previousCursor != nil && cursor.Sequence < previousCursor.Sequence {
			t.Fatalf("cursor regressed from %#v to %#v after %q", previousCursor, cursor, event.Id)
		}
		previousCursor = cursor
	}

	if len(transitions) != 2 || transitions[0].EventID != "move-1" || transitions[1].EventID != "move-2" {
		t.Fatalf("transitions = %#v, want exactly move-1 then move-2", transitions)
	}
	if cursor := reducer.Cursor(); cursor == nil || cursor.EventID != "move-2" || cursor.Sequence != 5 {
		t.Fatalf("final cursor = %#v, want move-2 at sequence 5", cursor)
	}
	if !reducer.Completed() {
		t.Fatal("reducer did not complete after the terminal Work transition")
	}
}

func TestWatchFiniteStreamIgnoresRefreshedModelRequestRetry(t *testing.T) {
	metadata, request, firstTransition := watchReconnectSetup(t)
	retryInitial := watchModelRequestEvent(t, "model-retry-finite", 4, "model-before-refresh")
	retryRefreshed := watchModelRequestEvent(t, "model-retry-finite", 4, "model-after-refresh")
	terminal := watchTransitionEvent(t, "move-terminal", 5, "work-1", "processing", "done", true)
	stream := &finiteWatchEventStream{events: []factoryapi.FactoryEvent{
		metadata, request, firstTransition, retryInitial, retryRefreshed, terminal,
	}}
	var output bytes.Buffer
	err := watchWithSource(
		WatchConfig{Context: context.Background(), SessionID: "session-finite-retry", Output: &output},
		watchEventOpenFunc(func(context.Context, *watchEventCursor) (watchEventStream, error) {
			return stream, nil
		}),
	)
	if err != nil {
		t.Fatalf("watchWithSource() error = %v", err)
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("output lines = %d, want two Work transition lines: %q", len(lines), output.String())
	}
	var first, second watchLine
	if err := decodeWatchLine(lines[0], &first); err != nil {
		t.Fatal(err)
	}
	if err := decodeWatchLine(lines[1], &second); err != nil {
		t.Fatal(err)
	}
	if first.EventID != "move-1" || second.EventID != "move-terminal" || first.Sequence >= second.Sequence || !second.Terminal {
		t.Fatalf("finite transitions = %#v, %#v, want ordered move-1 then terminal move", first, second)
	}
}

func TestWatchFollowIgnoresRefreshedModelRequestRetry(t *testing.T) {
	metadata, request, terminal, later := watchFollowSetup(t)
	retryInitial := watchModelRequestEvent(t, "model-retry-follow", 4, "model-before-refresh")
	retryRefreshed := watchModelRequestEvent(t, "model-retry-follow", 4, "model-after-refresh")
	later.Context.Sequence = 5
	stream := &cancellableWatchEventStream{
		events:  []factoryapi.FactoryEvent{metadata, request, terminal, retryInitial, retryRefreshed, later},
		blocked: make(chan struct{}),
		closed:  make(chan struct{}),
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var output bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- watchWithRetry(
			WatchConfig{Context: ctx, SessionID: "session-follow-retry", Follow: true, Output: &output},
			watchEventOpenFunc(func(context.Context, *watchEventCursor) (watchEventStream, error) {
				return stream, nil
			}),
			watchRetryPolicy{maxAttempts: 0},
		)
	}()

	select {
	case <-stream.blocked:
	case <-time.After(time.Second):
		t.Fatal("follow watch did not remain attached after retry enrichment")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("watch error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("follow watch did not cancel while reading")
	}

	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("output lines = %d, want terminal plus later follow line: %q", len(lines), output.String())
	}
	var first, second watchLine
	if err := decodeWatchLine(lines[0], &first); err != nil {
		t.Fatal(err)
	}
	if err := decodeWatchLine(lines[1], &second); err != nil {
		t.Fatal(err)
	}
	if first.EventID != "move-terminal" || second.EventID != "move-later" || first.Sequence >= second.Sequence || !first.Terminal || second.Terminal {
		t.Fatalf("follow transitions = %#v, %#v, want terminal followed by later non-terminal transition", first, second)
	}
}

func watchModelRequestEvent(t *testing.T, id string, sequence int, model string) factoryapi.FactoryEvent {
	t.Helper()
	payload := factoryapi.ModelRequestEventPayload{
		ModelRequestId:   id + "/request",
		Attempt:          1,
		Operation:        "GENERATE",
		Worker:           "worker",
		Model:            model,
		ProviderLocality: "CLOUD",
	}
	var union factoryapi.FactoryEvent_Payload
	if err := union.FromModelRequestEventPayload(payload); err != nil {
		t.Fatalf("encode model request event: %v", err)
	}
	return factoryapi.FactoryEvent{
		SchemaVersion: factoryapi.AgentFactoryEventV1,
		Type:          factoryapi.FactoryEventTypeModelRequest,
		Id:            id,
		Context:       watchEventContext(sequence),
		Payload:       union,
	}
}

func TestBuildListRequest_NormalizesQueryWithoutHTTP(t *testing.T) {
	nextToken := encodeCursor("work/42")
	request, err := buildListRequest(ListConfig{
		Context:      context.Background(),
		Server:       "https://factory.example",
		SessionID:    "session/alpha",
		StateName:    "in review",
		StateType:    "PROCESSING",
		Name:         "Plan & review",
		WorkTypeName: "story/type",
		TraceID:      "trace+1",
		SortBy:       "state.type",
		MaxResults:   25,
		NextToken:    nextToken,
	}, workdomain.PreparedListRequest{
		Options: workdomain.ListOptions{
			StateName: "in review", StateType: "PROCESSING", Name: "Plan & review",
			WorkTypeName: "story/type", TraceID: "trace+1", SortBy: "state.type",
			MaxResults: 25, NextToken: nextToken,
		},
		FilterSummary: "state.name,state.type,name,workTypeName,traceId,sortBy",
	})
	if err != nil {
		t.Fatalf("buildListRequest: %v", err)
	}

	wantQuery := url.Values{
		"state.name":   {"in review"},
		"state.type":   {"PROCESSING"},
		"name":         {"Plan & review"},
		"workTypeName": {"story/type"},
		"traceId":      {"trace+1"},
		"sortBy":       {"state.type"},
		"maxResults":   {"25"},
		"nextToken":    {nextToken},
	}
	if got := request.endpoint.Path; got != "/factory-sessions/session/alpha/work" {
		t.Fatalf("endpoint path = %q", got)
	}
	if got := request.endpoint.EscapedPath(); got != "/factory-sessions/session%2Falpha/work" {
		t.Fatalf("escaped endpoint path = %q", got)
	}
	if got := request.endpoint.RawQuery; got != wantQuery.Encode() {
		t.Fatalf("query = %q, want %q", got, wantQuery.Encode())
	}
	if got := request.filterSummary; got != "state.name,state.type,name,workTypeName,traceId,sortBy" {
		t.Fatalf("filter summary = %q", got)
	}
}

func encodeCursor(value string) string {
	return base64.StdEncoding.EncodeToString([]byte(value))
}
