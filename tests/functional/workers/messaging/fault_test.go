package messaging

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	api "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

// Gate the selected profile's actual outcome flush, leaving journal policy and
// provider admission in production. No service or journal internals are replaced.
type messageOutcomeFileSystem struct {
	platformfilesystem.Local
	entered, release chan struct{}
	unblock          sync.Once
	reject           bool
}

type messageOutcomeFile struct {
	*os.File
	gate    *messageOutcomeFileSystem
	outcome bool
}

func (g *messageOutcomeFileSystem) OpenFile(path string, flags int, mode fs.FileMode) (io.WriteCloser, error) {
	file, err := os.OpenFile(path, flags, mode)
	if err != nil {
		return nil, err
	}
	return &messageOutcomeFile{File: file, gate: g}, nil
}

func (f *messageOutcomeFile) Write(data []byte) (int, error) {
	var record struct {
		Kind string `json:"kind"`
	}
	if json.Unmarshal(data, &record) == nil {
		f.outcome = record.Kind == "REVIVE_OUTCOME"
	}
	return f.File.Write(data)
}

func (f *messageOutcomeFile) Sync() error {
	if f.outcome {
		f.outcome = false // Rollback's Sync must flush without entering again.
		close(f.gate.entered)
		<-f.gate.release
		if f.gate.reject {
			return errors.New("controlled-private-outcome-flush-failure")
		}
	}
	return f.File.Sync()
}

type outcomeSendResponse struct {
	status int
	body   []byte
}

func TestReviveDeliveryWaitsForOutcomeFlush(t *testing.T) {
	t.Parallel()
	for _, reject := range []bool{false, true} {
		t.Run(map[bool]string{false: "committed", true: "refused"}[reject], func(t *testing.T) {
			t.Parallel()
			exerciseOutcomeFlush(t, reject)
		})
	}
}

func exerciseOutcomeFlush(t *testing.T, reject bool) {
	t.Helper()
	gate := &messageOutcomeFileSystem{entered: make(chan struct{}), release: make(chan struct{}), reject: reject}
	defer gate.unblock.Do(func() { close(gate.release) })
	input := &reviveInputGate{entered: make(chan struct{}), release: make(chan struct{})}
	defer input.unblock.Do(func() { close(input.release) })
	core, _ := observer.New(zap.InfoLevel)
	signal := &subscriptionSignal{Core: core, ready: make(chan struct{}), once: &sync.Once{}}
	revival := &workerGate{started: make(chan map[string]string, 1), requests: make(chan platformprocess.CommandRequest, 1), release: make(chan struct{})}
	dir := t.TempDir()
	f := startHostRunner(t, t.TempDir(), dir, "", serviceedges.Edges{
		AgentMessageStoreFileSystem: gate, ProcessLogger: zap.New(signal),
		FactorySessionsWorkingDirectory: reviveRecordingDirectory(dir), WorkerRecordingWriter: input,
		WorkerRecordingStoreObserver: func(store recordings.WorkerRecordingStore) { input.WorkerRecordingStore = store },
	}, nil, revival, "lead", "lane")
	lead := f.invoke(t, "lead", nil)
	lane := f.invoke(t, "lane", lead)
	f.endWorker(t, "lead")
	before := messageFactoryStreams(t, f, "lead", "lane")
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	frames, finished := make(chan []byte, 4), make(chan error, 1)
	go streamFramesAt(ctx, f.server.URL(), "/messages?follow=true", lane, frames, finished)
	select {
	case <-signal.ready:
	case <-ctx.Done():
		t.Fatal("follow did not register")
	}
	request := map[string]any{"requestId": "flush", "to": map[string]string{"workerSessionId": "lead"}, "body": "flush before delivery facts"}
	result := make(chan outcomeSendResponse, 1)
	go func() {
		status, body := f.request(t, http.MethodPost, "/messages", request, lane)
		result <- outcomeSendResponse{status, body}
	}()
	select {
	case <-input.entered:
	case <-ctx.Done():
		t.Fatal("delivery did not reach admission input")
	}
	var sent api.AgentMessageObservation
	if json.Unmarshal(nextFrame(t, ctx, frames, finished), &sent) != nil || sent.Kind != "SENT" || sent.Message.Status != "QUEUED" || sent.Message.RevivedWorkerSessionId != nil {
		t.Fatal("initial follow did not preserve queued admission")
	}
	input.unblock.Do(func() { close(input.release) })
	assertFlushPublicationBlocked(t, ctx, gate, result, frames)
	assertOutcomeFlushResult(t, ctx, f, lane, lead, revival, sent.Message, result, frames, finished, reject)
	if after := messageFactoryStreams(t, f, "lead", "lane"); !reflect.DeepEqual(before, after) {
		t.Fatal("outcome flush changed Factory Events")
	}
	cancel()
	select {
	case <-finished:
	case <-time.After(15 * time.Second):
		t.Fatal("outcome follow did not join")
	}
	f.endWorker(t, "lane")
}

