package wire

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	costs "github.com/portpowered/infinite-you/pkg/services/costs"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

func TestNewCostsQueryUsesNarrowDependencies(t *testing.T) {
	t.Parallel()

	query, err := NewCostsQuery(
		settingsReader{document: operatorsettings.Document{}},
		func(context.Context, factoryvisualization.RuntimeMetricsQueryRequest) (factoryvisualization.RuntimeMetricsQueryResult, error) {
			return factoryvisualization.RuntimeMetricsQueryResult{}, nil
		},
		logging.NoopLogger{},
	)
	if err != nil {
		t.Fatalf("NewCostsQuery() error = %v", err)
	}
	if query == nil {
		t.Fatal("NewCostsQuery() returned nil operation")
	}
	result, err := query.Query(context.Background(), costs.QueryRequest{MetricsRoot: "metrics", OperatorSettingsPath: "settings"})
	if err != nil {
		t.Fatalf("Costs query = %v", err)
	}
	if result.Status != costs.StatusNoUsage || result.Currency != "USD" {
		t.Fatalf("result = %#v, want empty USD report", result)
	}
}

type settingsReader struct {
	document operatorsettings.Document
}

func (reader settingsReader) LoadDocument(operatorsettings.LoadDocumentRequest) (operatorsettings.LoadDocumentResult, error) {
	return operatorsettings.LoadDocumentResult{Document: reader.document}, nil
}
