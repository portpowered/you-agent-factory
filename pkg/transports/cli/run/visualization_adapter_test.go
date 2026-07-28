package run

import (
	"io"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	visualizationcli "github.com/portpowered/infinite-you/pkg/services/factory_visualization/transports/cli"
)

const responseStreamPrimaryResultHeader = "--- primary result ---"

const factoryEventJSONRecordType = "factory_event"

type factoryEventJSONRecord struct {
	RecordType string                  `json:"recordType"`
	Event      interfaces.FactoryEvent `json:"event"`
}

func openTestHumanFactoryEventRenderer(
	t *testing.T,
	output io.Writer,
	presentation factoryvisualization.ResponsePresentation,
) visualizationcli.FactoryEventRenderer {
	t.Helper()
	service := visualizationcli.NewFromPresentation(presentation)
	renderer, err := service.OpenFactoryEventRenderer(visualizationcli.FactoryEventRendererConfig{
		Output:               output,
		InvocationOutputMode: visualizationcli.InvocationOutputResponseStream,
	})
	if err != nil {
		t.Fatalf("open human Factory Event renderer: %v", err)
	}
	if renderer == nil {
		t.Fatal("renderer = nil, want human Factory Event renderer")
	}
	return renderer
}

func openTestJSONFactoryEventRenderer(
	t *testing.T,
	output io.Writer,
	presentation factoryvisualization.ResponsePresentation,
) visualizationcli.FactoryEventRenderer {
	t.Helper()
	service := visualizationcli.NewFromPresentation(presentation)
	renderer, err := service.OpenFactoryEventRenderer(visualizationcli.FactoryEventRendererConfig{
		Output:               output,
		JSON:                 true,
		InvocationOutputMode: visualizationcli.InvocationOutputResponseStream,
	})
	if err != nil {
		t.Fatalf("open JSON Factory Event renderer: %v", err)
	}
	if renderer == nil {
		t.Fatal("renderer = nil, want JSON Factory Event renderer")
	}
	return renderer
}
