package run

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntimecli "github.com/portpowered/infinite-you/pkg/services/factory_runtime/transports/cli"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/transports/cli/runconfig"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"go.uber.org/zap"
)

func remoteDurableLifecycleStatus(status *factoryapi.FactorySessionDurableLifecycleStatus) string {
	if status == nil {
		return ""
	}
	return string(*status)
}

func writeRemoteInvocationResult(cfg RunConfig, result apisurface.FactoryInvocationResult) error {
	if cfg.Output == nil {
		cfg.Output = cfg.StartupOutput
	}
	if isResponseStreamOutputMode(cfg.InvocationOutputMode) {
		if cfg.JSONOutput {
			if err := writeRemoteInvocationNDJSON(cfg.Output, result); err != nil {
				return err
			}
			if result.Status != interfaces.InvocationTerminalStatusCompleted {
				return invocationResultFailure(result)
			}
			return nil
		}
		if result.Status != interfaces.InvocationTerminalStatusCompleted {
			if err := writeRemoteInvocationHumanFailure(cfg.Output, result); err != nil {
				return err
			}

			return invocationResultFailure(result)
		}
	}
	if result.Status != interfaces.InvocationTerminalStatusCompleted {
		return writeInvocationFailure(cfg, result, nil)
	}
	return writeInvocationSuccess(cfg, result, nil)
}

type remoteInvocationNDJSONRecord struct {
	RecordType string                        `json:"recordType"`
	Response   factoryapi.InvocationResponse `json:"response"`
}

func writeRemoteInvocationNDJSON(output io.Writer, result apisurface.FactoryInvocationResult) error {
	if output == nil {
		return fmt.Errorf("write remote invocation response stream: process output is required")
	}
	encoded, err := json.Marshal(remoteInvocationNDJSONRecord{
		RecordType: "invocation_result",
		Response:   apisurface.InvocationResponseFromResult(result),
	})
	if err != nil {
		return fmt.Errorf("marshal remote invocation terminal record: %w", err)
	}
	_, err = fmt.Fprintln(output, string(encoded))
	return err
}

func writeRemoteInvocationHumanFailure(output io.Writer, result apisurface.FactoryInvocationResult) error {
	if output == nil {
		return fmt.Errorf("write remote invocation outcome: process output is required")
	}
	if _, err := fmt.Fprintln(output, "--- invocation outcome ---"); err != nil {
		return err
	}
	lines := []string{"status: " + string(result.Status)}
	if code := strings.TrimSpace(result.ErrorCode); code != "" {
		lines = append(lines, "error: "+code)
	}
	if message := strings.TrimSpace(result.Message); message != "" {
		lines = append(lines, "message: "+message)
	}
	if sessionID := strings.TrimSpace(result.SessionID); sessionID != "" {
		lines = append(lines, "session: "+sessionID)
	}
	if workID := strings.TrimSpace(result.WorkID); workID != "" {
		lines = append(lines, "workId: "+workID)
	}
	if workName := strings.TrimSpace(result.WorkName); workName != "" {
		lines = append(lines, "workName: "+workName)
	}
	if workState := strings.TrimSpace(result.WorkState); workState != "" {
		lines = append(lines, "workState: "+workState)
	}
	for _, line := range lines {
		if _, err := fmt.Fprintln(output, line); err != nil {
			return err
		}
	}
	return nil
}

func replayMetadataOutput(cfg RunConfig) io.Writer {
	if cfg.Output != nil {
		return cfg.Output
	}
	if cfg.ReplayMetadataOutput != nil {
		return cfg.ReplayMetadataOutput
	}
	return cfg.StartupOutput
}

func emitReplayMetadataWarnings(
	output io.Writer,
	warnings []recordings.MetadataMismatchWarning,
) error {
	if output == nil || len(warnings) == 0 {
		return nil
	}
	components := replayMetadataWarningComponents(warnings)
	if len(components) == 0 {
		return nil
	}
	if _, err := fmt.Fprintf(
		output,
		"Replay warning: current Factory Definition differs from the recording; affected components: %s. Replay continues with recorded inputs.\n",
		strings.Join(components, ", "),
	); err != nil {
		return fmt.Errorf("write replay drift warning: %w", err)
	}
	return nil
}

