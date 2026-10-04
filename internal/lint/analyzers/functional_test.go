package analyzers

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestFunctionalCompilerBoundaries(t *testing.T) {
	useFixtures(t)
	dir, cleanup, err := analysistest.WriteFiles(map[string]string{
		"m/pkg/root/root.go":                    "package root",
		"m/pkg/services/b/sub/sub.go":           "package sub",
		"m/pkg/services/work/work.go":           "package work",
		"m/pkg/services/providers/wire/wire.go": "package wire",
		"m/tests/functional/providers/example/escape.go": `package example
import (
 _ "m/pkg/root" // want "functional-provider-boundary:"
 _ "m/pkg/services/b/sub" // want "functional-provider-boundary:"
 _ "m/pkg/services/work"
 _ "m/pkg/services/providers/wire" // want
)
`,
		"m/tests/functional/providers/support/support.go":  "package support // want `functional-provider-support:`",
		"m/tests/functional/internal/support/canonical.go": "package support; import _ \"m/pkg/root\"",
		"m/tests/functional/composition/config.go": `package composition
type config struct { Configure, ConfigureEdges, ConfigureRuntime, Arguments string }
var _ = config{
 Configure: "", // want "functional-configuration:"
 ConfigureEdges: "", // want "functional-configuration:"
 ConfigureRuntime: "", // want "functional-configuration:"
 Arguments: "customer flags",
}
`,
		"m/tests/integration/composition/config.go": "package composition; var _ = struct{ Configure string }{Configure: \"\"}",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)
	analysistest.Run(t, dir, Layering, "m/tests/functional/providers/example", "m/tests/functional/providers/support", "m/tests/functional/internal/support")
	analysistest.Run(t, dir, Behavior, "m/tests/functional/composition", "m/tests/integration/composition")
}

func TestFunctionalProviderExactPorts(t *testing.T) {
	for path := range functionalProviderPorts {
		if violatesFunctionalProvider(edge{importer: "tests/functional/providers/example", importee: path}) {
			t.Errorf("public port rejected: %s", path)
		}
		if !violatesFunctionalProvider(edge{importer: "tests/functional/providers/example", importee: path + "/private"}) && !containsSegment(path, "internal") {
			t.Errorf("port descendant allowed: %s", path)
		}
	}
}
