package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	startupcli "github.com/portpowered/infinite-you/pkg/initializer/process"
	costscli "github.com/portpowered/infinite-you/pkg/services/costs/transports/cli"
	generatedclient "github.com/portpowered/infinite-you/pkg/transports/http/client"
)

func TestProductionMetricsCostsTimeoutDiagnosticPreservesEndpointAcrossModes(t *testing.T) {
	t.Parallel()

	const requestTimeout = 25 * time.Millisecond
	modes := [][]string{
		{"--server", "PLACEHOLDER", "metrics", "costs"},
		{"--server", "PLACEHOLDER", "--verbose", "metrics", "costs"},
		{"--server", "PLACEHOLDER", "--debug", "metrics", "costs"},
	}
	started := make(chan struct{}, len(modes))
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		started <- struct{}{}
		<-request.Context().Done()
	}))
	defer server.Close()

	operation := costscli.NewOperation(func(serverURL string) (costscli.Client, error) {
		return generatedclient.NewClientWithResponses(
			serverURL,
			generatedclient.WithHTTPClient(&http.Client{}),
		)
	})
	factory := withTestInjectedPlatformRoles(CommandFactory{
		costsCLI: func(ctx context.Context, config costscli.CostsConfig) error {
			config.RequestTimeout = requestTimeout
			return operation(ctx, config)
		},
	})

	wantMessage := fmt.Sprintf(
		"GET /metrics/costs at %s timed out within the configured %s request timeout; retry or narrow the request with --session",
		server.URL,
		requestTimeout,
	)
	for _, template := range modes {
		args := append([]string(nil), template...)
		args[1] = server.URL
		var stdout, stderr bytes.Buffer
		err := factory.ExecuteCommand(startupcli.CommandInvocation{
			Arguments: args,
			Stdin:     strings.NewReader(""),
			Stdout:    &stdout,
			Stderr:    &stderr,
			Context:   context.Background(),
			HomeDir:   func() (string, error) { return "operator-home", nil },
			LookupEnv: func(string) (string, bool) { return "", false },
		})
		assertMetricsTimeoutCommand(t, args, err, stdout.String(), stderr.String(), wantMessage)
		waitForDelayedCostsRequest(t, started, args)
	}
}

func assertMetricsTimeoutCommand(
	t *testing.T,
	args []string,
	err error,
	stdout string,
	stderr string,
	wantMessage string,
) {
	t.Helper()
	if err == nil {
		t.Fatalf("ExecuteCommand(%v) error = nil, want timeout", args)
	}
	if stdout != "" {
		t.Fatalf("stdout for %v = %q, want empty failure output", args, stdout)
	}
	assertSingleMetricsDiagnostic(t, stderr, costscli.CostsRequestTimeoutCode, "INTERNAL_SERVER_ERROR", wantMessage)
	if strings.Contains(stderr, "CLI_COMMAND_FAILED") || strings.Contains(stderr, "INTERNAL_SERVER_ERROR: command failed") {
		t.Fatalf("timeout diagnostic for %v collapsed to a generic failure: %q", args, stderr)
	}
	var costsErr *costscli.CostsError
	if !errors.As(err, &costsErr) || costsErr.CLIErrorCode() != costscli.CostsRequestTimeoutCode {
		t.Fatalf("ExecuteCommand(%v) error = %v, want typed timeout", args, err)
	}
}

func waitForDelayedCostsRequest(t *testing.T, started <-chan struct{}, args []string) {
	t.Helper()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatalf("delayed costs server did not receive %v", args)
	}
}
