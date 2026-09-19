package workscope_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func measurePrebuiltWorkscopeJourney(
	t *testing.T,
	ctx context.Context,
	factorySessionSelector string,
	serverURL string,
) prebuiltWorkscopeJourney {
	t.Helper()
	requestCounter := &prebuiltWorkscopeRequestCounter{}
	client := &http.Client{Timeout: 10 * time.Second, Transport: requestCounter}
	baseURL := strings.TrimSuffix(serverURL, "/")
	journey := prebuiltWorkscopeJourney{factorySessionSelector: factorySessionSelector}
	listPath := "/factory-sessions/" + url.PathEscape(factorySessionSelector) + "/worker-sessions"
	listURL := baseURL + listPath + "?workId=" + url.QueryEscape(prebuiltWorkscopeWorkID)
	measurePrebuiltWorkscopeList(t, ctx, client, listURL, &journey)
	detailURL := baseURL + listPath + "/" + url.PathEscape(prebuiltWorkscopeWorkerSession)
	measurePrebuiltWorkscopeObservationAndTranscript(t, ctx, client, detailURL, &journey)
	streamURL := detailURL + "/events?replayOnly=true"
	measurePrebuiltWorkscopeEventStream(t, ctx, client, streamURL, &journey)
	journey.cancellationRequested, journey.cancellationBodyClosed = cancelPrebuiltWorkscopeStream(t, ctx, client, streamURL)
	assertPrebuiltWorkscopeLatency(t, "before/after journey", journey.measurements)
	journey.sourceCalls = prebuiltWorkscopeMeasuredSourceCalls(t, requestCounter, factorySessionSelector)
	return journey
}

