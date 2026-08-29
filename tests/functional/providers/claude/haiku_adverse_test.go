package claude

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const haikuAdverseSecret = "haiku-route-input-must-not-escape"

func TestClaudeHaikuGoldenAdverseValidationFailsBeforeRouting(t *testing.T) {
	manifest := loadHaikuGoldenManifest(t)
	golden := manifest.Cases[0]
	valid := loadHaikuGoldenStdout(t, golden)
	router := newHaikuGoldenCommandRouter(t, []haikuGoldenReplayCase{{
		golden: golden, factoryDir: filepath.Join(t.TempDir(), "route"), stdout: valid,
	}})
	t.Cleanup(router.Close)

	tests := []struct {
		name  string
		check func() error
		want  string
	}{
		{name: "empty metadata", check: func() error { return validateHaikuGoldenCase(haikuGoldenCase{}) }, want: "metadata"},
		{name: "empty stream", check: func() error { return validateHaikuGoldenNativeShape(nil, golden) }, want: "line 1"},
		{name: "malformed stream", check: func() error { return validateHaikuGoldenNativeShape([]byte("{"), golden) }, want: "line 1"},
		{name: "partial result", check: func() error {
			return validateHaikuGoldenNativeShape(
				[]byte(`{"type":"system","subtype":"init","session_id":"partial-session"}`+"\n"),
				golden,
			)
		}, want: "shape"},
		{name: "unsanitized stream", check: func() error {
			unsafe := append(append([]byte(nil), valid...), []byte(`{"headers":{"Authorization":"Bearer sk-live-haiku-secret"}}`+"\n")...)
			return validateHaikuGoldenStdout(golden, unsafe)
		}, want: "sanitization"},
		{name: "checksum mismatch", check: func() error {
			mismatched := golden
			mismatched.SHA256 = strings.Repeat("0", 64)
			return validateHaikuGoldenStdout(mismatched, valid)
		}, want: "sha256"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.check()
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), test.want) {
				t.Fatalf("validation error = %v, want %q", err, test.want)
			}
			if got := len(router.Requests()); got != 0 {
				t.Fatalf("route calls after pre-start validation failure = %d, want 0", got)
			}
		})
	}
	if got := router.RouteCount(); got != 1 {
		t.Fatalf("routes after pre-start validation failures = %d, want 1", got)
	}
}

func TestClaudeHaikuGoldenRouterRejectsInvalidRoutesWithoutLeaks(t *testing.T) {
	manifest := loadHaikuGoldenManifest(t)
	cases := prepareHaikuGoldenReplayCases(t, manifest)
	router := newHaikuGoldenCommandRouter(t, cases)
	t.Cleanup(router.Close)
	if got := router.RouteCount(); got != len(cases) {
		t.Fatalf("initial route count = %d, want %d", got, len(cases))
	}

	duplicateDirectory := append([]haikuGoldenReplayCase(nil), cases...)
	duplicateDirectory[1].factoryDir = duplicateDirectory[0].factoryDir
	assertDuplicateRouteRejected(t, duplicateDirectory, "Factory directory")
	duplicateSelector := append([]haikuGoldenReplayCase(nil), cases...)
	duplicateSelector[1].golden.Selector = duplicateSelector[0].golden.Selector
	assertDuplicateRouteRejected(t, duplicateSelector, "selector")

	request := platformprocess.CommandRequest{
		Command: string(modelprovider.ProviderClaude),
		WorkDir: filepath.Join(t.TempDir(), haikuAdverseSecret),
		Args:    []string{"--model", haikuAdverseSecret},
		Env:     []string{"HAIKU_ROUTE_SECRET=" + haikuAdverseSecret},
		Stdin:   []byte(haikuAdverseSecret),
	}
	assertHaikuRouteRejected(t, router, request, "unavailable")
	if got := router.RouteCount(); got != len(cases) {
		t.Fatalf("routes after unknown-route rejection = %d, want %d", got, len(cases))
	}

	unexpectedCommand := request
	unexpectedCommand.Command = "unexpected-provider"
	assertHaikuRouteRejected(t, router, unexpectedCommand, "unexpected provider command")

	request.WorkDir = cases[0].factoryDir
	assertHaikuRouteRejected(t, router, request, "selector")
	router.Close()
	assertHaikuRouteRejected(t, router, request, "closed")
	if got := router.RouteCount(); got != 0 {
		t.Fatalf("routes after close = %d, want 0", got)
	}
}

func assertDuplicateRouteRejected(t *testing.T, cases []haikuGoldenReplayCase, want string) {
	t.Helper()
	router, err := buildHaikuGoldenCommandRouter(cases)
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("duplicate route error = %v, want %q", err, want)
	}
	if router != nil {
		t.Fatal("duplicate route builder returned a partially registered router")
	}
}

func assertHaikuRouteRejected(
	t *testing.T,
	router *haikuGoldenCommandRouter,
	request platformprocess.CommandRequest,
	want string,
) {
	t.Helper()
	requestCount := len(router.Requests())
	_, err := router.Run(context.Background(), request)
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("route error = %v, want %q", err, want)
	}
	if strings.Contains(err.Error(), haikuAdverseSecret) {
		t.Fatalf("route error leaked request or environment data: %q", err)
	}
	if got := len(router.Requests()); got != requestCount {
		t.Fatalf("route calls after rejected request = %d, want unchanged at %d", got, requestCount)
	}
}

