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
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// Gate the selected profile's actual outcome flush, leaving journal policy and
// provider admission in production. No service or journal internals are replaced.
type messageOutcomeFileSystem struct {
	platformfilesystem.Local
	entered, release chan struct{}
	unblock          sync.Once
	reject           bool
	state            string
	kind             string
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
		Kind            string `json:"kind"`
		DeliveryIntents []struct {
			State string `json:"state"`
		} `json:"deliveryIntents"`
	}
	if json.Unmarshal(data, &record) == nil {
		kind := f.gate.kind
		if kind == "" {
			kind = "REVIVE_OUTCOME"
		}
		f.outcome = record.Kind == kind
		if f.gate.state != "" {
			f.outcome = f.outcome && len(record.DeliveryIntents) == 1 && record.DeliveryIntents[0].State == f.gate.state
		}
	}
	return f.File.Write(data)
}

func (f *messageOutcomeFile) Sync() error {
	if f.outcome {
		f.outcome = false // Rollback's Sync must flush without entering again.
		f.gate.unblockEnter()
		<-f.gate.release
		if f.gate.reject {
			return errors.New("controlled-private-outcome-flush-failure")
		}
	}
	return f.File.Sync()
}

func (g *messageOutcomeFileSystem) unblockEnter() {
	select {
	case <-g.entered:
	default:
		close(g.entered)
	}
}

type outcomeSendResponse struct {
	status int
	body   []byte
}

// The diagnostic sink acknowledges completion of the controlled failure.
// Durable facts and public streams remain the behavioral assertions.
type outcomeFailureSignal struct {
	zapcore.Core
	failed chan struct{}
	once   *sync.Once
}

func (c *outcomeFailureSignal) With(fields []zapcore.Field) zapcore.Core {
	return &outcomeFailureSignal{Core: c.Core.With(fields), failed: c.failed, once: c.once}
}

func (c *outcomeFailureSignal) Check(entry zapcore.Entry, checked *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(entry.Level) {
		return checked.AddCore(entry, c)
	}
	return checked
}

