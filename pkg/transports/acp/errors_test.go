package acp_test

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/acp"
)

func TestCompatibilityErrorImplementsError(t *testing.T) {
	err := &acp.CompatibilityError{
		Code:    acp.CompatibilityErrorUnsupportedProtocolVersion,
		Message: "unsupported protocol version",
	}
	if err.Error() != "unsupported protocol version" {
		t.Fatalf("Error() = %q, want %q", err.Error(), "unsupported protocol version")
	}
	var nilErr *acp.CompatibilityError
	if nilErr.Error() != "" {
		t.Fatalf("nil *CompatibilityError.Error() = %q, want empty string", nilErr.Error())
	}
}

func TestRequestIdentityErrorImplementsError(t *testing.T) {
	err := &acp.RequestIdentityError{
		Code:    acp.RequestIdentityErrorBlankConnectionID,
		Message: "blank connection id",
	}
	if err.Error() != "blank connection id" {
		t.Fatalf("Error() = %q, want %q", err.Error(), "blank connection id")
	}
	var nilErr *acp.RequestIdentityError
	if nilErr.Error() != "" {
		t.Fatalf("nil *RequestIdentityError.Error() = %q, want empty string", nilErr.Error())
	}
}
