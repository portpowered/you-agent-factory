package run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
	"github.com/portpowered/infinite-you/pkg/factorysessions/responseevents"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/invocations"
	"github.com/portpowered/infinite-you/pkg/packagedfactories/tts"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/service"
	"go.uber.org/zap"
)

const (
	cleanInvocationLogMessageCompleted = "run.invocation.completed"
	cleanInvocationLogMessageRejected  = "run.invocation.rejected"
	cleanInvocationModeLabel           = "clean"
	cleanInvocationRejectReason        = "ambiguous_input"
	cleanInvocationOutcomeSuccess      = "success"
	cleanInvocationOutcomeFailure      = "failure"
	cleanInvocationOutcomeCancelled    = "cancelled"
	cleanInvocationOutcomeTimeout      = "timeout"
	cleanInvocationErrorSummaryLimit   = 160
)

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
	StartedAt time.Time
	Snapshot  *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]
	Target    *cleanInvocationWorkTarget
	Success   *cleanInvocationSuccess
	Err       error
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
	duration := time.Since(input.StartedAt)
	if input.StartedAt.IsZero() {
		duration = 0
	}

	fields := []zap.Field{
		zap.String("mode", cleanInvocationModeLabel),
		zap.String("inputSource", invocationInputSourceLogLabel(cfg.CleanInvocationInputSource)),
		zap.Int64("durationMs", duration.Milliseconds()),
	}

	if input.Success != nil {
		cleanInvocationMetrics.successes.Add(1)
		fields = append(fields,
			zap.String("outcome", cleanInvocationOutcomeSuccess),
			zap.String("workId", input.Success.WorkID),
			zap.String("workTypeName", input.Success.WorkTypeName),
		)
		if strings.TrimSpace(input.Success.TraceID) != "" {
			fields = append(fields, zap.String("traceId", input.Success.TraceID))
		}
		if strings.TrimSpace(input.Success.SessionID) != "" {
			fields = append(fields, zap.String("sessionId", input.Success.SessionID))
		}
		logger.Info(cleanInvocationLogMessageCompleted, fields...)
		return
	}

	if input.Target != nil {
		fields = append(fields,
			zap.String("workId", input.Target.WorkID),
			zap.String("workTypeName", input.Target.WorkTypeName),
		)
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
		switch invocationErr.Code {
		case InvocationErrorCodeCancelled:
			return cleanInvocationOutcomeCancelled, invocationErr.Code, boundedInvocationErrorSummary(invocationErr.Message)
		case InvocationErrorCodeTimeout:
			return cleanInvocationOutcomeTimeout, invocationErr.Code, boundedInvocationErrorSummary(invocationErr.Message)
		default:
			return cleanInvocationOutcomeFailure, invocationErr.Code, boundedInvocationErrorSummary(invocationErr.Message)
		}
	}
	summary := boundedInvocationErrorSummary(errString(err))
	if summary == "" {
		return cleanInvocationOutcomeFailure, InvocationErrorCodeFailed, ""
	}
	return cleanInvocationOutcomeFailure, InvocationErrorCodeFailed, summary
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

