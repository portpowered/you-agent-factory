package runtime_metrics_test

import (
	"os"
	"path/filepath"
	"testing"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	globalconfigmapping "github.com/portpowered/infinite-you/pkg/services/operator_settings/transports/globalconfig"
)

func writeReplayOperatorPriceTable(
	t *testing.T,
	homeDir string,
	models ...operatorsettings.PriceTableModel,
) {
	t.Helper()
	payload, err := globalconfigmapping.Encode(operatorsettings.Config{
		PriceTable: operatorsettings.PriceTable{
			Currency: operatorsettings.PriceTableCurrencyUSD,
			Models:   models,
		},
	})
	if err != nil {
		t.Fatalf("encode replay operator price table: %v", err)
	}
	configPath := operatorsettings.DefaultConfigPath(homeDir)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		t.Fatalf("create replay operator settings directory: %v", err)
	}
	if err := os.WriteFile(configPath, payload, 0o600); err != nil {
		t.Fatalf("write replay operator price table: %v", err)
	}
}
