package factorysessionsse

import (
	"bufio"
	"encoding/json"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func testAPIFactoryEvent(t *testing.T, eventType factoryapi.FactoryEventType, id string, context factoryapi.FactoryEventContext, payload any) factoryapi.FactoryEvent {
	t.Helper()

	var eventPayload factoryapi.FactoryEvent_Payload
	var err error
	switch typed := payload.(type) {
	case factoryapi.RunRequestEventPayload:
		err = eventPayload.FromRunRequestEventPayload(typed)
	case factoryapi.InitialStructureRequestEventPayload:
		err = eventPayload.FromInitialStructureRequestEventPayload(typed)
	case factoryapi.WorkRequestEventPayload:
		err = eventPayload.FromWorkRequestEventPayload(typed)
	case factoryapi.DispatchRequestEventPayload:
		err = eventPayload.FromDispatchRequestEventPayload(typed)
	default:
		t.Fatalf("unsupported test factory event payload %T", payload)
	}
	if err != nil {
		t.Fatalf("encode test factory event payload: %v", err)
	}
	return factoryapi.FactoryEvent{
		SchemaVersion: factoryapi.AgentFactoryEventV1,
		Type:          eventType,
		Id:            id,
		Context:       context,
		Payload:       eventPayload,
	}
}

func tryReadSSEFactoryEvent(reader *bufio.Reader) (factoryapi.FactoryEvent, bool, error) {
	var dataLine string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if len(dataLine) == 0 {
				return factoryapi.FactoryEvent{}, false, nil
			}
			return factoryapi.FactoryEvent{}, false, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if strings.HasPrefix(line, "data: ") {
			dataLine = strings.TrimPrefix(line, "data: ")
		}
	}
	if dataLine == "" {
		return factoryapi.FactoryEvent{}, false, nil
	}
	var event factoryapi.FactoryEvent
	if err := json.Unmarshal([]byte(dataLine), &event); err != nil {
		return factoryapi.FactoryEvent{}, false, err
	}
	return event, true, nil
}

func stringPointerForAPIServerTest(value string) *string {
	return &value
}
