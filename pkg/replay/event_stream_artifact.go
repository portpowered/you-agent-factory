package replay

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	factoryapi "github.com/portpowered/agent-factory/pkg/api/generated"
	"github.com/portpowered/agent-factory/pkg/config"
	"github.com/portpowered/agent-factory/pkg/interfaces"
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

// ArtifactFromEventStreamFile opens and parses a saved event stream file into a
// replay artifact.
func ArtifactFromEventStreamFile(path string) (*EventStreamArtifactResult, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open event stream %q: %w", path, err)
	}
	defer file.Close()

	result, err := ArtifactFromEventStream(file)
	if err != nil {
		return nil, fmt.Errorf("parse event stream %q: %w", path, err)
	}
	if err := hydrateArtifactFromAdjacentFactory(path, result.Artifact); err != nil {
		return nil, fmt.Errorf("hydrate replay artifact from adjacent factory for %q: %w", path, err)
	}
	return result, nil
}

// SaveArtifactFromEventStreamFile converts an event stream file into the
// canonical replay artifact JSON format.
func SaveArtifactFromEventStreamFile(eventStreamPath string, artifactPath string) (*EventStreamArtifactResult, error) {
	result, err := ArtifactFromEventStreamFile(eventStreamPath)
	if err != nil {
		return nil, err
	}
	if err := Save(artifactPath, result.Artifact); err != nil {
		return nil, fmt.Errorf("save replay artifact from event stream %q: %w", eventStreamPath, err)
	}
	return result, nil
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

func hydrateArtifactFromAdjacentFactory(eventStreamPath string, artifact *interfaces.ReplayArtifact) error {
	if artifact == nil {
		return nil
	}
	factoryDir, ok := adjacentFactoryDir(eventStreamPath)
	if !ok {
		return nil
	}
	loaded, err := config.LoadRuntimeConfig(factoryDir, nil)
	if err != nil {
		return nil
	}
	generated, err := GeneratedFactoryFromRuntimeConfig(
		loaded.FactoryDir(),
		loaded.FactoryConfig(),
		loaded,
		WithGeneratedFactorySourceDirectory(loaded.FactoryDir()),
	)
	if err != nil {
		return nil
	}
	merged := mergeGeneratedFactoryMissingRuntimeFields(artifact.Factory, generated)
	if err := rewriteArtifactFactoryEvents(artifact, merged); err != nil {
		return err
	}
	artifact.Factory = merged
	return nil
}

func adjacentFactoryDir(eventStreamPath string) (string, bool) {
	candidates := []string{
		filepath.Dir(eventStreamPath),
		filepath.Dir(filepath.Dir(eventStreamPath)),
	}
	for _, dir := range candidates {
		if dir == "" || dir == "." {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, interfaces.FactoryConfigFile)); err == nil {
			return dir, true
		}
	}
	return "", false
}

