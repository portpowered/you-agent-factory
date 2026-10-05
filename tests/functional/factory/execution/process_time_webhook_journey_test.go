package execution_test

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/webhooks"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func webhookNow(c *timeCohort, specialized bool) time.Time {
	if specialized {
		return c.webhook.Now()
	}
	return c.wall.Now()
}

func runWebhookJourney(t *testing.T, c *timeCohort, specialized bool) {
	t.Helper()
	runWebhookRecovery(t, c, specialized)
	peer := openWebhookSession(t, c, "healthy")
	defer support.CloseFactorySessionAt(t, c.url, peer)
	runWebhookTerminal(t, c, specialized, "exhaust", 3, http.StatusServiceUnavailable, "retry_exhausted")
	assertHealthyWebhook(t, c, peer, specialized)
	t.Log("W02: configured exhaustion, original body/event, selected first/last/terminal times and redaction; healthy peer delivers")
	runWebhookTerminal(t, c, specialized, "badstatus", 1, http.StatusBadRequest, "non_retryable_http_status")
	assertHealthyWebhook(t, c, peer, specialized)
	t.Log("W05: single non-retryable 400 terminalizes with redacted attributed dead letter; peer delivers")
	runWebhookTerminal(t, c, specialized, "storefail", 3, http.StatusServiceUnavailable, "retry_exhausted")
	assertWebhookDiagnostic(t, c, "factory webhook dead-letter append failed", "storefail")
	assertHealthyWebhook(t, c, peer, specialized)
	t.Log("W07: append edge failure diagnosed safely, no persisted-letter claim or recursive retry; peer delivers")
	id := openWebhookSession(t, c, "secretfail")
	emitWebhookWork(t, c, id)
	support.CloseFactorySessionAt(t, c.url, id)
	assertWebhookSuccess(t, c, "recover")
	assertWebhookDiagnostic(t, c, "factory webhook secret resolution failed", "secretfail")
	c.webhookEffects.routes["secretfail"].assertHeld(t)
	assertHealthyWebhook(t, c, peer, specialized)
	t.Log("W06: secret failure produces redacted diagnostic, no HTTP, public close joins; peer delivers")
	runWebhookClose(t, c, peer, specialized)
	assertWebhookRedaction(t, c)
}

func runWebhookRecovery(t *testing.T, c *timeCohort, specialized bool) {
	t.Helper()
	id := openWebhookSession(t, c, "recover")
	emitWebhookWork(t, c, id)
	route := c.webhookEffects.routes["recover"]
	first := route.await(t)
	body := assertWebhookRequest(t, c, id, first, webhookNow(c, specialized))
	first.reply <- journeyReply{status: http.StatusServiceUnavailable, header: http.Header{"Retry-After": {webhookNow(c, specialized).Add(2 * time.Second).Format(http.TimeFormat)}}}
	wait := c.webhook.await(t, 2*time.Second)
	// Advancing only the observation source must not release default scheduling.
	c.wall.SetTick(int(c.wall.Now().Sub(time.Date(2040, 1, 1, 0, 0, 0, 0, time.UTC))/time.Second) + 10)
	if specialized {
		advanceWebhookScheduler(c.process, 10)
	}
	assertSourceWaitHeld(t, wait)
	route.assertHeld(t)
	advanceWebhookScheduler(c.webhook, 1)
	assertSourceWaitHeld(t, wait)
	route.assertHeld(t)
	advanceWebhookScheduler(c.webhook, 1)
	second := route.await(t)
	retried := assertWebhookRequest(t, c, id, second, webhookNow(c, specialized))
	if !bytes.Equal(body, retried) || first.request.Header.Get(webhooks.EventIDHeader) != second.request.Header.Get(webhooks.EventIDHeader) {
		t.Fatal("retry changed canonical delivery identity")
	}
	if first.request.Header.Get(webhooks.TimestampHeader) == second.request.Header.Get(webhooks.TimestampHeader) {
		t.Fatal("retry timestamp did not refresh")
	}
	second.reply <- journeyReply{status: http.StatusNoContent}
	support.CloseFactorySessionAt(t, c.url, id)
	route.assertHeld(t)
	assertNoWebhookLetters(t, c)
	t.Log("W01: held HTTP-date retry, selected scheduling release, stable canonical identity, refreshed selected-wall timestamp and independently verified HMAC; no dead letter")
	if specialized {
		t.Log("W04: exact specialized observations and waits win over both process sources")
	}
}

