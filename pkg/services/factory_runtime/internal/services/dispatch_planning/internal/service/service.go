// Package service implements deterministic Factory Runtime dispatch planning.
package service

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"

	dispatchplanning "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/dispatch_planning"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// Planner translates complete runnable decisions and owns their per-runtime
// outbox publication state.
type Planner struct {
	mu            sync.Mutex
	publisher     dispatchplanning.WorkersPublisher
	canceler      dispatchplanning.WorkersCanceler
	byDispatch    map[string]*intentRecord
	byCorrelation map[string]*intentRecord
	byWorkID      map[string][]*intentRecord
	ordered       []*intentRecord
	mode          dispatchplanning.RuntimeOutboxMode
	stopReason    dispatchplanning.RuntimeStopReason
}

type intentRecord struct {
	action      dispatchplanning.OutboxAction
	status      dispatchplanning.OutboxIntentStatus
	attempts    int
	result      *dispatchplanning.TerminalResult
	cancelled   bool
	cancelling  bool
	published   bool
	publishDone chan struct{}
}

var _ dispatchplanning.Service = (*Planner)(nil)

// New constructs a dispatch planner without a Workers cancellation edge. It is
// useful for inert planning and publication-only tests.
func New(publisher dispatchplanning.WorkersPublisher) *Planner {
	return NewWithCancellation(publisher, nil)
}

// NewWithCancellation constructs the complete Runtime-owned outbox boundary.
func NewWithCancellation(
	publisher dispatchplanning.WorkersPublisher,
	canceler dispatchplanning.WorkersCanceler,
) *Planner {
	return &Planner{
		publisher:     publisher,
		canceler:      canceler,
		byDispatch:    make(map[string]*intentRecord),
		byCorrelation: make(map[string]*intentRecord),
		byWorkID:      make(map[string][]*intentRecord),
		mode:          dispatchplanning.RuntimeOutboxModeActive,
	}
}

// Plan validates the entire batch before returning any outbox action.
func (p *Planner) Plan(ctx context.Context, req dispatchplanning.PlanRequest) (dispatchplanning.PlanResult, error) {
	if ctx == nil {
		return dispatchplanning.PlanResult{}, fmt.Errorf("%w: context is required", dispatchplanning.ErrInvalidRunnableDecision)
	}
	if err := ctx.Err(); err != nil {
		return dispatchplanning.PlanResult{}, err
	}
	if len(req.Decisions) == 0 {
		return dispatchplanning.PlanResult{}, nil
	}

	dispatchIDs := make(map[string]struct{}, len(req.Decisions))
	correlationIDs := make(map[string]struct{}, len(req.Decisions))
	for index, decision := range req.Decisions {
		if err := validateDecision(decision, dispatchIDs, correlationIDs); err != nil {
			return dispatchplanning.PlanResult{}, fmt.Errorf(
				"%w at scheduler position %d: %v",
				dispatchplanning.ErrInvalidRunnableDecision,
				index,
				err,
			)
		}
	}

	actions := make([]dispatchplanning.OutboxAction, 0, len(req.Decisions))
	for _, decision := range req.Decisions {
		execution := executionRequest(decision)
		actions = append(actions, dispatchplanning.OutboxAction{
			CorrelationID: decision.CorrelationID,
			Request: workers.WorkstationDispatchRequest{
				WorkstationName: execution.Dispatch.WorkstationName,
				Execution:       execution,
			},
		})
	}
	return dispatchplanning.PlanResult{Actions: actions}, nil
}

// Publish reserves both identities before performing IO. Equivalent redelivery
// observes the accepted logical intent without reordering or republishing it.
func (p *Planner) Publish(
	ctx context.Context,
	action dispatchplanning.OutboxAction,
) (dispatchplanning.PublicationResult, error) {
	if err := validateAction(action); err != nil {
		return dispatchplanning.PublicationResult{}, err
	}

	stable := cloneAction(action)
	p.mu.Lock()
	if existing := p.existingIntent(stable); existing != nil {
		result, err := duplicateResult(existing, stable)
		p.mu.Unlock()
		return result, err
	}
	if p.mode == dispatchplanning.RuntimeOutboxModeStopped {
		p.mu.Unlock()
		return dispatchplanning.PublicationResult{}, fmt.Errorf(
			"%w: stopped by %s",
			dispatchplanning.ErrDispatchRuntimeStopped,
			p.stopReason,
		)
	}
	record := &intentRecord{
		action: stable,
		status: dispatchplanning.OutboxIntentStatusPending,
	}
	p.byDispatch[stable.Request.Execution.Dispatch.DispatchID] = record
	p.byCorrelation[stable.CorrelationID] = record
	indexedWorkIDs := make(map[string]struct{}, len(stable.Request.Execution.Dispatch.Execution.WorkIDs))
	for _, workID := range stable.Request.Execution.Dispatch.Execution.WorkIDs {
		workID = strings.TrimSpace(workID)
		if _, indexed := indexedWorkIDs[workID]; indexed {
			continue
		}
		indexedWorkIDs[workID] = struct{}{}
		p.byWorkID[workID] = append(p.byWorkID[workID], record)
	}
	p.ordered = append(p.ordered, record)
	paused := p.mode == dispatchplanning.RuntimeOutboxModePaused
	p.mu.Unlock()

	result := publicationResult(dispatchplanning.PublicationOutcomeAccepted, stable)
	if paused {
		return result, nil
	}
	return result, p.publish(ctx, record)
}