func TestClaudeHaikuGoldenAdverseProcessPathsReclaimResources(t *testing.T) {
	t.Run("partial result and assertion failure", func(t *testing.T) {
		replayCase := prepareHaikuGoldenReplayCase(t, loadHaikuGoldenManifest(t).Cases[0])
		replayCase.stdout = []byte(`{"type":"system","subtype":"init","session_id":"partial-session"}` + "\n")
		server, router, sessionID := startHaikuAdverseSession(t, replayCase)
		finished := false
		defer func() {
			if !finished {
				finishHaikuAdverseSession(t, server, router, sessionID)
			}
		}()

		submitHaikuAdverseWork(t, server, sessionID, "partial-result")
		// Partial completion is reported asynchronously by the public Factory
		// Session projection. The injected runner has no equivalent signal for
		// Work classification, so this bounded public observation is necessary.
		support.WaitForSessionTerminalStatus(t, server.URL(), sessionID, 20*time.Second)
		listed := support.GetJSON[factoryapi.ListWorkResponse](t, sessionWorkURL(server.URL(), sessionID))
		if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 0 {
			t.Fatalf("partial result completed work = %d, want 0", got)
		}
		if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
			t.Fatalf("partial result failed work = %d, want 1", got)
		}
		if err := assertSuccessfulHaikuGoldenWork(t, server, router, replayCase, sessionID); err == nil {
			t.Fatal("partial result unexpectedly satisfied the successful work assertion")
		}
		if got := router.CallsFor(replayCase.factoryDir); got == 0 {
			t.Fatal("partial result did not reach the controlled Claude route")
		}
		finishHaikuAdverseSession(t, server, router, sessionID)
		finished = true
	})

	t.Run("cancellation", func(t *testing.T) {
		replayCase := prepareHaikuGoldenReplayCase(t, loadHaikuGoldenManifest(t).Cases[0])
		replayCase.blockUntilCancellation = true
		replayCase.started = make(chan struct{})
		server, router, sessionID := startHaikuAdverseSession(t, replayCase)
		finished := false
		defer func() {
			if !finished {
				finishHaikuAdverseSession(t, server, router, sessionID)
			}
		}()

		submitHaikuAdverseWork(t, server, sessionID, "cancellation")
		// The runner's started channel is the deterministic signal that
		// Process.Execute reached the controlled dependency edge. Retain only a
		// bounded timeout so a dispatch regression fails instead of hanging; no
		// public endpoint can establish that this exact command was reached.
		select {
		case <-replayCase.started:
		case <-time.After(10 * time.Second):
			t.Fatal("cancellation route did not start")
		}
		support.TerminateFactorySessionAt(t, server.URL(), sessionID)
		// Termination is asynchronous at the public Factory Session boundary.
		// The route-start signal cannot prove that the session projection has
		// stopped, so observe the public status before deleting the session.
		support.WaitForSessionStopped(t, server.URL(), sessionID, 20*time.Second)
		if got := router.CallsFor(replayCase.factoryDir); got != 1 {
			t.Fatalf("canceled route calls = %d, want 1", got)
		}
		finishHaikuAdverseSession(t, server, router, sessionID)
		finished = true
	})
}

func startHaikuAdverseSession(
	t *testing.T,
	replayCase haikuGoldenReplayCase,
) (*support.FunctionalAPIServer, *haikuGoldenCommandRouter, string) {
	t.Helper()
	router := newHaikuGoldenCommandRouter(t, []haikuGoldenReplayCase{replayCase})
	t.Cleanup(router.Close)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                replayCase.factoryDir,
		WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{
			ProviderCommandRunner: router,
		},
	})
	opened := support.OpenFactorySessionAt(t, server.URL(), replayCase.factoryDir)
	if opened.Session == nil || opened.Session.Id == "" {
		t.Fatal("adverse Claude session has no id")
	}
	return server, router, opened.Session.Id
}

func submitHaikuAdverseWork(t *testing.T, server *support.FunctionalAPIServer, sessionID, name string) {
	t.Helper()
	support.SubmitSessionWorkAt(t, server.URL(), sessionID, factoryapi.SubmitWorkRequest{
		Name:         &name,
		WorkTypeName: "task",
		Payload:      map[string]string{"title": "controlled Claude adverse replay"},
	})
}

func finishHaikuAdverseSession(
	t *testing.T,
	server *support.FunctionalAPIServer,
	router *haikuGoldenCommandRouter,
	sessionID string,
) {
	t.Helper()
	// CloseFactorySessionAt observes the public stopped state before deletion;
	// that lifecycle transition is asynchronous and cannot be replaced by the
	// runner's command signal without bypassing the Factory Session boundary.
	support.CloseFactorySessionAt(t, server.URL(), sessionID)
	assertHaikuSessionDeleted(t, server.URL(), sessionID)
	// ProcessCommand.Stop cancels and joins Process.Execute. Waiting on Done
	// again would duplicate that deterministic shutdown observation.
	server.Stop(t)
	server.Close(t)
	router.Close()
	if got := router.RouteCount(); got != 0 {
		t.Errorf("adverse cleanup route count = %d, want 0", got)
	}
	assertHaikuServerListenerClosed(t, server.URL())
}

func assertHaikuSessionDeleted(t *testing.T, baseURL, sessionID string) {
	t.Helper()
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID)
	client := &http.Client{Timeout: 5 * time.Second}
	response, err := client.Get(endpoint)
	if err != nil {
		t.Fatalf("GET deleted Claude session: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		_, _ = io.Copy(io.Discard, response.Body)
		t.Fatalf("deleted Claude session status = %d, want 404", response.StatusCode)
	}
}

func assertHaikuServerListenerClosed(t *testing.T, endpoint string) {
	t.Helper()
	client := &http.Client{Timeout: 2 * time.Second}
	response, err := client.Get(strings.TrimSuffix(endpoint, "/") + "/status")
	if err == nil {
		response.Body.Close()
		t.Fatal("Claude adverse server listener remained reachable after stop")
	}
}
