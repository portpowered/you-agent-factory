package run

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/initializer"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestMapServerFailure_CharacterizesUncodedRestoreCauseRedaction(t *testing.T) {
	t.Parallel()
	for _, debug := range []bool{false, true} {
		t.Run(fmt.Sprintf("debug=%t", debug), func(t *testing.T) {
			t.Parallel()
			cause := errors.New(`restore Work board: active Work "synthetic-cron-input" has no current place occupancy; payload=PRIVATE`)
			startup := &initializer.RuntimeHostStartupError{Cause: fmt.Errorf("activate resumed runtime: %w", cause)}
			mapped := MapServerFailureForInvocation(startup, true)
			var stderr bytes.Buffer
			if !WriteInvocationError(&stderr, mapped, debug) {
				t.Fatal("startup failure did not render a standard error response")
			}
			var response factoryapi.ErrorResponse
			if err := json.Unmarshal(stderr.Bytes(), &response); err != nil {
				t.Fatalf("decode error response: %v", err)
			}
			want := "requested server did not start: runtime startup failed (failure_class=runtime_startup_failed)"
			if string(response.Code) != ServerStartFailedCode || response.Family != factoryapi.ErrorFamilyInternalServerError || response.Message != want {
				t.Fatalf("response = %#v, want generic startup failure", response)
			}
			if strings.Contains(stderr.String(), "PRIVATE") || strings.Contains(stderr.String(), "synthetic-cron-input") {
				t.Fatal("uncoded restore cause leaked into stderr")
			}
			if !errors.Is(mapped, cause) {
				t.Fatal("startup mapping lost the underlying cause identity")
			}
		})
	}
}
