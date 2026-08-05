package stdio

import (
	"context"

	acpsdk "github.com/coder/acp-go-sdk"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/mapping"
)

type workerChildProjectionNotificationError struct {
	err error
}

func (err *workerChildProjectionNotificationError) Error() string {
	return err.err.Error()
}

func (err *workerChildProjectionNotificationError) Unwrap() error {
	return err.err
}

// boundWorkerChildProjection applies the pure mapping package's named bounds
// to one already-associated child update while rebuilding canonical history.
// Live and retained delivery instead uses deliverBoundWorkerChildProjection so
// a failed notification cannot advance the accounting.
func (c *attachmentCache) boundWorkerChildProjection(
	sessionID string,
	item chatsessions.SequencedItem,
	update *acpsdk.SessionUpdate,
) (*acpsdk.SessionUpdate, error) {
	if update == nil || update.ToolCallUpdate == nil {
		return update, nil
	}
	parentItemID := string(update.ToolCallUpdate.ToolCallId)
	if c == nil || parentItemID == "" {
		budget := mapping.NewChildProjectionBudget(mapping.DefaultChildProjectionLimits())
		_, bounded, err := mapping.BoundChildProjection(budget, item, update)
		return bounded, err
	}

	c.workerChildProjectionMu.Lock()
	defer c.workerChildProjectionMu.Unlock()
	return c.boundWorkerChildProjectionLocked(sessionID, parentItemID, item, update)
}

func (c *attachmentCache) boundWorkerChildProjectionLocked(
	sessionID, parentItemID string,
	item chatsessions.SequencedItem,
	update *acpsdk.SessionUpdate,
) (*acpsdk.SessionUpdate, error) {
	next, bounded, err := c.previewWorkerChildProjection(sessionID, parentItemID, item, update)
	if err != nil {
		return nil, err
	}
	c.commitWorkerChildProjection(sessionID, parentItemID, next)
	return bounded, nil
}

// deliverBoundWorkerChildProjection keeps a child budget transaction open
// until the notification has succeeded. A failed notifier leaves the exact
// prior budget in place, so retrying the unacknowledged canonical record has
// the same content and elision position as a clean replay. The transaction
// lock also keeps concurrent live and retained drains from observing or
// committing the same child budget out of order.
func (c *attachmentCache) deliverBoundWorkerChildProjection(
	sessionID string,
	item chatsessions.SequencedItem,
	update *acpsdk.SessionUpdate,
	notify func(*acpsdk.SessionUpdate) error,
) (*acpsdk.SessionUpdate, error) {
	if update == nil {
		return update, nil
	}
	if update.ToolCallUpdate == nil {
		if notify == nil {
			return update, nil
		}
		if err := notify(update); err != nil {
			return nil, &workerChildProjectionNotificationError{err: err}
		}
		return update, nil
	}
	parentItemID := string(update.ToolCallUpdate.ToolCallId)
	if c == nil || parentItemID == "" {
		budget := mapping.NewChildProjectionBudget(mapping.DefaultChildProjectionLimits())
		_, bounded, err := mapping.BoundChildProjection(budget, item, update)
		if err != nil || bounded == nil || notify == nil {
			return bounded, err
		}
		if err := notify(bounded); err != nil {
			return nil, &workerChildProjectionNotificationError{err: err}
		}
		return bounded, nil
	}

	c.workerChildProjectionMu.Lock()
	defer c.workerChildProjectionMu.Unlock()
	next, bounded, err := c.previewWorkerChildProjection(sessionID, parentItemID, item, update)
	if err != nil || bounded == nil {
		return bounded, err
	}
	if notify != nil {
		if err := notify(bounded); err != nil {
			return nil, &workerChildProjectionNotificationError{err: err}
		}
	}
	c.commitWorkerChildProjection(sessionID, parentItemID, next)
	return bounded, nil
}

