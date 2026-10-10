package factorysession

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

// GetFactorySession maps only RUN's bounded progress observation. It is not a
// general live read-model inverse; the HTTP host owns all lifecycle decisions.
func (host *hostSessions) GetFactorySession(ctx context.Context, id string) (factorysessions.SessionProjection, error) {
	session, err := host.readChild(ctx, id)
	if err != nil {
		return factorysessions.SessionProjection{}, err
	}
	progress := session.Runtime.Progress
	result := factorysessions.SessionProjection{Runtime: factorysessions.RuntimeProjection{Progress: factorysessions.RuntimeProgress{
		InFlightCount: progress.InFlightCount,
		Categories: factorysessions.RuntimeStatusCategories{
			Initial: progress.Categories.Initial, Processing: progress.Categories.Processing,
			Terminal: progress.Categories.Terminal, Failed: progress.Categories.Failed,
		},
	}}}
	if script := session.Runtime.Javascript; script != nil {
		result.Runtime.JavaScript = &factorysessions.JavaScriptRuntimeProjection{
			ScriptStatus: factorydefinitions.FactorySessionJavaScriptScriptStatus(script.ScriptStatus),
			ChildDispatchCounts: factorydefinitions.FactorySessionChildDispatchCounts{
				Completed: script.ChildDispatchCounts.Completed, Queued: script.ChildDispatchCounts.Queued, Running: script.ChildDispatchCounts.Running,
			},
		}
	}
	return result, nil
}

// SubscribeFactoryResponseEvents supplies RUN's retained timeout snapshot. The
// public prefix count avoids waiting for a live event on an idle session.
func (host *hostSessions) SubscribeFactoryResponseEvents(ctx context.Context, input factorysessions.ResponseEventSubscriptionRequest) (*factorysessions.ResponseEventCursor, error) {
	if !exactChildID(input.SessionID, nil) || input.AfterSequence != 0 || input.DispatchID != "" || len(input.Kinds) != 0 {
		return nil, errors.New("subagent response snapshot requires an exact child and retained prefix")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, host.server+childPath(input.SessionID)+"/response-events", nil)
	if err != nil {
		return nil, errors.New("subagent response snapshot request is invalid")
	}
	response, err := host.http.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("subagent response snapshot failed")
	}
	if response == nil || response.Body == nil {
		return nil, errors.New("subagent response snapshot is unavailable")
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil, hostResponseError(response)
	}
	count, err := strconv.Atoi(response.Header.Get(factorysessions.ResponseEventStreamRetainedCountHeader))
	if err != nil || count < 0 {
		return nil, errors.New("subagent response snapshot prefix is unavailable")
	}
	events, err := readHostEventPrefix(response.Body, count, input.SessionID)
	if err != nil {
		return nil, err
	}
	return &factorysessions.ResponseEventCursor{
		DrainEvents:  func() ([]factorysessions.FactoryResponseEvent, error) { return events, nil },
		NextEvents:   func(context.Context) ([]factorysessions.FactoryResponseEvent, error) { return nil, io.EOF },
		DetachCursor: func() {}, RetainedEventCountFn: func() (int, bool) { return len(events), true },
	}, nil
}

func readHostEventPrefix(reader io.Reader, count int, id string) ([]factorysessions.FactoryResponseEvent, error) {
	events := make([]factorysessions.FactoryResponseEvent, 0)
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), 4*1024*1024)
	var data []string
	for len(events) < count && scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data:") {
			data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
			continue
		}
		if line != "" || len(data) == 0 {
			continue
		}
		var event factorysessions.FactoryResponseEvent
		if json.Unmarshal([]byte(strings.Join(data, "\n")), &event) != nil || event.FactorySessionID != id {
			return nil, errors.New("subagent response snapshot event is invalid")
		}
		events = append(events, event)
		data = nil
	}
	if len(events) != count {
		return nil, errors.New("subagent response snapshot prefix is incomplete")
	}
	return events, nil
}