func recordCLIInvocationResolved(cfg RunConfig, source invocations.InputSourceLabel) {
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
	inputErr, ok := err.(*invocations.InputError)
	if !ok {
		return
	}
	if inputErr.Code == invocations.InputErrorCodeSourceConflict {
		recordInvocationMetric(cfg.InvocationMetricsRecorder, service.InvocationMetric{
			Name: "invocation.source_conflict",
			Labels: map[string]string{
				"input_source": "conflict",
			},
		})
		recordInvocationMetric(cfg.InvocationMetricsRecorder, service.InvocationMetric{
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

func recordInvocationMetric(recorder service.InvocationMetricsRecorder, metric service.InvocationMetric) {
	if recorder == nil {
		return
	}
	recorder.RecordInvocationMetric(metric)
}

func invocationSourceLabels(labels []invocations.InputSourceLabel) []string {
	if len(labels) == 0 {
		return nil
	}
	out := make([]string, 0, len(labels))
	for _, label := range labels {
		out = append(out, string(label))
	}
	return out
}

type sessionResponseStreamAttachable interface {
	SubscribeSessionResponseStream(
		sessionID string,
		dispatchID string,
		afterSequence int64,
	) (*factorysessions.SessionResponseStreamSubscription, error)
	SessionResponseStreamDispatchIDs(sessionID string) ([]string, error)
}

type responseStreamEventSink interface {
	onStreamSegment(factorysessions.SessionResponseStreamReadResult)
}

// onResponseEvents consumes validated, session-ordered FactoryResponseEvent
// values. Rendering policy is selected only from canonical kind, phase, and
// typed payload fields; provider-native names and raw payload fallbacks are not
// presentation inputs.
func (r *humanResponseStreamRenderer) onResponseEvents(events []responseevents.FactoryResponseEvent) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, event := range events {
		if event.Sequence > 0 && event.Sequence <= r.lastResponseSequence {
			continue
		}
		if event.Sequence > 0 {
			r.lastResponseSequence = event.Sequence
		}
		r.renderResponseEventLocked(event)
	}
}

func (r *humanResponseStreamRenderer) renderResponseEventLocked(event responseevents.FactoryResponseEvent) {
	if line, ok := formatHumanResponseEvent(event); ok {
		r.writeProgressLineLocked(line)
	}
}

func formatHumanResponseEvent(event responseevents.FactoryResponseEvent) (string, bool) {
	if err := responseevents.ValidateEvent(event); err != nil {
		return "", false
	}

	var line string
	var ok bool
	switch event.Kind {
	case responseevents.KindReasoning:
		line, ok = formatHumanReasoningEvent(event)
	case responseevents.KindTool:
		line, ok = formatHumanToolEvent(event)
	case responseevents.KindError:
		line, ok = formatHumanRetryEvent(event)
	case responseevents.KindProgress:
		line, ok = formatHumanProgressEvent(event)
	case responseevents.KindStreamGap:
		line, ok = formatHumanStreamGapEvent(event)
	default:
		return "", false
	}
	if !ok {
		return "", false
	}
	line = boundedHumanProgressPayload(line)
	return line, line != ""
}

func formatHumanToolEvent(event responseevents.FactoryResponseEvent) (string, bool) {
	status, ok := map[responseevents.Phase]string{
		responseevents.PhaseStarted:   "started",
		responseevents.PhaseCompleted: "completed",
		responseevents.PhaseFailed:    "failed",
		responseevents.PhaseCanceled:  "canceled",
	}[event.Phase]
	if !ok {
		return "", false
	}
	var payload responseevents.ToolPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return "", false
	}
	name := normalizeHumanProgressField(payload.ToolName)
	callID := normalizeHumanProgressField(payload.ToolCallID)
	if name == "" || callID == "" {
		return "", false
	}
	return "tool: name=" + name + " call=" + callID + " status=" + status, true
}

func formatHumanReasoningEvent(event responseevents.FactoryResponseEvent) (string, bool) {
	var payload responseevents.ReasoningPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return "", false
	}
	switch event.Phase {
	case responseevents.PhaseStarted:
		return "reasoning: started", true
	case responseevents.PhaseDelta:
		if summary := normalizeHumanProgressField(payload.SummaryDelta); summary != "" {
			return "reasoning: " + summary, true
		}
	case responseevents.PhaseCompleted:
		if summary := normalizeHumanProgressField(payload.Summary); summary != "" {
			return "reasoning: " + summary, true
		}
		return "reasoning: completed", true
	}
	return "", false
}

func formatHumanRetryEvent(event responseevents.FactoryResponseEvent) (string, bool) {
	if event.Phase != responseevents.PhaseUpdated {
		return "", false
	}
	var payload responseevents.ErrorPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil || !humanRetryStatus(payload) {
		return "", false
	}
	code := normalizeHumanProgressField(payload.Code)
	if code == "" {
		return "", false
	}
	parts := []string{"retry: code=" + code}
	if payload.RetryAttempt != nil {
		parts = append(parts, "attempt="+strconv.Itoa(*payload.RetryAttempt))
	}
	if payload.RetryAfterSeconds != nil {
		parts = append(parts, "retry-in="+strconv.FormatInt(*payload.RetryAfterSeconds, 10)+"s")
	}
	return strings.Join(parts, " "), true
}

func humanRetryStatus(payload responseevents.ErrorPayload) bool {
	if payload.Retryable || payload.RetryAfterSeconds != nil || payload.RetryAttempt != nil {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(payload.Code)) {
	case "rate_limited", "throttled", "too_many_requests":
		return true
	default:
		return false
	}
}

func formatHumanProgressEvent(event responseevents.FactoryResponseEvent) (string, bool) {
	if event.Phase != responseevents.PhaseUpdated {
		return "", false
	}
	var payload responseevents.ProgressPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return "", false
	}
	label := normalizeHumanProgressField(payload.Label)
	if label == "" {
		return "", false
	}
	line := "progress: " + label
	if message := normalizeHumanProgressField(payload.Message); message != "" {
		line += " — " + message
	}
	if payload.PercentComplete != nil && !math.IsNaN(*payload.PercentComplete) &&
		!math.IsInf(*payload.PercentComplete, 0) && *payload.PercentComplete >= 0 && *payload.PercentComplete <= 100 {
		line += " (" + strconv.FormatFloat(*payload.PercentComplete, 'f', -1, 64) + "%)"
	}
	return line, true
}

