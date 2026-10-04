package support

import (
	"os"
	"path/filepath"
	"testing"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	globalconfigmapping "github.com/portpowered/infinite-you/pkg/services/operator_settings/transports/globalconfig"
)

// SeedACPAgentProfile authors an isolated fixture before the scenario starts.
// The scenario's BuildProcess-composed Settings owner reads this real document.
func SeedACPAgentProfile(t testing.TB, home, defaultTarget string, allowedTargets []string) {
	t.Helper()
	configPath := operatorsettings.DefaultConfigPath(home)
	config := operatorsettings.Config{}
	data, err := os.ReadFile(configPath)
	if err == nil {
		config, err = globalconfigmapping.Decode(data)
	} else if os.IsNotExist(err) {
		err = nil
	}
	if err != nil {
		t.Fatalf("load ACP profile fixture: %v", err)
	}
	profile, err := (operatorsettings.ACPAgentProfile{
		DefaultTarget: defaultTarget, AllowedTargets: allowedTargets,
	}).Normalize()
	if err != nil {
		t.Fatalf("normalize ACP profile fixture: %v", err)
	}
	config.Workers.ACP.AgentProfile = &profile
	data, err = globalconfigmapping.Encode(config)
	if err != nil {
		t.Fatalf("encode ACP profile fixture: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("create ACP profile fixture directory: %v", err)
	}
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		t.Fatalf("write ACP profile fixture: %v", err)
	}
}
