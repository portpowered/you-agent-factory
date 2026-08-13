package cli_test

import (
	"bytes"
	"io"
	"strings"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	visualizationcli "github.com/portpowered/infinite-you/pkg/services/factory_visualization/transports/cli"
	"github.com/portpowered/infinite-you/pkg/services/work"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

func TestHumanFactoryEventRendererDefersTerminalSuccessClaimsUntilInvocationOutcome(t *testing.T) {
	t.Parallel()

	terminalEvents := []interfaces.FactoryEvent{
		{
			Type:    interfaces.FactoryEventTypeSessionResultUpdated,
			Context: interfaces.FactoryEventContext{Sequence: 1},
			Payload: mustJSON(t, interfaces.FactorySessionResultUpdatedEventPayload{
				ResultStatus: interfaces.FactorySessionResultStatusFinal,
			}),
		},
		{
			Type:    interfaces.FactoryEventTypeSessionCompleted,
			Context: interfaces.FactoryEventContext{Sequence: 2},
			Payload: mustJSON(t, interfaces.FactorySessionCompletedEventPayload{
				FinalStatus: interfaces.FactorySessionLifecycleStatusSucceeded,
			}),
		},
	}

	t.Run("canceled invocation discards success claims", func(t *testing.T) {
		var output bytes.Buffer
		renderer := openHumanRenderer(t, &output)
		renderer.PresentFactoryEvents(terminalEvents)
		if got := output.String(); got != "" {
			t.Fatalf("output before terminal outcome = %q, want deferred success claims", got)
		}
		if err := renderer.WriteFinalInvocationResult(apisurface.FactoryInvocationResult{
			Status:    interfaces.InvocationTerminalStatusCanceled,
			ErrorCode: string(interfaces.InvocationErrorCodeCanceled),
		}); err != nil {
			t.Fatalf("WriteFinalInvocationResult: %v", err)
		}
		got := output.String()
		if !strings.Contains(got, "status: CANCELED") {
			t.Fatalf("output = %q, want canceled invocation outcome", got)
		}
		for _, forbidden := range []string{"final output updated: FINAL", "factory completed: SUCCEEDED", "--- primary result ---"} {
			if strings.Contains(got, forbidden) {
				t.Fatalf("output = %q, contains canceled-run success claim %q", got, forbidden)
			}
		}
	})

	t.Run("completed invocation retains success claims", func(t *testing.T) {
		var output bytes.Buffer
		renderer := openHumanRenderer(t, &output)
		renderer.PresentFactoryEvents(terminalEvents)
		if err := renderer.WriteFinalInvocationResult(apisurface.FactoryInvocationResult{
			Status:        interfaces.InvocationTerminalStatusCompleted,
			PrimaryResult: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "done"}},
		}); err != nil {
			t.Fatalf("WriteFinalInvocationResult: %v", err)
		}
		got := output.String()
		for _, required := range []string{"final output updated: FINAL", "factory completed: SUCCEEDED", "--- primary result ---", "done"} {
			if !strings.Contains(got, required) {
				t.Fatalf("output = %q, missing successful-run output %q", got, required)
			}
		}
		if strings.Index(got, "factory completed: SUCCEEDED") > strings.Index(got, "--- primary result ---") {
			t.Fatalf("output = %q, completion claim should precede primary result", got)
		}
	})
}

func openHumanRenderer(t *testing.T, output io.Writer) visualizationcli.FactoryEventRenderer {
	t.Helper()
	renderer, err := newTestService().OpenFactoryEventRenderer(visualizationcli.FactoryEventRendererConfig{
		Output:               output,
		InvocationOutputMode: visualizationcli.InvocationOutputResponseStream,
	})
	if err != nil {
		t.Fatalf("OpenFactoryEventRenderer: %v", err)
	}
	return renderer
}