func replayMetadataWarningComponents(
	warnings []recordings.MetadataMismatchWarning,
) []string {
	seen := make(map[string]struct{}, len(warnings))
	components := make([]string, 0, len(warnings))
	for _, warning := range warnings {
		component := replayMetadataWarningComponent(warning.Key)
		if component == "" {
			continue
		}
		if _, ok := seen[component]; ok {
			continue
		}
		seen[component] = struct{}{}
		components = append(components, component)
	}
	sort.Strings(components)
	return components
}

func replayMetadataWarningComponent(key string) string {
	switch key {
	case "factory_hash":
		return "Factory Definition"
	case "workers_hash":
		return "workers"
	case "workstations_hash":
		return "workstations"
	case "runtime_config_hash":
		return "runtime configuration"
	default:
		return strings.TrimSpace(key)
	}
}

type cleanInvocationCounterSet struct {
	attempts          atomic.Int64
	successes         atomic.Int64
	failures          atomic.Int64
	ambiguityRejected atomic.Int64
	cancellations     atomic.Int64
}

type CleanInvocationMetricsSnapshot struct {
	Attempts          int64
	Successes         int64
	Failures          int64
	AmbiguityRejected int64
	Cancellations     int64
}

type cleanInvocationCompletionLogInput struct {
	Duration time.Duration
	Result   *apisurface.FactoryInvocationResult
	Err      error
}

var cleanInvocationMetrics cleanInvocationCounterSet

func recordCleanInvocationAttempt() {
	cleanInvocationMetrics.attempts.Add(1)
}

func ObserveInvocationRejection(logger *zap.Logger, err error) {
	var ambiguousErr *AmbiguousInvocationInputError
	if !errors.As(err, &ambiguousErr) {
		return
	}
	recordCleanInvocationAttempt()
	cleanInvocationMetrics.ambiguityRejected.Add(1)
	cleanInvocationLogger(logger).Info(
		cleanInvocationLogMessageRejected,
		zap.String("mode", cleanInvocationModeLabel),
		zap.String("reason", cleanInvocationRejectReason),
		zap.Strings("conflictingSources", invocationInputSourceLogLabels(ambiguousErr.Sources)),
	)
}

func recordCleanInvocationCompletion(logger *zap.Logger, cfg RunConfig, input cleanInvocationCompletionLogInput) {
	logger = cleanInvocationLogger(logger)
	fields := []zap.Field{
		zap.String("mode", cleanInvocationModeLabel),
		zap.String("inputSource", invocationInputSourceLogLabel(cfg.CleanInvocationInputSource)),
		zap.Int64("durationMs", input.Duration.Milliseconds()),
	}

	if input.Result != nil && input.Result.Status == interfaces.InvocationTerminalStatusCompleted && input.Err == nil {
		cleanInvocationMetrics.successes.Add(1)
		fields = append(fields,
			zap.String("outcome", cleanInvocationOutcomeSuccess),
			zap.String("workId", input.Result.WorkID),
			zap.String("workTypeName", input.Result.WorkName),
		)
		if strings.TrimSpace(input.Result.TraceID) != "" {
			fields = append(fields, zap.String("traceId", input.Result.TraceID))
		}
		if strings.TrimSpace(input.Result.SessionID) != "" {
			fields = append(fields, zap.String("sessionId", input.Result.SessionID))
		}
		logger.Info(cleanInvocationLogMessageCompleted, fields...)
		return
	}
	if input.Result != nil {
		if strings.TrimSpace(input.Result.WorkID) != "" {
			fields = append(fields, zap.String("workId", input.Result.WorkID))
		}
		if strings.TrimSpace(input.Result.WorkName) != "" {
			fields = append(fields, zap.String("workTypeName", input.Result.WorkName))
		}
	}

	outcome, code, summary := cleanInvocationFailureLogFields(input.Err)
	switch outcome {
	case cleanInvocationOutcomeCancelled:
		cleanInvocationMetrics.cancellations.Add(1)
	case cleanInvocationOutcomeFailure, cleanInvocationOutcomeTimeout:
		cleanInvocationMetrics.failures.Add(1)
	}
	fields = append(fields,
		zap.String("outcome", outcome),
		zap.String("errorCode", code),
	)
	if summary != "" {
		fields = append(fields, zap.String("errorSummary", summary))
	}
	logger.Info(cleanInvocationLogMessageCompleted, fields...)
}