func (c *attachmentCache) previewWorkerChildProjection(
	sessionID, parentItemID string,
	item chatsessions.SequencedItem,
	update *acpsdk.SessionUpdate,
) (mapping.ChildProjectionBudget, *acpsdk.SessionUpdate, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.workerChildBudgets == nil {
		c.workerChildBudgets = make(map[string]map[string]mapping.ChildProjectionBudget)
	}
	byParent := c.workerChildBudgets[sessionID]
	if byParent == nil {
		byParent = make(map[string]mapping.ChildProjectionBudget)
		c.workerChildBudgets[sessionID] = byParent
	}
	budget, found := byParent[parentItemID]
	if !found {
		budget = mapping.NewChildProjectionBudget(mapping.DefaultChildProjectionLimits())
	}
	next, bounded, err := mapping.BoundChildProjection(budget, item, update)
	if err != nil {
		return mapping.ChildProjectionBudget{}, nil, err
	}
	return next, bounded, nil
}

func (c *attachmentCache) commitWorkerChildProjection(sessionID, parentItemID string, budget mapping.ChildProjectionBudget) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.workerChildBudgets == nil {
		c.workerChildBudgets = make(map[string]map[string]mapping.ChildProjectionBudget)
	}
	byParent := c.workerChildBudgets[sessionID]
	if byParent == nil {
		byParent = make(map[string]mapping.ChildProjectionBudget)
		c.workerChildBudgets[sessionID] = byParent
	}
	byParent[parentItemID] = budget
}

// dispatchFactoryInvocation calls invoke -- one of the two
// factorysessions.Service.InvokeFactorySession-forwarding calls
// startFactorySessionForEpisode/invokeFactorySessionForEpisode make -- and
// returns its result, error, and liveDelivered=false unchanged. When
// s.responseBridge, s.chatSessions, and s.factoryTarget are all configured, it
// instead calls s.responseBridge with invoke and a liveDrain closure over
// s.liveDrainTurnUpdates: the injected collaborator (see acp.ResponseBridge's
// own doc comment) starts the Chat Sessions-owned Factory response-event
// bridge AND this transport's own genuine mid-generation consumer loop both
// concurrently with invoke, then drains the bridge's terminal retained tail
// before a successful prompt can return. An invocation error remains
// authoritative; a bridge failure after a successful invocation returns a
// bounded prompt failure. This method itself starts no goroutine and owns no
// concurrency primitive: it only ever holds and calls the one plain function
// value pkg/wire injected, and only ever supplies liveDrain as a plain
// callback for that collaborator to run.
//
// liveDelivered reports whether the liveDrain callback itself observed and
// delivered at least one canonical agent_message_chunk before invoke
// returned (see dispatchOutcome's own doc comment for why this must be
// threaded through to deliverPromptUpdates' V1 suppression decision). Writing
// to it from inside the liveDrain closure -- which s.responseBridge runs on a
// goroutine it owns, never one this method spawns itself -- is race-free
// without its own synchronization: s.responseBridge only returns once that
// goroutine has fully joined (see responsebridge.Service.Run's own doc
// comment), and that join's happens-before guarantee is what makes reading
// liveDelivered after s.responseBridge returns safe.
//
// connectionID is threaded through from dispatchFactoryTurn's own
// reqIdentity so liveDrainTurnUpdates can call ensureAttachment exactly the
// way the post-invocation streamTurnUpdates call already does; notify is read
// from ctx (promptNotifierFromContext), matching deliverPromptUpdates' own
// convention, since liveDrain runs against the bridge-derived context the
// response bridge passes it, not necessarily this ctx directly.
func (s *Server) dispatchFactoryInvocation(
	ctx context.Context,
	connectionID, chatSessionID string,
	sessionVersion uint64,
	factorySessionID string,
	invoke func(context.Context) (factorysessions.InvocationResult, error),
) (result factorysessions.InvocationResult, liveDelivered bool, err error) {
	if s.responseBridge == nil || s.chatSessions == nil || s.factoryTarget == nil {
		result, err = invoke(ctx)
		return result, false, err
	}
	notify := promptNotifierFromContext(ctx)
	liveDrain := func(drainCtx context.Context) {
		liveDelivered = s.liveDrainTurnUpdates(drainCtx, connectionID, chatSessionID, sessionVersion, notify)
	}
	result, err = s.responseBridge(ctx, chatSessionID, sessionVersion, factorySessionID, liveDrain, invoke)
	return result, liveDelivered, err
}
