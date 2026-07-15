package replay

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/work"

	"github.com/portpowered/infinite-you/pkg/config"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factoryresource "github.com/portpowered/infinite-you/pkg/factory/resource"
	factorytoken "github.com/portpowered/infinite-you/pkg/factory/token"
	"github.com/portpowered/infinite-you/pkg/workers"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"
	workerdiagnostics "github.com/portpowered/infinite-you/pkg/workers/diagnostics"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
)

const (
	replayRunStartedEventID  = "factory-event/run-started"
	replayRunFinishedEventID = "factory-event/run-finished"
	replayMetadataReplayKey  = "replayKey"
)

func artifactForStorage(artifact *interfaces.ReplayArtifact) (*interfaces.ReplayArtifact, error) {
	if artifact == nil {
		return nil, fmt.Errorf("replay artifact is required")
	}
	out := *artifact
	out.Events = append([]interfaces.FactoryEvent(nil), artifact.Events...)
	out.Factory = artifact.Factory.Clone()
	assignEventSequences(out.Events)
	if err := canonicalizeRunRequestEventPayloads(&out); err != nil {
		return nil, err
	}
	if err := hydrateArtifactFromEvents(&out); err != nil {
		return nil, err
	}
	return &out, nil
}

func canonicalizeRunRequestEventPayloads(artifact *interfaces.ReplayArtifact) error {
	for index := range artifact.Events {
		event := &artifact.Events[index]
		if event.Type != interfaces.FactoryEventTypeRunRequest {
			continue
		}
		payload, err := runStartedPayloadFromEvent(*event)
		if err != nil {
			return err
		}
		if payload.RecordedAt.IsZero() {
			payload.RecordedAt = artifact.RecordedAt
		}
		if payload.RecordedAt.IsZero() {
			payload.RecordedAt = event.Context.EventTime
		}
		generatedEvent, err := generatedEventFromDomain(*event)
		if err != nil {
			return err
		}
		var union factoryapi.FactoryEvent_Payload
		if err := union.FromRunRequestEventPayload(payload); err != nil {
			return fmt.Errorf("encode run started event payload: %w", err)
		}
		generatedEvent.Payload = union
		converted, err := interfaces.NewFactoryEvent(generatedEvent)
		if err != nil {
			return err
		}
		*event = converted
	}
	return nil
}

// NewEventLogArtifactFromFactory creates a replay artifact shell whose first
// event carries the already-serialized generated Factory config contract.
func NewEventLogArtifactFromFactory(recordedAt time.Time, generatedFactory factoryapi.Factory, wallClock *interfaces.ReplayWallClockMetadata, diagnostics interfaces.ReplayDiagnostics) (*interfaces.ReplayArtifact, error) {
	event, err := runStartedEventFromFactory(recordedAt, generatedFactory, wallClock, diagnostics)
	if err != nil {
		return nil, err
	}
	factorySnapshot, err := interfaces.NewFactorySnapshot(generatedFactory)
	if err != nil {
		return nil, fmt.Errorf("capture replay factory: %w", err)
	}
	artifact := &interfaces.ReplayArtifact{
		SchemaVersion: CurrentSchemaVersion,
		RecordedAt:    recordedAt,
		Events:        []interfaces.FactoryEvent{event},
		Factory:       factorySnapshot,
		WallClock:     wallClock,
		Diagnostics:   diagnostics,
	}
	assignEventSequences(artifact.Events)
	return artifact, nil
}

func runStartedEventFromFactory(recordedAt time.Time, generatedFactory factoryapi.Factory, wallClock *interfaces.ReplayWallClockMetadata, diagnostics interfaces.ReplayDiagnostics) (interfaces.FactoryEvent, error) {
	if recordedAt.IsZero() {
		recordedAt = time.Now().UTC()
	}
	payload := factoryapi.RunRequestEventPayload{
		RecordedAt:  recordedAt,
		Factory:     generatedFactory,
		WallClock:   generatedWallClock(wallClock),
		Diagnostics: generatedDiagnostics(diagnostics),
	}
	var union factoryapi.FactoryEvent_Payload
	if err := union.FromRunRequestEventPayload(payload); err != nil {
		return interfaces.FactoryEvent{}, fmt.Errorf("encode run started event payload: %w", err)
	}
	return interfaces.NewFactoryEvent(factoryapi.FactoryEvent{
		Id:            replayRunStartedEventID,
		SchemaVersion: factoryapi.AgentFactoryEventV1,
		Type:          factoryapi.FactoryEventTypeRunRequest,
		Context: factoryapi.FactoryEventContext{
			EventTime: recordedAt,
			Tick:      0,
		},
		Payload: union,
	})
}

