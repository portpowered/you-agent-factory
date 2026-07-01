package runtimehost

import "testing"

func TestCoordinatorPolicyFromConfig_MapsFactoryDir(t *testing.T) {
	t.Parallel()

	policy := CoordinatorPolicyFromConfig(&Config{Dir: "/tmp/factory"})
	if policy.FactoryDir() != "/tmp/factory" {
		t.Fatalf("FactoryDir = %q, want /tmp/factory", policy.FactoryDir())
	}
}

func TestValidateReplayModeConfig_RejectsRecordAndReplayTogether(t *testing.T) {
	t.Parallel()

	err := ValidateReplayModeConfig(&Config{
		RecordPath: "/tmp/record.json",
		ReplayPath: "/tmp/replay.json",
	})
	if err == nil {
		t.Fatal("expected record+replay conflict error")
	}
}