func assertFlushPublicationBlocked(t *testing.T, ctx context.Context, gate *messageOutcomeFileSystem, result <-chan outcomeSendResponse, frames <-chan []byte) {
	t.Helper()
	select {
	case <-gate.entered:
	case <-ctx.Done():
		t.Fatal("delivery did not reach outcome flush")
	}
	select {
	case <-result:
		t.Fatal("send completed before outcome flush")
	default:
	}
	select {
	case <-frames:
		t.Fatal("delivery published before outcome flush")
	default:
	}
	gate.unblock.Do(func() { close(gate.release) })
}

func assertOutcomeFlushResult(t *testing.T, ctx context.Context, f *fixture, lane, lead map[string]string, revival *workerGate, queued api.AgentMessage, result <-chan outcomeSendResponse, frames <-chan []byte, finished <-chan error, reject bool) {
	t.Helper()
	var response outcomeSendResponse
	select {
	case response = <-result:
	case <-ctx.Done():
		t.Fatal("send did not finish after outcome flush")
	}
	if reject {
		assertRefusedOutcomeFlush(t, f, lane, revival, queued, response, frames)
		return
	}
	var responseBody api.AgentMessageSendResponse
	if response.status != http.StatusAccepted || json.Unmarshal(response.body, &responseBody) != nil {
		t.Fatal("committed flush lost send outcome")
	}
	var outcome api.AgentMessageObservation
	if json.Unmarshal(nextFrame(t, ctx, frames, finished), &outcome) != nil || outcome.Kind != "DELIVERED_REVIVE" || !reflect.DeepEqual(outcome.Message, responseBody.Message) {
		t.Fatal("committed delivery differs from follow")
	}
	got := readPage(t, f, "/messages?threadId="+queued.ThreadId, lane)
	if len(got.Messages) != 1 || !reflect.DeepEqual(got.Messages[0], responseBody.Message) {
		t.Fatal("committed delivery differs from read")
	}
	successor := assertReviveAdmission(t, f, revival, responseBody.Message, lead)
	close(revival.release)
	f.waitEndedWorker(t, successor["YOU_WORKER_SESSION_ID"])
}

func assertRefusedOutcomeFlush(t *testing.T, f *fixture, lane map[string]string, revival *workerGate, queued api.AgentMessage, response outcomeSendResponse, frames <-chan []byte) {
	t.Helper()
	var failure api.ErrorResponse
	if response.status != http.StatusServiceUnavailable || json.Unmarshal(response.body, &failure) != nil || string(failure.Code) != "MESSAGE_STORE_UNAVAILABLE" {
		t.Fatalf("flush failure: %d %s", response.status, response.body)
	}
	got := readPage(t, f, "/messages?threadId="+queued.ThreadId, lane)
	if len(got.Messages) != 1 || !reflect.DeepEqual(got.Messages[0], queued) {
		t.Fatal("failed flush changed queued record")
	}
	select {
	case <-frames:
		t.Fatal("failed flush published delivery")
	default:
	}
	// The admitted worker is real even when its message outcome cannot flush.
	successor := <-revival.started
	<-revival.requests
	close(revival.release)
	f.waitEndedWorker(t, successor["YOU_WORKER_SESSION_ID"])
}

func TestMessagingFaultsLeaveFactoryWorkHealthy(t *testing.T) {
	t.Parallel()
	for _, cell := range []struct{ name, settings, journal, code string }{
		{"enabled", "", "", ""},
		{"disabled", `{"messaging":{"enabled":false}}`, "", "MESSAGING_DISABLED"},
		{"invalid settings", `{"messaging":{"limits":{"maxBodyBytes":0}}}`, "", "MESSAGE_STORE_UNAVAILABLE"},
		{"corrupt store", "", "existing-customer-bytes\n", "MESSAGE_STORE_CORRUPT"},
	} {
		t.Run(cell.name, func(t *testing.T) {
			t.Parallel()
			home, dir := t.TempDir(), t.TempDir()
			path := filepath.Join(home, ".you-agent-factory", "messages", "messages.jsonl")
			if cell.journal != "" {
				writeJournal(t, path, cell.journal)
			}
			f := startHostEffects(t, home, dir, cell.settings, healthyProviderEffects())
			if cell.code != "" {
				assertError(t, f, http.MethodGet, "/messages", nil, nil, http.StatusServiceUnavailable, cell.code)
			} else {
				status, body := f.request(t, http.MethodGet, "/messages", nil, nil)
				if status != http.StatusOK || !strings.Contains(string(body), `"messages":[]`) {
					t.Fatalf("empty configured inbox: %d %s", status, body)
				}
			}
			assertHealthyWork(t, f)
			if cell.journal != "" {
				bytes, err := os.ReadFile(path)
				if err != nil || string(bytes) != cell.journal {
					t.Fatal("quarantine repaired or erased customer bytes")
				}
			}
		})
	}
}

