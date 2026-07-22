package service

import (
	"strings"
	"testing"
)

func TestNewAssemblyRequiresWireConstructedRuntimeFactory(t *testing.T) {
	assembly, err := NewAssembly(nil)
	if err == nil || !strings.Contains(err.Error(), "Factory Runtime factory is required") {
		t.Fatalf("NewAssembly(nil) error = %v, want required dependency", err)
	}
	if assembly != nil {
		t.Fatalf("NewAssembly(nil) = %#v, want nil assembly", assembly)
	}
}

func TestNewAssemblyBindsRuntimeFactory(t *testing.T) {
	runtimeFactory := &RuntimeFactory{}
	assembly, err := NewAssembly(runtimeFactory)
	if err != nil {
		t.Fatalf("NewAssembly() error = %v", err)
	}
	if assembly == nil || assembly.runtimeFactory != runtimeFactory {
		t.Fatalf("NewAssembly() = %#v, want supplied Runtime Factory", assembly)
	}
}