func assertWebhookRequest(t *testing.T, c *timeCohort, id string, call journeyCall, now time.Time) []byte {
	t.Helper()
	body, err := io.ReadAll(call.request.Body)
	if err != nil {
		t.Fatal(err)
	}
	timestamp := call.request.Header.Get(webhooks.TimestampHeader)
	if timestamp != strconv.FormatInt(now.Unix(), 10) {
		t.Fatalf("timestamp=%s, want %v", timestamp, now)
	}
	mac := hmac.New(sha256.New, []byte(webhookSecret))
	_, _ = mac.Write([]byte(timestamp + "."))
	_, _ = mac.Write(body)
	want := "v1=" + hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(want), []byte(call.request.Header.Get(webhooks.SignatureHeader))) {
		t.Fatal("invalid HMAC")
	}
	var event factoryapi.FactoryEvent
	if err := json.Unmarshal(body, &event); err != nil {
		t.Fatal(err)
	}
	if event.Context.SessionId == nil || *event.Context.SessionId != id || event.Id != call.request.Header.Get(webhooks.EventIDHeader) {
		t.Fatalf("foreign delivery: %s", body)
	}
	for _, canonical := range support.GetFactoryEventsForSessionAt(t, c.url, id) {
		if canonical.Id == event.Id {
			encoded, err := json.Marshal(canonical)
			if err != nil {
				t.Fatal(err)
			}
			var left, right any
			_ = json.Unmarshal(encoded, &left)
			_ = json.Unmarshal(body, &right)
			// Public canonical fields must agree, regardless of object key order.
			if !equalWebhookJSON(left, right) {
				t.Fatalf("delivery differs from public canonical event: %s / %s", body, encoded)
			}
			return body
		}
	}
	t.Fatal("delivered event absent from public scoped history")
	return nil
}

func equalWebhookJSON(a, b any) bool {
	left, _ := json.Marshal(a)
	right, _ := json.Marshal(b)
	return bytes.Equal(left, right)
}

func assertNoWebhookLetters(t *testing.T, c *timeCohort) {
	t.Helper()
	select {
	case line := <-c.webhookEffects.letters:
		t.Fatalf("unexpected dead letter: %s", line)
	default:
	}
}

func assertWebhookDiagnostic(t *testing.T, c *timeCohort, message, endpoint string) {
	t.Helper()
	for _, entry := range c.logs.All() {
		if entry.Message == message && entry.ContextMap()["endpoint"] == endpoint {
			return
		}
	}
	t.Fatalf("missing %q diagnostic for %s", message, endpoint)
}

func assertWebhookSuccess(t *testing.T, c *timeCohort, endpoint string) {
	t.Helper()
	for _, entry := range c.logs.All() {
		fields := entry.ContextMap()
		if entry.Message == "factory webhook delivery attempt" && fields["endpoint"] == endpoint && fields["outcome"] == "success" {
			return
		}
	}
	t.Fatalf("missing successful delivery for %s", endpoint)
}

func assertWebhookRedaction(t *testing.T, c *timeCohort) {
	t.Helper()
	for _, entry := range c.logs.All() {
		data, err := json.Marshal(entry.ContextMap())
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), webhookSecret) || strings.Contains(entry.Message, webhookSecret) {
			t.Fatal("webhook diagnostics leaked sensitive material")
		}
	}
}