func measurePrebuiltWorkscopeList(
	t *testing.T,
	ctx context.Context,
	client *http.Client,
	listURL string,
	journey *prebuiltWorkscopeJourney,
) {
	started := time.Now()
	response := doPrebuiltWorkscopeGET(t, ctx, client, listURL)
	journey.measurements.ListHeadersMillis = elapsedMilliseconds(started)
	journey.measurements.ListHTTPStatus = response.StatusCode
	journey.measurements.ListContentType = response.Header.Get("Content-Type")
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		t.Fatalf("prebuilt Work-scoped list HTTP status=%d body=%s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	var body bytes.Buffer
	if _, err := io.Copy(&body, response.Body); err != nil {
		response.Body.Close()
		t.Fatalf("read complete prebuilt Work-scoped list: %v", err)
	}
	response.Body.Close()
	journey.measurements.CompleteListMillis = elapsedMilliseconds(started)
	journey.measurements.ListBodyBytes = body.Len()
	if err := json.Unmarshal(body.Bytes(), &journey.list); err != nil {
		t.Fatalf("decode prebuilt Work-scoped list: %v", err)
	}
	journey.measurements.Rows = len(journey.list.Sessions)
	if journey.measurements.Rows != 1 || journey.list.Sessions[0].WorkerSessionId != prebuiltWorkscopeWorkerSession {
		t.Fatalf("prebuilt Work-scoped list rows=%d sessions=%#v, want exact fixture Worker Session %q", journey.measurements.Rows, journey.list.Sessions, prebuiltWorkscopeWorkerSession)
	}
	if journey.measurements.ListContentType == "" {
		t.Fatal("prebuilt Work-scoped list response omitted Content-Type")
	}
}

func measurePrebuiltWorkscopeObservationAndTranscript(
	t *testing.T,
	ctx context.Context,
	client *http.Client,
	detailURL string,
	journey *prebuiltWorkscopeJourney,
) {
	started := time.Now()
	detailResponse := doPrebuiltWorkscopeGET(t, ctx, client, detailURL)
	detailBody, status := readPrebuiltWorkscopeResponse(t, detailResponse)
	journey.measurements.ObservationReadMillis = elapsedMilliseconds(started)
	journey.measurements.ObservationReadBytes = len(detailBody)
	if status != http.StatusOK {
		t.Fatalf("prebuilt Worker Session observation HTTP status=%d body=%s", status, strings.TrimSpace(string(detailBody)))
	}
	if err := json.Unmarshal(detailBody, &journey.detail); err != nil {
		t.Fatalf("decode prebuilt Worker Session observation: %v", err)
	}
	if journey.detail.WorkerSessionId != prebuiltWorkscopeWorkerSession || !journey.detail.ProviderSessionAvailable ||
		journey.detail.ProviderSession == nil || journey.detail.ProviderSession.Provider != "codex" ||
		journey.detail.ProviderSession.Kind != "session_id" || journey.detail.ProviderSession.Id != prebuiltWorkscopeProviderSession {
		t.Fatalf("prebuilt observation lost Worker/Provider Session association: %#v available=%t", journey.detail.ProviderSession, journey.detail.ProviderSessionAvailable)
	}
	measurePrebuiltWorkscopeTranscript(t, ctx, client, detailURL+"/transcript", journey)
}

func measurePrebuiltWorkscopeTranscript(
	t *testing.T,
	ctx context.Context,
	client *http.Client,
	transcriptURL string,
	journey *prebuiltWorkscopeJourney,
) {
	started := time.Now()
	response := doPrebuiltWorkscopeGET(t, ctx, client, transcriptURL)
	body, status := readPrebuiltWorkscopeResponse(t, response)
	journey.measurements.StableIDReadMillis = elapsedMilliseconds(started)
	journey.measurements.StableIDReadBodyBytes = len(body)
	if status != http.StatusOK {
		t.Fatalf("prebuilt stable-ID transcript HTTP status=%d body=%s", status, strings.TrimSpace(string(body)))
	}
	if err := json.Unmarshal(body, &journey.transcript); err != nil {
		t.Fatalf("decode prebuilt stable-ID transcript: %v", err)
	}
	journey.measurements.TranscriptEntries = len(journey.transcript.Entries)
	if journey.transcript.WorkerSessionId != prebuiltWorkscopeWorkerSession || journey.transcript.AttemptId != journey.detail.AttemptId ||
		journey.transcript.ProviderSession.Provider != "codex" || journey.transcript.ProviderSession.Kind != "session_id" ||
		journey.transcript.ProviderSession.Id != prebuiltWorkscopeProviderSession ||
		!reflect.DeepEqual(journey.transcript.WorkIds, []string{prebuiltWorkscopeWorkID}) || journey.measurements.TranscriptEntries == 0 {
		t.Fatalf("prebuilt stable-ID transcript lost Worker/Work/Provider Session association: %#v", journey.transcript)
	}
	encoded, err := json.Marshal(journey.transcript.Entries)
	if err != nil || !bytes.Contains(encoded, []byte("Codex fixture answer COMPLETE")) {
		t.Fatalf("prebuilt stable-ID transcript omitted controlled provider answer: marshalErr=%v entries=%s", err, encoded)
	}
}

func measurePrebuiltWorkscopeEventStream(
	t *testing.T,
	ctx context.Context,
	client *http.Client,
	streamURL string,
	journey *prebuiltWorkscopeJourney,
) {
	started := time.Now()
	response := doPrebuiltWorkscopeGET(t, ctx, client, streamURL)
	stream, status := readPrebuiltWorkscopeReplay(t, response, started)
	if status != http.StatusOK {
		t.Fatalf("prebuilt retained Worker Session stream HTTP status=%d", status)
	}
	journey.events = stream.events
	journey.measurements.Events = len(journey.events)
	journey.measurements.FirstRetainedEventMS = stream.firstEventMillis
	journey.measurements.StreamBodyBytes = stream.bodyBytes
	journey.measurements.StreamHeaderBytes = stream.headerBytes
	if journey.measurements.Events == 0 || stream.firstEvent.Event.Position == 0 {
		t.Fatal("prebuilt retained Worker Session stream returned no first event")
	}
	if stream.firstEvent.WorkerSessionId != prebuiltWorkscopeWorkerSession {
		t.Fatalf("prebuilt first retained event belongs to Worker Session %q", stream.firstEvent.WorkerSessionId)
	}
}

func prebuiltWorkscopeMeasuredSourceCalls(
	t *testing.T,
	counter *prebuiltWorkscopeRequestCounter,
	factorySessionSelector string,
) prebuiltWorkscopeSourceCallCounts {
	t.Helper()
	listPath := "/factory-sessions/" + url.PathEscape(factorySessionSelector) + "/worker-sessions"
	detailPath := listPath + "/" + url.PathEscape(prebuiltWorkscopeWorkerSession)
	calls := prebuiltWorkscopeSourceCallCounts{
		PublicHTTPRequests: counter.total(), ListRequests: counter.count(listPath),
		ObservationReadRequests: counter.count(detailPath),
		StableIDReadRequests:    counter.count(detailPath + "/transcript"),
		RetainedStreamRequests:  counter.count(detailPath+"/events") - 1,
		CancellationStreamReads: 1,
	}
	if calls.ListRequests != 1 || calls.ObservationReadRequests != 1 || calls.StableIDReadRequests != 1 ||
		counter.count(detailPath+"/events") != 2 || calls.PublicHTTPRequests != 5 {
		t.Fatalf("measured prebuilt public source calls = %#v", calls)
	}
	return calls
}

type prebuiltWorkscopeReplay struct {
	events           []factoryapi.WorkerSessionEvent
	firstEvent       factoryapi.WorkerSessionEvent
	firstEventMillis float64
	bodyBytes        int64
	headerBytes      int
}

func readPrebuiltWorkscopeReplay(
	t *testing.T,
	response *http.Response,
	started time.Time,
) (prebuiltWorkscopeReplay, int) {
	t.Helper()
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		return prebuiltWorkscopeReplay{}, response.StatusCode
	}
	body := &prebuiltWorkscopeCountedBody{reader: response.Body, closer: response.Body}
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 4096), 2*1024*1024)
	result := prebuiltWorkscopeReplay{events: make([]factoryapi.WorkerSessionEvent, 0)}
	var header bytes.Buffer
	_ = response.Header.Write(&header)
	result.headerBytes = header.Len()
	complete := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		var frame factoryapi.WorkerSessionEvent
		if err := json.Unmarshal([]byte(payload), &frame); err != nil {
			body.Close()
			t.Fatalf("decode prebuilt Worker Session SSE frame: %v", err)
		}
		if frame.Event.Position > 0 {
			if result.firstEvent.Event.Position == 0 {
				result.firstEvent = frame
				result.firstEventMillis = elapsedMilliseconds(started)
			}
			result.events = append(result.events, frame)
		}
		if frame.ReplaySummary != nil && frame.ReplaySummary.Complete {
			complete = true
			break
		}
	}
	if err := scanner.Err(); err != nil {
		body.Close()
		t.Fatalf("read complete prebuilt Worker Session replay: %v", err)
	}
	if !complete {
		body.Close()
		t.Fatalf("prebuilt Worker Session replay ended without a complete summary: events=%d", len(result.events))
	}
	if err := body.Close(); err != nil {
		t.Fatalf("close prebuilt Worker Session replay: %v", err)
	}
	result.bodyBytes = body.bytes
	return result, response.StatusCode
}

