package support

import (
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestDecodeInvocationResponseJSONAcceptsSingleJSONAndNDJSON(t *testing.T) {
	t.Run("single response", func(t *testing.T) {
		response := DecodeInvocationResponseJSON(t, `{"status":"COMPLETED"}`)
		if response.Status != factoryapi.InvocationTerminalStatusCompleted {
			t.Fatalf("status = %q, want COMPLETED", response.Status)
		}
	})

	t.Run("response stream", func(t *testing.T) {
		response := DecodeInvocationResponseJSON(t, "{\"recordType\":\"factory_event\",\"event\":{}}\n{\"recordType\":\"invocation_result\",\"response\":{\"status\":\"FAILED\"}}\n")
		if response.Status != factoryapi.InvocationTerminalStatusFailed {
			t.Fatalf("status = %q, want FAILED", response.Status)
		}
	})
}
