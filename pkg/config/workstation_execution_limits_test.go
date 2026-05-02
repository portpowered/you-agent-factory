package config

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestNormalizeWorkstationExecutionLimit_MovesLegacyTimeoutIntoCanonicalLimit(t *testing.T) {
	cfg := &interfaces.FactoryWorkstationConfig{
		Timeout: "45m",
	}

	NormalizeWorkstationExecutionLimit(cfg)

	if cfg.Limits.MaxExecutionTime != "45m" {
		t.Fatalf("MaxExecutionTime = %q, want %q", cfg.Limits.MaxExecutionTime, "45m")
	}
	if cfg.Timeout != "" {
		t.Fatalf("Timeout = %q, want empty string", cfg.Timeout)
	}
}

func TestWorkstationExecutionTimeout_UsesCanonicalLimitOnly(t *testing.T) {
	cfg := &interfaces.FactoryWorkstationConfig{
		Timeout: "45m",
	}

	timeout, err := WorkstationExecutionTimeout(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if timeout != 0 {
		t.Fatalf("timeout = %v, want 0", timeout)
	}
}

func TestWorkstationExecutionTimeout_ParsesCanonicalLimit(t *testing.T) {
	cfg := &interfaces.FactoryWorkstationConfig{
		Limits: interfaces.WorkstationLimits{MaxExecutionTime: "45m"},
	}

	timeout, err := WorkstationExecutionTimeout(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if timeout != 45*time.Minute {
		t.Fatalf("timeout = %v, want %v", timeout, 45*time.Minute)
	}
}

func TestWorkstationExecutionTimeout_ReturnsCanonicalParseError(t *testing.T) {
	cfg := &interfaces.FactoryWorkstationConfig{
		Limits:  interfaces.WorkstationLimits{MaxExecutionTime: "not-a-duration"},
		Timeout: "45m",
	}

	_, err := WorkstationExecutionTimeout(cfg)
	if err == nil {
		t.Fatal("expected error")
	}
	if got, want := err.Error(), `invalid workstation limits.maxExecutionTime "not-a-duration": time: invalid duration "not-a-duration"`; got != want {
		t.Fatalf("error = %q, want %q", got, want)
	}
}
