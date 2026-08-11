package cli

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

const humanWorkerProgressInterval = 120 * time.Millisecond

var humanWorkerSpinnerFrames = [...]string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// humanWorkerProgressRenderer owns the stderr-only, TTY-only worker spinner.
// Its state is independent from the response stream so JSON/NDJSON output
// remains byte-for-byte owned by the stream writer.
type humanWorkerProgressRenderer struct {
	output           io.Writer
	ticks            <-chan time.Time
	ticker           *time.Ticker
	stop             chan struct{}
	done             chan struct{}
	once             sync.Once
	mu               sync.Mutex
	active           map[string]humanWorkerProgressState
	pending          map[string]humanWorkerProgressState
	workerByDispatch map[string]string
	frame            int
	drawn            bool
	stopped          bool
}

type humanWorkerProgressState struct {
	dispatchID  string
	workerID    string
	workstation string
	workIDs     []string
}

func newHumanWorkerProgressRenderer(output io.Writer, isTTY bool, ticks <-chan time.Time) *humanWorkerProgressRenderer {
	renderer := &humanWorkerProgressRenderer{}
	if output == nil || !isTTY {
		return renderer
	}
	if ticks == nil {
		ticker := time.NewTicker(humanWorkerProgressInterval)
		ticks = ticker.C
		renderer.ticker = ticker
	}
	renderer.output = output
	renderer.ticks = ticks
	renderer.stop = make(chan struct{})
	renderer.done = make(chan struct{})
	renderer.active = make(map[string]humanWorkerProgressState)
	renderer.pending = make(map[string]humanWorkerProgressState)
	renderer.workerByDispatch = make(map[string]string)
	go renderer.run()
	return renderer
}

func (renderer *humanWorkerProgressRenderer) PresentFactoryEvents(events []interfaces.FactoryEvent) {
	if renderer == nil || renderer.output == nil {
		return
	}
	renderer.mu.Lock()
	if renderer.stopped {
		renderer.mu.Unlock()
		return
	}
	for _, event := range events {
		renderer.applyEventLocked(event)
	}
	renderer.drawLocked()
	renderer.mu.Unlock()
}

func (renderer *humanWorkerProgressRenderer) applyEventLocked(event interfaces.FactoryEvent) {
	switch event.Type {
	case interfaces.FactoryEventTypeDispatchQueued:
		renderer.applyDispatchQueuedLocked(event)
	case interfaces.FactoryEventTypeDispatchWorkerSessionAssoc:
		renderer.applyWorkerAssociationLocked(event)
	case interfaces.FactoryEventTypeDispatchRequest:
		renderer.applyDispatchRequestLocked(event)
	case interfaces.FactoryEventTypeDispatchResponse,
		interfaces.FactoryEventTypeDispatchInterrupted,
		interfaces.FactoryEventTypeDispatchReconciled:
		renderer.removeTerminalDispatchLocked(event)
	}
}

func (renderer *humanWorkerProgressRenderer) applyDispatchQueuedLocked(event interfaces.FactoryEvent) {
	dispatchID := humanWorkerProgressDispatchID(event)
	if dispatchID == "" {
		return
	}
	state := renderer.pending[dispatchID]
	state.dispatchID = dispatchID
	payload, ok := decodeFactoryEventPayload[interfaces.DispatchQueuedEventPayload](event)
	if ok {
		state.workstation = boundedHumanProgressPayload(stringPointerValue(payload.Label))
		state.workIDs = mergeHumanWorkIDs(state.workIDs, stringSlicePointerValue(payload.InputWorkIDs))
	}
	renderer.pending[dispatchID] = state
	if active, exists := renderer.active[dispatchID]; exists {
		renderer.active[dispatchID] = mergeHumanWorkerProgressState(active, state)
	}
}

func (renderer *humanWorkerProgressRenderer) applyWorkerAssociationLocked(event interfaces.FactoryEvent) {
	dispatchID := humanWorkerProgressDispatchID(event)
	if dispatchID == "" {
		return
	}
	payload, ok := decodeFactoryEventPayload[interfaces.DispatchWorkerSessionAssociationEventPayload](event)
	workerID := boundedHumanProgressPayload(payload.WorkerSessionID)
	if !ok || workerID == "" {
		return
	}
	renderer.workerByDispatch[dispatchID] = workerID
	if state, exists := renderer.active[dispatchID]; exists {
		state.workerID = workerID
		renderer.active[dispatchID] = state
	}
	if state, exists := renderer.pending[dispatchID]; exists {
		state.workerID = workerID
		renderer.pending[dispatchID] = state
	}
}

func (renderer *humanWorkerProgressRenderer) applyDispatchRequestLocked(event interfaces.FactoryEvent) {
	dispatchID := humanWorkerProgressDispatchID(event)
	identity := humanWorkerProgressIdentity(event)
	if identity == "" {
		return
	}
	state := renderer.active[identity]
	if dispatchID != "" {
		if pending, exists := renderer.pending[dispatchID]; exists {
			state = mergeHumanWorkerProgressState(pending, state)
			delete(renderer.pending, dispatchID)
		}
	}
	state.dispatchID = dispatchID
	if state.dispatchID == "" {
		state.dispatchID = identity
	}
	if state.workerID == "" && dispatchID != "" {
		state.workerID = renderer.workerByDispatch[dispatchID]
	}
	payload, ok := decodeFactoryEventPayload[interfaces.DispatchRequestEventPayload](event)
	if ok {
		if workstation := boundedHumanProgressPayload(payload.TransitionID); workstation != "" {
			state.workstation = workstation
		}
		state.workIDs = mergeHumanWorkIDs(state.workIDs, dispatchInputWorkIDs(payload))
	}
	state.workIDs = mergeHumanWorkIDs(state.workIDs, factoryEventWorkIDs(event))
	renderer.active[identity] = state
}

