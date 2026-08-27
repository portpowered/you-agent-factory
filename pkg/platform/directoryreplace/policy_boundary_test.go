package directoryreplace

import (
	"os"
	"strings"
	"testing"
)

func TestPlatformDirectoryReplacementContainsNoFactoryPolicy(t *testing.T) {
	t.Parallel()

	source, err := os.ReadFile("replace.go")
	if err != nil {
		t.Fatalf("read replace.go: %v", err)
	}
	if strings.Contains(strings.ToLower(string(source)), "factory") {
		t.Fatal("Platform directory replacement must remain product-policy free; Factory diagnostics belong to Factory Definitions")
	}
}