func runFinishedEvent(finishedAt time.Time, wallClock *interfaces.ReplayWallClockMetadata, diagnostics interfaces.ReplayDiagnostics) interfaces.FactoryEvent {
	state := interfaces.FactoryStateCompleted
	payload := interfaces.RunResponseEventPayload{
		State:       &state,
		WallClock:   runEventWallClock(wallClock),
		Diagnostics: replayDiagnosticsPtr(diagnostics),
	}
	encodedPayload, err := json.Marshal(payload)
	if err != nil {
		panic(fmt.Sprintf("encode run finished event payload: %v", err))
	}
	return interfaces.FactoryEvent{
		Id:            replayRunFinishedEventID,
		SchemaVersion: interfaces.FactoryEventSchemaVersionV1,
		Type:          interfaces.FactoryEventTypeRunResponse,
		Context: interfaces.FactoryEventContext{
			EventTime: finishedAt,
		},
		Payload: encodedPayload,
	}
}

func hydrateArtifactFromEvents(artifact *interfaces.ReplayArtifact) error {
	if artifact == nil {
		return fmt.Errorf("replay artifact is required")
	}
	for _, event := range artifact.Events {
		switch event.Type {
		case interfaces.FactoryEventTypeRunRequest:
			payload, err := runStartedPayloadFromEvent(event)
			if err != nil {
				return err
			}
			factorySnapshot, err := interfaces.NewFactorySnapshot(payload.Factory)
			if err != nil {
				return fmt.Errorf("capture run started event %q factory: %w", event.Id, err)
			}
			artifact.Factory = factorySnapshot
			artifact.RecordedAt = payload.RecordedAt
			artifact.WallClock = replayWallClockFromGenerated(payload.WallClock)
			artifact.Diagnostics = replayDiagnosticsFromGenerated(payload.Diagnostics)
		case interfaces.FactoryEventTypeRunResponse:
			var payload interfaces.RunResponseEventPayload
			if err := event.DecodePayload(&payload); err != nil {
				return fmt.Errorf("decode run finished event %q: %w", event.Id, err)
			}
			if wallClock := replayWallClockFromRunEvent(payload.WallClock); wallClock != nil {
				artifact.WallClock = wallClock
			}
			if diagnostics := replayDiagnosticsFromRunEvent(payload.Diagnostics); len(diagnostics.Notes) > 0 || len(diagnostics.Workers) > 0 {
				artifact.Diagnostics = diagnostics
			}
		}
	}
	return nil
}

func runStartedPayloadFromEvent(event interfaces.FactoryEvent) (factoryapi.RunRequestEventPayload, error) {
	generatedEvent, err := generatedEventFromDomain(event)
	if err != nil {
		return factoryapi.RunRequestEventPayload{}, err
	}
	payload, err := generatedEvent.Payload.AsRunRequestEventPayload()
	if err != nil {
		payload, err = decodeRunStartedPayloadEnvelope(event)
		if err != nil {
			return factoryapi.RunRequestEventPayload{}, fmt.Errorf("decode run started event %q: %w", event.Id, err)
		}
	}

	factoryBoundary, err := runStartedFactoryBoundaryFromEvent(event)
	if err != nil {
		return factoryapi.RunRequestEventPayload{}, err
	}
	payload.Factory = factoryBoundary
	return payload, nil
}

func decodeRunStartedPayloadEnvelope(event interfaces.FactoryEvent) (factoryapi.RunRequestEventPayload, error) {
	payloadJSON := event.Payload

	var raw struct {
		RecordedAt  time.Time               `json:"recordedAt"`
		WallClock   *factoryapi.WallClock   `json:"wallClock"`
		Diagnostics *factoryapi.Diagnostics `json:"diagnostics"`
	}
	if err := json.Unmarshal(payloadJSON, &raw); err != nil {
		return factoryapi.RunRequestEventPayload{}, err
	}

	return factoryapi.RunRequestEventPayload{
		RecordedAt:  raw.RecordedAt,
		WallClock:   raw.WallClock,
		Diagnostics: raw.Diagnostics,
	}, nil
}