func mergeGeneratedFactoryMissingRuntimeFields(recorded factoryapi.Factory, authored factoryapi.Factory) factoryapi.Factory {
	merged := recorded
	if merged.FactoryDirectory == nil {
		merged.FactoryDirectory = authored.FactoryDirectory
	}
	if merged.SourceDirectory == nil {
		merged.SourceDirectory = authored.SourceDirectory
	}
	if merged.Id == nil {
		merged.Id = authored.Id
	}
	if merged.Metadata == nil || len(*merged.Metadata) == 0 {
		merged.Metadata = authored.Metadata
	}
	if merged.InputTypes == nil || len(*merged.InputTypes) == 0 {
		merged.InputTypes = authored.InputTypes
	}
	if merged.Workers != nil && authored.Workers != nil {
		authoredByName := make(map[string]factoryapi.Worker, len(*authored.Workers))
		for _, worker := range *authored.Workers {
			authoredByName[worker.Name] = worker
		}
		for i := range *merged.Workers {
			worker := &(*merged.Workers)[i]
			authoredWorker, ok := authoredByName[worker.Name]
			if !ok {
				continue
			}
			if worker.Type == nil {
				worker.Type = authoredWorker.Type
			}
			if worker.Command == nil {
				worker.Command = authoredWorker.Command
			}
			if worker.Args == nil || len(*worker.Args) == 0 {
				worker.Args = authoredWorker.Args
			}
			if worker.ModelProvider == nil {
				worker.ModelProvider = authoredWorker.ModelProvider
			}
			if worker.ExecutorProvider == nil {
				worker.ExecutorProvider = authoredWorker.ExecutorProvider
			}
			if worker.Timeout == nil {
				worker.Timeout = authoredWorker.Timeout
			}
			if worker.StopToken == nil {
				worker.StopToken = authoredWorker.StopToken
			}
			if worker.SkipPermissions == nil {
				worker.SkipPermissions = authoredWorker.SkipPermissions
			}
			if worker.Body == nil {
				worker.Body = authoredWorker.Body
			}
			if worker.Resources == nil || len(*worker.Resources) == 0 {
				worker.Resources = authoredWorker.Resources
			}
		}
	}
	if merged.Workstations != nil && authored.Workstations != nil {
		authoredByName := make(map[string]factoryapi.Workstation, len(*authored.Workstations))
		for _, workstation := range *authored.Workstations {
			authoredByName[workstation.Name] = workstation
		}
		for i := range *merged.Workstations {
			workstation := &(*merged.Workstations)[i]
			authoredWorkstation, ok := authoredByName[workstation.Name]
			if !ok {
				continue
			}
			if workstation.Id == nil {
				workstation.Id = authoredWorkstation.Id
			}
			if workstation.Behavior == nil {
				workstation.Behavior = authoredWorkstation.Behavior
			}
			if workstation.Type == nil {
				workstation.Type = authoredWorkstation.Type
			}
			if workstation.Worker == "" {
				workstation.Worker = authoredWorkstation.Worker
			}
			if len(workstation.Inputs) == 0 {
				workstation.Inputs = authoredWorkstation.Inputs
			}
			if len(workstation.Outputs) == 0 {
				workstation.Outputs = authoredWorkstation.Outputs
			}
			if workstation.OnFailure == nil {
				workstation.OnFailure = authoredWorkstation.OnFailure
			}
			if workstation.OnContinue == nil {
				workstation.OnContinue = authoredWorkstation.OnContinue
			}
			if workstation.OnRejection == nil {
				workstation.OnRejection = authoredWorkstation.OnRejection
			}
			if workstation.Resources == nil || len(*workstation.Resources) == 0 {
				workstation.Resources = authoredWorkstation.Resources
			}
			if workstation.Cron == nil {
				workstation.Cron = authoredWorkstation.Cron
			}
			if workstation.Guards == nil || len(*workstation.Guards) == 0 {
				workstation.Guards = authoredWorkstation.Guards
			}
			if workstation.Limits == nil {
				workstation.Limits = authoredWorkstation.Limits
			}
			if workstation.Worktree == nil {
				workstation.Worktree = authoredWorkstation.Worktree
			}
			if workstation.WorkingDirectory == nil {
				workstation.WorkingDirectory = authoredWorkstation.WorkingDirectory
			}
			if workstation.PromptFile == nil {
				workstation.PromptFile = authoredWorkstation.PromptFile
			}
			if workstation.PromptTemplate == nil {
				workstation.PromptTemplate = authoredWorkstation.PromptTemplate
			}
			if workstation.StopWords == nil || len(*workstation.StopWords) == 0 {
				workstation.StopWords = authoredWorkstation.StopWords
			}
		}
	}
	return merged
}

func rewriteArtifactFactoryEvents(artifact *interfaces.ReplayArtifact, factory factoryapi.Factory) error {
	if artifact == nil {
		return nil
	}
	for index := range artifact.Events {
		event := &artifact.Events[index]
		switch event.Type {
		case factoryapi.FactoryEventTypeRunRequest:
			payload, err := runStartedPayloadFromEvent(*event)
			if err != nil {
				return err
			}
			payload.Factory = factory
			var union factoryapi.FactoryEvent_Payload
			if err := union.FromRunRequestEventPayload(payload); err != nil {
				return fmt.Errorf("rewrite run request factory payload: %w", err)
			}
			event.Payload = union
		case factoryapi.FactoryEventTypeInitialStructureRequest:
			payload, err := event.Payload.AsInitialStructureRequestEventPayload()
			if err != nil {
				return fmt.Errorf("decode initial structure event %q: %w", event.Id, err)
			}
			payload.Factory = factory
			var union factoryapi.FactoryEvent_Payload
			if err := union.FromInitialStructureRequestEventPayload(payload); err != nil {
				return fmt.Errorf("rewrite initial structure factory payload: %w", err)
			}
			event.Payload = union
		}
	}
	return nil
}
