package messaging

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	api "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestRecipientEndHonorsSelectedReviveLimit(t *testing.T) {
	t.Parallel()
	revival := &workerGate{started: make(chan map[string]string, 1), requests: make(chan platformprocess.CommandRequest, 1), release: make(chan struct{})}
	core, _ := observer.New(zap.InfoLevel)
	signal := &subscriptionSignal{Core: core, ready: make(chan struct{}), once: &sync.Once{}}
	f := startHostRunner(t, t.TempDir(), t.TempDir(), `{"messaging":{"limits":{"revivesPerChainPerHour":1}}}`, serviceedges.Edges{ProcessLogger: zap.New(signal)}, nil, revival, "lead", "lane")
	lead := f.invoke(t, "lead", nil)
	lane := f.invoke(t, "lane", lead)
	f.endWorker(t, "lead")
	before := messageFactoryStreams(t, f, "lead", "lane")
	first := f.cliSend(t, lane, "lead", "--request-id", "first", "--body", "first allowed resume")
	head := assertReviveAdmission(t, f, revival, first, lead)
	queued := f.cliSend(t, lane, "lead", "--request-id", "second", "--body", "ask while successor runs")
	if queued.Status != "QUEUED" || queued.Reason != nil {
		t.Fatal("running successor did not queue the second ask")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	frames, finished := make(chan []byte, 16), make(chan error, 1)
	go streamFramesAt(ctx, f.server.URL(), "/messages?follow=true", lane, frames, finished)
	select {
	case <-signal.ready:
	case <-ctx.Done():
		t.Fatal("quota follow did not register")
	}
	close(revival.release)
	f.waitEndedWorker(t, head["YOU_WORKER_SESSION_ID"])
	limited := awaitQueuedReviveLimit(t, ctx, queued.MessageId, frames, finished)
	if limited.Body != queued.Body || limited.ThreadId != queued.ThreadId || limited.RevivedWorkerSessionId != nil {
		t.Fatal("on-end quota lost the original queued ask")
	}
	if retry := f.cliSend(t, lane, "lead", "--request-id", "second", "--body", queued.Body); !reflect.DeepEqual(retry, limited) {
		t.Fatal("CLI retry differs from the on-end limit observation")
	}
	assertProviderRetry(t, f, map[string]any{"requestId": "second", "to": map[string]string{"workerSessionId": "lead"}, "body": queued.Body}, lane, limited)
	if after := messageFactoryStreams(t, f, "lead", "lane"); !reflect.DeepEqual(before, after) {
		t.Fatal("on-end quota changed Factory Events")
	}
	select {
	case <-revival.requests:
		t.Fatal("on-end quota or retries started another provider")
	default:
	}
	cancel()
	select {
	case <-finished:
	case <-time.After(15 * time.Second):
		t.Fatal("quota follow did not join")
	}
	f.endWorker(t, "lane")
}

func awaitQueuedReviveLimit(t *testing.T, ctx context.Context, id string, frames <-chan []byte, finished <-chan error) api.AgentMessage {
	t.Helper()
	for {
		var observation api.AgentMessageObservation
		if err := json.Unmarshal(nextFrame(t, ctx, frames, finished), &observation); err != nil {
			t.Fatal(err)
		}
		message := observation.Message
		if message.MessageId == id && message.Reason != nil && *message.Reason == "REVIVE_LIMIT" {
			if message.Status != "QUEUED" || observation.Kind != "DELIVERY_UPDATED" {
				t.Fatal("on-end quota published an invalid transition")
			}
			return message
		}
	}
}

func TestRecipientChainReviveLimitCountsConfirmedSuccessors(t *testing.T) {
	t.Parallel()
	for _, cell := range []struct {
		name, settings string
		limit          int
	}{
		{"default six", "", 6},
		{"selected two", `{"messaging":{"limits":{"revivesPerChainPerHour":2}}}`, 2},
	} {
		t.Run(cell.name, func(t *testing.T) {
			t.Parallel()
			assertRecipientChainReviveLimit(t, cell.settings, cell.limit)
		})
	}
}

