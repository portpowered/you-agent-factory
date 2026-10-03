package cli

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"golang.org/x/text/width"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

const humanWorkerProgressInterval = 120 * time.Millisecond

var humanWorkerSpinnerFrames = [...]string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// humanWorkerProgressRenderer owns stderr-only worker progress. Interactive
// terminals receive a transient spinner; redirected output uses only the
// response stream milestones. Its state is independent from the response stream so
// JSON/NDJSON output remains byte-for-byte owned by the stream writer.
type humanWorkerProgressRenderer struct {
	output      io.Writer
	columns     func() int
	interactive bool
	ticks       <-chan time.Time
	ticker      *time.Ticker
	stop        chan struct{}
	done        chan struct{}
	once        sync.Once
	mu          sync.Mutex
	active      map[string]humanWorkerProgressState
	pending     map[string]humanWorkerProgressState
	frame       int
	drawn       bool
	stopped     bool
}

type humanWorkerProgressState struct {
	workstation string
}

func newHumanWorkerProgressRenderer(output io.Writer, isTTY bool, ticks <-chan time.Time, columns func() int) *humanWorkerProgressRenderer {
	renderer := &humanWorkerProgressRenderer{output: output, interactive: isTTY, columns: columns}
	if output == nil || !isTTY {
		renderer.output = nil
		return renderer
	}
	renderer.active = make(map[string]humanWorkerProgressState)
	renderer.pending = make(map[string]humanWorkerProgressState)
	if ticks == nil {
		ticker := time.NewTicker(humanWorkerProgressInterval)
		ticks = ticker.C
		renderer.ticker = ticker
	}
	renderer.ticks = ticks
	renderer.stop = make(chan struct{})
	renderer.done = make(chan struct{})
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
		if !renderer.interactive {
			renderer.drawLocked()
		}
	}
	if renderer.interactive {
		renderer.drawLocked()
	}
	renderer.mu.Unlock()
}

func (renderer *humanWorkerProgressRenderer) applyEventLocked(event interfaces.FactoryEvent) {
	switch event.Type {
	case interfaces.FactoryEventTypeDispatchQueued:
		renderer.applyDispatchQueuedLocked(event)
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
	payload, ok := decodeFactoryEventPayload[interfaces.DispatchQueuedEventPayload](event)
	if ok {
		state.workstation = boundedHumanProgressPayload(stringPointerValue(payload.Label))
	}
	renderer.pending[dispatchID] = state
	if active, exists := renderer.active[dispatchID]; exists {
		if state.workstation != "" {
			active.workstation = state.workstation
		}
		renderer.active[dispatchID] = active
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
			if state.workstation == "" {
				state.workstation = pending.workstation
			}
			delete(renderer.pending, dispatchID)
		}
	}
	payload, ok := decodeFactoryEventPayload[interfaces.DispatchRequestEventPayload](event)
	if ok {
		if workstation := boundedHumanProgressPayload(payload.TransitionID); workstation != "" {
			state.workstation = workstation
		}
	}
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
}

func (renderer *humanWorkerProgressRenderer) Stop() {
	if renderer == nil || renderer.output == nil {
		return
	}
	renderer.once.Do(func() {
		if renderer.stop == nil {
			renderer.mu.Lock()
			renderer.stopped = true
			renderer.active = nil
			renderer.pending = nil
			renderer.mu.Unlock()
			return
		}
		if renderer.ticker != nil {
			renderer.ticker.Stop()
		}
		close(renderer.stop)
		<-renderer.done
	})
}

func (renderer *humanWorkerProgressRenderer) run() {
	defer close(renderer.done)
	defer func() {
		renderer.mu.Lock()
		defer renderer.mu.Unlock()
		renderer.stopped = true
		renderer.active = nil
		renderer.pending = nil
		renderer.clearLocked()
	}()
	for {
		select {
		case <-renderer.stop:
			return
		case _, open := <-renderer.ticks:
			if !open {
				return
			}
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
	if !renderer.interactive {
		return
	}
	if len(renderer.active) == 0 {
		renderer.clearLocked()
		return
	}
	columns := 80
	if renderer.columns != nil {
		if observed := renderer.columns(); observed > 0 {
			columns = observed
		}
	}
	budget := columns - 1 // Never write the final column: terminals may wrap there.
	if budget < 1 {
		renderer.clearLocked()
		return
	}
	identities := make([]string, 0, len(renderer.active))
	for identity := range renderer.active {
		identities = append(identities, identity)
	}
	sort.Strings(identities)
	line := humanWorkerSpinnerFrames[renderer.frame] + " "
	labels := make([]string, 0, len(identities))
	for _, identity := range identities {
		state := renderer.active[identity]
		label := state.workstation
		if label == "" {
			label = "worker"
		}
		labels = append(labels, label)
	}
	line += strings.Join(labels, ", ")
	if len(identities) > 1 {
		line = humanWorkerSpinnerFrames[renderer.frame] + fmt.Sprintf(" %d workers: ", len(identities)) + strings.Join(labels, ", ")
	}
	_, _ = fmt.Fprint(renderer.output, "\r\x1b[2K"+fitHumanProgressColumns(line, budget))
	renderer.drawn = true
}

func fitHumanProgressColumns(text string, columns int) string {
	used := 0
	var result strings.Builder
	for _, r := range text {
		cells := 1
		if unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) {
			cells = 0
		}
		kind := width.LookupRune(r).Kind()
		if kind == width.EastAsianWide || kind == width.EastAsianFullwidth {
			cells = 2
		}
		if used+cells > columns {
			return result.String()
		}
		result.WriteRune(r)
		used += cells
	}
	return result.String()
}

// Each stream write interrupts the transient line under the same lock used by
// event updates and animation. The response stream remains the record owner.
type humanProgressOutput struct {
	renderer *humanWorkerProgressRenderer
	output   io.Writer
}

func (writer humanProgressOutput) Write(data []byte) (int, error) {
	renderer := writer.renderer
	renderer.mu.Lock()
	defer renderer.mu.Unlock()
	renderer.clearLocked()
	n, err := writer.output.Write(data)
	if !renderer.stopped && renderer.interactive {
		renderer.drawLocked()
	}
	return n, err
}

func (renderer *humanWorkerProgressRenderer) clearLocked() {
	if !renderer.interactive {
		return
	}
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
