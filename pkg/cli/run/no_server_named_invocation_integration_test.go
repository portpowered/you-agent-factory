package run

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
)

// TestNoServerNamedInvocationIntegrationAndEquivalenceProof is the consolidated
// package integration and invocation-equivalence proof for hermetic named
// one-shot invocation on the shared no-server bootstrap path. It fails if named
// runs regress to requiring a listening HTTP server or drift from shared CLI/API
// input-resolution and primary-result contracts.
func TestNoServerNamedInvocationIntegrationAndEquivalenceProof(t *testing.T) {
	if testing.Short() {
		t.Skip("consolidated package integration and invocation-equivalence proof for no-server named invocation")
	}

	t.Run("hermetic named success without listener", func(t *testing.T) {
		preserveRunGlobals(t)

		goalText := "consolidated no-server named integration proof"
		probePort := reserveNoServerInvocationProbePort(t)
		cfg := namedGoalNoServerInvocationRunConfig(t, goalText)
		cfg.Port = probePort
		cfg.AutoPort = true
		var output bytes.Buffer
		cfg.Output = &output

		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()

		if err := Run(ctx, cfg); err != nil {
			t.Fatalf("Run: %v", err)
		}
		if got := output.String(); got != "mock worker accepted" {
			t.Fatalf("stdout = %q, want invocationReturn primary result mock worker accepted", got)
		}
		assertNoServerInvocationProbePortFree(t, probePort)
	})

	t.Run("shared input resolution and primary-result equivalence", func(t *testing.T) {
		preserveRunGlobals(t)
		capture := installCapturingRealInvocationBootstrap(t)

		goalText := "consolidated no-server equivalence proof"
		cfg := namedGoalNoServerInvocationRunConfig(t, goalText)
		cfg.JSONOutput = true
		var output bytes.Buffer
		cfg.Output = &output

		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()

		if err := Run(ctx, cfg); err != nil {
			t.Fatalf("Run: %v", err)
		}

		apiRequest, err := invocationRequestFromLogicalAPIText(goalText)
		if err != nil {
			t.Fatalf("invocationRequestFromLogicalAPIText: %v", err)
		}
		if capture.lastRequest == nil {
			t.Fatal("expected InvokeFactorySession request capture on real no-server bootstrap")
		}
		assertEquivalentInvocationRequests(t, capture.lastRequest, apiRequest)

		if capture.lastResult == nil {
			t.Fatal("expected InvokeFactorySession result capture on real no-server bootstrap")
		}
		if capture.lastResult.Status != factoryapi.InvocationTerminalStatusCompleted {
			t.Fatalf("status = %q, want %q", capture.lastResult.Status, factoryapi.InvocationTerminalStatusCompleted)
		}

		var cliResponse factoryapi.InvocationResponse
		if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &cliResponse); err != nil {
			t.Fatalf("decode CLI invocation response: %v\n%s", err, output.String())
		}
		apiResponse := apisurface.InvocationResponseFromResult(*capture.lastResult)
		if !reflect.DeepEqual(cliResponse, apiResponse) {
			t.Fatalf("CLI response = %#v, API projection = %#v", cliResponse, apiResponse)
		}
		assertInvocationResponseMatchesFactoryResult(t, cliResponse, *capture.lastResult)
	})
}
