package cli

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

const humanWorkerProgressInterval = 120 * time.Millisecond

var humanWorkerSpinnerFrames = [...]string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// humanWorkerProgressRenderer owns the stderr-only, TTY-only worker spinner.
// Its state is independent from the response stream so JSON/NDJSON output
// remains byte-for-byte owned by the stream writer.
type humanWorkerProgressRenderer struct {
	output  io.Writer
	ticks   <-chan time.Time
	ticker  *time.Ticker
	stop    chan struct{}
	done    chan struct{}
	once    sync.Once
	mu      sync.Mutex
	active  map[string]struct{}
	frame   int
	drawn   bool
	stopped bool
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
	renderer.active = make(map[string]struct{})
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
		identity := humanWorkerProgressIdentity(event)
		if identity == "" {
			continue
		}
		switch event.Type {
		case interfaces.FactoryEventTypeDispatchRequest:
			renderer.active[identity] = struct{}{}
		case interfaces.FactoryEventTypeDispatchResponse, interfaces.FactoryEventTypeDispatchInterrupted:
			delete(renderer.active, identity)
		}
	}
	renderer.drawLocked()
	renderer.mu.Unlock()
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
		frame := humanWorkerSpinnerFrames[(renderer.frame+index)%len(humanWorkerSpinnerFrames)]
		glyphs = append(glyphs, "\x1b["+stableWorkstationColor(identity)+"m"+frame+"\x1b[0m")
	}
	_, _ = fmt.Fprint(renderer.output, "\r\x1b[2K"+strings.Join(glyphs, " "))
	renderer.drawn = true
}

func (renderer *humanWorkerProgressRenderer) clearLocked() {
	if renderer.drawn {
		_, _ = fmt.Fprint(renderer.output, "\r\x1b[2K")
		renderer.drawn = false
	}
}

func humanWorkerProgressIdentity(event interfaces.FactoryEvent) string {
	if event.Context.DispatchID != nil && strings.TrimSpace(*event.Context.DispatchID) != "" {
		return *event.Context.DispatchID
	}
	if event.Type == interfaces.FactoryEventTypeDispatchRequest {
		payload, ok := decodeFactoryEventPayload[interfaces.DispatchRequestEventPayload](event)
		if ok {
			return strings.TrimSpace(payload.TransitionID)
		}
	}
	return ""
}
