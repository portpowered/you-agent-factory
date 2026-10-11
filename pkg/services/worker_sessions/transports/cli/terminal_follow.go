package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/workers"
)

const terminalSummaryRunes = 240
const terminalOpenTimeout = 15 * time.Second

type terminalLine struct {
	WorkerSessionID string `json:"workerSessionId"`
	State           string `json:"state"`
	Result          string `json:"result"`
}
type terminalObservation struct {
	line terminalLine
	err  error
}

// terminalFollow composes only selected-host observers. Cancellation never
// invokes Worker Session controls; joining readers closes their owned bodies.
func terminalFollow(config StreamConfig) error {
	ids, err := terminalSelection(config)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(config.Context)
	var readers sync.WaitGroup
	defer func() { cancel(); readers.Wait() }()
	results := make(chan terminalObservation, len(ids))
	for _, id := range ids {
		readers.Add(1)
		go func(id string) {
			defer readers.Done()
			selected := config
			selected.Context = ctx
			line, err := observeTerminal(ctx, selected, id)
			results <- terminalObservation{line, err}
		}(id)
	}
	for range ids {
		select {
		case <-ctx.Done():
			return newCLIError("WORKER_SESSION_STREAM_INTERRUPTED", "terminal follow interrupted; attempts remain running", context.Canceled)
		case result := <-results:
			if result.err != nil {
				return result.err
			}
			if err := writeTerminalLine(config, result.line); err != nil {
				return terminalDiagnostic(config, result.line.WorkerSessionID, newCLIError("WORKER_SESSION_FOLLOW_OUTPUT_FAILED", "cannot write terminal result", err))
			}
		}
	}
	return nil
}

