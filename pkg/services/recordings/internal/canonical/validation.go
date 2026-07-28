// Package canonical contains Recordings-internal canonical event validation
// shared by the root service and private subservices.
package canonical

import (
	"encoding/json"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	factoryeventkinds "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/canonical_ledger/events/kinds"
)

// ValidAppendEvent reports whether event carries the identity, kind, timestamp,
// scope, and JSON payload required for canonical append and replay validation.
func ValidAppendEvent(event recordings.CanonicalEvent) bool {
	return strings.TrimSpace(string(event.ID)) != "" &&
		strings.TrimSpace(string(event.Kind)) != "" &&
		factoryeventkinds.IsPublicEmittableFactoryEventKind(
			factorydefinitions.FactoryEventType(event.Kind),
		) &&
		!event.RecordedAt.IsZero() &&
		(event.Scope.FactorySessionID == "" ||
			strings.TrimSpace(event.Scope.FactorySessionID) != "") &&
		json.Valid([]byte(event.Payload)) &&
		(event.SourceContext == "" || json.Valid([]byte(event.SourceContext)))
}

// ValidateProjectionEvents reports malformed scope or canonical ordering facts.
func ValidateProjectionEvents(
	scope recordings.CanonicalEventScope,
	after *recordings.CanonicalEventCursor,
	events []recordings.CanonicalEvent,
) error {
	if scope.FactorySessionID != "" && strings.TrimSpace(scope.FactorySessionID) == "" {
		return recordings.ErrInvalidProjectionScope
	}
	expected := recordings.CanonicalEventSequence(0)
	generationID := ""
	if after != nil {
		if after.StreamGenerationID == "" || after.Sequence < 0 {
			return recordings.ErrMalformedProjectionOrder
		}
		expected = after.Sequence + 1
		generationID = after.StreamGenerationID
	}
	previous := expected - 1
	for _, event := range events {
		if err := validateProjectionEvent(
			scope,
			event,
			expected,
			previous,
			generationID,
		); err != nil {
			return err
		}
		generationID = event.Cursor.StreamGenerationID
		previous = event.Sequence
		expected++
	}
	return nil
}

func validateProjectionEvent(
	scope recordings.CanonicalEventScope,
	event recordings.CanonicalEvent,
	expected recordings.CanonicalEventSequence,
	previous recordings.CanonicalEventSequence,
	generationID string,
) error {
	if event.Scope != scope {
		return recordings.ErrInvalidProjectionScope
	}
	if event.Cursor.Sequence != event.Sequence ||
		event.Cursor.StreamGenerationID == "" {
		return recordings.ErrMalformedProjectionOrder
	}
	if scope.FactorySessionID == "" && event.Sequence != expected {
		return recordings.ErrMalformedProjectionOrder
	}
	if scope.FactorySessionID != "" && event.Sequence <= previous {
		return recordings.ErrMalformedProjectionOrder
	}
	if generationID != "" && event.Cursor.StreamGenerationID != generationID {
		return recordings.ErrMalformedProjectionOrder
	}
	return nil
}
