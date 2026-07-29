package cli

import (
	"fmt"
	"io"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

// FactoryEventRendererConfig carries CLI inputs for human or JSON Factory Event
// stream presentation.
type FactoryEventRendererConfig struct {
	Output               io.Writer
	JSON                 bool
	Color                bool
	InvocationOutputMode string
}

// FactoryEventRenderer presents accepted Factory Events then one terminal
// invocation result through Visualization-owned presentation.
type FactoryEventRenderer interface {
	PresentFactoryEvents([]interfaces.FactoryEvent)
	StopProgressRendering()
	WriteFinalInvocationResult(apisurface.FactoryInvocationResult) error
}

// OpenFactoryEventRenderer opens a human or JSON Factory Event renderer when
// response-stream output mode is selected. Returns nil without error when
// another output mode is active (accepted no-output outcome).
func (s *service) OpenFactoryEventRenderer(cfg FactoryEventRendererConfig) (FactoryEventRenderer, error) {
	if !isResponseStreamOutputMode(cfg.InvocationOutputMode) {
		return nil, nil
	}
	if cfg.Output == nil {
		return nil, fmt.Errorf("Factory Event output writer is required")
	}
	if cfg.JSON {
		return newJSONFactoryEventRenderer(cfg.Output, s.presentation), nil
	}
	return newHumanFactoryEventRenderer(cfg.Output, s.presentation, cfg.Color), nil
}

func isResponseStreamOutputMode(mode string) bool {
	return mode == InvocationOutputResponseStream
}

type factoryEventStream interface {
	PresentFactoryEvents([]interfaces.FactoryEvent)
	Finalize(factoryvisualization.FinalResponseWriter) (bool, error)
	CloseAndDrain() error
}

type humanFactoryEventRenderer struct {
	stream factoryEventStream
}

func newHumanFactoryEventRenderer(
	output io.Writer,
	presentation factoryvisualization.ResponsePresentation,
	color bool,
) *humanFactoryEventRenderer {
	formatter := formatHumanFactoryEvent
	if color {
		formatter = formatColorHumanFactoryEvent
	}
	return &humanFactoryEventRenderer{stream: presentation.OpenBestEffortFactoryEventStream(
		output,
		formatter,
	)}
}

func (renderer *humanFactoryEventRenderer) PresentFactoryEvents(events []interfaces.FactoryEvent) {
	if renderer != nil {
		renderer.stream.PresentFactoryEvents(events)
	}
}

func (renderer *humanFactoryEventRenderer) StopProgressRendering() {
	if renderer != nil {
		_ = renderer.stream.CloseAndDrain()
	}
}

func (renderer *humanFactoryEventRenderer) WriteFinalInvocationResult(
	result apisurface.FactoryInvocationResult,
) error {
	if renderer == nil {
		return fmt.Errorf("Factory Event renderer is nil")
	}
	_, err := renderer.stream.Finalize(func(writer io.Writer, progressSeen bool) error {
		if result.Status == interfaces.InvocationTerminalStatusCompleted {
			text, textErr := invocationPrimaryResultText(result.PrimaryResult)
			if textErr != nil {
				return textErr
			}
			return writeHumanPrimaryResult(writer, progressSeen, text)
		}
		return writeHumanInvocationOutcome(writer, progressSeen, result)
	})
	return err
}

type jsonFactoryEventRenderer struct {
	stream factoryEventStream
}

func newJSONFactoryEventRenderer(
	output io.Writer,
	presentation factoryvisualization.ResponsePresentation,
) *jsonFactoryEventRenderer {
	return &jsonFactoryEventRenderer{stream: presentation.OpenLosslessFactoryEventStream(
		output,
		func(event interfaces.FactoryEvent) ([]byte, bool) {
			event, ok := factoryEventForPublicPresentation(event)
			if !ok {
				return nil, false
			}
			encoded, err := jsonMarshalFactoryEventRecord(event)
			return encoded, err == nil
		},
	)}
}

func (renderer *jsonFactoryEventRenderer) PresentFactoryEvents(events []interfaces.FactoryEvent) {
	if renderer != nil {
		renderer.stream.PresentFactoryEvents(events)
	}
}

func (renderer *jsonFactoryEventRenderer) StopProgressRendering() {
	if renderer != nil {
		_ = renderer.stream.CloseAndDrain()
	}
}

func (renderer *jsonFactoryEventRenderer) WriteFinalInvocationResult(
	result apisurface.FactoryInvocationResult,
) error {
	if renderer == nil {
		return fmt.Errorf("Factory Event renderer is nil")
	}
	first, err := renderer.stream.Finalize(func(writer io.Writer, _ bool) error {
		return writeJSONInvocationResultRecord(writer, result)
	})
	if !first {
		return fmt.Errorf("Factory Event invocation result already written")
	}
	return err
}