func assertRecipientChainReviveLimit(t *testing.T, settings string, limit int) {
	t.Helper()
	revival := &workerGate{started: make(chan map[string]string, 6), requests: make(chan platformprocess.CommandRequest, 6), release: make(chan struct{})}
	// Every controlled resume completes; the public terminal stream gates the
	// next send against the ended head rather than racing its lifecycle.
	close(revival.release)
	clock := platformclock.NewDeterministic(time.Now().UTC(), time.Second)
	effects := serviceedges.Edges{Clock: struct{ platformclock.Source }{clock}}
	f := startHostRunner(t, t.TempDir(), t.TempDir(), settings, effects, nil, revival, "lead", "lane")
	lead := f.invoke(t, "lead", nil)
	lane := f.invoke(t, "lane", lead)
	f.endWorker(t, "lead")
	before := messageFactoryStreams(t, f, "lead", "lane")
	for i := range limit {
		request := map[string]any{"requestId": fmt.Sprintf("revive-%d", i), "to": map[string]string{"workerSessionId": "lead"}, "body": fmt.Sprintf("independent ask %d", i)}
		message := f.send(t, request, lane)
		successor := assertReviveAdmission(t, f, revival, message, lead)
		f.waitEndedWorker(t, successor["YOU_WORKER_SESSION_ID"])
		assertConcurrentReplay(t, f, request, lane, message.MessageId)
	}
	request := map[string]any{"requestId": "over-limit", "to": map[string]string{"workerSessionId": "lead"}, "body": "preserve the over-limit ask"}
	limited := f.sendRaw(t, request, lane)
	if limited.Status != "QUEUED" || limited.Reason == nil || *limited.Reason != "REVIVE_LIMIT" || limited.RevivedWorkerSessionId != nil {
		t.Fatal("confirmed-chain revive above the selected limit did not remain queued")
	}
	assertProviderRetry(t, f, request, lane, limited)
	page := readPage(t, f, "/messages?threadId="+limited.ThreadId, lane)
	if len(page.Messages) != 1 || page.Messages[0].MessageId != limited.MessageId || page.Messages[0].Body != limited.Body {
		t.Fatal("revive limit lost the readable queued ask")
	}
	select {
	case <-revival.requests:
		t.Fatal("revive limit or exact retries started another provider")
	default:
	}
	if after := messageFactoryStreams(t, f, "lead", "lane"); !reflect.DeepEqual(before, after) {
		t.Fatal("chain revive quota changed Factory Events")
	}
	f.endWorker(t, "lane")
	f.server.Close(t)
	assertReconstructedReviveQuota(t, f, limited, clock, effects, limit)
}

// Reconstruction is the owned behavior here, so only this cohort replaces the
// root graph. Fresh server-minted sender authority must retain the old chain's
// successful revive volume without making archived execution tokens valid.
func assertReconstructedReviveQuota(t *testing.T, first *fixture, limited api.AgentMessage, clock *platformclock.Deterministic, effects serviceedges.Edges, limit int) {
	t.Helper()
	revival := &workerGate{started: make(chan map[string]string, 1), requests: make(chan platformprocess.CommandRequest, 1), release: make(chan struct{})}
	close(revival.release)
	f := startHostRunner(t, first.home, first.dir, "", effects, nil, revival, "lane-next")
	lane := f.continueCaller(t, "lane", "lane-next")
	before := f.server.GetFactoryEvents(t)
	page := readPage(t, f, "/messages?toWorkerSessionId=lead", nil)
	if len(page.Messages) != limit+1 || !reflect.DeepEqual(page.Messages[limit], limited) {
		t.Fatal("reconstruction lost accepted revives or the limited ask")
	}
	start := make(chan struct{})
	results := make(chan api.AgentMessage, 2)
	for _, id := range []string{"after-restart-a", "after-restart-b"} {
		go func() {
			<-start
			request := map[string]any{"requestId": id, "to": map[string]string{"workerSessionId": "lead"}, "body": id}
			queued := f.sendRaw(t, request, lane)
			assertConcurrentReplay(t, f, request, lane, queued.MessageId)
			results <- queued
		}()
	}
	close(start)
	for range 2 {
		queued := <-results
		if queued.Status != "QUEUED" || queued.Reason == nil || *queued.Reason != "REVIVE_LIMIT" || queued.RevivedWorkerSessionId != nil {
			t.Fatal("restart or concurrent asks reset confirmed chain revive volume")
		}
	}
	select {
	case <-revival.requests:
		t.Fatal("reconstructed quota or retries started a provider")
	default:
	}
	assertReviveQuotaWindowRelease(t, f, revival, lane, limited, clock, before, limit)
	f.endWorker(t, "lane-next")
}

