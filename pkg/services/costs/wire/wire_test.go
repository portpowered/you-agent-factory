package wire

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	costs "github.com/portpowered/infinite-you/pkg/services/costs"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

func TestNewCostsQueryUsesNarrowDependencies(t *testing.T) {
	t.Parallel()

	query, err := NewCostsQuery(
		providers.PriceTableReaderFunc(func() (providers.PriceTable, error) {
			return providers.PriceTable{Currency: providers.PriceTableCurrencyUSD, Models: []providers.PriceTableModel{}}, nil
		}),
		operatorSettingsStub{},
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

type operatorSettingsStub struct {
	operatorsettings.Service
}

func (operatorSettingsStub) LoadFileConfig(string) (operatorsettings.Config, error) {
	return operatorsettings.Config{}, nil
}
