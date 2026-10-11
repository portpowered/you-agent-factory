package agentmessages_test

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"testing"
	"time"

	api "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// P-AM6 measures only send entry -> successor command STARTING. Prepare twenty
// independent authorized chains first; one profile/root, controlled Codex,
// zero subprocesses/downloads/paid calls. This is a dedicated load-lane budget.
func TestReviveSendToStartingLatency(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Minute)
	defer cancel()
	f := prepareLatencyHost(t, ctx)
	callers := make([]map[string]string, 20)
	for i := range callers {
		callers[i] = f.prepareChain(t, ctx, i)
	}
	values := make([]time.Duration, len(callers))
	for i, caller := range callers {
		started := time.Now()
		var response api.AgentMessageSendResponse
		f.request(t, ctx, http.MethodPost, "/messages", map[string]any{
			"requestId": fmt.Sprintf("measurement-%d", i), "to": map[string]string{"workerSessionId": fmt.Sprintf("source-%d", i)},
			"body": "latency follow-up",
		}, caller, http.StatusAccepted, &response)
		var admitted latencyCommand
		select {
		case admitted = <-f.runner.continued:
		case <-ctx.Done():
			t.Fatal("successor did not reach STARTING", ctx.Err())
		}
		values[i] = admitted.at.Sub(started)
		// This runner withholds all provider output. STARTING proves admission,
		// while the message must remain pending until resumed progress exists.
		if response.Message.Status != "QUEUED" || response.Message.Reason == nil || *response.Message.Reason != "REVIVE_PENDING" ||
			response.Message.RevivedWorkerSessionId != nil || response.Message.To.DeliveredWorkerSessionId != nil {
			t.Fatal("measurement claimed delivery before provider progress")
		}
		if admitted.env["YOU_WORKER_SESSION_ID"] == "" || admitted.env["YOU_WORKER_SESSION_TOKEN"] == "" {
			t.Fatal("measurement did not observe an authenticated successor")
		}
	}
	sort.Slice(values, func(a, b int) bool { return values[a] < values[b] })
	p95 := values[18]
	t.Logf("P-AM6 controlled Codex; prepared chains=20; samples=%v; nearest-rank p95=%s; range=%s..%s", values, p95, values[0], values[19])
	if p95 > 10*time.Second {
		t.Fatalf("send-to-STARTING p95 exceeds 10s: %s", p95)
	}
}
