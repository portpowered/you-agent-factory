package providerparity

import (
	"fmt"
	"reflect"

	"github.com/portpowered/infinite-you/pkg/factory/sessions/responseevents"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

// ObservationMode names one invocation stdout observation contract.
type ObservationMode string

const (
	// ObservationPrimaryOnly is the default authoritative final without live
	// response-stream progress on stdout.
	ObservationPrimaryOnly ObservationMode = "primary"
	// ObservationResponseStream enables live response events plus the terminal
	// invocation result on stdout.
	ObservationResponseStream ObservationMode = "response-stream"
)

// ModeParityOutcome captures authoritative finals for primary-only and
// response-stream observation of one fixture run.
type ModeParityOutcome struct {
	PrimaryOnlyInvocation factoryapi.InvocationResponse
	StreamEvents          []responseevents.FactoryResponseEvent
	StreamInvocation      factoryapi.InvocationResponse
}

// RunModeParity executes one fixture once and projects both observation modes
// from the same transport-neutral terminal outcome.
func RunModeParity(outcome TransportParityOutcome) (ModeParityOutcome, error) {
	primaryOnly := ProjectPrimaryOnlyInvocation(outcome)
	streamEvents, streamInvocation, err := ProjectResponseStreamInvocation(outcome)
	if err != nil {
		return ModeParityOutcome{}, err
	}
	return ModeParityOutcome{
		PrimaryOnlyInvocation: primaryOnly,
		StreamEvents:          streamEvents,
		StreamInvocation:      streamInvocation,
	}, nil
}

// ProjectPrimaryOnlyInvocation maps the authoritative terminal invocation for
// primary-only stdout mode.
func ProjectPrimaryOnlyInvocation(outcome TransportParityOutcome) factoryapi.InvocationResponse {
	return apisurface.InvocationResponseFromResult(outcome.InvocationResult)
}

// ProjectResponseStreamInvocation round-trips published events and the terminal
// invocation through the CLI response-stream NDJSON envelope.
func ProjectResponseStreamInvocation(outcome TransportParityOutcome) ([]responseevents.FactoryResponseEvent, factoryapi.InvocationResponse, error) {
	lines, err := EncodeTransportCLINDJSON(outcome.Events, outcome.InvocationResult)
	if err != nil {
		return nil, factoryapi.InvocationResponse{}, fmt.Errorf("encode response-stream NDJSON: %w", err)
	}
	events, invocation, err := DecodeTransportCLINDJSON(lines)
	if err != nil {
		return nil, factoryapi.InvocationResponse{}, fmt.Errorf("decode response-stream NDJSON: %w", err)
	}
	return events, invocation, nil
}

// AssertPrimaryStreamModeParity proves primary-only and response-stream modes
// agree on authoritative terminal InvocationResponse outcomes while response-stream
// mode still delivers published response events.
func AssertPrimaryStreamModeParity(outcome TransportParityOutcome) error {
	modeOutcome, err := RunModeParity(outcome)
	if err != nil {
		return err
	}
	if len(modeOutcome.StreamEvents) == 0 {
		return fmt.Errorf("response-stream mode produced no response events")
	}
	if !reflect.DeepEqual(modeOutcome.PrimaryOnlyInvocation, modeOutcome.StreamInvocation) {
		return fmt.Errorf(
			"mode parity mismatch: primary-only = %#v, response-stream = %#v",
			modeOutcome.PrimaryOnlyInvocation,
			modeOutcome.StreamInvocation,
		)
	}
	return nil
}
