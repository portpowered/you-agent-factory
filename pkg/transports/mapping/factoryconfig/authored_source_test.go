package factoryconfig

import (
	"strings"
	"testing"
)

func TestDecodeAuthoredFactoryAPIDecodesSuppliedBytes(t *testing.T) {
	factory, err := DecodeAuthoredFactoryAPI([]byte(`{"name":"authored"}`))
	if err != nil {
		t.Fatalf("DecodeAuthoredFactoryAPI() error = %v", err)
	}
	if factory.Name != "authored" {
		t.Fatalf("Name = %q, want authored", factory.Name)
	}
}

func TestDecodeAuthoredFactoryAPIReportsMalformedRepresentation(t *testing.T) {
	_, err := DecodeAuthoredFactoryAPI([]byte(`{"name":`))
	if err == nil || !strings.Contains(err.Error(), "parse factory config") {
		t.Fatalf("DecodeAuthoredFactoryAPI() error = %v", err)
	}
}
