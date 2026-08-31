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
	fixture := claudeSharedProcess(t)
	t.Run("partial result and assertion failure", func(t *testing.T) {
		route := fixture.route(t, "adverse-partial")
		callStart := fixture.runner.CallsFor(route.factoryDir)
		session := fixture.openSession(t, "adverse-partial")

		fixture.submitWork(t, session, "partial-result")
		// Partial completion is reported asynchronously by the public Factory
		// Session projection. The injected runner has no equivalent signal for
		// Work classification, so this bounded public observation is necessary.
		support.WaitForSessionTerminalStatus(t, fixture.baseURL, session.id, 20*time.Second)
		listed := support.GetJSON[factoryapi.ListWorkResponse](t, sessionWorkURL(fixture.baseURL, session.id))
		if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 0 {
			t.Fatalf("partial result completed work = %d, want 0", got)
		}
		if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
			t.Fatalf("partial result failed work = %d, want 1", got)
		}
		if err := assertSuccessfulHaikuGoldenWork(t, fixture.baseURL, fixture.runner, route, session.id); err == nil {
			t.Fatal("partial result unexpectedly satisfied the successful work assertion")
		}
		if got := fixture.runner.CallsFor(route.factoryDir) - callStart; got == 0 {
			t.Fatal("partial result did not reach the controlled Claude route")
		}
		fixture.closeSession(t, session)
		assertHaikuSessionDeleted(t, fixture.baseURL, session.id)
	})

	t.Run("cancellation", func(t *testing.T) {
		route := fixture.route(t, "adverse-cancellation")
		callStart := fixture.runner.CallsFor(route.factoryDir)
		session := fixture.openSession(t, "adverse-cancellation")

		fixture.submitWork(t, session, "cancellation")
		// The runner's started channel is the deterministic signal that
		// Process.Execute reached the controlled dependency edge. Retain only a
		// bounded timeout so a dispatch regression fails instead of hanging; no
		// public endpoint can establish that this exact command was reached.
		select {
		case <-route.started:
		case <-time.After(10 * time.Second):
			t.Fatal("cancellation route did not start")
		}
		support.TerminateFactorySessionAt(t, fixture.baseURL, session.id)
		// Termination is asynchronous at the public Factory Session boundary.
		// The route-start signal cannot prove that the session projection has
		// stopped, so observe the public status before deleting the session.
		support.WaitForSessionStopped(t, fixture.baseURL, session.id, 20*time.Second)
		if got := fixture.runner.CallsFor(route.factoryDir) - callStart; got != 1 {
			t.Fatalf("canceled route calls = %d, want 1", got)
		}
		if got := fixture.runner.ActiveCallCount(); got != 0 {
			t.Fatalf("active Claude calls after cancellation = %d, want 0", got)
		}
		fixture.closeSession(t, session)
		assertHaikuSessionDeleted(t, fixture.baseURL, session.id)
	})
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
