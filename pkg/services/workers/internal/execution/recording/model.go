// Package recording decorates worker runners with canonical model request and
// response event recording. It also owns the trace hooks used by managed local
// model runtimes so every construction path records the same evidence.
package recording

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

const (
	modelRequestEventIDPrefix      = "factory-event/model-request"
	modelResponseEventIDPrefix     = "factory-event/model-response"
	modelExecutionOutputPreviewMax = 200
)

type executionTrace struct {
	mu sync.Mutex

	resourceWaitStartedAt time.Time
	resourceWaitMillis    int64
	resourceAcquired      bool
	loadRequested         bool
	loadReused            bool
	loadStartedAt         time.Time
	loadMillis            int64
}

type executionTraceKey struct{}

func providerSessionForRequest(
	request workerexecution.RunnerExecutionRequest,
	workerDef *interfaces.FactoryWorkerConfig,
) *workerexecution.ProviderSessionMetadata {
	provider := strings.TrimSpace(request.RunnerID)
	executorProvider := strings.TrimSpace(request.ExecutorProvider)
	if strings.EqualFold(executorProvider, workers.ExecutorProviderACP) {
		provider = strings.TrimSpace(request.ModelProvider)
	} else if executorProvider != "" && !strings.EqualFold(executorProvider, "SCRIPT_WRAP") {
		provider = executorProvider
	}
	if strings.TrimSpace(provider) == "" {
		provider = strings.TrimSpace(request.ModelProvider)
	}
	if strings.TrimSpace(provider) == "" {
		provider = strings.TrimSpace(request.RunnerID)
	}
	if strings.TrimSpace(provider) == "" && workerDef != nil {
		provider = strings.TrimSpace(workerDef.ModelProvider)
	}
	provider = workers.CanonicalProviderSessionProvider(provider)
	if provider == "" {
		return nil
	}
	return &workerexecution.ProviderSessionMetadata{Provider: provider}
}

func continuationHasSessionIdentity(continuation *workerexecution.ProviderContinuationRef) bool {
	if continuation == nil {
		return false
	}
	return strings.TrimSpace(continuation.ProviderSessionID) != "" ||
		strings.TrimSpace(continuation.ExternalRef) != ""
}

// Diagnostics returns the redacted event-safe diagnostics payload shared by
// every model execution recording path.
func Diagnostics(success *workerexecution.WorkDiagnostics, executionErr error) json.RawMessage {
	var safe *workerexecution.SafeWorkDiagnostics
	if success != nil {
		safe = workerexecution.SafeWorkDiagnosticsFromWorkDiagnostics(success)
	} else {
		var providerErr *workers.ProviderError
		if errors.As(executionErr, &providerErr) {
			safe = workerexecution.SafeWorkDiagnosticsFromWorkDiagnostics(providerErr.Diagnostics)
		}
	}
	payload, encodeErr := workerexecution.SafeWorkDiagnosticsEventPayload(safe)
	if encodeErr != nil || string(payload) == "null" {
		return nil
	}
	return payload
}

// Hooks returns the local-model trace hooks paired with NewRunner.
func Hooks() workerexecution.LocalRuntimeHooks {
	return workerexecution.LocalRuntimeHooks{
		MarkResourceWaitStarted:  markResourceWaitStarted,
		MarkResourceWaitFinished: markResourceWaitFinished,
		MarkLoadRequested:        markLoadRequested,
		MarkLoadFinished:         markLoadFinished,
		MarkLoadReused:           markLoadReused,
	}
}

func traceFromContext(ctx context.Context) *executionTrace {
	if ctx == nil {
		return nil
	}
	trace, _ := ctx.Value(executionTraceKey{}).(*executionTrace)
	return trace
}

func markResourceWaitStarted(ctx context.Context, startedAt time.Time) {
	if trace := traceFromContext(ctx); trace != nil {
		trace.mu.Lock()
		trace.resourceWaitStartedAt = startedAt
		trace.mu.Unlock()
	}
}

func markResourceWaitFinished(ctx context.Context, finishedAt time.Time, acquired bool) {
	if trace := traceFromContext(ctx); trace != nil {
		trace.mu.Lock()
		if !trace.resourceWaitStartedAt.IsZero() {
			trace.resourceWaitMillis = finishedAt.Sub(trace.resourceWaitStartedAt).Milliseconds()
		}
		trace.resourceAcquired = acquired
		trace.mu.Unlock()
	}
}

func markLoadRequested(ctx context.Context, startedAt time.Time) {
	if trace := traceFromContext(ctx); trace != nil {
		trace.mu.Lock()
		trace.loadRequested = true
		trace.loadStartedAt = startedAt
		trace.mu.Unlock()
	}
}

func markLoadFinished(ctx context.Context, finishedAt time.Time) {
	if trace := traceFromContext(ctx); trace != nil {
		trace.mu.Lock()
		if !trace.loadStartedAt.IsZero() {
			trace.loadMillis = finishedAt.Sub(trace.loadStartedAt).Milliseconds()
		}
		trace.mu.Unlock()
	}
}

func markLoadReused(ctx context.Context) {
	if trace := traceFromContext(ctx); trace != nil {
		trace.mu.Lock()
		trace.loadRequested = true
		trace.loadReused = true
		trace.mu.Unlock()
	}
}

func boolPtr(value bool) *bool { return &value }
func int64PtrIfPositive(value int64) *int64 {
	if value <= 0 {
		return nil
	}
	return &value
}
func truncate(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit]
}
func stringPtr(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return &value
}

func stringsIfPresent(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			out = append(out, value)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
