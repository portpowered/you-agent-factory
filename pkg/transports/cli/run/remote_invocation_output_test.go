package run

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	runtimehost "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func remoteSuccessHandler(
	t *testing.T,
	gotRequest *factoryapi.FactorySessionExecutionRequest,
	resultCalls *int,
) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/selected/factory-sessions/async":
			if err := json.NewDecoder(r.Body).Decode(gotRequest); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(factoryapi.FactorySessionExecutionResponse{
				SessionId:        "dur-sess-remote",
				Status:           factoryapi.FactorySessionDurableLifecycleStatusQueued,
				OrchestratorKind: factoryapi.JAVASCRIPT,
			})
		case r.Method == http.MethodGet && r.URL.Path == "/selected/factory-sessions/dur-sess-remote/results":
			*resultCalls = *resultCalls + 1
			if *resultCalls == 1 {
				status := factoryapi.FactorySessionDurableLifecycleStatusRunning
				retryable := true
				_ = json.NewEncoder(w).Encode(factoryapi.FactorySessionResult{
					SessionId:     "dur-sess-remote",
					ResultStatus:  factoryapi.FactorySessionResultStatusNotReady,
					SessionStatus: &status,
					Availability:  &factoryapi.FactorySessionResultAvailabilityDetail{Retryable: &retryable},
				})
				return
			}
			status := factoryapi.FactorySessionDurableLifecycleStatusSucceeded
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(factoryapi.FactorySessionResult{
				SessionId:     "dur-sess-remote",
				ResultStatus:  factoryapi.FactorySessionResultStatusFinal,
				SessionStatus: &status,
				PrimaryResult: remoteTextContent(t, "remote result"),
			})
		default:
			t.Errorf("request = %s %s, want selected durable start or result endpoint", r.Method, r.URL.Path)
		}
	})
}

func TestSafeRemoteEndpointRedactsCredentialsAndFailsClosedForInvalidInput(t *testing.T) {
	if got := safeRemoteEndpoint("https://user:secret@selected.test/path"); got != "https://selected.test/path" {
		t.Fatalf("safe endpoint = %q, want credentials removed", got)
	}
	for _, test := range []struct {
		name     string
		endpoint string
		secret   string
	}{
		{name: "malformed URI", endpoint: "http://[::1", secret: "::1"},
		{name: "malformed URI with userinfo", endpoint: "http://user:secret@[::1", secret: "secret"},
		{name: "opaque HTTPS URI", endpoint: "https:user:secret@selected.test", secret: "secret"},
		{name: "unsupported scheme", endpoint: "ftp://user:secret@selected.test", secret: "secret"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := safeRemoteEndpoint(test.endpoint); got != invalidRemoteEndpointLabel {
				t.Fatalf("invalid safe endpoint %q = %q, want fixed redacted label", test.endpoint, got)
			}
			if strings.Contains(safeRemoteEndpoint(test.endpoint), test.secret) {
				t.Fatalf("safe endpoint leaked %q", test.secret)
			}
		})
	}
}

func TestEmitReplayMetadataWarningsUsesConciseDeterministicComponents(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	warnings := []recordings.MetadataMismatchWarning{
		{Key: "workstations_hash"},
		{Key: "workers_hash"},
		{Key: "factory_hash"},
		{Key: "workers_hash"},
		{Key: "runtime_config_hash"},
	}
	if err := emitReplayMetadataWarnings(&output, warnings); err != nil {
		t.Fatalf("emitReplayMetadataWarnings() error = %v", err)
	}
	want := "Replay warning: current Factory Definition differs from the recording; affected components: Factory Definition, runtime configuration, workers, workstations. Replay continues with recorded inputs.\n"
	if output.String() != want {
		t.Errorf("replay metadata warning = %q, want %q", output.String(), want)
	}
}

func TestOperationRunDisclosesReplayDriftAfterSuccessfulReplay(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	operation := &Operation{
		cfg:                    RunConfig{Output: &output, SuppressDashboardRendering: true},
		runner:                 stubFactoryService{run: func(context.Context) error { return nil }},
		replayMetadataWarnings: []recordings.MetadataMismatchWarning{{Key: "workers_hash"}},
	}
	if err := operation.Run(context.Background()); err != nil {
		t.Fatalf("Operation.Run() error = %v, want successful replay", err)
	}
	want := "Replay warning: current Factory Definition differs from the recording; affected components: workers. Replay continues with recorded inputs.\n"
	if output.String() != want {
		t.Errorf("replay output = %q, want %q", output.String(), want)
	}
}

func TestOperationRunDoesNotDiscloseWhenReplayMetadataMatches(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	operation := &Operation{
		cfg:    RunConfig{Output: &output, SuppressDashboardRendering: true},
		runner: stubFactoryService{run: func(context.Context) error { return nil }},
	}
	if err := operation.Run(context.Background()); err != nil {
		t.Fatalf("Operation.Run() error = %v, want successful replay", err)
	}
	if output.Len() != 0 {
		t.Fatalf("replay output = %q, want no drift warning", output.String())
	}
}

var _ runtimehost.Service = stubFactoryService{}
