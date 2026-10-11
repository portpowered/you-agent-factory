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

	api "github.com/portpowered/infinite-you/pkg/transports/http/generated"
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
