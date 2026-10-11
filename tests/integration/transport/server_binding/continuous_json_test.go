package server_binding_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// The artifact owner builds separately from an exact clean checkout. This test
// never builds, discovers another executable, or borrows a running operator host.
func TestBuiltContinuousJSONStartupAndStop(t *testing.T) {
	binary := os.Getenv("INFINITE_YOU_PREBUILT_ARTIFACT")
	if binary == "" {
		if os.Getenv("INFINITE_YOU_REQUIRE_PREBUILT_ARTIFACT") == "1" {
			t.Fatal("prebuilt artifact required")
		}
		t.Skip("continuous JSON integration requires a separately built artifact")
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	command, address, recording := newBuiltContinuousJSONCommand(t, ctx, binary)
	baseURL := "http://" + address
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	joined := false
	t.Cleanup(func() {
		cancel()
		if !joined {
			select {
			case <-done:
			case <-time.After(time.Minute):
				t.Error("compiled host did not join")
			}
		}
	})
	client := &http.Client{Timeout: time.Second}
	observeContinuousStatus(t, ctx, client, baseURL, func(status factoryapi.StatusResponse) bool { return status.RuntimeStatus != "" })
	var admitted factoryapi.SubmitWorkResponse
	continuousJSONRequest(t, client, http.MethodPost, baseURL+"/factory-sessions/~default/work", `{"workTypeName":"task","payload":{"value":"useful"}}`, http.StatusCreated, &admitted)
	if !admitted.Accepted {
		t.Fatalf("Work rejected: %#v", admitted)
	}
	observeContinuousStatus(t, ctx, client, baseURL, func(status factoryapi.StatusResponse) bool {
		return status.Categories.Terminal == 1 && status.Categories.Failed == 0
	})
	continuousJSONRequest(t, client, http.MethodPost, baseURL+"/shutdown", "", http.StatusAccepted, nil)
	select {
	case err := <-done:
		joined = true
		if err != nil {
			t.Fatalf("ordinary exit=%v stdout=%q stderr=%q", err, stdout.String(), stderr.String())
		}
	case <-ctx.Done():
		t.Fatal("ordinary stop did not join")
	}
	if stdout.Len() != 0 {
		t.Fatalf("continuous JSON invented records: %q", stdout.String())
	}
	assertBuiltContinuousHistory(t, recording, admitted)
	listener, err := net.Listen("tcp4", address)
	if err != nil {
		t.Fatalf("joined host retained listener: %v", err)
	}
	listener.Close()
}

func newBuiltContinuousJSONCommand(t *testing.T, ctx context.Context, binary string) (*exec.Cmd, string, string) {
	t.Helper()
	dir, home := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "factory.json"), []byte(continuousJSONFactory), 0600); err != nil {
		t.Fatal(err)
	}
	recording := filepath.Join(t.TempDir(), "owned.json")
	address := fmt.Sprintf("127.0.0.1:%d", reserveAvailablePort(t))
	command := exec.CommandContext(ctx, binary, "--json", "run", "--dir", dir, "--continuously", "--with-server", "--listen", address, "--record", recording)
	command.Dir = dir
	command.Env = builtcliacceptance.ProcessEnvForIsolatedHome(home)
	return command, address, recording
}

// OS readiness and asynchronous logical Work completion are observable only
// through HTTP here. Bounded polling cannot be replaced by a controlled edge in
// this compiled-artifact integration witness.
func observeContinuousStatus(t *testing.T, ctx context.Context, client *http.Client, baseURL string, accept func(factoryapi.StatusResponse) bool) {
	t.Helper()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		response, err := client.Get(baseURL + "/status")
		if err == nil {
			var status factoryapi.StatusResponse
			decodeErr := json.NewDecoder(response.Body).Decode(&status)
			response.Body.Close()
			if response.StatusCode == http.StatusOK && decodeErr == nil && accept(status) {
				return
			}
		}
		select {
		case <-ctx.Done():
			t.Fatal("compiled host status deadline")
		case <-ticker.C:
		}
	}
}

func continuousJSONRequest(t *testing.T, client *http.Client, method, endpoint, payload string, want int, output any) {
	t.Helper()
	request, err := http.NewRequest(method, endpoint, bytes.NewBufferString(payload))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != want {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("%s status=%d body=%s", endpoint, response.StatusCode, body)
	}
	if output != nil {
		if err := json.NewDecoder(response.Body).Decode(output); err != nil {
			t.Fatal(err)
		}
	}
}

func assertBuiltContinuousHistory(t *testing.T, path string, admitted factoryapi.SubmitWorkResponse) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var recording struct {
		Events []factoryapi.FactoryEvent `json:"events"`
	}
	if err := json.Unmarshal(data, &recording); err != nil {
		t.Fatal(err)
	}
	previous, completed := -1, false
	for _, event := range recording.Events {
		if event.Context.Sequence <= previous {
			t.Fatal("unordered public history")
		}
		previous = event.Context.Sequence
		if event.Type == factoryapi.FactoryEventTypeWorkStateChange {
			payload, err := event.Payload.AsWorkStateChangeEventPayload()
			if err != nil {
				t.Fatal(err)
			}
			completed = completed || (admitted.WorkId != nil && payload.ToState == "complete" && payload.WorkId == *admitted.WorkId)
		}
	}
	if !completed {
		t.Fatal("recording lost useful completed Work")
	}
	last := recording.Events[len(recording.Events)-1]
	state, err := last.Payload.AsSessionCompletedEventPayload()
	if last.Type != factoryapi.FactoryEventTypeSessionCompleted || err != nil || state.FinalStatus != "SUCCEEDED" {
		t.Fatalf("history missing terminal factory state: %#v, %v", last, err)
	}
}

const continuousJSONFactory = `{"name":"continuous-json","workTypes":[{"name":"task","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],"workstations":[{"name":"finish","type":"LOGICAL_MOVE","inputs":[{"workType":"task","state":"init"}],"outputs":[{"workType":"task","state":"complete"}]}]}`