func formatHumanStreamGapEvent(event responseevents.FactoryResponseEvent) (string, bool) {
	if event.Phase != responseevents.PhaseUpdated {
		return "", false
	}
	var payload responseevents.StreamGapPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return "", false
	}
	line := fmt.Sprintf(
		"stream gap: sequences %d-%d unavailable",
		payload.FromSequence,
		payload.ToSequence,
	)
	if reason := normalizeHumanProgressField(payload.Reason); reason != "" {
		line += " (reason=" + reason + ")"
	}
	return line, true
}

type responseStreamAttachment struct {
	cancel    context.CancelFunc
	done      chan struct{}
	consumers sync.WaitGroup
}

func startResponseStreamAttachment(
	ctx context.Context,
	attachable sessionResponseStreamAttachable,
	sessionID string,
	sink responseStreamEventSink,
) *responseStreamAttachment {
	if attachable == nil || sink == nil {
		return nil
	}
	attachCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	attachment := &responseStreamAttachment{
		cancel: cancel,
		done:   done,
	}
	go func() {
		defer close(done)
		runResponseStreamAttachment(attachCtx, attachable, sessionID, sink, &attachment.consumers)
	}()
	return attachment
}

func (a *responseStreamAttachment) stop() {
	if a == nil {
		return
	}
	a.cancel()
	<-a.done
	a.consumers.Wait()
}

func runResponseStreamAttachment(
	ctx context.Context,
	attachable sessionResponseStreamAttachable,
	sessionID string,
	sink responseStreamEventSink,
	consumers *sync.WaitGroup,
) {
	subscribed := make(map[string]*factorysessions.SessionResponseStreamSubscription)
	var subscribedMu sync.Mutex

	discover := func() {
		dispatchIDs, _ := attachable.SessionResponseStreamDispatchIDs(sessionID)
		for _, dispatchID := range dispatchIDs {
			subscribedMu.Lock()
			_, alreadySubscribed := subscribed[dispatchID]
			subscribedMu.Unlock()
			if alreadySubscribed {
				continue
			}

			subscription, err := attachable.SubscribeSessionResponseStream(sessionID, dispatchID, 0)
			if err != nil {
				continue
			}

			subscribedMu.Lock()
			subscribed[dispatchID] = subscription
			subscribedMu.Unlock()
			consumers.Add(1)
			go func() {
				defer consumers.Done()
				consumeResponseStreamSubscription(ctx, subscription, sink)
			}()
		}
	}

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		discover()

		select {
		case <-ctx.Done():
			discover()
			return
		case <-ticker.C:
		}
	}
}

func consumeResponseStreamSubscription(
	ctx context.Context,
	subscription *factorysessions.SessionResponseStreamSubscription,
	sink responseStreamEventSink,
) {
	defer subscription.Detach()
	for {
		result, err := subscription.Next(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				drainResponseStreamSubscription(subscription, sink)
			}
			return
		}
		sink.onStreamSegment(result)
	}
}

func drainResponseStreamSubscription(
	subscription *factorysessions.SessionResponseStreamSubscription,
	sink responseStreamEventSink,
) {
	drainCtx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	for {
		result, err := subscription.Next(drainCtx)
		if err != nil {
			return
		}
		if len(result.Events) == 0 && !result.BehindRetainedWindow && result.Compaction == nil {
			return
		}
		sink.onStreamSegment(result)
	}
}

const (
	defaultResponseStreamProgressQueueCapacity = 64
	responseStreamProgressDrainTimeout         = 250 * time.Millisecond
)

// responseStreamProgressWriter decouples internal stream consumption from
// terminal stdout writes so a slow or blocked consumer does not stall provider
// dispatch or invocation completion indefinitely.
type responseStreamProgressWriter struct {
	mu            sync.Mutex
	outputMu      sync.Mutex
	output        io.Writer
	queue         chan []byte
	wg            sync.WaitGroup
	closed        bool
	drainTimedOut bool
	droppedLines  int
	pendingNotice []byte
}