func writeJournal(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}

func healthyProviderEffects() serviceedges.Edges {
	return serviceedges.Edges{ProviderCommandRunner: testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout: support.CodexSuccessStdout("healthy Factory independent of inbox COMPLETE"),
	})}
}

func assertHealthyWork(t *testing.T, f *fixture) {
	t.Helper()
	stream := support.OpenFactoryEventStreamAt(t, support.SessionEventsURL(f.server.URL(), "~default"))
	defer stream.Close()
	request := map[string]any{"name": "healthy-work", "workTypeName": "task", "payload": map[string]string{"title": "healthy Factory independent of inbox"}}
	status, body := f.request(t, http.MethodPost, "/factory-sessions/~default/work", request, nil)
	if status != http.StatusCreated {
		t.Fatalf("Factory admission after Messaging fault: %d %s", status, body)
	}
	var submitted api.SubmitWorkResponse
	if err := json.Unmarshal(body, &submitted); err != nil {
		t.Fatal(err)
	}
	// This isolated session admits exactly one Work. Its public dispatch
	// completion signal precedes the single terminal Work assertion.
	deadline := time.Now().Add(15 * time.Second)
	for {
		event := stream.NextEvent(time.Until(deadline))
		if event.Type == api.FactoryEventTypeDispatchResponse {
			break
		}
	}
	page := support.GetJSON[api.ListWorkResponse](t, f.server.URL()+"/factory-sessions/~default/work")
	if support.CountWorkAtCustomerState(page, "task:done") != 1 {
		t.Fatal("Factory Work did not finish after Messaging outcome")
	}
}

func TestMessagingSettingsPreserveCanonicalFactoryOutcome(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	var baseline []string
	for _, cell := range []struct{ name, settings, journal, code string }{
		{"disabled", `{"messaging":{"enabled":false}}`, "", "MESSAGING_DISABLED"},
		{"enabled", `{"messaging":{"enabled":true}}`, "", ""},
		{"corrupt", "", "preserved corrupt bytes\n", "MESSAGE_STORE_CORRUPT"},
		{"malformed", `{"messaging":{"limits":{"maxBodyBytes":0}}}`, "", "MESSAGE_STORE_UNAVAILABLE"},
	} {
		clock := platformclock.NewDeterministic(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Second)
		// Control timestamps while keeping production lifecycle timers real.
		effects := serviceedges.Edges{Clock: struct{ platformclock.Source }{clock}}
		home := t.TempDir()
		if cell.journal != "" {
			writeJournal(t, filepath.Join(home, ".you-agent-factory", "messages", "messages.jsonl"), cell.journal)
		}
		f := startHostEffects(t, home, dir, cell.settings, serviceedges.Merge(healthyProviderEffects(), effects))
		if cell.code != "" {
			assertError(t, f, http.MethodGet, "/messages", nil, nil, http.StatusServiceUnavailable, cell.code)
		} else if page := readPage(t, f, "/messages", nil); len(page.Messages) != 0 {
			t.Fatal("isolated inbox is not empty")
		}
		assertHealthyWork(t, f)
		events := comparableFactoryEvents(t, f.server.GetFactoryEvents(t))
		f.server.Close(t)
		if baseline == nil {
			baseline = events
		} else if !reflect.DeepEqual(baseline, events) {
			t.Fatalf("messaging %s changed canonical Factory events:\ndisabled: %s\nactual: %s", cell.name, baseline, events)
		}
	}
}

func comparableFactoryEvents(t *testing.T, events []api.FactoryEvent) []string {
	t.Helper()
	// Replace only identities taken from canonical context, everywhere they
	// occur. Relationships, event order, ticks and all payload fields survive.
	identities := make(map[string]string)
	for _, event := range events {
		for _, identity := range []*string{event.Context.RequestId, event.Context.DispatchId} {
			if identity != nil {
				if _, exists := identities[*identity]; !exists {
					identities[*identity] = fmt.Sprintf("identity-%d", len(identities))
				}
			}
		}
	}
	result := make([]string, 0, len(events))
	for _, event := range events {
		event.Id = ""
		encoded, err := json.Marshal(event)
		if err != nil {
			t.Fatal(err)
		}
		value := string(encoded)
		for identity, replacement := range identities {
			value = strings.ReplaceAll(value, identity, replacement)
		}
		result = append(result, value)
	}
	return result
}