func assertReviveQuotaWindowRelease(t *testing.T, f *fixture, revival *workerGate, lane map[string]string, limited api.AgentMessage, clock *platformclock.Deterministic, before []api.FactoryEvent, limit int) {
	t.Helper()
	// The window excludes successful admissions at its exact one-hour boundary.
	// The clock edge changes timestamps only; completion remains stream-driven.
	clock.SetTick(3600)
	page := readPage(t, f, "/messages?toWorkerSessionId=lead", nil)
	if len(page.Messages) != limit+3 {
		t.Fatal("concurrent asks or retries lost messages or created duplicates")
	}
	last := f.cliSend(t, lane, "lead", "--request-id", "after-window", "--body", "ask after window")
	batch := last
	batch.Body = limited.Body + "\n\n" + page.Messages[limit+1].Body + "\n\n" + page.Messages[limit+2].Body + "\n\n" + last.Body
	assertReviveAdmission(t, f, revival, batch, nil)
	f.waitEndedWorker(t, *last.RevivedWorkerSessionId)
	page = readPage(t, f, "/messages?toWorkerSessionId=lead", nil)
	if len(page.Messages) != limit+4 {
		t.Fatal("window release invented or lost messages")
	}
	for _, message := range page.Messages[limit:] {
		if message.Status != "DELIVERED_REVIVE" || message.Reason != nil || message.RevivedWorkerSessionId == nil || *message.RevivedWorkerSessionId != *last.RevivedWorkerSessionId {
			t.Fatal("window release lost ordered shared delivery")
		}
	}
	if after := f.server.GetFactoryEvents(t); !reflect.DeepEqual(before, after) {
		t.Fatal("reconstructed quota or ordered batch changed Factory Events")
	}
}

func TestConfiguredLimitsPrivacyAndConcurrentReplay(t *testing.T) {
	t.Parallel()
	f := startHost(t, t.TempDir(), t.TempDir(), `{"messaging":{"limits":{"sendsPerChainPerHour":2,"messagesPerThread":2,"maxBodyBytes":128,"defaultExpirySeconds":60}}}`, "lead", "lane")
	lead := f.invoke(t, "lead", nil)
	lane := f.invoke(t, "lane", lead)
	secret := "declared-secret-value-" + lane["YOU_WORKER_SESSION_TOKEN"]
	request := map[string]any{"requestId": "secret", "to": map[string]string{"workerSessionId": "lead"}, "body": secret, "bodySecret": true}
	message := f.send(t, request, lane)
	hash := sha256.Sum256([]byte(message.Body))
	if strings.Contains(message.Body, secret) || message.BodyRedactionCount == 0 || message.BodySha256 != hex.EncodeToString(hash[:]) || message.ExpiresAt.Sub(message.SentAt) != time.Minute {
		t.Fatal("selected expiry or privacy normalization was not applied before success")
	}
	assertConcurrentReplay(t, f, request, lane, message.MessageId)
	reply := f.send(t, map[string]any{"requestId": "secret-reply", "inReplyTo": message.MessageId, "body": "another-declared-secret", "bodySecret": true}, lead)
	assertError(t, f, http.MethodPost, "/messages", map[string]any{"requestId": "thread-overflow", "inReplyTo": reply.MessageId, "body": "thread overflow"}, lane, http.StatusTooManyRequests, "MESSAGE_LIMIT_EXCEEDED")
	assertError(t, f, http.MethodPost, "/messages", map[string]any{"requestId": "interrupt", "to": map[string]string{"workerSessionId": "lead"}, "body": "interrupt", "delivery": "INTERRUPT"}, lane, http.StatusBadRequest, "MESSAGE_INTERRUPT_UNSUPPORTED")
	f.send(t, map[string]any{"requestId": "second", "to": map[string]string{"workerSessionId": "lead"}, "body": "second successful send"}, lane)
	assertError(t, f, http.MethodPost, "/messages", map[string]any{"requestId": "rate-overflow", "to": map[string]string{"workerSessionId": "lead"}, "body": "rate overflow"}, lane, http.StatusTooManyRequests, "MESSAGE_LIMIT_EXCEEDED")
	status, bytes := f.request(t, http.MethodGet, "/messages?threadId="+message.ThreadId, nil, nil)
	var page api.AgentMessageListResponse
	if status != http.StatusOK || json.Unmarshal(bytes, &page) != nil || len(page.Messages) != 2 {
		t.Fatalf("failed writes or replays created records: %d %s", status, bytes)
	}
	f.server.Close(t)
	assertPrivateArtifacts(t, f.home, []string{secret, "another-declared-secret", lead["YOU_WORKER_SESSION_TOKEN"], lane["YOU_WORKER_SESSION_TOKEN"]})
	assertPrivateArtifacts(t, f.dir, []string{secret, "another-declared-secret", lead["YOU_WORKER_SESSION_TOKEN"], lane["YOU_WORKER_SESSION_TOKEN"]})
}

