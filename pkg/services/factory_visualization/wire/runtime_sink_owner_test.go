package wire

import (
	"strings"
	"testing"

	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
)

func TestRuntimeSinkOwnerRegistersLooksUpAndClosesTypedSink(t *testing.T) {
	owner := NewRuntimeSinkOwner()
	first := factoryvisualization.SinkFunc(func(factoryvisualization.View) {})
	second := factoryvisualization.SinkFunc(func(factoryvisualization.View) {})

	firstID, err := owner.RegisterRuntimeSink(first)
	if err != nil {
		t.Fatalf("RegisterRuntimeSink(first): %v", err)
	}
	secondID, err := owner.RegisterRuntimeSink(second)
	if err != nil {
		t.Fatalf("RegisterRuntimeSink(second): %v", err)
	}
	if firstID == secondID || firstID == "" || secondID == "" {
		t.Fatalf("sink IDs = %q and %q, want distinct non-empty IDs", firstID, secondID)
	}
	if got, ok := owner.RuntimeSink(firstID); !ok || got == nil {
		t.Fatal("first typed sink was not retained")
	}
	owner.CloseRuntimeSink(firstID)
	if _, ok := owner.RuntimeSink(firstID); ok {
		t.Fatal("closed sink remained addressable")
	}
	if _, ok := owner.RuntimeSink(secondID); !ok {
		t.Fatal("closing first sink removed second sink")
	}
}

func TestRuntimeSinkOwnerRejectsNilSink(t *testing.T) {
	owner := NewRuntimeSinkOwner()
	if _, err := owner.RegisterRuntimeSink(nil); err == nil || !strings.Contains(err.Error(), "sink is required") {
		t.Fatalf("RegisterRuntimeSink(nil) error = %v, want required-sink error", err)
	}
}