// InvalidateWork marks every accepted intent whose canonical Work lineage
// contains workID as non-publishable. Publishing intents are allowed to cross
// the existing publication completion boundary before a best-effort Workers
// cancellation is attempted; a successful publication after invalidation is
// still cancelled exactly once. Retired intents remain tombstones.
func (p *Planner) InvalidateWork(
	ctx context.Context,
	workID string,
) (dispatchplanning.WorkInvalidationResult, error) {
	workID, err := validateWorkInvalidation(ctx, workID)
	if err != nil {
		return dispatchplanning.WorkInvalidationResult{}, err
	}

	p.mu.Lock()
	records := append([]*intentRecord(nil), p.byWorkID[workID]...)
	waits := make([]chan struct{}, len(records))
	previousStatuses := make([]dispatchplanning.OutboxIntentStatus, len(records))
	for index, record := range records {
		previousStatuses[index] = record.status
		switch record.status {
		case dispatchplanning.OutboxIntentStatusPending,
			dispatchplanning.OutboxIntentStatusPublishing,
			dispatchplanning.OutboxIntentStatusPublished,
			dispatchplanning.OutboxIntentStatusInvalidated:
			record.status = dispatchplanning.OutboxIntentStatusInvalidated
		}
		if previousStatuses[index] == dispatchplanning.OutboxIntentStatusPublishing {
			waits[index] = record.publishDone
		}
	}
	p.mu.Unlock()

	var firstErr error
	for index, record := range records {
		if waits[index] != nil {
			select {
			case <-waits[index]:
			case <-ctx.Done():
				if firstErr == nil {
					firstErr = ctx.Err()
				}
				continue
			}
		}
		if err := p.cancelInvalidated(ctx, record); err != nil && firstErr == nil {
			firstErr = fmt.Errorf(
				"invalidate Work %q dispatch %q: %w",
				workID,
				record.action.Request.Execution.Dispatch.DispatchID,
				err,
			)
		}
	}

	p.mu.Lock()
	intents := make([]dispatchplanning.WorkInvalidationIntent, 0, len(records))
	for index, record := range records {
		intents = append(intents, dispatchplanning.WorkInvalidationIntent{
			DispatchID:            record.action.Request.Execution.Dispatch.DispatchID,
			CorrelationID:         record.action.CorrelationID,
			PreviousStatus:        previousStatuses[index],
			Status:                record.status,
			CancellationRequested: record.cancelled,
		})
	}
	p.mu.Unlock()

	outcome := dispatchplanning.WorkInvalidationOutcomeNoOp
	if len(intents) > 0 {
		outcome = dispatchplanning.WorkInvalidationOutcomeInvalidated
	}
	return dispatchplanning.WorkInvalidationResult{
		WorkID:  workID,
		Outcome: outcome,
		Intents: intents,
	}, firstErr
}