func cancelPrebuiltWorkscopeStream(t *testing.T, parent context.Context, client *http.Client, endpoint string) (bool, bool) {
	t.Helper()
	ctx, cancel := context.WithCancel(parent)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		cancel()
		t.Fatalf("build prebuilt cancellation stream request: %v", err)
	}
	response, err := client.Do(request)
	if err != nil {
		cancel()
		t.Fatalf("open prebuilt cancellation stream: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		cancel()
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		t.Fatalf("prebuilt cancellation stream HTTP status=%d body=%s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 4096), 2*1024*1024)
	seenEvent := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		var frame factoryapi.WorkerSessionEvent
		if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &frame); err != nil {
			cancel()
			response.Body.Close()
			t.Fatalf("decode prebuilt cancellation stream frame: %v", err)
		}
		if frame.Event.Position > 0 {
			seenEvent = true
			break
		}
	}
	if !seenEvent {
		cancel()
		response.Body.Close()
		t.Fatalf("prebuilt cancellation stream ended before a retained event: %v", scanner.Err())
	}
	cancel()
	canceled := errors.Is(ctx.Err(), context.Canceled)
	closeErr := response.Body.Close()
	if !canceled || closeErr != nil {
		t.Fatalf("cancel prebuilt stream: contextCanceled=%t bodyCloseError=%v", canceled, closeErr)
	}
	return canceled, true
}

func assertPrebuiltWorkscopeLatency(t *testing.T, phase string, measurements prebuiltWorkscopeMeasurements) {
	t.Helper()
	for name, millis := range map[string]float64{
		"HTTP list headers":         measurements.ListHeadersMillis,
		"complete Work-scoped list": measurements.CompleteListMillis,
		"stable-ID transcript read": measurements.StableIDReadMillis,
		"first retained event":      measurements.FirstRetainedEventMS,
	} {
		if millis <= 0 || millis > float64(prebuiltWorkscopeBudget.Milliseconds()) {
			t.Fatalf("%s %s = %.3fms, want >0ms and <=%s", phase, name, millis, prebuiltWorkscopeBudget)
		}
	}
}
