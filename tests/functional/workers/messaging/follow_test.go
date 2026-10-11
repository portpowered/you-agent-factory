package messaging

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	api "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// An injected diagnostic sink supplies a registration gate, never the
// behavioral assertion. Assertions consume only the customer HTTP stream.
type subscriptionSignal struct {
	zapcore.Core
	ready chan struct{}
	once  *sync.Once
}

func (c *subscriptionSignal) With(fields []zapcore.Field) zapcore.Core {
	return &subscriptionSignal{Core: c.Core.With(fields), ready: c.ready, once: c.once}
}
func (c *subscriptionSignal) Check(entry zapcore.Entry, checked *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(entry.Level) {
		return checked.AddCore(entry, c)
	}
	return checked
}
func (c *subscriptionSignal) Write(entry zapcore.Entry, fields []zapcore.Field) error {
	values := zapcore.NewMapObjectEncoder()
	for _, field := range fields {
		field.AddTo(values)
	}
	if entry.Message == "events subscribe outcome" && values.Fields["topic"] == "agent-messages/events" && values.Fields["outcome"] == "accepted" {
		c.once.Do(func() { close(c.ready) })
	}
	return c.Core.Write(entry, fields)
}

func TestInboxFollowRevalidatesAuthorityBeforePublication(t *testing.T) {
	t.Parallel()
	core, _ := observer.New(zap.InfoLevel)
	signal := &subscriptionSignal{Core: core, ready: make(chan struct{}), once: &sync.Once{}}
	f := startHostEffects(t, t.TempDir(), t.TempDir(), "", false, serviceedges.Edges{ProcessLogger: zap.New(signal)}, "lead", "lane")
	lead := f.invoke(t, "lead", nil)
	lane := f.invoke(t, "lane", lead)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	frames := make(chan []byte, 2)
	finished := make(chan error, 1)
	go streamFrames(ctx, f.server.URL(), lead, frames, finished)
	select {
	case <-signal.ready:
	case err := <-finished:
		t.Fatalf("follow before readiness: %v", err)
	case <-ctx.Done():
		t.Fatal("follow did not register")
	}
	message := f.send(t, map[string]any{"requestId": "live", "to": map[string]string{"workerSessionId": "lead"}, "body": "live inbox"}, lane)
	var observation api.AgentMessageObservation
	frame := nextFrame(t, ctx, frames, finished)
	if json.Unmarshal(frame, &observation) != nil || observation.Message.MessageId != message.MessageId || observation.Kind != "SENT" {
		t.Fatal("follow did not publish admitted message")
	}
	status, _ := f.request(t, http.MethodPost, "/worker-sessions/lead/cancel", map[string]any{}, nil)
	if status != http.StatusOK {
		t.Fatal("controlled recipient did not cancel")
	}
	f.send(t, map[string]any{"requestId": "after-end", "to": map[string]string{"workerSessionId": "lead"}, "body": "must not publish after owner loss"}, lane)
	frame = nextFrame(t, ctx, frames, finished)
	var failure api.ErrorResponse
	if json.Unmarshal(frame, &failure) != nil || failure.Code != "MESSAGE_NOT_PERMITTED" || strings.Contains(string(frame), "must not publish") {
		t.Fatal("follow published after recipient authority loss")
	}
	cancel()
	select {
	case <-finished:
	case <-t.Context().Done():
		t.Fatal("follow did not join")
	}
}

func nextFrame(t *testing.T, ctx context.Context, frames <-chan []byte, finished <-chan error) []byte {
	t.Helper()
	select {
	case frame := <-frames:
		return frame
	case err := <-finished:
		// The producer queues the last frame before reporting completion.
		// A select can choose completion while that frame is already buffered.
		select {
		case frame := <-frames:
			return frame
		default:
		}
		t.Fatalf("follow ended before frame: %v", err)
	case <-ctx.Done():
		t.Fatal("follow frame unavailable")
	}
	return nil
}

func streamFrames(ctx context.Context, server string, caller map[string]string, frames chan<- []byte, finished chan<- error) {
	defer close(finished)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server+"/messages?follow=true&toMe=true&fromWorkerSessionId=lane", nil)
	if err != nil {
		finished <- err
		return
	}
	req.Header.Set("X-You-Worker-Session-Id", caller["YOU_WORKER_SESSION_ID"])
	req.Header.Set("Authorization", "Bearer "+caller["YOU_WORKER_SESSION_TOKEN"])
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		finished <- err
		return
	}
	defer response.Body.Close()
	scanner := bufio.NewScanner(response.Body)
	for scanner.Scan() {
		if data, ok := strings.CutPrefix(scanner.Text(), "data: "); ok {
			select {
			case frames <- []byte(data):
			case <-ctx.Done():
				finished <- ctx.Err()
				return
			}
		}
	}
	finished <- scanner.Err()
}
