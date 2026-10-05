package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// Events wakes the reader; only the generation-bound durable cursor supplies
// output. This keeps transient ring eviction and asynchronous capture ordering
// from duplicating records or acknowledging bytes that have not committed.
type logsFollower struct {
	config     ReadConfig
	endpoint   url.URL
	generation string
	position   int64
}

func followLogs(config ReadConfig, endpoint url.URL) error {
	follower := logsFollower{config: config, endpoint: endpoint}
	done, err := follower.drain()
	if err != nil || done {
		return err
	}
	ctx, cancel := context.WithCancel(config.Context)
	wake, result, joined := follower.observe(ctx)
	defer func() { cancel(); <-joined }()
	// Capture commits after Events publication and has no HTTP notification.
	// Periodic durable reads cover that race, including the final terminal.
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		var sourceErr error
		select {
		case <-config.Context.Done():
			return logsTransportError(config.Context, config.Context.Err())
		case <-wake:
		case <-ticker.C:
		case sourceErr = <-result:
			result = nil
		}
		done, err = follower.drain()
		if done || err != nil {
			return err
		}
		if sourceErr != nil {
			return sourceErr
		}
	}
}

func (f *logsFollower) drain() (bool, error) {
	for {
		page, err := f.page()
		if err != nil {
			return false, err
		}
		if err := f.writePage(page); err != nil {
			return false, err
		}
		if page.NextToken == nil {
			if page.Health != factoryapi.COMPLETE {
				return false, newCLIError("WORKER_SESSION_LOGS_GAP", "Worker Session captured history is incomplete", nil)
			}
			return true, nil
		}
		query := f.endpoint.Query()
		previous := query.Get("nextToken")
		query.Set("nextToken", *page.NextToken)
		f.endpoint.RawQuery = query.Encode()
		if len(page.Events) == 0 || f.position >= page.CommittedPosition || previous == *page.NextToken {
			if page.Health == factoryapi.DEGRADED {
				return false, newCLIError("WORKER_SESSION_LOGS_GAP", "Worker Session captured history is incomplete", nil)
			}
			return false, nil
		}
	}
}

func (f *logsFollower) page() (factoryapi.WorkerSessionLogPage, error) {
	var page factoryapi.WorkerSessionLogPage
	response, err := f.config.HTTP.GetJSON(f.config.Context, f.endpoint.String(), &page)
	if err != nil {
		return page, logsTransportError(f.config.Context, err)
	}
	if response.HTTP == nil {
		return page, newCLIError("WORKER_SESSION_READ_FAILED", "Worker Session logs returned no HTTP response", nil)
	}
	defer func() { _ = response.HTTP.Body.Close() }()
	if response.HTTP.StatusCode != http.StatusOK {
		return page, workerSessionReadHTTPError(response.HTTP, response.HTTP.StatusCode)
	}
	return page, nil
}

func (f *logsFollower) writePage(page factoryapi.WorkerSessionLogPage) error {
	if page.WorkerSessionId != f.config.WorkerSessionID || page.RecordingGenerationId == "" || (f.generation != "" && f.generation != page.RecordingGenerationId) {
		return newCLIError("WORKER_SESSION_LOGS_INVALID", "Worker Session logs changed identity or recording generation", nil)
	}
	f.generation = page.RecordingGenerationId
	for _, event := range page.Events {
		position := event.Event.Position
		if event.WorkerSessionId != f.config.WorkerSessionID || position <= 0 || position > page.CommittedPosition || (f.position != 0 && position != f.position+1) {
			return newCLIError("WORKER_SESSION_LOGS_GAP", "Worker Session captured positions are not contiguous", nil)
		}
		if err := json.NewEncoder(f.config.Output).Encode(event); err != nil {
			return err
		}
		f.position = position
	}
	return nil
}

