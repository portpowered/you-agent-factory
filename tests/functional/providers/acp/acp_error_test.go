package acp_test

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestACPProtocolFailuresMapToStableWorkerFailureClasses(t *testing.T) {
	for _, test := range []struct {
		mode string
		want factoryapi.WorkFailureType
	}{
		{mode: "version", want: factoryapi.WorkFailureTypeMisconfigured},
		{mode: "init-fail", want: factoryapi.WorkFailureTypeUnknown},
		{mode: "malformed", want: factoryapi.WorkFailureTypeUnknown},
		{mode: "eof", want: factoryapi.WorkFailureTypeUnknown},
		{mode: "fail", want: factoryapi.WorkFailureTypeUnknown},
	} {
		t.Run(test.mode, func(t *testing.T) {
			dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
			testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"ACP failure"}`))
			writeACPWorker(t, dir, "cursor-acp")
			t.Setenv(acpHelperEnvironment, test.mode)

			var starts atomic.Int32
			_, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
				PlatformProcessCommandFactory: acpHelperCommandFactory(&starts),
				ProvidersExecutableLocator:    availableExecutableLocator{},
			}, 20*time.Second)
			if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
				t.Fatalf("failed work = %d, want 1", got)
			}
			if starts.Load() == 0 {
				t.Fatal("ACP protocol failure did not start the Agent process")
			}
			assertFactoryFailureReason(t, events, test.want)
		})
	}
}

func TestUnavailableACPExecutableFailsBeforeStartWithMissingExecutableClass(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"missing ACP executable"}`))
	writeACPWorker(t, dir, "cursor-acp")

	var starts atomic.Int32
	_, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
		PlatformProcessCommandFactory: acpHelperCommandFactory(&starts),
		ProvidersExecutableLocator:    missingExecutableLocator{},
	}, 20*time.Second)
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
		t.Fatalf("failed work = %d, want 1", got)
	}
	if starts.Load() != 0 {
		t.Fatalf("ACP starts = %d, want 0 for unavailable executable", starts.Load())
	}
	assertFactoryFailureReason(t, events, factoryapi.WorkFailureTypeMissingExecutable)
}

func assertFactoryFailureReason(t *testing.T, events []factoryapi.FactoryEvent, want factoryapi.WorkFailureType) {
	t.Helper()
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeInferenceResponse {
			continue
		}
		payload, err := event.Payload.AsInferenceResponseEventPayload()
		if err != nil {
			t.Fatalf("decode inference response: %v", err)
		}
		if payload.FailureDetail != nil && payload.FailureDetail.Reason == want {
			return
		}
	}
	t.Fatalf("Factory events omitted failure reason %q: %#v", want, events)
}

type missingExecutableLocator struct{}

func (missingExecutableLocator) LookPath(string) (string, error) {
	return "", errors.New("executable not found")
}
