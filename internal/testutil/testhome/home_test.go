package testhome

import (
	"os"
	"testing"
)

func TestIsolateHomeRedirectsProfileVariables(t *testing.T) {
	real, _ := os.UserHomeDir()
	home := IsolateHome(t)
	got, err := os.UserHomeDir()
	if err != nil || got != home || got == real {
		t.Fatalf("UserHomeDir = %q, %v; want isolated %q (real %q)", got, err, home, real)
	}
}