func terminalSelection(config StreamConfig) ([]string, error) {
	if config.Context == nil || config.HTTP == nil || config.Output == nil {
		return nil, newCLIError("WORKER_SESSION_FOLLOW_INVALID", "terminal follow requires context, HTTP protocol and output", nil)
	}
	if !config.Terminal || len(config.TerminalIDs) == 0 {
		return nil, newCLIError("WORKER_SESSION_FOLLOW_INVALID", "use worker-sessions follow --terminal ID [ID...]", nil)
	}
	if config.SessionID != "" || config.Provider != "" || config.Kind != "" || config.ID != "" || config.WorkerSessionID != "" || config.ReplayOnly {
		return nil, newCLIError("WORKER_SESSION_FOLLOW_INVALID", "terminal follow accepts only selected-host Worker Session IDs", nil)
	}
	seen := map[string]bool{}
	var ids []string
	for _, input := range config.TerminalIDs {
		id := strings.TrimSpace(input)
		if id == "" {
			return nil, newCLIError("WORKER_SESSION_FOLLOW_INVALID", "Worker Session IDs must be nonempty", nil)
		}
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func observeTerminal(ctx context.Context, config StreamConfig, id string) (terminalLine, error) {
	line, err := readTerminal(ctx, config, id)
	if err != nil {
		err = terminalDiagnostic(config, id, err)
	}
	return line, err
}

func terminalDiagnostic(config StreamConfig, id string, err error) error {
	return newCLIError(cliErrorCode(err), fmt.Sprintf("Worker Session %q at %q: %s", id, config.Server, cliErrorMessage(err)), err)
}

func readTerminal(parent context.Context, config StreamConfig, id string) (terminalLine, error) {
	endpoint, err := workerSessionEventsEndpoint(config.Server, "", id, "", "", "", false)
	if err != nil {
		return terminalLine{}, newCLIError("WORKER_SESSION_FOLLOW_INVALID", "invalid selected host", err)
	}
	// Bound connection/header establishment, without imposing an attempt or
	// observation lifetime deadline after the stream is open.
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	timer := time.AfterFunc(terminalOpenTimeout, cancel)
	defer timer.Stop()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return terminalLine{}, newCLIError("WORKER_SESSION_FOLLOW_INVALID", "invalid stream request", err)
	}
	request.Header.Set("Accept", "text/event-stream")
	response, err := config.HTTP.Execute(request)
	if !timer.Stop() && config.Context.Err() == nil {
		if response.HTTP != nil && response.HTTP.Body != nil {
			_ = response.HTTP.Body.Close()
		}
		return terminalLine{}, newCLIError("WORKER_SESSION_STREAM_UNAVAILABLE", "stream did not open within 15 seconds", err)
	}
	if err != nil {
		return terminalLine{}, streamTransportError(config, err)
	}
	if response.HTTP == nil || response.HTTP.Body == nil {
		return terminalLine{}, newCLIError("WORKER_SESSION_STREAM_UNAVAILABLE", "stream returned no body", nil)
	}
	defer func() { _ = response.HTTP.Body.Close() }()
	if response.HTTP.StatusCode != http.StatusOK {
		return terminalLine{}, workerSessionStreamHTTPError(response.HTTP, response.HTTP.StatusCode)
	}
	if !strings.HasPrefix(strings.ToLower(response.HTTP.Header.Get("Content-Type")), "text/event-stream") {
		return terminalLine{}, newCLIError("WORKER_SESSION_STREAM_UNAVAILABLE", "selected host did not return an event stream", nil)
	}
	return consumeTerminal(config, id, response.HTTP.Body)
}

func consumeTerminal(config StreamConfig, id string, body io.Reader) (terminalLine, error) {
	reader := bufio.NewReader(body)
	summary := ""
	for {
		data, eof, err := readSSEData(reader)
		if err != nil {
			if errors.Is(config.Context.Err(), context.Canceled) {
				return terminalLine{}, newCLIError("WORKER_SESSION_STREAM_INTERRUPTED", "terminal follow interrupted", context.Canceled)
			}
			return terminalLine{}, newCLIError("WORKER_SESSION_STREAM_CLOSED", "stream closed before an authoritative terminal record", err)
		}
		var frame streamJSONFrame
		if len(data) == 0 {
			continue
		}
		if json.Unmarshal(data, &frame) != nil || frame.WorkerSessionID != id {
			return terminalLine{}, newCLIError("WORKER_SESSION_STREAM_FAILED", "invalid or mismatched stream frame", nil)
		}
		if frame.Delivery == "SOURCE_FAILURE" {
			return terminalLine{}, newCLIError(stringValue(frame.ErrorCode, "WORKER_SESSION_STREAM_FAILED"), stringValue(frame.ErrorMessage, "stream source failed"), nil)
		}
		if frame.Event != nil {
			line, next, err := terminalFrame(id, frame, summary)
			if err != nil {
				return terminalLine{}, err
			}
			summary = next
			if line.State != "" {
				return line, nil
			}
		} else if frame.Delivery == "TERMINAL" || frame.Delivery == "TERMINAL_REPLAY" {
			return terminalLine{}, newCLIError("WORKER_SESSION_STREAM_FAILED", "terminal frame has no authoritative session record", nil)
		}
		if eof || frame.Delivery == "REPLAY_SUMMARY" {
			return terminalLine{}, newCLIError("WORKER_SESSION_STREAM_CLOSED", "stream ended before terminal", nil)
		}
	}
}

func terminalFrame(id string, frame streamJSONFrame, summary string) (terminalLine, string, error) {
	var draft workers.Draft
	if json.Unmarshal(frame.Event.Payload, &draft) != nil {
		return terminalLine{}, summary, newCLIError("WORKER_SESSION_STREAM_FAILED", "invalid published event", nil)
	}
	if draft.Kind == workers.KindStreamGap {
		return terminalLine{}, summary, newCLIError("WORKER_SESSION_STREAM_GAP", "observation gap; terminal result cannot be established", nil)
	}
	summary = publishedTerminalSummary(draft, summary)
	if frame.Delivery != "TERMINAL" && frame.Delivery != "TERMINAL_REPLAY" {
		return terminalLine{}, summary, nil
	}
	var session struct {
		Status        string `json:"status"`
		FailureDetail string `json:"failureDetail"`
		FailureCause  string `json:"failureCause"`
	}
	if draft.Kind != workers.KindSession || json.Unmarshal(draft.Payload, &session) != nil {
		return terminalLine{}, summary, newCLIError("WORKER_SESSION_STREAM_FAILED", "terminal frame is not a SESSION record", nil)
	}
	phase := workers.PhaseCompleted
	switch session.Status {
	case "COMPLETED":
	case "FAILED":
		phase = workers.PhaseFailed
	case "CANCELED", "TERMINATED":
		phase = workers.PhaseCanceled
	default:
		return terminalLine{}, summary, newCLIError("WORKER_SESSION_STREAM_FAILED", "terminal SESSION has no terminal status", nil)
	}
	if draft.Phase != phase {
		return terminalLine{}, summary, newCLIError("WORKER_SESSION_STREAM_FAILED", "terminal SESSION phase and status disagree", nil)
	}
	if session.Status != "COMPLETED" {
		summary = shortTerminalSummary(session.FailureDetail)
		if summary == "" {
			summary = shortTerminalSummary(session.FailureCause)
		}
	}
	if summary == "" {
		summary = session.Status
	}
	return terminalLine{id, session.Status, summary}, summary, nil
}

func publishedTerminalSummary(draft workers.Draft, summary string) string {
	if draft.Kind == workers.KindMessage && draft.Phase == workers.PhaseCompleted {
		var message workers.MessagePayload
		if json.Unmarshal(draft.Payload, &message) == nil && message.Role == "assistant" && workers.IsAuthoritativeMessageSnapshot(message) {
			var text strings.Builder
			for _, block := range message.ContentBlocks {
				if block.Kind == workers.ContentBlockText {
					text.WriteString(block.Text)
				}
			}
			summary = shortTerminalSummary(text.String())
		}
	}
	return summary
}

func shortTerminalSummary(text string) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) > terminalSummaryRunes {
		return string(runes[:terminalSummaryRunes]) + "…"
	}
	return string(runes)
}

func writeTerminalLine(config StreamConfig, line terminalLine) error {
	var err error
	if config.JSON {
		err = json.NewEncoder(config.Output).Encode(line)
	} else {
		_, err = fmt.Fprintf(config.Output, "workerSessionId=%q state=%s result=%q\n", line.WorkerSessionID, line.State, line.Result)
	}
	if err != nil {
		return err
	}
	if flusher, ok := config.Output.(interface{ Flush() error }); ok {
		return flusher.Flush()
	}
	if flusher, ok := config.Output.(interface{ Flush() }); ok {
		flusher.Flush()
	}
	return nil
}
