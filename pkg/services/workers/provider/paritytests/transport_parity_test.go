package providerparity

import (
	"context"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"reflect"
	"testing"
)

func TestTransportParity_FullStreamCLIAndAPIAgree(t *testing.T) {
	t.Parallel()
	assertTransportParityForFixture(t, FixtureFullStreamClaude)
}

func TestTransportParity_PartialStreamCLIAndAPIAgree(t *testing.T) {
	t.Parallel()
	assertTransportParityForFixture(t, FixturePartialStreamCodex)
}

func TestTransportParity_SnapshotOnlyCLIAndAPIAgree(t *testing.T) {
	t.Parallel()
	assertTransportParityForFixture(t, FixtureSnapshotOnlyOpenCode)
}

func TestTransportParity_FinalOnlyOpenCodeCLIAndAPIAgree(t *testing.T) {
	t.Parallel()
	assertTransportParityForFixture(t, FixtureFinalOnlyOpenCode)
}

func TestTransportParity_AgyFinalOnlyCLIAndAPIAgree(t *testing.T) {
	t.Parallel()
	assertTransportParityForFixture(t, FixtureAgyFinalOnly)
}

func TestTransportParity_ToolLifecycleCLIAndAPIAgree(t *testing.T) {
	t.Parallel()
	assertTransportParityForFixture(t, FixtureToolLifecycleClaude)
}

func TestTransportParity_SSEFramesMatchAPIRecordDecoding(t *testing.T) {
	t.Parallel()

	fixture, ok := FixtureByID(FixtureFullStreamClaude)
	if !ok {
		t.Fatal("missing full-stream fixture")
	}
	outcome, err := RunTransportParity(context.Background(), fixture)
	if err != nil {
		t.Fatalf("RunTransportParity() error = %v", err)
	}
	records, err := EncodeTransportAPIRecords(outcome.Events)
	if err != nil {
		t.Fatalf("EncodeTransportAPIRecords() error = %v", err)
	}
	recordEvents, err := DecodeTransportAPIRecords(records)
	if err != nil {
		t.Fatalf("DecodeTransportAPIRecords() error = %v", err)
	}
	for index, event := range outcome.Events {
		frame, err := EncodeTransportSSEFrame(event)
		if err != nil {
			t.Fatalf("EncodeTransportSSEFrame(event[%d]) error = %v", index, err)
		}
		decodedFrame, err := DecodeTransportSSEFrame(frame)
		if err != nil {
			t.Fatalf("DecodeTransportSSEFrame(event[%d]) error = %v", index, err)
		}
		if !reflect.DeepEqual(decodedFrame, recordEvents[index]) {
			t.Fatalf("SSE decode mismatch for event[%d]: frame=%#v record=%#v", index, decodedFrame, recordEvents[index])
		}
	}
}

func assertTransportParityForFixture(t *testing.T, fixtureID string) {
	t.Helper()

	fixture, ok := FixtureByID(fixtureID)
	if !ok {
		t.Fatalf("unknown fixture %q", fixtureID)
	}
	outcome, err := RunTransportParity(context.Background(), fixture)
	if err != nil {
		t.Fatalf("RunTransportParity(%q) error = %v", fixtureID, err)
	}
	if err := AssertTruthfulStreamingFidelity(fixture, outcome); err != nil {
		t.Fatalf("AssertTruthfulStreamingFidelity(%q) error = %v", fixtureID, err)
	}
	if err := AssertCLIAPITransportParity(outcome); err != nil {
		t.Fatalf("AssertCLIAPITransportParity(%q) error = %v", fixtureID, err)
	}
	if fixture.ToolLifecycle {
		if err := AssertObservableToolLifecycle(outcome.Events); err != nil {
			t.Fatalf("AssertObservableToolLifecycle(%q) error = %v", fixtureID, err)
		}
	}
	if outcome.Terminal.Response.Content != fixture.WantContent {
		t.Fatalf("terminal content = %q, want %q", outcome.Terminal.Response.Content, fixture.WantContent)
	}
	apiInvocation := apisurface.InvocationResponseFromResult(outcome.InvocationResult)
	if apiInvocation.Status == "" {
		t.Fatalf("API invocation projection missing status")
	}
}
