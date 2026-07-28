package cliversion_test

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/cliversion"
)

func TestStringReturnsNonEmptyMachineReadableVersion(t *testing.T) {
	version := cliversion.String()
	if strings.TrimSpace(version) == "" {
		t.Fatal("cliversion.String() returned empty version")
	}
	if strings.Contains(version, "\n") {
		t.Fatalf("cliversion.String() = %q, want single-line token", version)
	}
}