func runStartedFactoryBoundaryFromEvent(event interfaces.FactoryEvent) (factoryapi.Factory, error) {
	payloadJSON := event.Payload

	var raw struct {
		Factory json.RawMessage `json:"factory"`
	}
	if err := json.Unmarshal(payloadJSON, &raw); err != nil {
		return factoryapi.Factory{}, fmt.Errorf("decode run started event %q payload envelope: %w", event.Id, err)
	}
	if len(raw.Factory) == 0 {
		return factoryapi.Factory{}, fmt.Errorf("run started event %q factory is required", event.Id)
	}

	normalizedFactoryJSON, err := normalizeLegacyReplayFactoryBoundary(raw.Factory)
	if err != nil {
		return factoryapi.Factory{}, fmt.Errorf("normalize run started event %q factory boundary: %w", event.Id, err)
	}

	factoryBoundary, err := config.GeneratedFactoryFromOpenAPIJSON(normalizedFactoryJSON)
	if err != nil {
		return factoryapi.Factory{}, fmt.Errorf("decode run started event %q factory boundary: %w", event.Id, err)
	}
	return factoryBoundary, nil
}

func normalizeLegacyReplayFactoryBoundary(data json.RawMessage) ([]byte, error) {
	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	root, ok := raw.(map[string]any)
	if !ok {
		return []byte(data), nil
	}

	workstations, ok := root["workstations"].([]any)
	if !ok {
		return json.Marshal(root)
	}
	for _, item := range workstations {
		workstation, ok := item.(map[string]any)
		if !ok {
			continue
		}
		normalizeLegacyReplayTransitionRoutes(workstation)
		if definition, ok := workstation["definition"].(map[string]any); ok {
			normalizeLegacyReplayTransitionRoutes(definition)
		}
	}
	return json.Marshal(root)
}

func normalizeLegacyReplayTransitionRoutes(workstation map[string]any) {
	for _, key := range []string{"onContinue", "onFailure", "onRejection"} {
		value, ok := workstation[key]
		if !ok {
			continue
		}
		if route, ok := value.(map[string]any); ok {
			workstation[key] = []any{route}
		}
	}
	if _, hasBody := workstation["body"]; !hasBody {
		if promptTemplate, ok := workstation["promptTemplate"]; ok {
			workstation["body"] = promptTemplate
		}
	}
	delete(workstation, "promptTemplate")
}

func mergeGeneratedWorkers(factory *factoryapi.Factory, runtimeWorkers map[string]workerconfig.Config, workstations []interfaces.FactoryWorkstationConfig) error {
	if len(runtimeWorkers) == 0 {
		return nil
	}
	workers, err := mergeGeneratedEntries(
		generatedWorkerSlice(factory.Workers),
		generatedWorkerIndexes,
		sortedWorkerNames(runtimeWorkers),
		func(name string) (factoryapi.Worker, error) {
			return generatedWorkerFromReplayConfig(name, runtimeWorkers[name], workstations)
		},
		func(worker factoryapi.Worker) string {
			return worker.Name
		},
	)
	if err != nil {
		return err
	}
	factory.Workers = slicePtr(workers)
	return nil
}

func generatedWorkerFromReplayConfig(name string, worker workerconfig.Config, workstations []interfaces.FactoryWorkstationConfig) (factoryapi.Worker, error) {
	generated := generatedWorkerAPIFromConfig(name, worker, workstations)
	if generated.Name == "" {
		generated.Name = name
	}
	return generated, nil
}

func mergeGeneratedWorkstations(factory *factoryapi.Factory, workstationsByName map[string]interfaces.FactoryWorkstationConfig, runtimeWorkers map[string]workerconfig.Config) error {
	if len(workstationsByName) == 0 {
		return nil
	}
	workstations, err := mergeGeneratedEntries(
		generatedWorkstationSlice(factory.Workstations),
		generatedWorkstationIndexes,
		sortedWorkstationNames(workstationsByName),
		func(name string) (factoryapi.Workstation, error) {
			return generatedWorkstationFromReplayConfig(name, workstationsByName[name], runtimeWorkers)
		},
		func(workstation factoryapi.Workstation) string {
			return workstation.Name
		},
	)
	if err != nil {
		return err
	}
	factory.Workstations = slicePtr(workstations)
	return nil
}