func cleanInvocationFailureLogFields(err error) (string, string, string) {
	var invocationErr *InvocationError
	if errors.As(err, &invocationErr) {
		return cleanInvocationFailureLogFieldsForCode(invocationErr.Code, invocationErr.Message)
	}
	var cliErr factoryruntimecli.InvocationCLIError
	if errors.As(err, &cliErr) {
		return cleanInvocationFailureLogFieldsForCode(cliErr.InvocationErrorCode(), cliErr.InvocationErrorMessage())
	}
	summary := boundedInvocationErrorSummary(errString(err))
	if summary == "" {
		return cleanInvocationOutcomeFailure, InvocationErrorCodeFailed, ""
	}
	return cleanInvocationOutcomeFailure, InvocationErrorCodeFailed, summary
}

func cleanInvocationFailureLogFieldsForCode(code, message string) (string, string, string) {
	switch code {
	case InvocationErrorCodeCancelled:
		return cleanInvocationOutcomeCancelled, code, boundedInvocationErrorSummary(message)
	case InvocationErrorCodeTimeout:
		return cleanInvocationOutcomeTimeout, code, boundedInvocationErrorSummary(message)
	default:
		return cleanInvocationOutcomeFailure, code, boundedInvocationErrorSummary(message)
	}
}

func cleanInvocationLogger(logger *zap.Logger) *zap.Logger {
	if logger == nil {
		return zap.NewNop()
	}
	return logger
}

func invocationInputSourceLogLabels(sources []InvocationInputSource) []string {
	labels := make([]string, 0, len(sources))
	for _, source := range sources {
		labels = append(labels, invocationInputSourceLogLabel(source))
	}
	return labels
}

func invocationInputSourceLogLabel(source InvocationInputSource) string {
	switch source {
	case InvocationInputSourcePositional:
		return "positional_prompt"
	case InvocationInputSourceStdin:
		return "stdin"
	case InvocationInputSourceWorkFile:
		return "work_file"
	default:
		return "unknown"
	}
}

