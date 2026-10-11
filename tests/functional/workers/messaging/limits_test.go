package messaging

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	api "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

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
	f := startHostEffects(t, t.TempDir(), t.TempDir(), `{"messaging":{"limits":{"sendsPerChainPerHour":1,"maxBodyBytes":64}}}`, false, serviceedges.Edges{ProcessLogger: zap.New(core)}, "lead", "lane")
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
	f := startHostEffects(t, home, dir, settings, false, effects, "lead", "lane")
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
	second := startHostEffects(t, home, dir, settings, false, effects)
	assertError(t, second, http.MethodGet, "/messages/"+old.MessageId, nil, nil, http.StatusNotFound, "MESSAGE_NOT_FOUND")
	page := readPage(t, second, "/messages", nil)
	if len(page.Messages) != 1 || page.Messages[0].MessageId != recent.MessageId || page.Messages[0].BodySha256 != recent.BodySha256 || page.Messages[0].Status != "EXPIRED" {
		t.Fatal("selected retention removed recent content or failed recovered expiry")
	}
}