func mergeGeneratedEntries[T any](generated []T, indexes func([]T) map[string]int, sortedNames []string, build func(string) (T, error), name func(T) string) ([]T, error) {
	byName := indexes(generated)
	for _, entryName := range sortedNames {
		entry, err := build(entryName)
		if err != nil {
			return nil, err
		}
		if index, ok := byName[name(entry)]; ok {
			generated[index] = entry
			continue
		}
		byName[name(entry)] = len(generated)
		generated = append(generated, entry)
	}
	return generated, nil
}

func generatedWorkstationFromReplayConfig(name string, cfg interfaces.FactoryWorkstationConfig, runtimeWorkers map[string]workerconfig.Config) (factoryapi.Workstation, error) {
	workerType := ""
	if worker, ok := runtimeWorkers[cfg.WorkerTypeName]; ok {
		workerType = worker.Type
	}
	generated := generatedWorkstationAPIFromConfig(name, cfg, workerType)
	preserveGeneratedWorkstationResources(cfg.Resources, &generated)
	if generated.Name == "" {
		generated.Name = name
	}
	if generated.Inputs == nil {
		generated.Inputs = []factoryapi.WorkstationIO{}
	}
	if generated.Outputs == nil {
		generated.Outputs = &[]factoryapi.WorkstationIO{}
	}
	return generated, nil
}

func preserveGeneratedWorkstationResources(resources []factoryresource.Config, target *factoryapi.Workstation) {
	if len(resources) == 0 || target == nil {
		return
	}
	usage := make([]factoryapi.ResourceRequirement, 0, len(resources))
	for _, resource := range resources {
		usage = append(usage, factoryapi.ResourceRequirement{
			Name:     resource.Name,
			Capacity: resource.Capacity,
		})
	}
	target.Resources = slicePtr(usage)
}

func generatedWorkerSlice(workers *[]factoryapi.Worker) []factoryapi.Worker {
	if workers == nil {
		return nil
	}
	return append([]factoryapi.Worker(nil), (*workers)...)
}

func generatedWorkerIndexes(workers []factoryapi.Worker) map[string]int {
	indexes := make(map[string]int, len(workers))
	for i, worker := range workers {
		indexes[worker.Name] = i
	}
	return indexes
}

