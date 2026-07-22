package run

import (
	"encoding/json"
	"fmt"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

func encodePublicResponseStreamFixtures(
	events []factorysessions.FactoryResponseEvent,
	invocation interfaces.FactoryInvocationResult,
) ([]string, error) {
	lines := make([]string, 0, len(events)+1)
	for _, event := range events {
		line, err := json.Marshal(responseStreamJSONResponseEventRecord{
			RecordType: responseStreamJSONRecordResponseEvent,
			Event:      event,
		})
		if err != nil {
			return nil, fmt.Errorf("encode response event: %w", err)
		}
		lines = append(lines, string(line))
	}
	line, err := json.Marshal(responseStreamJSONInvocationResultRecord{
		RecordType: responseStreamJSONRecordInvocationResult,
		Invocation: apisurface.InvocationResponseFromResult(invocation),
	})
	if err != nil {
		return nil, fmt.Errorf("encode invocation result: %w", err)
	}
	return append(lines, string(line)), nil
}

func decodePublicResponseStreamFixtures(
	lines []string,
) ([]factorysessions.FactoryResponseEvent, factoryapi.InvocationResponse, error) {
	if len(lines) == 0 {
		return nil, factoryapi.InvocationResponse{}, fmt.Errorf("response stream is empty")
	}
	events := make([]factorysessions.FactoryResponseEvent, 0, len(lines)-1)
	for index, line := range lines[:len(lines)-1] {
		var record responseStreamJSONResponseEventRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			return nil, factoryapi.InvocationResponse{}, fmt.Errorf("decode response event %d: %w", index, err)
		}
		if record.RecordType != responseStreamJSONRecordResponseEvent {
			return nil, factoryapi.InvocationResponse{}, fmt.Errorf(
				"response event %d recordType = %q",
				index,
				record.RecordType,
			)
		}
		events = append(events, record.Event)
	}
	var final responseStreamJSONInvocationResultRecord
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &final); err != nil {
		return nil, factoryapi.InvocationResponse{}, fmt.Errorf("decode invocation result: %w", err)
	}
	if final.RecordType != responseStreamJSONRecordInvocationResult {
		return nil, factoryapi.InvocationResponse{}, fmt.Errorf(
			"invocation result recordType = %q",
			final.RecordType,
		)
	}
	return events, final.Invocation, nil
}
