package http

import (
	"os"
	"strings"
	"testing"
)

func TestServerConstructionBoundary_RetiredAggregateSurfaceCannotReturn(t *testing.T) {
	t.Parallel()

	source, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatalf("read server source: %v", err)
	}
	text := string(source)
	for _, retired := range []string{
		"NewServerFromSurface",
		"type Binding struct",
		"type StableDependencies struct",
		"func NewHandler(",
		"func NewStrictRoleServer(",
		"optionalDurableExecutionSessionLister",
		"legacyDurableExecutionSessionLister",
	} {
		if strings.Contains(text, retired) {
			t.Fatalf("server source contains retired aggregate construction surface %q", retired)
		}
	}
}
