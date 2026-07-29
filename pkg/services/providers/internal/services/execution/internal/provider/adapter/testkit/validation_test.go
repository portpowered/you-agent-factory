package testkit

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/provider/adapter"
)

func TestValidateFullStreamFixture(t *testing.T) {
	valid := FullStreamFixture{
		NewAdapter:          func() adapter.Adapter { return nil },
		ContentAndTools:     []adapter.Observation{{}},
		RetryableFailure:    []adapter.Observation{{}},
		UnsafeAndRecovering: []adapter.Observation{{}},
		UnterminatedFinal:   []adapter.Observation{{}},
	}
	if err := validateFullStreamFixture(valid); err != nil {
		t.Fatalf("validate valid fixture: %v", err)
	}
	valid.NewAdapter = nil
	if err := validateFullStreamFixture(valid); err == nil {
		t.Fatal("validate fixture without adapter = nil, want error")
	}
	valid.NewAdapter = func() adapter.Adapter { return nil }
	valid.ContentAndTools = nil
	if err := validateFullStreamFixture(valid); err == nil {
		t.Fatal("validate fixture without observations = nil, want error")
	}
}

func TestValidateFinalOnlyFixture(t *testing.T) {
	valid := FinalOnlyFixture{
		NewAdapter: func() adapter.Adapter { return nil },
		Failures:   []FinalOnlyFailureCase{{}, {}},
		Expected:   FinalOnlyExpected{Content: "completed"},
	}
	if err := validateFinalOnlyFixture(valid); err != nil {
		t.Fatalf("validate valid fixture: %v", err)
	}
	valid.Expected.Content = " "
	if err := validateFinalOnlyFixture(valid); err == nil {
		t.Fatal("validate fixture without content = nil, want error")
	}
	valid.Expected.Content = "completed"
	valid.Failures = valid.Failures[:1]
	if err := validateFinalOnlyFixture(valid); err == nil {
		t.Fatal("validate fixture without failure cases = nil, want error")
	}
}

func TestValidateSafeText(t *testing.T) {
	if err := validateSafeText("bounded diagnostic", []string{"secret"}); err != nil {
		t.Fatalf("validate safe text: %v", err)
	}
	if err := validateSafeText(strings.Repeat("x", maximumDiagnosticLength+1), nil); err == nil {
		t.Fatal("validate oversized text = nil, want error")
	}
	if err := validateSafeText("leaked secret", []string{"", "secret"}); err == nil {
		t.Fatal("validate secret-bearing text = nil, want error")
	}
}

func TestFindDraftReturnsNilWhenKindAndPhaseAreMissing(t *testing.T) {
	if got := findDraft(nil, "MESSAGE", "COMPLETED"); got != nil {
		t.Fatalf("findDraft() = %#v, want nil", got)
	}
}
