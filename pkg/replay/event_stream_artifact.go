package replay

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

const maxEventStreamLineBytes = 16 * 1024 * 1024

// EventStreamArtifactResult captures the parsed replay artifact plus a small
// amount of recovery metadata for partially written SSE logs.
type EventStreamArtifactResult struct {
	Artifact              *interfaces.ReplayArtifact
	ParsedEvents          int
	SkippedTrailingBlocks int
}

const legacyEventStreamCronPlaceholderSchedule = "* * * * *"

// ArtifactFromEventStream parses an SSE-style event stream whose payloads are
// canonical FactoryEvent JSON documents and returns a hydrated replay artifact.
// If the stream ends with a truncated final event block, that final block is
// skipped so long as at least one complete event was already recovered.
func ArtifactFromEventStream(r io.Reader) (*EventStreamArtifactResult, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), maxEventStreamLineBytes)

	var (
		dataLines             []string
		events                []factoryapi.FactoryEvent
		skippedTrailingBlocks int
		blockIndex            int
	)

	flushBlock := func(atEOF bool) error {
		if len(dataLines) == 0 {
			return nil
		}
		blockIndex += 1
		payload := strings.Join(dataLines, "\n")
		dataLines = dataLines[:0]

		var event factoryapi.FactoryEvent
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			if atEOF && len(events) > 0 {
				skippedTrailingBlocks += 1
				return nil
			}
			return fmt.Errorf("decode event stream block %d: %w", blockIndex, err)
		}
		if event.Id == "" || event.Type == "" {
			if atEOF && len(events) > 0 {
				skippedTrailingBlocks += 1
				return nil
			}
			return fmt.Errorf("decode event stream block %d: required replay event fields missing", blockIndex)
		}
		if event.SchemaVersion == "" {
			event.SchemaVersion = factoryapi.AgentFactoryEventV1
		}
		events = append(events, event)
		return nil
	}

	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case line == "":
			if err := flushBlock(false); err != nil {
				return nil, err
			}
		case strings.HasPrefix(line, "data: "):
			dataLines = append(dataLines, line[6:])
		case strings.HasPrefix(line, "data:"):
			dataLines = append(dataLines, strings.TrimLeft(line[5:], " \t"))
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan event stream: %w", err)
	}
	if err := flushBlock(true); err != nil {
		return nil, err
	}
	if len(events) == 0 {
		return nil, fmt.Errorf("event stream contained no replayable events")
	}
	if err := normalizeEventStreamRunRequestFactories(events); err != nil {
		return nil, err
	}

	artifact := &interfaces.ReplayArtifact{
		SchemaVersion: CurrentSchemaVersion,
		Events:        append([]factoryapi.FactoryEvent(nil), events...),
	}
	if err := hydrateArtifactFromEvents(artifact); err != nil {
		return nil, err
	}
	if err := Validate(artifact); err != nil {
		return nil, err
	}
	return &EventStreamArtifactResult{
		Artifact:              artifact,
		ParsedEvents:          len(events),
		SkippedTrailingBlocks: skippedTrailingBlocks,
	}, nil
}

func normalizeEventStreamRunRequestFactories(events []factoryapi.FactoryEvent) error {
	for index := range events {
		event := &events[index]
		if event.Type != factoryapi.FactoryEventTypeRunRequest {
			continue
		}

		payload, err := runStartedPayloadFromEvent(*event)
		if err != nil {
			return err
		}
		if payload.Factory.Workstations != nil {
			for workstationIndex := range *payload.Factory.Workstations {
				workstation := &(*payload.Factory.Workstations)[workstationIndex]
				if workstation.Behavior != nil &&
					*workstation.Behavior == factoryapi.WorkstationKindCron &&
					workstation.Cron == nil {
					workstation.Cron = &factoryapi.WorkstationCron{
						Schedule: legacyEventStreamCronPlaceholderSchedule,
					}
				}
			}
		}

		var union factoryapi.FactoryEvent_Payload
		if err := union.FromRunRequestEventPayload(payload); err != nil {
			return fmt.Errorf("normalize run started event payload: %w", err)
		}
		event.Payload = union
	}
	return nil
}