func newResponseStreamProgressWriter(output io.Writer) *responseStreamProgressWriter {
	if output == nil {
		panic("response stream progress writer output is nil")
	}
	writer := &responseStreamProgressWriter{
		output: output,
		queue:  make(chan []byte, defaultResponseStreamProgressQueueCapacity),
	}
	writer.wg.Add(1)
	go writer.run()
	return writer
}

func (w *responseStreamProgressWriter) enqueue(payload []byte) bool {
	if w == nil {
		return false
	}
	line := appendPayloadLine(payload)

	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return false
	}
	w.mu.Unlock()

	select {
	case w.queue <- line:
		return true
	default:
		w.mu.Lock()
		w.droppedLines++
		w.mu.Unlock()
		return false
	}
}

func (w *responseStreamProgressWriter) enqueueNotice(payload []byte) {
	if w == nil {
		return
	}
	line := appendPayloadLine(payload)

	w.mu.Lock()
	if w.closed {
		w.pendingNotice = line
		w.mu.Unlock()
		return
	}
	w.mu.Unlock()

	select {
	case w.queue <- line:
		return
	default:
	}

	select {
	case w.queue <- line:
	default:
		w.mu.Lock()
		w.pendingNotice = line
		w.mu.Unlock()
	}
}

func (w *responseStreamProgressWriter) droppedProgressLines() int {
	if w == nil {
		return 0
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.droppedLines
}

func (w *responseStreamProgressWriter) stopAndDrain() {
	if w == nil {
		return
	}

	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		if !waitProgressWriter(&w.wg, responseStreamProgressDrainTimeout) {
			w.abandonDrain()
		}
		return
	}
	w.closed = true
	w.mu.Unlock()
	close(w.queue)
	if !waitProgressWriter(&w.wg, responseStreamProgressDrainTimeout) {
		w.abandonDrain()
	}
}

func (w *responseStreamProgressWriter) abandonDrain() {
	if w == nil {
		return
	}
	w.mu.Lock()
	w.drainTimedOut = true
	w.mu.Unlock()
}

func (w *responseStreamProgressWriter) acquireOutputExclusive() {
	if w == nil {
		return
	}
	w.outputMu.Lock()
}

func (w *responseStreamProgressWriter) releaseOutputExclusive() {
	if w == nil {
		return
	}
	w.outputMu.Unlock()
}

func (w *responseStreamProgressWriter) drainAbandoned() bool {
	if w == nil {
		return false
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.drainTimedOut
}

func waitProgressWriter(wg *sync.WaitGroup, timeout time.Duration) bool {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}

func (w *responseStreamProgressWriter) run() {
	defer w.wg.Done()
	for line := range w.queue {
		if w.drainAbandoned() {
			return
		}
		if !w.writeOutputLine(line) {
			return
		}
		if !w.flushPendingNoticeLocked() {
			return
		}
	}
	w.flushPendingNoticeLocked()
}

func (w *responseStreamProgressWriter) writeOutputLine(line []byte) bool {
	w.outputMu.Lock()
	defer w.outputMu.Unlock()
	if w.drainAbandoned() {
		return false
	}
	_, _ = w.output.Write(line)
	return !w.drainAbandoned()
}

func (w *responseStreamProgressWriter) flushPendingNoticeLocked() bool {
	for {
		w.mu.Lock()
		notice := w.pendingNotice
		w.pendingNotice = nil
		w.mu.Unlock()
		if notice == nil {
			return true
		}
		if !w.writeOutputLine(notice) {
			return false
		}
	}
}

func appendPayloadLine(payload []byte) []byte {
	line := make([]byte, len(payload)+1)
	copy(line, payload)
	line[len(payload)] = '\n'
	return line
}

func isPackagedTTSRun(cfg RunConfig) bool {
	return strings.TrimSpace(cfg.NamedFactoryName) == tts.PackagedFactoryName
}

func logPackagedTTSInvocationStart(cfg RunConfig) {
	if !isPackagedTTSRun(cfg) {
		return
	}
	logger := cfg.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	fields := []zap.Field{
		zap.String("packaged_factory_name", tts.PackagedFactoryName),
		zap.String("tts_backend", tts.BackendRuntimeLabel()),
		zap.String("readiness_outcome", tts.FailureClassLoading),
	}
	if resolution := cfg.NamedFactoryResolution; resolution != nil {
		fields = append(fields,
			zap.String("named_factory_resolution_source", string(resolution.Source)),
			zap.String("named_factory_dir", resolution.FactoryDir),
		)
	}
	logger.Info("packaged tts invocation started", fields...)
}
