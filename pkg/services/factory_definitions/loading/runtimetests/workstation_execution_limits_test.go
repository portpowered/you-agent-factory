package runtimetests

import (
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/workstationexecution"
)

func TestNormalizeWorkstationExecutionLimit_MovesLegacyTimeoutIntoCanonicalLimit(t *testing.T) {
	cfg := &factorydefinitions.FactoryWorkstationConfig{
		Timeout: "45m",
	}

	workstationexecution.NormalizeExecutionLimit(cfg)

	if cfg.Limits.MaxExecutionTime != "45m" {
		t.Fatalf("MaxExecutionTime = %q, want %q", cfg.Limits.MaxExecutionTime, "45m")
	}
	if cfg.Timeout != "" {
		t.Fatalf("Timeout = %q, want empty string", cfg.Timeout)
	}
}

func TestWorkstationExecutionTimeout_UsesCanonicalLimitOnly(t *testing.T) {
	cfg := &factorydefinitions.FactoryWorkstationConfig{
		Timeout: "45m",
	}

	timeout, err := workstationexecution.NewService().ExecutionTimeout(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if timeout != 0 {
		t.Fatalf("timeout = %v, want 0", timeout)
	}
}

func TestWorkstationExecutionTimeout_ParsesCanonicalLimit(t *testing.T) {
	cfg := &factorydefinitions.FactoryWorkstationConfig{
		Limits: factorydefinitions.WorkstationLimits{MaxExecutionTime: "45m"},
	}

	timeout, err := workstationexecution.NewService().ExecutionTimeout(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if timeout != 45*time.Minute {
		t.Fatalf("timeout = %v, want %v", timeout, 45*time.Minute)
	}
}

func TestWorkstationExecutionTimeout_ReturnsCanonicalParseError(t *testing.T) {
	cfg := &factorydefinitions.FactoryWorkstationConfig{
		Limits:  factorydefinitions.WorkstationLimits{MaxExecutionTime: "not-a-duration"},
		Timeout: "45m",
	}

	_, err := workstationexecution.NewService().ExecutionTimeout(cfg)
	if err == nil {
		t.Fatal("expected error")
	}
	if got, want := err.Error(), `invalid workstation limits.maxExecutionTime "not-a-duration": time: invalid duration "not-a-duration"`; got != want {
		t.Fatalf("error = %q, want %q", got, want)
	}
}
