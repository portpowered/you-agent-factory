package agentmessages_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	api "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func fmtID(prefix string, index int) string { return fmt.Sprintf("%s%d", prefix, index) }

func (f *latencyHost) prepareChain(t *testing.T, ctx context.Context, index int) map[string]string {
	t.Helper()
	sourceID := fmtID("source-", index)
	source := f.invoke(t, ctx, sourceID, nil)
	lane := f.invoke(t, ctx, fmtID("lane-", index), source)
	close(f.runner.gates[sourceID])
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.url+"/worker-sessions/"+sourceID+"/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	scanner := bufio.NewScanner(response.Body)
	for scanner.Scan() {
		if !strings.HasPrefix(scanner.Text(), "data: ") {
			continue
		}
		var frame api.WorkerSessionEvent
		if err := json.Unmarshal([]byte(strings.TrimPrefix(scanner.Text(), "data: ")), &frame); err != nil {
			t.Fatal(err)
		}
		if frame.Event.Payload["kind"] == "SESSION" && frame.Event.Payload["phase"] == "COMPLETED" {
			return lane
		}
	}
	t.Fatal("source never completed", scanner.Err())
	return nil
}

func (f *latencyHost) invoke(t *testing.T, ctx context.Context, id string, caller map[string]string) map[string]string {
	t.Helper()
	var opened api.OpenFactorySessionResponse
	f.request(t, ctx, http.MethodPost, "/factory-sessions", map[string]string{"folderPath": f.dir}, nil, http.StatusOK, &opened)
	var response api.WorkerSessionStartResponse
	f.request(t, ctx, http.MethodPost, "/worker-sessions", map[string]any{"requestId": id, "workerSessionId": id, "execution": map[string]any{
		"workstationName": "direct", "workingDirectory": f.dir, "factorySessionId": opened.Session.Id,
		"workerType": "direct-worker", "runnerId": "codex", "executorProvider": "codex", "modelProvider": "codex", "model": "functional-model", "userMessage": "load preparation",
		"dispatch": map[string]any{"dispatchId": id + "-attempt", "workstationName": "direct", "workerType": "direct-worker"},
	}}, caller, http.StatusAccepted, &response)
	select {
	case command := <-f.runner.started:
		if command.env["YOU_WORKER_SESSION_ID"] != id || command.env["YOU_WORKER_SESSION_TOKEN"] == "" {
			t.Fatal("preparation lost authenticated worker identity")
		}
		return command.env
	case <-ctx.Done():
		t.Fatal("preparation never reached command edge", ctx.Err())
	}
	return nil
}