func sortedWorkerNames(workers map[string]workerconfig.Config) []string {
	names := make([]string, 0, len(workers))
	for name := range workers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func generatedWorkstationSlice(workstations *[]factoryapi.Workstation) []factoryapi.Workstation {
	if workstations == nil {
		return nil
	}
	return append([]factoryapi.Workstation(nil), (*workstations)...)
}

func generatedWorkstationIndexes(workstations []factoryapi.Workstation) map[string]int {
	indexes := make(map[string]int, len(workstations))
	for i, workstation := range workstations {
		indexes[workstation.Name] = i
	}
	return indexes
}

func sortedWorkstationNames(workstations map[string]interfaces.FactoryWorkstationConfig) []string {
	names := make([]string, 0, len(workstations))
	for name := range workstations {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func assignEventSequences(events []interfaces.FactoryEvent) {
	for i := range events {
		events[i].SchemaVersion = interfaces.FactoryEventSchemaVersionV1
		events[i].Context.Sequence = i
		if events[i].Context.EventTime.IsZero() {
			events[i].Context.EventTime = time.Now().UTC()
		}
	}
}

func generatedEventFromDomain(event interfaces.FactoryEvent) (factoryapi.FactoryEvent, error) {
	var generated factoryapi.FactoryEvent
	if err := event.Decode(&generated); err != nil {
		return factoryapi.FactoryEvent{}, fmt.Errorf("decode factory event %q at replay compatibility boundary: %w", event.Id, err)
	}
	return generated, nil
}

func generatedDispatchConsumedWorkRefsFromReplayDispatch(dispatch work.WorkDispatch) []factoryapi.DispatchConsumedWorkRef {
	tokens := workers.WorkDispatchInputTokens(dispatch)
	out := make([]factoryapi.DispatchConsumedWorkRef, 0, len(tokens))
	for _, token := range tokens {
		if token.Color.DataType == factorytoken.DataTypeResource {
			continue
		}
		workID := token.Color.WorkID
		if workID == "" {
			workID = token.ID
		}
		if workID == "" {
			continue
		}
		out = append(out, factoryapi.DispatchConsumedWorkRef{WorkId: workID})
	}
	if len(out) == 0 {
		for _, workID := range dispatch.Execution.WorkIDs {
			if workID == "" {
				continue
			}
			out = append(out, factoryapi.DispatchConsumedWorkRef{WorkId: workID})
		}
	}
	return out
}

func generatedResourcesFromReplayDispatch(dispatch work.WorkDispatch) *[]factoryapi.Resource {
	tokens := workers.WorkDispatchInputTokens(dispatch)
	resources := make([]factoryapi.Resource, 0, len(tokens))
	for _, token := range tokens {
		if token.Color.DataType != factorytoken.DataTypeResource {
			continue
		}
		name := token.Color.WorkTypeID
		if name == "" {
			name = token.Color.Name
		}
		resources = append(resources, factoryapi.Resource{Name: name})
	}
	return slicePtr(resources)
}

func preserveGeneratedResourceUsage(source *interfaces.FactoryConfig, target *factoryapi.Factory) {
	if source == nil || target == nil || target.Workstations == nil {
		return
	}
	byName := make(map[string][]factoryresource.Config, len(source.Workstations))
	for _, workstation := range source.Workstations {
		byName[workstation.Name] = workstation.Resources
	}
	for i := range *target.Workstations {
		resources := byName[(*target.Workstations)[i].Name]
		if len(resources) == 0 {
			continue
		}
		usage := make([]factoryapi.ResourceRequirement, 0, len(resources))
		for _, resource := range resources {
			usage = append(usage, factoryapi.ResourceRequirement{
				Name:     resource.Name,
				Capacity: resource.Capacity,
			})
		}
		(*target.Workstations)[i].Resources = slicePtr(usage)
	}
}

func restoreReplayResourceUsage(source factoryapi.Factory, target *interfaces.FactoryConfig) {
	if target == nil || source.Workstations == nil {
		return
	}
	byName := make(map[string][]factoryresource.Config, len(*source.Workstations))
	for _, workstation := range *source.Workstations {
		if workstation.Resources == nil {
			continue
		}
		resources := make([]factoryresource.Config, 0, len(*workstation.Resources))
		for _, usage := range *workstation.Resources {
			resources = append(resources, factoryresource.Config{
				Name:     usage.Name,
				Capacity: usage.Capacity,
			})
		}
		byName[workstation.Name] = resources
	}
	for i := range target.Workstations {
		if resources := byName[target.Workstations[i].Name]; len(resources) > 0 {
			target.Workstations[i].Resources = resources
		}
	}
}

func generatedDiagnostics(diagnostics interfaces.ReplayDiagnostics) *factoryapi.Diagnostics {
	return &factoryapi.Diagnostics{
		Notes:   slicePtr(diagnostics.Notes),
		Workers: generatedWorkDiagnosticsMapPtr(diagnostics.Workers),
	}
}

func replayDiagnosticsFromGenerated(diagnostics *factoryapi.Diagnostics) interfaces.ReplayDiagnostics {
	if diagnostics == nil {
		return interfaces.ReplayDiagnostics{}
	}
	workers := make(map[string]workerdiagnostics.SafeWorkDiagnostics)
	if diagnostics.Workers != nil {
		for key, value := range *diagnostics.Workers {
			if converted := workerdiagnostics.SafeWorkDiagnosticsFromGenerated(&value); converted != nil {
				workers[key] = *converted
			}
		}
	}
	return interfaces.ReplayDiagnostics{
		Notes:   stringSliceValue(diagnostics.Notes),
		Workers: workers,
	}
}

func replayDiagnosticsPtr(diagnostics interfaces.ReplayDiagnostics) *interfaces.ReplayDiagnostics {
	detached := replayDiagnosticsFromRunEvent(&diagnostics)
	return &detached
}

func replayDiagnosticsFromRunEvent(diagnostics *interfaces.ReplayDiagnostics) interfaces.ReplayDiagnostics {
	if diagnostics == nil {
		return interfaces.ReplayDiagnostics{}
	}
	workers := make(map[string]workerdiagnostics.SafeWorkDiagnostics, len(diagnostics.Workers))
	for key, value := range diagnostics.Workers {
		cloned := workerdiagnostics.CloneSafeWorkDiagnostics(&value)
		if cloned != nil {
			workers[key] = *cloned
		}
	}
	return interfaces.ReplayDiagnostics{
		Notes:   append([]string(nil), diagnostics.Notes...),
		Workers: workers,
	}
}

func generatedWorkDiagnosticsMapPtr(in map[string]workerdiagnostics.SafeWorkDiagnostics) *map[string]factoryapi.SafeWorkDiagnostics {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]factoryapi.SafeWorkDiagnostics, len(in))
	for key, value := range in {
		if converted := workerdiagnostics.GeneratedSafeWorkDiagnostics(&value); converted != nil {
			out[key] = *converted
		}
	}
	return &out
}

func generatedWallClock(wallClock *interfaces.ReplayWallClockMetadata) *factoryapi.WallClock {
	if wallClock == nil {
		return nil
	}
	return &factoryapi.WallClock{
		StartedAt:  timePtrIfNotZero(wallClock.StartedAt),
		FinishedAt: timePtrIfNotZero(wallClock.FinishedAt),
	}
}

func replayWallClockFromGenerated(wallClock *factoryapi.WallClock) *interfaces.ReplayWallClockMetadata {
	if wallClock == nil {
		return nil
	}
	return &interfaces.ReplayWallClockMetadata{
		StartedAt:  timeValue(wallClock.StartedAt),
		FinishedAt: timeValue(wallClock.FinishedAt),
	}
}

func runEventWallClock(wallClock *interfaces.ReplayWallClockMetadata) *interfaces.RunEventWallClock {
	if wallClock == nil {
		return nil
	}
	return &interfaces.RunEventWallClock{
		StartedAt:  timePtrIfNotZero(wallClock.StartedAt),
		FinishedAt: timePtrIfNotZero(wallClock.FinishedAt),
	}
}

func replayWallClockFromRunEvent(wallClock *interfaces.RunEventWallClock) *interfaces.ReplayWallClockMetadata {
	if wallClock == nil {
		return nil
	}
	return &interfaces.ReplayWallClockMetadata{
		StartedAt:  timeValue(wallClock.StartedAt),
		FinishedAt: timeValue(wallClock.FinishedAt),
	}
}

func generatedWorkMetrics(metrics workerexecution.WorkMetrics) *factoryapi.WorkMetrics {
	if metrics.Duration == 0 && metrics.Cost == 0 && metrics.RetryCount == 0 {
		return nil
	}
	return &factoryapi.WorkMetrics{
		DurationMillis: int64PtrIfNonZero(metrics.Duration.Milliseconds()),
		Cost:           float64PtrIfNonZero(metrics.Cost),
		RetryCount:     intPtrIfNonZero(metrics.RetryCount),
	}
}

func generatedStringMapPtr(values map[string]string) *factoryapi.StringMap {
	if len(values) == 0 {
		return nil
	}
	converted := factoryapi.StringMap(cloneStringMap(values))
	return &converted
}

func generatedDispatchRequestMetadata(values map[string]string) *factoryapi.DispatchRequestEventMetadata {
	if len(values) == 0 {
		return nil
	}
	return &factoryapi.DispatchRequestEventMetadata{
		ReplayKey: stringPtrIfNotEmpty(values[replayMetadataReplayKey]),
	}
}

func stringMapValue(values *factoryapi.StringMap) map[string]string {
	if values == nil || len(*values) == 0 {
		return nil
	}
	out := make(map[string]string, len(*values))
	for key, value := range *values {
		out[key] = value
	}
	return out
}

func uniqueNonEmpty(values []string) []string {
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func firstString(values *[]string) string {
	if values == nil || len(*values) == 0 {
		return ""
	}
	return (*values)[0]
}

func stringValue[T ~string](value *T) string {
	if value == nil {
		return ""
	}
	return string(*value)
}

func stringSliceValue(values *[]string) []string {
	if values == nil {
		return nil
	}
	out := make([]string, len(*values))
	copy(out, *values)
	return out
}

func intValue(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func int64Value(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}

func float64Value(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}

func timeValue(value *time.Time) time.Time {
	if value == nil {
		return time.Time{}
	}
	return *value
}

func stringPtrIfNotEmpty[T ~string](value T) *T {
	if value == "" {
		return nil
	}
	return &value
}

func intPtrIfNonZero(value int) *int {
	if value == 0 {
		return nil
	}
	return &value
}

func int64PtrIfNonZero(value int64) *int64 {
	if value == 0 {
		return nil
	}
	return &value
}

func float64PtrIfNonZero(value float64) *float64 {
	if value == 0 {
		return nil
	}
	return &value
}

func timePtrIfNotZero(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	return &value
}

func slicePtr[T any](values []T) *[]T {
	if len(values) == 0 {
		return nil
	}
	out := make([]T, len(values))
	copy(out, values)
	return &out
}
