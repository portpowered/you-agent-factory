package wire

import "testing"

func TestNewServiceConstructsProjectionQueryCapability(t *testing.T) {
	t.Parallel()

	if service := NewService(); service == nil {
		t.Fatal("NewService() = nil")
	}
}