func (f *logsFollower) observe(ctx context.Context) (<-chan struct{}, <-chan error, <-chan struct{}) {
	wake := make(chan struct{}, 1)
	result := make(chan error, 1)
	joined := make(chan struct{})
	config, position := f.config, f.position
	go func() {
		defer close(joined)
		result <- observeLogsEvents(ctx, config, position, wake)
	}()
	return wake, result, joined
}

func observeLogsEvents(ctx context.Context, config ReadConfig, position int64, wake chan<- struct{}) error {
	endpoint, err := workerSessionEventsEndpoint(config.Server, "", config.WorkerSessionID, "", "", "", false)
	if err != nil {
		return err
	}
	query := endpoint.Query()
	// An at-head durable token can yield no events in this invocation. Zero
	// means no known Events position, not a valid exclusive Events cursor.
	if position > 0 {
		query.Set("after_position", strconv.FormatInt(position, 10))
	}
	endpoint.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "text/event-stream")
	response, err := config.HTTP.Execute(request)
	if err != nil {
		return logsTransportError(ctx, err)
	}
	if response.HTTP == nil {
		return newCLIError("WORKER_SESSION_READ_FAILED", "Worker Session Events returned no HTTP response", nil)
	}
	defer func() { _ = response.HTTP.Body.Close() }()
	if response.HTTP.StatusCode != http.StatusOK {
		err := workerSessionStreamHTTPError(response.HTTP, response.HTTP.StatusCode)
		var cliErr *CLIError
		if errors.As(err, &cliErr) && cliErr.Code == "WORKER_SESSION_EVENT_CURSOR_STALE" {
			return nil // Durable polling backfills the evicted Events prefix.
		}
		if errors.As(err, &cliErr) && cliErr.Code == "WORKER_SESSION_NOT_FOUND" {
			// drain already established a captured prefix. Missing live Events
			// ownership cannot make that retained history nonexistent or complete.
			return newCLIError("WORKER_SESSION_LOGS_GAP", "Worker Session captured history is incomplete", nil)
		}
		return err
	}
	return consumeLogsEvents(ctx, response.HTTP.Body, wake)
}

func consumeLogsEvents(ctx context.Context, body io.Reader, wake chan<- struct{}) error {
	reader := bufio.NewReader(body)
	for {
		payload, eof, err := readSSEData(reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return newCLIError("WORKER_SESSION_STREAM_CLOSED", "Worker Session Events closed before terminal", nil)
			}
			return logsTransportError(ctx, err)
		}
		var frame streamJSONFrame
		if len(payload) == 0 && !eof {
			continue
		}
		if err := json.Unmarshal(payload, &frame); err != nil {
			return newCLIError("WORKER_SESSION_STREAM_FAILED", "Worker Session Events returned invalid JSON", err)
		}
		select {
		case wake <- struct{}{}:
		default:
		}
		if done, err := logsSourceOutcome(frame); done {
			return err
		}
		if eof {
			return newCLIError("WORKER_SESSION_STREAM_CLOSED", "Worker Session Events closed before terminal", nil)
		}
	}
}

func logsSourceOutcome(frame streamJSONFrame) (bool, error) {
	if frame.Delivery == "TERMINAL" || frame.Delivery == "TERMINAL_REPLAY" {
		return true, nil // Continue polling until this terminal has committed.
	}
	if frame.Delivery == "SOURCE_FAILURE" {
		code := stringValue(frame.ErrorCode, "WORKER_SESSION_STREAM_FAILED")
		if code == "WORKER_SESSION_STREAM_GAP" {
			return true, nil // Capture health, rather than ring health, proves continuity.
		}
		return true, newCLIError(code, stringValue(frame.ErrorMessage, "Worker Session event stream failed"), nil)
	}
	if frame.Delivery == "REPLAY_SUMMARY" && frame.ReplaySummary != nil && !frame.ReplaySummary.Complete {
		return true, newCLIError("WORKER_SESSION_LOGS_GAP", "Worker Session captured history is incomplete", nil)
	}
	return false, nil
}