func validateWorkInvalidation(ctx context.Context, workID string) (string, error) {
	if ctx == nil {
		return "", fmt.Errorf(
			"%w: context is required",
			dispatchplanning.ErrInvalidRunnableDecision,
		)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	workID = strings.TrimSpace(workID)
	if workID == "" {
		return "", fmt.Errorf(
			"%w: Work ID is required",
			dispatchplanning.ErrInvalidRunnableDecision,
		)
	}
	return workID, nil
}

// Retry republishes only an explicitly pending intent. Concurrent retry or an
// already published intent is an idempotent observation, not another call.
func (p *Planner) Retry(
	ctx context.Context,
	dispatchID string,
) (dispatchplanning.PublicationResult, error) {
	dispatchID = strings.TrimSpace(dispatchID)
	p.mu.Lock()
	record := p.byDispatch[dispatchID]
	if record == nil {
		p.mu.Unlock()
		return dispatchplanning.PublicationResult{}, fmt.Errorf(
			"%w: dispatch ID %q is not pending",
			dispatchplanning.ErrInvalidRunnableDecision,
			dispatchID,
		)
	}
	result := publicationResult(dispatchplanning.PublicationOutcomeDuplicateIdempotent, record.action)
	if record.status != dispatchplanning.OutboxIntentStatusPending ||
		p.mode == dispatchplanning.RuntimeOutboxModePaused {
		p.mu.Unlock()
		return result, nil
	}
	if p.mode == dispatchplanning.RuntimeOutboxModeStopped {
		p.mu.Unlock()
		return dispatchplanning.PublicationResult{}, fmt.Errorf(
			"%w: stopped by %s",
			dispatchplanning.ErrDispatchRuntimeStopped,
			p.stopReason,
		)
	}
	p.mu.Unlock()
	return result, p.publish(ctx, record)
}

// Intent returns a detached snapshot suitable for retry and diagnostics.
func (p *Planner) Intent(dispatchID string) (dispatchplanning.OutboxIntent, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	record, ok := p.byDispatch[strings.TrimSpace(dispatchID)]
	if !ok {
		return dispatchplanning.OutboxIntent{}, false
	}
	return dispatchplanning.OutboxIntent{
		Action:                cloneAction(record.action),
		Status:                record.status,
		Attempts:              record.attempts,
		CancellationRequested: record.cancelled,
		Result:                cloneTerminalResult(record.result),
	}, true
}

func (p *Planner) publish(ctx context.Context, record *intentRecord) error {
	p.mu.Lock()
	if record.status != dispatchplanning.OutboxIntentStatusPending {
		p.mu.Unlock()
		return nil
	}
	mode := p.mode
	stopReason := p.stopReason
	if mode != dispatchplanning.RuntimeOutboxModeActive {
		p.mu.Unlock()
		if mode == dispatchplanning.RuntimeOutboxModeStopped {
			return fmt.Errorf(
				"%w: stopped by %s",
				dispatchplanning.ErrDispatchRuntimeStopped,
				stopReason,
			)
		}
		return nil
	}
	record.status = dispatchplanning.OutboxIntentStatusPublishing
	record.attempts++
	record.publishDone = make(chan struct{})
	action := cloneAction(record.action)
	publisher := p.publisher
	p.mu.Unlock()

	var err error
	if ctx == nil {
		err = fmt.Errorf("publish dispatch intent: context is required")
	} else if contextErr := ctx.Err(); contextErr != nil {
		err = contextErr
	} else if publisher == nil {
		err = fmt.Errorf("publish dispatch intent: Workers publisher is unavailable")
	} else {
		err = publisher(ctx, action.Request)
	}

	p.mu.Lock()
	if record.status == dispatchplanning.OutboxIntentStatusRetired {
		close(record.publishDone)
		record.publishDone = nil
		p.mu.Unlock()
		return err
	}
	if record.status == dispatchplanning.OutboxIntentStatusInvalidated {
		if err == nil {
			record.published = true
		}
		close(record.publishDone)
		record.publishDone = nil
		p.mu.Unlock()
		return err
	}
	if err != nil {
		record.status = dispatchplanning.OutboxIntentStatusPending
	} else {
		record.status = dispatchplanning.OutboxIntentStatusPublished
		record.published = true
	}
	close(record.publishDone)
	record.publishDone = nil
	p.mu.Unlock()
	return err
}

func (p *Planner) existingIntent(action dispatchplanning.OutboxAction) *intentRecord {
	if record := p.byDispatch[action.Request.Execution.Dispatch.DispatchID]; record != nil {
		return record
	}
	return p.byCorrelation[action.CorrelationID]
}

func duplicateResult(
	existing *intentRecord,
	action dispatchplanning.OutboxAction,
) (dispatchplanning.PublicationResult, error) {
	if existing.action.CorrelationID != action.CorrelationID ||
		existing.action.Request.Execution.Dispatch.DispatchID != action.Request.Execution.Dispatch.DispatchID ||
		!reflect.DeepEqual(existing.action, action) {
		return dispatchplanning.PublicationResult{}, fmt.Errorf(
			"%w: dispatch ID %q or correlation ID %q conflicts with an accepted intent",
			dispatchplanning.ErrDuplicateDispatchIntent,
			action.Request.Execution.Dispatch.DispatchID,
			action.CorrelationID,
		)
	}
	return publicationResult(dispatchplanning.PublicationOutcomeDuplicateIdempotent, action), nil
}

func publicationResult(
	outcome dispatchplanning.PublicationOutcome,
	action dispatchplanning.OutboxAction,
) dispatchplanning.PublicationResult {
	return dispatchplanning.PublicationResult{
		Outcome:       outcome,
		DispatchID:    action.Request.Execution.Dispatch.DispatchID,
		CorrelationID: action.CorrelationID,
	}
}

func validateAction(action dispatchplanning.OutboxAction) error {
	dispatch := action.Request.Execution.Dispatch
	required := []struct {
		name  string
		value string
	}{
		{name: "dispatch ID", value: dispatch.DispatchID},
		{name: "correlation ID", value: action.CorrelationID},
		{name: "replay key", value: dispatch.Execution.ReplayKey},
		{name: "workstation name", value: action.Request.WorkstationName},
		{name: "worker type", value: action.Request.Execution.WorkerType},
	}
	for _, field := range required {
		if strings.TrimSpace(field.value) == "" {
			return fmt.Errorf("%w: %s is required", dispatchplanning.ErrInvalidRunnableDecision, field.name)
		}
	}
	if action.Request.WorkstationName != dispatch.WorkstationName {
		return fmt.Errorf("%w: workstation conflicts with canonical dispatch", dispatchplanning.ErrInvalidRunnableDecision)
	}
	if len(dispatch.Execution.WorkIDs) == 0 || containsBlank(dispatch.Execution.WorkIDs) {
		return fmt.Errorf("%w: Work lineage is required", dispatchplanning.ErrInvalidRunnableDecision)
	}
	return nil
}

func cloneAction(action dispatchplanning.OutboxAction) dispatchplanning.OutboxAction {
	return dispatchplanning.OutboxAction{
		CorrelationID: action.CorrelationID,
		Request: workers.WorkstationDispatchRequest{
			WorkstationName: action.Request.WorkstationName,
			Execution:       workers.CloneWorkstationExecutionRequest(action.Request.Execution),
		},
	}
}

func validateDecision(
	decision dispatchplanning.RunnableDecision,
	dispatchIDs map[string]struct{},
	correlationIDs map[string]struct{},
) error {
	dispatch := decision.Dispatch
	required := []struct {
		name  string
		value string
	}{
		{name: "dispatch ID", value: dispatch.DispatchID},
		{name: "correlation ID", value: decision.CorrelationID},
		{name: "replay key", value: dispatch.Execution.ReplayKey},
		{name: "workstation name", value: dispatch.WorkstationName},
		{name: "worker type", value: decision.Execution.WorkerType},
	}
	for _, field := range required {
		if strings.TrimSpace(field.value) == "" {
			return fmt.Errorf("%s is required", field.name)
		}
	}
	if dispatch.WorkerType != "" && dispatch.WorkerType != decision.Execution.WorkerType {
		return fmt.Errorf("worker type conflicts with canonical dispatch")
	}
	if len(dispatch.Execution.WorkIDs) == 0 || containsBlank(dispatch.Execution.WorkIDs) {
		return fmt.Errorf("Work lineage is required")
	}
	if dispatch.InputTokens == nil {
		return fmt.Errorf("dispatch input payload is required")
	}
	if decision.Execution.InputPayload == nil {
		return fmt.Errorf("Workers input payload is required")
	}
	if _, exists := dispatchIDs[dispatch.DispatchID]; exists {
		return fmt.Errorf("dispatch ID %q appears more than once", dispatch.DispatchID)
	}
	if _, exists := correlationIDs[decision.CorrelationID]; exists {
		return fmt.Errorf("correlation ID %q appears more than once", decision.CorrelationID)
	}
	dispatchIDs[dispatch.DispatchID] = struct{}{}
	correlationIDs[decision.CorrelationID] = struct{}{}
	return nil
}

func executionRequest(decision dispatchplanning.RunnableDecision) workers.WorkstationExecutionRequest {
	facts := decision.Execution
	return workers.CloneWorkstationExecutionRequest(workers.WorkstationExecutionRequest{
		Dispatch:                 decision.Dispatch,
		WorkerType:               facts.WorkerType,
		WorkstationType:          facts.WorkstationType,
		RunnerID:                 facts.RunnerID,
		RunnerSelectionSource:    facts.RunnerSelectionSource,
		ProjectID:                facts.ProjectID,
		FactorySessionID:         facts.FactorySessionID,
		RecordingID:              facts.RecordingID,
		Capabilities:             facts.Capabilities,
		InputTokens:              facts.InputPayload,
		ModelOperation:           facts.ModelOperation,
		ModelBindings:            facts.ModelBindings,
		Model:                    facts.Model,
		ModelProvider:            facts.ModelProvider,
		SystemPrompt:             facts.SystemPrompt,
		UserMessage:              facts.UserMessage,
		OutputSchema:             facts.OutputSchema,
		Timeout:                  facts.Timeout,
		EnvVars:                  facts.EnvVars,
		ProcessEnvironment:       facts.ProcessEnvironment,
		Worktree:                 facts.Worktree,
		WorkingDirectory:         facts.WorkingDirectory,
		WorkingDirectoryAuthored: facts.WorkingDirectoryAuthored,
	})
}

func containsBlank(values []string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return true
		}
	}
	return false
}