func TestFailedReplyWritePreservesParentAndRetryAuthority(t *testing.T) {
	t.Parallel()
	f := startHost(t, t.TempDir(), t.TempDir(), `{"messaging":{"limits":{"sendsPerChainPerHour":1}}}`, "lead", "lane")
	lead := f.invoke(t, "lead", nil)
	lane := f.invoke(t, "lane", lead)
	parent := f.cliSend(t, lane, "lead", "--request-id", "parent", "--body", "durable parent")
	path := filepath.Join(f.home, ".you-agent-factory", "messages", "messages.jsonl")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	backup := path + ".preserved"
	// A directory at the selected file path is a real, portable write fault.
	// Preserve the customer's bytes while making only this test-owned path
	// unwritable; restore it before retrying the same public request.
	if err := os.Rename(path, backup); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	request := map[string]any{"requestId": "retry-reply", "inReplyTo": parent.MessageId, "body": "retryable reply"}
	assertError(t, f, http.MethodPost, "/messages", request, lead, http.StatusServiceUnavailable, "MESSAGE_STORE_UNAVAILABLE")
	page := readPage(t, f, "/messages?threadId="+parent.ThreadId, lane)
	if len(page.Messages) != 1 || page.Messages[0].Status != "QUEUED" || page.Messages[0].RepliedByMessageId != nil {
		t.Fatal("failed flush published reply or changed its parent")
	}
	preserved, err := os.ReadFile(backup)
	if err != nil || !bytes.Equal(before, preserved) {
		t.Fatal("failed write altered previously durable records")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(backup, path); err != nil {
		t.Fatal(err)
	}
	reply := f.send(t, request, lead)
	if reply.ThreadId != parent.ThreadId || reply.To.WorkerSessionId != "lane" {
		t.Fatal("write failure spent request alias or successful-send authority")
	}
	assertRepliedParent(t, f, parent.MessageId, reply.MessageId)
	if replay := f.send(t, request, lead); replay.MessageId != reply.MessageId {
		t.Fatal("successful retry was not idempotent")
	}
	f.server.Close(t)
	second := startHost(t, f.home, f.dir, "")
	assertRepliedParent(t, second, parent.MessageId, reply.MessageId)
	if page := readPage(t, second, "/messages?threadId="+parent.ThreadId, nil); len(page.Messages) != 2 {
		t.Fatal("reconstruction lost atomic reply or retained a failed attempt")
	}
}

func TestReadOnlyRetentionStorePreservesInboxAndFactoryWork(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("directory mode bits do not revoke Windows write access; hosted Linux proves the real filesystem refusal")
	}
	clock := platformclock.NewDeterministic(time.Now().UTC(), time.Second)
	effects := serviceedges.Edges{Clock: struct{ platformclock.Source }{clock}}
	home, dir := t.TempDir(), t.TempDir()
	settings := `{"messaging":{"retentionDays":1,"limits":{"defaultExpirySeconds":60}}}`
	f := startHostEffects(t, home, dir, settings, effects, "lead", "lane")
	lead := f.invoke(t, "lead", nil)
	lane := f.invoke(t, "lane", lead)
	old := f.cliSend(t, lane, "lead", "--request-id", "old", "--body", "settled before retention")
	clock.SetTick(60)
	if page := readPage(t, f, "/messages?threadId="+old.ThreadId, lane); len(page.Messages) != 1 || page.Messages[0].Status != "EXPIRED" {
		t.Fatal("retention fixture did not durably settle its old message")
	}
	clock.SetTick(23 * 60 * 60)
	recent := f.cliSend(t, lane, "lead", "--request-id", "recent", "--body", "retain through refusal")
	f.server.Close(t)
	path := filepath.Join(home, ".you-agent-factory", "messages", "messages.jsonl")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	storeDir := filepath.Dir(path)
	if err := os.Chmod(storeDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(storeDir, 0o700); err != nil {
			t.Error(err)
		}
	})
	clock.SetTick(25 * 60 * 60)
	second := startHostEffects(t, home, dir, settings, serviceedges.Merge(healthyProviderEffects(), effects))
	assertError(t, second, http.MethodGet, "/messages", nil, nil, http.StatusServiceUnavailable, "MESSAGE_STORE_UNAVAILABLE")
	assertHealthyWork(t, second)
	second.server.Close(t)
	preserved, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, preserved) {
		t.Fatal("failed retention replaced the committed journal")
	}
	if err := os.Chmod(storeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	third := startHostEffects(t, home, dir, settings, effects)
	assertError(t, third, http.MethodGet, "/messages/"+old.MessageId, nil, nil, http.StatusNotFound, "MESSAGE_NOT_FOUND")
	if page := readPage(t, third, "/messages", nil); len(page.Messages) != 1 || page.Messages[0].MessageId != recent.MessageId || page.Messages[0].BodySha256 != recent.BodySha256 || page.Messages[0].Status != "EXPIRED" {
		t.Fatal("retention retry lost the surviving committed message")
	}
}