func (renderer *humanWorkerProgressRenderer) removeTerminalDispatchLocked(event interfaces.FactoryEvent) {
	dispatchID := humanWorkerProgressDispatchID(event)
	identity := dispatchID
	if identity == "" && event.Type == interfaces.FactoryEventTypeDispatchResponse {
		payload, ok := decodeFactoryEventPayload[workerexecution.DispatchResponseEventPayload](event)
		if ok {
			identity = strings.TrimSpace(payload.TransitionID)
		}
	}
	if identity == "" {
		return
	}
	delete(renderer.active, identity)
	delete(renderer.pending, dispatchID)
	delete(renderer.workerByDispatch, dispatchID)
}

func mergeHumanWorkerProgressState(
	base humanWorkerProgressState,
	additional humanWorkerProgressState,
) humanWorkerProgressState {
	if base.dispatchID == "" {
		base.dispatchID = additional.dispatchID
	}
	if additional.workerID != "" {
		base.workerID = additional.workerID
	}
	if additional.workstation != "" {
		base.workstation = additional.workstation
	}
	base.workIDs = mergeHumanWorkIDs(base.workIDs, additional.workIDs)
	return base
}

func (renderer *humanWorkerProgressRenderer) Stop() {
	if renderer == nil || renderer.output == nil {
		return
	}
	renderer.once.Do(func() {
		if renderer.ticker != nil {
			renderer.ticker.Stop()
		}
		close(renderer.stop)
		<-renderer.done
	})
}

func (renderer *humanWorkerProgressRenderer) run() {
	defer close(renderer.done)
	for {
		select {
		case <-renderer.stop:
			renderer.mu.Lock()
			renderer.stopped = true
			renderer.active = nil
			renderer.pending = nil
			renderer.workerByDispatch = nil
			renderer.clearLocked()
			renderer.mu.Unlock()
			return
		case <-renderer.ticks:
			renderer.mu.Lock()
			if len(renderer.active) > 0 {
				renderer.frame = (renderer.frame + 1) % len(humanWorkerSpinnerFrames)
				renderer.drawLocked()
			}
			renderer.mu.Unlock()
		}
	}
}

func (renderer *humanWorkerProgressRenderer) drawLocked() {
	if len(renderer.active) == 0 {
		renderer.clearLocked()
		return
	}
	identities := make([]string, 0, len(renderer.active))
	for identity := range renderer.active {
		identities = append(identities, identity)
	}
	sort.Strings(identities)
	glyphs := make([]string, 0, len(identities))
	for index, identity := range identities {
		state := renderer.active[identity]
		frame := humanWorkerSpinnerFrames[(renderer.frame+index)%len(humanWorkerSpinnerFrames)]
		line := formatHumanWorkerProgressLine(frame, state)
		colorIdentity := state.workerID
		if colorIdentity == "" {
			colorIdentity = identity
		}
		glyphs = append(glyphs, "\x1b["+stableWorkstationColor(colorIdentity)+"m"+line+"\x1b[0m")
	}
	_, _ = fmt.Fprint(renderer.output, "\r\x1b[2K"+strings.Join(glyphs, " "))
	renderer.drawn = true
}

func formatHumanWorkerProgressLine(frame string, state humanWorkerProgressState) string {
	identity := boundedHumanProgressPayload(state.workerID)
	if identity == "" {
		identity = boundedHumanProgressPayload(state.dispatchID)
	}
	label := "dispatch"
	if state.workerID != "" {
		label = "worker"
	}
	line := frame + " " + label + " " + identity + ": active"
	if workstation := boundedHumanProgressPayload(state.workstation); workstation != "" {
		line += " at " + workstation
	}
	if len(state.workIDs) > 0 {
		line += " (" + strings.Join(state.workIDs, ", ") + ")"
	}
	if state.workerID != "" && state.dispatchID != "" {
		line += " [dispatch " + boundedHumanProgressPayload(state.dispatchID) + "]"
	}
	return line
}

func (renderer *humanWorkerProgressRenderer) clearLocked() {
	if renderer.drawn {
		_, _ = fmt.Fprint(renderer.output, "\r\x1b[2K")
		renderer.drawn = false
	}
}

func humanWorkerProgressIdentity(event interfaces.FactoryEvent) string {
	if dispatchID := humanWorkerProgressDispatchID(event); dispatchID != "" {
		return dispatchID
	}
	if event.Type == interfaces.FactoryEventTypeDispatchRequest {
		payload, ok := decodeFactoryEventPayload[interfaces.DispatchRequestEventPayload](event)
		if ok {
			return strings.TrimSpace(payload.TransitionID)
		}
	}
	return ""
}

func humanWorkerProgressDispatchID(event interfaces.FactoryEvent) string {
	return strings.TrimSpace(stringPointerValue(event.Context.DispatchID))
}