func boundedInvocationErrorSummary(message string) string {
	message = strings.Join(strings.Fields(strings.TrimSpace(message)), " ")
	if len(message) <= cleanInvocationErrorSummaryLimit {
		return message
	}
	return message[:cleanInvocationErrorSummaryLimit] + "..."
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func snapshotCleanInvocationMetrics() CleanInvocationMetricsSnapshot {
	return CleanInvocationMetricsSnapshot{
		Attempts:          cleanInvocationMetrics.attempts.Load(),
		Successes:         cleanInvocationMetrics.successes.Load(),
		Failures:          cleanInvocationMetrics.failures.Load(),
		AmbiguityRejected: cleanInvocationMetrics.ambiguityRejected.Load(),
		Cancellations:     cleanInvocationMetrics.cancellations.Load(),
	}
}

func resetCleanInvocationMetricsForTest() {
	cleanInvocationMetrics.attempts.Store(0)
	cleanInvocationMetrics.successes.Store(0)
	cleanInvocationMetrics.failures.Store(0)
	cleanInvocationMetrics.ambiguityRejected.Store(0)
	cleanInvocationMetrics.cancellations.Store(0)
}

func recordCLIInvocationResolved(cfg RunConfig, source work.InputSourceLabel) {
	logger := cfg.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	logger.Info("factory invocation input resolved", zap.String("input_source", string(source)))
}

func recordCLIInvocationFailure(cfg RunConfig, err error) {
	logger := cfg.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	inputErr, ok := err.(*work.InputError)
	if !ok {
		return
	}
	if inputErr.Code == work.InputErrorCodeSourceConflict {
		recordInvocationMetric(cfg.InvocationMetricsRecorder, factorysessions.InvocationMetric{
			Name: "invocation.source_conflict",
			Labels: map[string]string{
				"input_source": "conflict",
			},
		})
		recordInvocationMetric(cfg.InvocationMetricsRecorder, factorysessions.InvocationMetric{
			Name: "invocation.failure",
			Labels: map[string]string{
				"input_source": "conflict",
			},
		})
		logger.Warn(
			"factory invocation input resolution failed",
			zap.String("failure_class", "source_conflict"),
			zap.Strings("conflicting_sources", invocationSourceLabels(inputErr.ConflictingSources)),
			zap.String("error_code", string(inputErr.Code)),
		)
		return
	}
	logger.Warn(
		"factory invocation input resolution failed",
		zap.String("failure_class", "input_invalid"),
		zap.String("error_code", string(inputErr.Code)),
	)
}

func recordInvocationMetric(
	recorder runconfig.InvocationMetricsRecorder,
	metric factorysessions.InvocationMetric,
) {
	if recorder == nil {
		return
	}
	recorder.RecordInvocationMetric(metric)
}

func invocationSourceLabels(labels []work.InputSourceLabel) []string {
	if len(labels) == 0 {
		return nil
	}
	out := make([]string, 0, len(labels))
	for _, label := range labels {
		out = append(out, string(label))
	}
	return out
}

func emitHistoricalReplayInspection(
	output io.Writer,
	inspection factorysessions.HistoricalReplayInspection,
) error {
	if output == nil {
		return nil
	}
	if _, err := fmt.Fprintf(
		output,
		"Replayed Factory Session: %s\nSource: %s\nStatus: %s\nResult: %s\n",
		inspection.Session.SessionID,
		inspection.Session.ResolvedSource.SourceRef,
		inspection.Session.Status,
		inspection.Result.ResultStatus,
	); err != nil {
		return fmt.Errorf("write historical replay inspection: %w", err)
	}
	if _, err := fmt.Fprintf(
		output,
		"Worker history: %s (reason=%s)\n",
		inspection.WorkerHistory.Availability,
		inspection.WorkerHistory.Reason,
	); err != nil {
		return fmt.Errorf("write historical replay inspection: %w", err)
	}
	factoryProjection := normalizedHistoricalReplayFactoryProjection(inspection.FactoryProjection)
	if _, err := fmt.Fprintf(
		output,
		"Factory projection: %s (reason=%s)\n",
		factoryProjection.Availability,
		factoryProjection.Reason,
	); err != nil {
		return fmt.Errorf("write historical replay inspection: %w", err)
	}
	controlStatus, terminal, finalStatus := historicalReplayLifecycle(inspection)
	if _, err := fmt.Fprintf(
		output,
		"Session lifecycle: control=%s terminal=%t final=%s\n",
		quoteHistoricalReplayValue(controlStatus),
		terminal,
		quoteHistoricalReplayValue(finalStatus),
	); err != nil {
		return fmt.Errorf("write historical replay inspection: %w", err)
	}
	if inspection.Checkpoint != nil {
		if _, err := fmt.Fprintf(
			output,
			"Checkpoint: %s (%s)\n",
			inspection.Checkpoint.ID,
			inspection.Checkpoint.Summary,
		); err != nil {
			return fmt.Errorf("write historical replay inspection: %w", err)
		}
	}
	if _, err := fmt.Fprintf(
		output,
		"Artifacts: %d\nEvents: %d\nRedaction: runtimeStateOmitted=%t checkpointBodiesOmitted=%t providerTranscriptsOmitted=%t childDispatchesOmitted=%t secretsRedacted=%d\n",
		len(inspection.Artifacts.Artifacts),
		len(inspection.Events.Events),
		inspection.Redaction.RuntimeStateOmitted,
		inspection.Redaction.CheckpointBodiesOmitted,
		inspection.Redaction.ProviderTranscriptsOmitted,
		inspection.Redaction.ChildDispatchesOmitted,
		inspection.Redaction.SecretsRedacted,
	); err != nil {
		return fmt.Errorf("write historical replay inspection: %w", err)
	}
	for _, artifact := range inspection.Artifacts.Artifacts {
		if _, err := fmt.Fprintf(output, "Artifact: %s (%s)\n", artifact.ID, artifact.Kind); err != nil {
			return fmt.Errorf("write historical replay inspection: %w", err)
		}
	}
	if err := emitHistoricalReplayFactoryFacts(output, inspection); err != nil {
		return err
	}
	for index, event := range inspection.Events.Events {
		var summary struct {
			ID   string `json:"id"`
			Type string `json:"type"`
		}
		if err := json.Unmarshal(event, &summary); err != nil {
			return fmt.Errorf("write historical replay inspection: decode event %d: %w", index, err)
		}
		if _, err := fmt.Fprintf(output, "Event %d: %s (%s)\n", index, summary.Type, summary.ID); err != nil {
			return fmt.Errorf("write historical replay inspection: %w", err)
		}
	}
	return nil
}

func normalizedHistoricalReplayFactoryProjection(
	projection factorysessions.HistoricalReplayFactoryProjection,
) factorysessions.HistoricalReplayFactoryProjection {
	if projection.Availability != "" {
		return projection
	}
	projection.Availability = factorysessions.HistoricalReplayFactoryProjectionUnavailable
	projection.Reason = factorysessions.HistoricalReplayFactoryProjectionReasonNotRecorded
	projection.State = nil
	return projection
}

func quoteHistoricalReplayValue(value string) string {
	return strconv.Quote(value)
}

func historicalReplayLifecycle(
	inspection factorysessions.HistoricalReplayInspection,
) (control string, terminal bool, final string) {
	projection := normalizedHistoricalReplayFactoryProjection(inspection.FactoryProjection)
	if projection.State != nil && projection.State.SessionBracket != nil {
		bracket := projection.State.SessionBracket
		return bracket.LifecycleControlStatus, bracket.Terminal, bracket.FinalStatus
	}
	status := inspection.Session.Status
	switch status {
	case factorysessions.LifecycleStatusSucceeded,
		factorysessions.LifecycleStatusFailed,
		factorysessions.LifecycleStatusCanceled,
		factorysessions.LifecycleStatusTimedOut,
		factorysessions.LifecycleStatusInterrupted,
		factorysessions.LifecycleStatusTerminated:
		return "", true, string(status)
	default:
		return "", false, ""
	}
}

func emitHistoricalReplayFactoryFacts(
	output io.Writer,
	inspection factorysessions.HistoricalReplayInspection,
) error {
	projection := normalizedHistoricalReplayFactoryProjection(inspection.FactoryProjection)
	if projection.State == nil {
		return nil
	}
	state := projection.State
	knownIDs, hasCanonicalWorkEvidence := historicalReplayKnownWorkIDs(*state)
	for _, item := range historicalReplayWorkItems(*state, knownIDs) {
		if _, err := fmt.Fprintf(
			output,
			"Work: id=%s type=%s state=%s trace=%s parent=%s\n",
			quoteHistoricalReplayValue(item.ID),
			quoteHistoricalReplayValue(item.WorkTypeID),
			quoteHistoricalReplayValue(item.State),
			quoteHistoricalReplayValue(item.TraceID),
			quoteHistoricalReplayValue(item.ParentID),
		); err != nil {
			return fmt.Errorf("write historical replay inspection: %w", err)
		}
		previous := stableHistoricalReplayTraceIDs(item.PreviousChainingTraceIDs)
		if _, err := fmt.Fprintf(
			output,
			"Lineage: work=%s current=%s previous=%s\n",
			quoteHistoricalReplayValue(item.ID),
			quoteHistoricalReplayValue(item.CurrentChainingTraceID),
			quoteHistoricalReplayValue(strings.Join(previous, ",")),
		); err != nil {
			return fmt.Errorf("write historical replay inspection: %w", err)
		}
	}
	for _, relation := range historicalReplayRelations(*state, knownIDs, hasCanonicalWorkEvidence) {
		if _, err := fmt.Fprintf(
			output,
			"Relation: source=%s type=%s target=%s required=%s request=%s trace=%s\n",
			quoteHistoricalReplayValue(relation.SourceWorkID),
			quoteHistoricalReplayValue(relation.Type),
			quoteHistoricalReplayValue(relation.TargetWorkID),
			quoteHistoricalReplayValue(relation.RequiredState),
			quoteHistoricalReplayValue(relation.RequestID),
			quoteHistoricalReplayValue(relation.TraceID),
		); err != nil {
			return fmt.Errorf("write historical replay inspection: %w", err)
		}
	}
	for _, failure := range historicalReplayFailures(*state, knownIDs, hasCanonicalWorkEvidence) {
		reason, message := "", ""
		if failure.FailureDetail != nil {
			reason = string(failure.FailureDetail.Reason)
			message = failure.FailureDetail.Message
		}
		if reason == "" && failure.ArtifactVerification != nil {
			reason = string(failure.ArtifactVerification.Code)
		}
		if message == "" && failure.ArtifactVerification != nil {
			message = "expected artifact verification failed"
		}
		if _, err := fmt.Fprintf(
			output,
			"Failure: work=%s dispatch=%s reason=%s message=%s\n",
			quoteHistoricalReplayValue(failure.WorkItem.ID),
			quoteHistoricalReplayValue(failure.DispatchID),
			quoteHistoricalReplayValue(reason),
			quoteHistoricalReplayValue(message),
		); err != nil {
			return fmt.Errorf("write historical replay inspection: %w", err)
		}
	}
	return nil
}

func historicalReplayKnownWorkIDs(
	state recordings.FactoryWorldState,
) (map[string]struct{}, bool) {
	known := make(map[string]struct{})
	hasEvidence := addHistoricalReplayRequestWorkIDs(known, state)
	if addHistoricalReplayLineageWorkIDs(known, state) {
		hasEvidence = true
	}
	if hasEvidence {
		return known, true
	}
	addHistoricalReplayMapWorkIDs(known, state.WorkItemsByID)
	addHistoricalReplayMapWorkIDs(known, state.ActiveWorkItemsByID)
	addHistoricalReplayMapWorkIDs(known, state.FailedWorkItemsByID)
	for id, terminal := range state.TerminalWorkByID {
		if normalizedID := strings.TrimSpace(id); normalizedID != "" {
			known[normalizedID] = struct{}{}
			continue
		}
		if itemID := strings.TrimSpace(terminal.WorkItem.ID); itemID != "" {
			known[itemID] = struct{}{}
		}
	}
	return known, false
}

func addHistoricalReplayRequestWorkIDs(
	known map[string]struct{},
	state recordings.FactoryWorldState,
) bool {
	hasEvidence := false
	for _, request := range state.WorkRequestsByID {
		for _, item := range request.WorkItems {
			if id := strings.TrimSpace(item.ID); id != "" {
				known[id] = struct{}{}
				hasEvidence = true
			}
		}
	}
	return hasEvidence
}

func addHistoricalReplayLineageWorkIDs(
	known map[string]struct{},
	state recordings.FactoryWorldState,
) bool {
	hasEvidence := false
	for _, snapshot := range state.PayloadLineage.SnapshotsByID {
		if snapshot.SourceKind != work.WorkPayloadSnapshotKindWorkRequest &&
			snapshot.SourceKind != work.WorkPayloadSnapshotKindDispatchOutput {
			continue
		}
		id := strings.TrimSpace(snapshot.WorkID)
		if id == "" {
			id = strings.TrimSpace(snapshot.WorkItem.ID)
		}
		if id != "" {
			known[id] = struct{}{}
			hasEvidence = true
		}
	}
	return hasEvidence
}

func addHistoricalReplayMapWorkIDs(
	known map[string]struct{},
	items map[string]work.FactoryWorkItem,
) {
	for id := range items {
		if normalizedID := strings.TrimSpace(id); normalizedID != "" {
			known[normalizedID] = struct{}{}
		}
	}
}

func historicalReplayWorkItems(
	state recordings.FactoryWorldState,
	known map[string]struct{},
) []work.FactoryWorkItem {
	ids := make([]string, 0, len(known))
	for id := range known {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	items := make([]work.FactoryWorkItem, 0, len(ids))
	for _, id := range ids {
		item, ok := historicalReplayWorkItem(state, id)
		if !ok {
			continue
		}
		if item.ID == "" {
			item.ID = id
		}
		items = append(items, item)
	}
	return items
}

func historicalReplayWorkItem(
	state recordings.FactoryWorldState,
	id string,
) (work.FactoryWorkItem, bool) {
	for _, items := range []map[string]work.FactoryWorkItem{
		state.WorkItemsByID,
		state.ActiveWorkItemsByID,
		state.FailedWorkItemsByID,
	} {
		if item, ok := items[id]; ok {
			return item, true
		}
	}
	if terminal, ok := state.TerminalWorkByID[id]; ok {
		return terminal.WorkItem, true
	}
	if snapshotID := state.PayloadLineage.LatestSnapshotIDByWorkID[id]; snapshotID != "" {
		if snapshot, ok := state.PayloadLineage.SnapshotsByID[snapshotID]; ok {
			return snapshot.WorkItem, true
		}
	}
	for _, request := range state.WorkRequestsByID {
		for _, item := range request.WorkItems {
			if item.ID == id {
				return item, true
			}
		}
	}
	return work.FactoryWorkItem{}, false
}

func historicalReplayRelations(
	state recordings.FactoryWorldState,
	known map[string]struct{},
	hasEvidence bool,
) []work.FactoryRelation {
	relations := make([]work.FactoryRelation, 0)
	keys := make([]string, 0, len(state.RelationsByWorkID))
	for key := range state.RelationsByWorkID {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		for _, relation := range state.RelationsByWorkID[key] {
			if relation.SourceWorkID == "" {
				relation.SourceWorkID = key
			}
			if hasEvidence && !historicalReplayRelationUsesKnownWork(known, relation) {
				continue
			}
			relations = append(relations, relation)
		}
	}
	sort.SliceStable(relations, func(left, right int) bool {
		l, r := relations[left], relations[right]
		return historicalReplayRelationKey(l) < historicalReplayRelationKey(r)
	})
	return relations
}

func historicalReplayRelationUsesKnownWork(
	known map[string]struct{},
	relation work.FactoryRelation,
) bool {
	if _, ok := known[relation.SourceWorkID]; !ok {
		return false
	}
	if relation.TargetWorkID == "" {
		return true
	}
	_, ok := known[relation.TargetWorkID]
	return ok
}

func historicalReplayRelationKey(relation work.FactoryRelation) string {
	return strings.Join([]string{
		relation.SourceWorkID, relation.Type, relation.TargetWorkID,
		relation.RequiredState, relation.RequestID, relation.TraceID,
	}, "\x00")
}

func historicalReplayFailures(
	state recordings.FactoryWorldState,
	known map[string]struct{},
	hasEvidence bool,
) []recordings.FactoryWorldFailureDetail {
	keys := make([]string, 0, len(state.FailureDetailsByWorkID))
	for key := range state.FailureDetailsByWorkID {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	failures := make([]recordings.FactoryWorldFailureDetail, 0, len(keys))
	for _, key := range keys {
		failure := state.FailureDetailsByWorkID[key]
		id := strings.TrimSpace(key)
		if id == "" {
			id = strings.TrimSpace(failure.WorkItem.ID)
		}
		if failure.WorkItem.ID == "" {
			failure.WorkItem.ID = id
		}
		if hasEvidence {
			if _, ok := known[id]; !ok {
				continue
			}
		}
		failures = append(failures, failure)
	}
	sort.SliceStable(failures, func(left, right int) bool {
		l, r := failures[left], failures[right]
		return strings.Join([]string{l.WorkItem.ID, l.DispatchID, l.TransitionID}, "\x00") <
			strings.Join([]string{r.WorkItem.ID, r.DispatchID, r.TransitionID}, "\x00")
	})
	return failures
}

func stableHistoricalReplayTraceIDs(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