func assertConcurrentReplay(t *testing.T, f *fixture, request map[string]any, caller map[string]string, id string) {
	t.Helper()
	var group sync.WaitGroup
	for range 8 {
		group.Add(1)
		go func() {
			defer group.Done()
			status, bytes := f.request(t, http.MethodPost, "/messages", request, caller)
			var response api.AgentMessageSendResponse
			if status != http.StatusAccepted || json.Unmarshal(bytes, &response) != nil || response.Message.MessageId != id {
				t.Errorf("concurrent retry changed admission: %d", status)
			}
		}()
	}
	group.Wait()
}

// These are emitted customer artifacts, not source/inventory inspection.
func assertPrivateArtifacts(t *testing.T, root string, private []string) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		bytes, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, value := range private {
			if strings.Contains(string(bytes), value) {
				t.Errorf("private message or credential leaked to emitted artifact %s", entry.Name())
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentRepliesCommitOneParentTransition(t *testing.T) {
	t.Parallel()
	f := startHost(t, t.TempDir(), t.TempDir(), "", "lead", "lane")
	lead := f.invoke(t, "lead", nil)
	lane := f.invoke(t, "lane", lead)
	parent := f.cliSend(t, lane, "lead", "--request-id", "parent", "--body", "concurrent reply parent")
	type outcome struct {
		status int
		bytes  []byte
	}
	results := make(chan outcome, 2)
	start := make(chan struct{})
	for _, id := range []string{"reply-a", "reply-b"} {
		go func() {
			<-start
			status, bytes := f.request(t, http.MethodPost, "/messages", map[string]any{"requestId": id, "inReplyTo": parent.MessageId, "body": id}, lead)
			results <- outcome{status, bytes}
		}()
	}
	close(start)
	winner := ""
	for range 2 {
		result := <-results
		switch result.status {
		case http.StatusAccepted:
			var reply api.AgentMessageSendResponse
			if json.Unmarshal(result.bytes, &reply) != nil || winner != "" || reply.Message.ThreadId != parent.ThreadId {
				t.Fatal("concurrent replies admitted multiple or malformed winners")
			}
			winner = reply.Message.MessageId
		case http.StatusBadRequest:
			var failure api.ErrorResponse
			if json.Unmarshal(result.bytes, &failure) != nil || failure.Code != "MESSAGE_INVALID_REQUEST" {
				t.Fatalf("reply loser was not typed: %s", result.bytes)
			}
		default:
			t.Fatalf("unexpected concurrent reply outcome: %d %s", result.status, result.bytes)
		}
	}
	if winner == "" {
		t.Fatal("no reply was admitted")
	}
	assertRepliedParent(t, f, parent.MessageId, winner)
	if page := readPage(t, f, "/messages?threadId="+parent.ThreadId, nil); len(page.Messages) != 2 {
		t.Fatal("concurrent reply left an orphan or duplicate thread row")
	}
}

func TestRefusedMessagesPreserveQuotaAndPrivateDiagnostics(t *testing.T) {
	t.Parallel()
	core, logs := observer.New(zap.DebugLevel)
	f := startHostEffects(t, t.TempDir(), t.TempDir(), `{"messaging":{"limits":{"sendsPerChainPerHour":1,"maxBodyBytes":64}}}`, serviceedges.Edges{ProcessLogger: zap.New(core)}, "lead", "lane")
	lead := f.invoke(t, "lead", nil)
	lane := f.invoke(t, "lane", lead)
	for _, cell := range []struct {
		name, body, target, parent, code string
		status                           int
	}{
		{"empty", "", "lead", "", "MESSAGE_INVALID_REQUEST", http.StatusBadRequest},
		{"byte overflow", strings.Repeat("🙂", 17), "lead", "", "MESSAGE_LIMIT_EXCEEDED", http.StatusTooManyRequests},
		{"self", "self refused", "lane", "", "MESSAGE_NOT_PERMITTED", http.StatusForbidden},
		{"missing parent", "unknown reply", "", "missing", "MESSAGE_NOT_FOUND", http.StatusNotFound},
	} {
		t.Run(cell.name, func(t *testing.T) {
			request := map[string]any{"requestId": "reusable", "body": cell.body}
			if cell.target != "" {
				request["to"] = map[string]string{"workerSessionId": cell.target}
			} else {
				request["inReplyTo"] = cell.parent
			}
			assertError(t, f, http.MethodPost, "/messages", request, lane, cell.status, cell.code)
		})
	}
	if page := readPage(t, f, "/messages", nil); len(page.Messages) != 0 {
		t.Fatal("refused sends persisted records")
	}
	request := map[string]any{"requestId": "reusable", "body": strings.Repeat("🙂", 16), "to": map[string]string{"workerSessionId": "lead"}}
	message := f.send(t, request, lane)
	if len(message.Body) != 64 || message.ExpiresAt.Sub(message.SentAt) != 24*time.Hour {
		t.Fatal("exact UTF-8 byte boundary or default expiry changed")
	}
	assertConcurrentReplay(t, f, request, lane, message.MessageId)
	secret := "error-secret-" + lane["YOU_WORKER_SESSION_TOKEN"]
	request["body"], request["bodySecret"] = secret, true
	assertError(t, f, http.MethodPost, "/messages", request, lane, http.StatusConflict, "MESSAGE_REQUEST_CONFLICT")
	request["requestId"] = "quota-overflow"
	assertError(t, f, http.MethodPost, "/messages", request, lane, http.StatusTooManyRequests, "MESSAGE_LIMIT_EXCEEDED")
	if page := readPage(t, f, "/messages", nil); len(page.Messages) != 1 || page.Messages[0].MessageId != message.MessageId {
		t.Fatal("refusals or replays changed successful-send quota or records")
	}
	f.server.Close(t)
	private := []string{secret, lane["YOU_WORKER_SESSION_TOKEN"], lead["YOU_WORKER_SESSION_TOKEN"]}
	for _, entry := range logs.All() {
		encoded, err := json.Marshal(map[string]any{"message": entry.Message, "fields": entry.ContextMap()})
		if err != nil {
			t.Fatal(err)
		}
		for _, value := range private {
			if strings.Contains(string(encoded), value) {
				t.Fatal("message error leaked private content to captured diagnostics")
			}
		}
	}
	assertPrivateArtifacts(t, f.home, private)
	assertPrivateArtifacts(t, f.dir, private)
}

func TestInboxExpiryAndSelectedRetentionAfterReconstruction(t *testing.T) {
	t.Parallel()
	clock := platformclock.NewDeterministic(time.Now().UTC(), time.Second)
	// Only timestamps are controlled; production lifecycle timers remain real.
	effects := serviceedges.Edges{Clock: struct{ platformclock.Source }{clock}}
	home, dir := t.TempDir(), t.TempDir()
	settings := `{"messaging":{"retentionDays":1,"limits":{"defaultExpirySeconds":60}}}`
	f := startHostEffects(t, home, dir, settings, effects, "lead", "lane")
	lead := f.invoke(t, "lead", nil)
	lane := f.invoke(t, "lane", lead)
	old := f.cliSend(t, lane, "lead", "--request-id", "old", "--body", "old message")
	clock.SetTick(59)
	if page := readPage(t, f, "/messages?threadId="+old.ThreadId, lane); len(page.Messages) != 1 || page.Messages[0].Status != "QUEUED" {
		t.Fatal("message expired before its declared deadline")
	}
	clock.SetTick(60)
	if page := readPage(t, f, "/messages?threadId="+old.ThreadId, lane); len(page.Messages) != 1 || page.Messages[0].Status != "EXPIRED" {
		t.Fatal("deadline read did not durably expire the message")
	}
	assertError(t, f, http.MethodPost, "/messages", map[string]any{"requestId": "expired-reply", "inReplyTo": old.MessageId, "body": "late reply"}, lead, http.StatusBadRequest, "MESSAGE_INVALID_REQUEST")
	readPage(t, f, "/messages?toMe=true&markRead=true", lead)
	if page := readPage(t, f, "/messages?threadId="+old.ThreadId, lane); page.Messages[0].Status != "EXPIRED" {
		t.Fatal("mark-read regressed an expired terminal status")
	}
	clock.SetTick(23 * 60 * 60)
	recent := f.cliSend(t, lane, "lead", "--request-id", "recent", "--body", "retained message")
	f.server.Close(t)
	clock.SetTick(25 * 60 * 60)
	second := startHostEffects(t, home, dir, settings, effects)
	assertError(t, second, http.MethodGet, "/messages/"+old.MessageId, nil, nil, http.StatusNotFound, "MESSAGE_NOT_FOUND")
	page := readPage(t, second, "/messages", nil)
	if len(page.Messages) != 1 || page.Messages[0].MessageId != recent.MessageId || page.Messages[0].BodySha256 != recent.BodySha256 || page.Messages[0].Status != "EXPIRED" {
		t.Fatal("selected retention removed recent content or failed recovered expiry")
	}
}

// Host-readiness timers of at most one second keep wall time; message
// and attempt deadlines use controlled time. This test owns expiry,
// not Factory bootstrap timers, and never advances time to make a host ready.
type expiryDeadlineScheduler struct{ platformclock.TimerSource }

func (s expiryDeadlineScheduler) NewTimer(duration time.Duration) platformclock.Timer {
	if duration > 0 && duration <= time.Second {
		return platformclock.Real{}.NewTimer(duration)
	}
	return s.TimerSource.NewTimer(duration)
}

// Timestamp and timer advancement cross only the injected clock edge. Follow
// observes the autonomous commit before any read can perform lazy maintenance.
func TestInboxExpiresWithoutCustomerRequestsAndCannotRevive(t *testing.T) {
	t.Parallel()
	clock := platformclock.NewDeterministic(time.Now().UTC(), time.Second)
	core, _ := observer.New(zap.InfoLevel)
	signal := &subscriptionSignal{Core: core, ready: make(chan struct{}), once: &sync.Once{}}
	effects := serviceedges.Edges{Clock: struct{ platformclock.Source }{clock}, ProcessScheduler: expiryDeadlineScheduler{clock}, ProcessLogger: zap.New(signal)}
	revival := &workerGate{started: make(chan map[string]string, 1), requests: make(chan platformprocess.CommandRequest, 1), release: make(chan struct{})}
	f := startHostRunner(t, t.TempDir(), t.TempDir(), "", effects, nil, revival, "lead", "lane")
	lead := f.invoke(t, "lead", nil)
	lane := f.invoke(t, "lane", lead)
	queued := f.cliSend(t, lane, "lead", "--request-id", "expire-revive", "--body", "unread ask", "--expires", "1m")
	held := f.cliSend(t, lane, "lead", "--request-id", "expire-hold", "--body", "held ask", "--expires", "1m", "--if-ended", "hold")
	read := f.cliSend(t, lane, "lead", "--request-id", "expire-read", "--body", "read ask", "--expires", "1m")
	readPage(t, f, "/messages?threadId="+read.ThreadId+"&markRead=true", lead)
	parent := f.cliSend(t, lane, "lead", "--request-id", "replied-parent", "--body", "answered ask", "--expires", "1m")
	reply := f.cliSend(t, lead, "reply", parent.MessageId, "--request-id", "reply", "--body", "held answer", "--if-ended", "hold", "--expires", "2m")
	defaultExpiry := f.cliSend(t, lane, "lead", "--request-id", "expire-default", "--body", "default deadline ask")
	maximumExpiry := f.cliSend(t, lane, "lead", "--request-id", "expire-maximum", "--body", "maximum deadline held ask", "--expires", "168h", "--if-ended", "hold")
	before := messageFactoryStreams(t, f, "lead", "lane")
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	frames, finished := make(chan []byte, 32), make(chan error, 1)
	// Operator observation survives the running caller's own attempt deadline
	// while the clock crosses the default and maximum message lifetimes.
	go streamFramesAt(ctx, f.server.URL(), "/messages?follow=true", nil, frames, finished)
	select {
	case <-signal.ready:
	case <-ctx.Done():
		t.Fatal("expiry follow did not register")
	}
	clock.SetTick(60)
	awaitExpiredMessages(t, ctx, frames, finished, queued, held, read)
	if message := f.get(t, parent.MessageId, lane); message.Status != "REPLIED" {
		t.Fatal("expiry regressed the replied parent")
	}
	if message := f.get(t, reply.MessageId, lead); message.Status != "QUEUED" {
		t.Fatal("expiry ignored the later answer deadline")
	}
	clock.SetTick(120)
	awaitExpiredMessages(t, ctx, frames, finished, reply)
	clock.SetTick(86400)
	awaitExpiredMessages(t, ctx, frames, finished, defaultExpiry)
	if message := f.get(t, maximumExpiry.MessageId, nil); message.Status != "QUEUED" {
		t.Fatal("default deadline expired the maximum-duration message early")
	}
	clock.SetTick(604800)
	awaitExpiredMessages(t, ctx, frames, finished, maximumExpiry)
	if message := f.get(t, parent.MessageId, nil); message.Status != "REPLIED" {
		t.Fatal("maximum deadline regressed the replied parent")
	}
	if after := messageFactoryStreams(t, f, "lead", "lane"); !reflect.DeepEqual(before, after) {
		t.Fatal("autonomous expiry changed Factory Events")
	}
	f.endWorker(t, "lead")
	select {
	case <-revival.requests:
		t.Fatal("expired unread asks revived the ended recipient")
	default:
	}
	cancel()
	select {
	case <-finished:
	case <-time.After(15 * time.Second):
		t.Fatal("expiry follow did not join")
	}
	f.endWorker(t, "lane")
	f.server.Close(t)
	second := startHostEffects(t, f.home, f.dir, "", effects)
	for _, original := range []api.AgentMessage{queued, held, read, reply, defaultExpiry, maximumExpiry} {
		if message := second.get(t, original.MessageId, nil); message.Status != "EXPIRED" || message.Body != original.Body || message.RevivedWorkerSessionId != nil {
			t.Fatal("reconstruction lost autonomous expiry or revived its content")
		}
	}
}

func awaitExpiredMessages(t *testing.T, ctx context.Context, frames <-chan []byte, finished <-chan error, messages ...api.AgentMessage) {
	t.Helper()
	pending := map[string]api.AgentMessage{}
	for _, message := range messages {
		pending[message.MessageId] = message
	}
	for len(pending) != 0 {
		var observation api.AgentMessageObservation
		if err := json.Unmarshal(nextFrame(t, ctx, frames, finished), &observation); err != nil {
			t.Fatal(err)
		}
		original, exists := pending[observation.Message.MessageId]
		if !exists || observation.Kind != "EXPIRED" {
			continue
		}
		message := observation.Message
		if message.Status != "EXPIRED" || message.Body != original.Body || message.ThreadId != original.ThreadId || message.RevivedWorkerSessionId != nil {
			t.Fatal("expiry observation changed content or claimed delivery")
		}
		delete(pending, message.MessageId)
	}
}

func (f *fixture) get(t *testing.T, id string, caller map[string]string) api.AgentMessage {
	t.Helper()
	status, body := f.request(t, http.MethodGet, "/messages/"+id, nil, caller)
	var message api.AgentMessage
	if status != http.StatusOK || json.Unmarshal(body, &message) != nil {
		t.Fatal("message lookup failed", status)
	}
	return message
}

func TestOversizedBodyCLIHTTPParity(t *testing.T) {
	t.Parallel()
	f := startHost(t, t.TempDir(), t.TempDir(), "", "lead", "lane")
	lead := f.invoke(t, "lead", nil)
	lane := f.invoke(t, "lane", lead)
	for _, cell := range []struct {
		name, body string
		stdin      bool
	}{
		{"explicit ASCII", strings.Repeat("x", 8193), false},
		{"stdin ASCII", strings.Repeat("x", 8193), true},
		{"stdin multibyte truncation", strings.Repeat("\u00e9", 4097), true},
	} {
		t.Run(cell.name, func(t *testing.T) {
			assertError(t, f, http.MethodPost, "/messages", map[string]any{"requestId": "boundary", "to": map[string]string{"workerSessionId": "lead"}, "body": cell.body}, lane, http.StatusTooManyRequests, "MESSAGE_LIMIT_EXCEEDED")
			assertCLIBodyLimit(t, f, lane, cell.body, cell.stdin)
		})
	}
	if page := readPage(t, f, "/messages", nil); len(page.Messages) != 0 {
		t.Fatal("oversized bodies persisted messages")
	}
	message := f.cliSend(t, lane, "lead", "--request-id", "boundary", "--body", strings.Repeat("\u00e9", 4096))
	if len(message.Body) != 8192 {
		t.Fatal("refusals spent request identity or changed the valid byte boundary")
	}
}

func TestExpiryBoundariesCLIHTTPPreserveAdmission(t *testing.T) {
	t.Parallel()
	f := startHost(t, t.TempDir(), t.TempDir(), `{"messaging":{"limits":{"sendsPerChainPerHour":3,"defaultExpirySeconds":120}}}`, "lead", "lane")
	lead := f.invoke(t, "lead", nil)
	lane := f.invoke(t, "lane", lead)
	before := messageFactoryStreams(t, f, "lead", "lane")
	for _, cell := range []struct {
		seconds int
		flag    string
	}{
		{0, "0s"}, {59, "59s"}, {604801, "168h1s"},
	} {
		request := map[string]any{"requestId": "minimum", "to": map[string]string{"workerSessionId": "lead"}, "body": "expiry boundary", "expiresInSeconds": cell.seconds}
		assertError(t, f, http.MethodPost, "/messages", request, lane, http.StatusBadRequest, "MESSAGE_INVALID_REQUEST")
		assertCLIExpiryRefused(t, f, lane, cell.flag)
	}
	// CLI durations must represent whole seconds; HTTP publishes integer seconds.
	assertCLIExpiryRefused(t, f, lane, "60.5s")
	if page := readPage(t, f, "/messages", nil); len(page.Messages) != 0 {
		t.Fatal("invalid expiry persisted a message")
	}
	for _, cell := range []struct {
		id, flag string
		seconds  int
	}{
		{"minimum", "1m", 60}, {"maximum", "168h", 604800}, {"selected-default", "", 120},
	} {
		// Different bodies avoid the preserved ten-minute content dedup policy.
		body := "expiry boundary " + cell.id
		args := []string{"lead", "--request-id", cell.id, "--body", body}
		request := map[string]any{"requestId": cell.id, "to": map[string]string{"workerSessionId": "lead"}, "body": body}
		if cell.flag != "" {
			args = append(args, "--expires", cell.flag)
			request["expiresInSeconds"] = cell.seconds
		}
		message := f.cliSend(t, lane, args...)
		if message.ExpiresAt.Sub(message.SentAt) != time.Duration(cell.seconds)*time.Second || message.Status != "QUEUED" || message.Reason != nil {
			t.Fatalf("%s expiry or running-recipient policy changed: duration=%s status=%s reason=%v", cell.id, message.ExpiresAt.Sub(message.SentAt), message.Status, message.Reason)
		}
		assertProviderRetry(t, f, request, lane, message)
		if stored := f.get(t, message.MessageId, lane); !reflect.DeepEqual(stored, message) {
			t.Fatal("expiry facts differ from the durable HTTP read")
		}
	}
	if page := readPage(t, f, "/messages", nil); len(page.Messages) != 3 {
		t.Fatal("expiry refusals or cross-transport retries spent admission quota or created messages")
	}
	if after := messageFactoryStreams(t, f, "lead", "lane"); !reflect.DeepEqual(before, after) {
		t.Fatal("expiry validation or queueing changed Factory Events")
	}
	// Join the sender first, then mark the recipient's inbox read so teardown
	// cannot start a continuation for these boundary-only queued messages.
	f.endWorker(t, "lane")
	readPage(t, f, "/messages?toMe=true&markRead=true", lead)
	f.endWorker(t, "lead")
}

func assertCLIExpiryRefused(t *testing.T, f *fixture, lane map[string]string, duration string) {
	t.Helper()
	args := []string{"you", "--json", "--server", f.server.URL(), "message", "lead", "--request-id", "minimum", "--body", "expiry boundary", "--expires", duration, "--output", "json"}
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.Env = append(append([]string{}, f.env...), "YOU_WORKER_SESSION_ID="+lane["YOU_WORKER_SESSION_ID"], "YOU_WORKER_SESSION_TOKEN="+lane["YOU_WORKER_SESSION_TOKEN"])
	inputs.Input.WorkingDirectory = f.dir
	err := f.server.Execute(t, inputs.Input)
	var failure *clihttp.APIError
	if !errors.As(err, &failure) || failure.StatusCode != http.StatusBadRequest || failure.Response.Code != "MESSAGE_INVALID_REQUEST" {
		t.Fatalf("CLI expiry refusal for %s: %v %s", duration, err, inputs.Stderr())
	}
}

func assertCLIBodyLimit(t *testing.T, f *fixture, lane map[string]string, body string, stdin bool) {
	t.Helper()
	args := []string{"you", "--json", "--server", f.server.URL(), "message", "lead", "--request-id", "boundary", "--output", "json"}
	if !stdin {
		args = append(args, "--body", body)
	}
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.Env = append(append([]string{}, f.env...), "YOU_WORKER_SESSION_ID="+lane["YOU_WORKER_SESSION_ID"], "YOU_WORKER_SESSION_TOKEN="+lane["YOU_WORKER_SESSION_TOKEN"])
	inputs.Input.WorkingDirectory = f.dir
	inputs.Input.Stdin = strings.NewReader(body)
	tty := false
	inputs.Input.StdinIsTTY = &tty
	err := f.server.Execute(t, inputs.Input)
	var failure *clihttp.APIError
	if !errors.As(err, &failure) || failure.StatusCode != http.StatusTooManyRequests || failure.Response.Code != "MESSAGE_LIMIT_EXCEEDED" {
		t.Fatalf("CLI oversized body parity: %v %s", err, inputs.Stderr())
	}
}
