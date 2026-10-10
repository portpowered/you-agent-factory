package execution_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/webhooks"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func advanceWebhookScheduler(s *journeyScheduler, seconds int) {
	s.SetTick(int(s.Now().Sub(s.base)/time.Second) + seconds)
}

func runWebhookTerminal(t *testing.T, c *timeCohort, specialized bool, key string, attempts, status int, reason string) {
	t.Helper()
	id := openWebhookSession(t, c, key)
	emitWebhookWork(t, c, id)
	route := c.webhookEffects.routes[key]
	firstAt := webhookNow(c, specialized)
	firstBody, eventID := runWebhookTerminalAttempts(t, c, id, specialized, key, attempts, status)
	var line []byte
	select {
	case line = <-c.webhookEffects.letters:
	case <-time.After(30 * time.Second):
		t.Fatal("terminal dead-letter append not observed")
	}
	// Close joins the delivery and append before checking diagnostics/no storm.
	support.CloseFactorySessionAt(t, c.url, id)
	assertWebhookTerminalRecord(t, line, key, eventID, firstBody, attempts, status, reason, firstAt, webhookNow(c, specialized))
	route.assertHeld(t)
	assertNoWebhookLetters(t, c)
	assertWebhookNoSuccess(t, c, key)
}

// The selected HTTP edge returns the redirect unchanged. This proves the
// runtime's terminal classification, not the default HTTP client's redirect
// policy, which belongs to the real HTTP boundary's integration coverage.
func runWebhookRedirect(t *testing.T, c *timeCohort, specialized bool) {
	t.Helper()
	id := openWebhookSession(t, c, "redirect")
	defer support.CloseFactorySessionAt(t, c.url, id)
	route := c.webhookEffects.routes["redirect"]
	for _, status := range []int{http.StatusMovedPermanently, http.StatusFound, http.StatusTemporaryRedirect, http.StatusPermanentRedirect} {
		emitWebhookWork(t, c, id)
		call := route.await(t)
		now := webhookNow(c, specialized)
		body := assertWebhookRequest(t, c, id, call, now)
		call.reply <- journeyReply{status: status, header: http.Header{
			"Location":    {"https://foreign.invalid/credential-target?token=" + webhookSecret},
			"Retry-After": {"2"},
		}, body: "sensitive redirect " + webhookSecret}
		select {
		case line := <-c.webhookEffects.letters:
			assertWebhookTerminalRecord(t, line, "redirect", call.request.Header.Get(webhooks.EventIDHeader), body,
				1, status, "non_retryable_http_status", now, now)
		case <-time.After(30 * time.Second):
			t.Fatal("redirect did not terminalize")
		}
		// The append follows attempt logging synchronously. It acknowledges that
		// this event terminalized without a retry, even with Retry-After supplied.
		assertWebhookNoSuccess(t, c, "redirect")
		route.assertHeld(t)
		assertNoWebhookLetters(t, c)
	}
	t.Log("W8-F: 301/302/307/308 responses produce one attributed redacted terminal record each; no successful-delivery claim or scheduled retry")
}

func assertWebhookNoSuccess(t *testing.T, c *timeCohort, endpoint string) {
	t.Helper()
	for _, entry := range c.logs.All() {
		fields := entry.ContextMap()
		if entry.Message == "factory webhook delivery attempt" && fields["endpoint"] == endpoint && fields["outcome"] == "success" {
			t.Fatalf("failed endpoint %s reported successful delivery", endpoint)
		}
	}
}

func runWebhookTerminalAttempts(t *testing.T, c *timeCohort, id string, specialized bool, key string, attempts, status int) ([]byte, string) {
	t.Helper()
	route := c.webhookEffects.routes[key]
	var firstBody []byte
	var eventID string
	for attempt := 1; attempt <= attempts; attempt++ {
		call := route.await(t)
		body := assertWebhookRequest(t, c, id, call, webhookNow(c, specialized))
		if attempt == 1 {
			firstBody, eventID = body, call.request.Header.Get(webhooks.EventIDHeader)
		}
		if !bytes.Equal(body, firstBody) || eventID != call.request.Header.Get(webhooks.EventIDHeader) {
			t.Fatal("terminal retries changed canonical identity")
		}
		call.reply <- journeyReply{status: status, body: "sensitive response " + webhookSecret}
		if attempt < attempts {
			delay := time.Duration(attempt) * time.Second
			wait := c.webhook.await(t, delay)
			assertSourceWaitHeld(t, wait)
			route.assertHeld(t)
			advanceWebhookScheduler(c.webhook, attempt)
		}
	}
	return firstBody, eventID
}

func assertWebhookTerminalRecord(t *testing.T, line []byte, key, eventID string, firstBody []byte, attempts, status int, reason string, firstAt, lastAt time.Time) {
	t.Helper()
	var record struct {
		Endpoint string          `json:"endpointName"`
		EventID  string          `json:"eventId"`
		Attempts int             `json:"attemptCount"`
		Status   int             `json:"statusCode"`
		Reason   string          `json:"terminalReason"`
		First    time.Time       `json:"firstAttemptAt"`
		Last     time.Time       `json:"lastAttemptAt"`
		Terminal time.Time       `json:"terminalAt"`
		Body     json.RawMessage `json:"canonicalBody"`
	}
	if err := json.Unmarshal(line, &record); err != nil {
		t.Fatal(err)
	}
	if record.Endpoint != key || record.EventID != eventID || record.Attempts != attempts || record.Status != status || record.Reason != reason || !bytes.Equal(record.Body, firstBody) {
		t.Fatalf("terminal outcome=%s", line)
	}
	if !record.First.Equal(firstAt) || !record.Last.Equal(lastAt) || !record.Terminal.Equal(lastAt) {
		t.Fatalf("terminal selected timestamps=%s", line)
	}
	if strings.Contains(string(line), webhookSecret) {
		t.Fatal("dead-letter leaked secret query or response")
	}
}

func assertHealthyWebhook(t *testing.T, c *timeCohort, id string, specialized bool) {
	t.Helper()
	emitWebhookWork(t, c, id)
	call := c.webhookEffects.routes["healthy"].await(t)
	assertWebhookRequest(t, c, id, call, webhookNow(c, specialized))
	call.reply <- journeyReply{status: http.StatusNoContent}
	// A later scoped event is an ordered acknowledgement that the preceding
	// delivery returned successfully; no sleep or global quiescence is needed.
}

func runWebhookClose(t *testing.T, c *timeCohort, peer string, specialized bool) {
	t.Helper()
	id := openWebhookSession(t, c, "closing")
	emitWebhookWork(t, c, id)
	route := c.webhookEffects.routes["closing"]
	call := route.await(t)
	assertWebhookRequest(t, c, id, call, webhookNow(c, specialized))
	call.reply <- journeyReply{status: http.StatusServiceUnavailable}
	c.webhook.await(t, time.Second)
	support.CloseFactorySessionAt(t, c.url, id)
	advanceWebhookScheduler(c.webhook, 1)
	assertHealthyWebhook(t, c, peer, specialized)
	route.assertHeld(t)
	assertClosedTimeSession(t, c, id)
	assertNoWebhookLetters(t, c)
	t.Log("W03: public close joins pending retry; old timer release has no delivery/dead letter, healthy scope keeps attributed canonical events")
}