func (c *outcomeFailureSignal) Write(entry zapcore.Entry, fields []zapcore.Field) error {
	if entry.Message == "Agent Message revival outcome unavailable" {
		c.once.Do(func() { close(c.failed) })
	}
	return c.Core.Write(entry, fields)
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

// The provider can confirm progress after send has returned. Its final
// observation still waits for the selected profile's durable outcome flush.
func TestProviderConfirmedDeliveryWaitsForOutcomeFlush(t *testing.T) {
	t.Parallel()
	for _, reject := range []bool{false, true} {
		t.Run(map[bool]string{false: "committed", true: "refused"}[reject], func(t *testing.T) {
			t.Parallel()
			exerciseProviderOutcomeFlush(t, reject)
		})
	}
}

func exerciseProviderOutcomeFlush(t *testing.T, reject bool) {
	t.Helper()
	gate := &messageOutcomeFileSystem{entered: make(chan struct{}), release: make(chan struct{}), reject: reject, state: "ACCEPTED"}
	defer gate.unblock.Do(func() { close(gate.release) })
	progress := make(chan struct{})
	revival := &workerGate{started: make(chan map[string]string, 1), requests: make(chan platformprocess.CommandRequest, 1), release: make(chan struct{}), progress: progress}
	core, _ := observer.New(zap.InfoLevel)
	failure := &outcomeFailureSignal{Core: core, failed: make(chan struct{}), once: &sync.Once{}}
	signal := &subscriptionSignal{Core: failure, ready: make(chan struct{}), once: &sync.Once{}}
	f := startHostRunner(t, t.TempDir(), t.TempDir(), "", serviceedges.Edges{AgentMessageStoreFileSystem: gate, ProcessLogger: zap.New(signal)}, nil, revival, "lead", "lane")
	lead := f.invoke(t, "lead", nil)
	lane := f.invoke(t, "lane", lead)
	f.endWorker(t, "lead")
	before := messageFactoryStreams(t, f, "lead", "lane")
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	frames, finished := make(chan []byte, 8), make(chan error, 1)
	go streamFramesAt(ctx, f.server.URL(), "/messages?follow=true", lane, frames, finished)
	select {
	case <-signal.ready:
	case <-ctx.Done():
		t.Fatal("provider flush follow did not register")
	}
	request := map[string]any{"requestId": "provider-flush", "to": map[string]string{"workerSessionId": "lead"}, "body": "durable provider confirmation"}
	pending := f.sendRaw(t, request, lane)
	assertPendingProviderResponse(t, pending)
	assertPendingProviderFrames(t, ctx, pending, frames, finished)
	successor := <-revival.started
	<-revival.requests
	close(progress)
	select {
	case <-gate.entered:
	case <-ctx.Done():
		t.Fatal("provider confirmation did not reach outcome flush")
	}
	select {
	case <-frames:
		t.Fatal("provider delivery published before durable flush")
	default:
	}
	gate.unblock.Do(func() { close(gate.release) })
	final := pending
	if reject {
		select {
		case <-failure.failed:
		case <-ctx.Done():
			t.Fatal("provider outcome failure did not complete")
		}
		assertProviderRetry(t, f, request, lane, pending)
		select {
		case <-frames:
			t.Fatal("refused provider flush published delivery")
		default:
		}
	} else {
		final = f.awaitDeliveryOutcome(t, lane, pending)
		assertProviderFinalOutcome(t, ctx, f, final, successor, frames, finished, "")
		assertProviderRetry(t, f, request, lane, final)
	}
	if after := messageFactoryStreams(t, f, "lead", "lane"); !reflect.DeepEqual(before, after) {
		t.Fatal("provider flush changed Factory Events")
	}
	cancel()
	select {
	case <-finished:
	case <-time.After(15 * time.Second):
		t.Fatal("provider flush follow did not join")
	}
	close(revival.release)
	f.waitEndedWorker(t, successor["YOU_WORKER_SESSION_ID"])
	f.endWorker(t, "lane")
	f.server.Close(t)
	second := startHost(t, f.home, f.dir, "")
	page := readPage(t, second, "/messages?threadId="+pending.ThreadId, nil)
	if len(page.Messages) != 1 || !reflect.DeepEqual(page.Messages[0], final) {
		t.Fatal("reconstruction changed the last durable provider outcome")
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
	assertPendingOutcomeFrame(t, ctx, frames, finished)
	message := f.awaitDeliveryOutcome(t, lane, responseBody.Message)
	if json.Unmarshal(nextFrame(t, ctx, frames, finished), &outcome) != nil || outcome.Kind != "DELIVERED_REVIVE" || !reflect.DeepEqual(outcome.Message, message) {
		t.Fatal("committed delivery differs from follow")
	}
	got := readPage(t, f, "/messages?threadId="+queued.ThreadId, lane)
	if len(got.Messages) != 1 || !reflect.DeepEqual(got.Messages[0], message) {
		t.Fatal("committed delivery differs from read")
	}
	successor := assertReviveAdmission(t, f, revival, message, lead)
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

func TestRecipientEndDuringSendFlushDeliversOnce(t *testing.T) {
	t.Parallel()
	gate := &messageOutcomeFileSystem{entered: make(chan struct{}), release: make(chan struct{}), kind: "SENT"}
	revival := &workerGate{started: make(chan map[string]string, 1), requests: make(chan platformprocess.CommandRequest, 1), release: make(chan struct{})}
	f := startHostRunner(t, t.TempDir(), t.TempDir(), "", serviceedges.Edges{AgentMessageStoreFileSystem: gate}, nil, revival, "lead", "lane")
	t.Cleanup(func() { gate.unblock.Do(func() { close(gate.release) }) })
	lead := f.invoke(t, "lead", nil)
	lane := f.invoke(t, "lane", lead)
	result := make(chan outcomeSendResponse, 1)
	request := map[string]any{"requestId": "end-during-send", "to": map[string]string{"workerSessionId": "lead"}, "body": "input committed after recipient end"}
	go func() {
		status, body := f.request(t, http.MethodPost, "/messages", request, lane)
		result <- outcomeSendResponse{status: status, body: body}
	}()
	select {
	case <-gate.entered:
	case <-time.After(15 * time.Second):
		t.Fatal("send did not reach its durable flush")
	}
	// The recipient ends before SENT commits or an end observer can register.
	// Worker completion is independent of the blocked Messaging journal.
	f.endWorker(t, "lead")
	before := messageFactoryStreams(t, f, "lead", "lane")
	select {
	case <-revival.requests:
		t.Fatal("uncommitted send reached the provider")
	case <-result:
		t.Fatal("send returned before its durable flush")
	default:
	}
	gate.unblock.Do(func() { close(gate.release) })
	var response outcomeSendResponse
	select {
	case response = <-result:
	case <-time.After(15 * time.Second):
		t.Fatal("committed send did not finish")
	}
	var sent api.AgentMessageSendResponse
	if response.status != http.StatusAccepted || json.Unmarshal(response.body, &sent) != nil {
		t.Fatalf("send after recipient end: %d %s", response.status, response.body)
	}
	final := f.awaitDeliveryOutcome(t, lane, sent.Message)
	head := assertReviveAdmission(t, f, revival, final, lead)
	if replay := f.cliSend(t, lane, "lead", "--request-id", "end-during-send", "--body", request["body"].(string)); !reflect.DeepEqual(replay, final) {
		t.Fatal("CLI retry changed the HTTP delivery or started a second send")
	}
	close(revival.release)
	f.waitEndedWorker(t, head["YOU_WORKER_SESSION_ID"])
	assertContinuationLinks(t, f, "lead", head["YOU_WORKER_SESSION_ID"])
	if after := messageFactoryStreams(t, f, "lead", "lane"); !reflect.DeepEqual(before, after) {
		t.Fatal("send/end overlap changed canonical Factory Events")
	}
	select {
	case <-revival.requests:
		t.Fatal("send/end overlap or retry branched the continuation")
	default:
	}
}

func TestStartupDeliversUnreadMessagesWithoutAnotherSend(t *testing.T) {
	t.Parallel()
	home, dir := t.TempDir(), t.TempDir()
	gate := &messageOutcomeFileSystem{entered: make(chan struct{}), release: make(chan struct{}), reject: true, kind: "REVIVE_INTENT"}
	revival := &workerGate{started: make(chan map[string]string, 1), requests: make(chan platformprocess.CommandRequest, 1), release: make(chan struct{})}
	f := startHostRunner(t, home, dir, "", serviceedges.Edges{AgentMessageStoreFileSystem: gate}, nil, revival, "lead", "lane")
	t.Cleanup(func() { gate.unblock.Do(func() { close(gate.release) }) })
	lead := f.invoke(t, "lead", nil)
	lane := f.invoke(t, "lane", lead)
	first := f.cliSend(t, lane, "lead", "--request-id", "first", "--body", "first retained input")
	held := f.cliSend(t, lane, "lead", "--request-id", "held", "--body", "held retained input", "--if-ended", "hold")
	second := f.cliSend(t, lane, "lead", "--request-id", "second", "--body", "second retained input")
	f.endWorker(t, "lead")
	select {
	case <-gate.entered:
	case <-time.After(15 * time.Second):
		t.Fatal("recipient end did not attempt durable delivery")
	}
	gate.unblock.Do(func() { close(gate.release) })
	// An unavailable intent flush has no continuation effect. Startup must
	// recover these unread messages without the former process's Events or
	// credentials, and without a new send to wake delivery.
	if page := readPage(t, f, "/messages?toWorkerSessionId=lead", nil); len(page.Messages) != 3 || page.Messages[0].Status != "QUEUED" || page.Messages[0].RevivedWorkerSessionId != nil {
		t.Fatal("refused intent flush claimed delivery or lost the inbox")
	}
	f.endWorker(t, "lane")
	f.server.Close(t)
	select {
	case <-revival.requests:
		t.Fatal("refused intent flush reached the provider")
	default:
	}
	assertInactiveStartupPreservesUnread(t, f, revival, first.MessageId)
	next := startHostRunner(t, home, dir, `{"messaging":{"enabled":true}}`, serviceedges.Edges{}, nil, revival)
	pending := "REVIVE_PENDING"
	first.Reason = &pending
	final := next.awaitDeliveryOutcome(t, nil, first)
	batch := final
	batch.Body = first.Body + "\n\n" + second.Body
	head := assertReviveAdmission(t, next, revival, batch, lead)
	page := readPage(t, next, "/messages?toWorkerSessionId=lead", nil)
	assertStartupDeliverySelection(t, page, head["YOU_WORKER_SESSION_ID"], first, held, second)
	assertError(t, next, http.MethodGet, "/messages/"+first.MessageId, nil, lane, http.StatusForbidden, "MESSAGE_NOT_PERMITTED")
	before := next.server.GetFactoryEvents(t)
	close(revival.release)
	next.waitEndedWorker(t, head["YOU_WORKER_SESSION_ID"])
	if after := next.server.GetFactoryEvents(t); !reflect.DeepEqual(before, after) {
		t.Fatal("direct recovered successor changed canonical Factory Events")
	}
	next.server.Close(t)
	third := startHostRunner(t, home, dir, "", serviceedges.Edges{}, nil, revival)
	if reopened := readPage(t, third, "/messages?toWorkerSessionId=lead", nil); !reflect.DeepEqual(page, reopened) {
		t.Fatal("second startup changed delivered or held messages")
	}
	third.server.Close(t)
	select {
	case <-revival.requests:
		t.Fatal("restart or held input duplicated the continuation")
	default:
	}
}

// Replacing the graph is necessary to select a different immutable profile
// policy. Both inactive hosts retain the exact inbox; later re-enabling must
// deliver the existing REVIVE messages without a new send.
func assertInactiveStartupPreservesUnread(t *testing.T, first *fixture, revival *workerGate, messageID string) {
	t.Helper()
	path := filepath.Join(first.home, ".you-agent-factory", "messages", "messages.jsonl")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, cell := range []struct{ name, settings, code string }{
		{"disabled", `{"messaging":{"enabled":false}}`, "MESSAGING_DISABLED"},
		{"quarantined", `{"messaging":{"limits":{"defaultExpirySeconds":59}}}`, "MESSAGE_STORE_UNAVAILABLE"},
	} {
		f := startHostRunner(t, first.home, first.dir, cell.settings, serviceedges.Edges{}, nil, revival, cell.name)
		caller := f.invoke(t, cell.name, nil)
		for _, route := range []string{"/messages", "/messages/" + messageID, "/messages?follow=true"} {
			assertError(t, f, http.MethodGet, route, nil, nil, http.StatusServiceUnavailable, cell.code)
		}
		assertError(t, f, http.MethodPost, "/messages", map[string]any{"requestId": "inactive-send", "to": map[string]string{"workerSessionId": "lead"}, "body": "inactive send"}, caller, http.StatusServiceUnavailable, cell.code)
		// Worker execution and owned shutdown still function while Messaging
		// cannot activate delivery; join the host before inspecting effects.
		f.endWorker(t, cell.name)
		f.server.Close(t)
		select {
		case <-revival.requests:
			t.Fatalf("%s startup revived the retained recipient", cell.name)
		default:
		}
		after, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(before, after) {
			t.Fatalf("%s startup changed or erased the retained inbox", cell.name)
		}
	}
}

func assertStartupDeliverySelection(t *testing.T, page api.AgentMessageListResponse, head string, messages ...api.AgentMessage) {
	t.Helper()
	if len(page.Messages) != len(messages) {
		t.Fatal("startup invented or lost an original message")
	}
	for i, message := range page.Messages {
		if message.MessageId != messages[i].MessageId || message.Body != messages[i].Body {
			t.Fatal("startup changed durable send order or input")
		}
		if i == 1 {
			if message.Status != "QUEUED" || message.RevivedWorkerSessionId != nil {
				t.Fatal("startup revived a held message")
			}
		} else if message.Status != "DELIVERED_REVIVE" || message.RevivedWorkerSessionId == nil || *message.RevivedWorkerSessionId != head {
			t.Fatal("startup did not share exactly one confirmed successor")
		}
	}
}
