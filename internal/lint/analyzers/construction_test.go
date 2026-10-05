package analyzers

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// Fixtures override analyzer globals, so these tests must remain serialized.
func TestConstructionOwnerAndQualifiedUses(t *testing.T) {
	useFixtures(t)
	analysistest.Run(t, analysistest.TestData(), Construction,
		"m/pkg/services/b", "m/pkg/wire", "m/pkg/transports/http", "m/pkg/services/workers",
		"m/pkg/consumer", "m/pkg/dot", "m/pkg/external")
}
func TestConstructionExactDebtAndStale(t *testing.T) {
	useFixtures(t, "service-construction|pkg/listed|pkg/services/b.NewThing", "service-construction|pkg/ctorstale|pkg/services/b.NewThing")
	analysistest.Run(t, analysistest.TestData(), Construction, "m/pkg/listed", "m/pkg/ctorstale")
}
func TestConstructionDefersTaggedStale(t *testing.T) {
	useFixtures(t, "service-construction|pkg/tagged|pkg/services/b.NewThing")
	old := Construction.Flags.Lookup("check-stale").Value.String()
	t.Cleanup(func() { _ = Construction.Flags.Set("check-stale", old) })
	_ = Construction.Flags.Set("check-stale", "false")
	analysistest.Run(t, analysistest.TestData(), Construction, "m/pkg/tagged")
}

func TestConstructionTaggedDebtStrict(t *testing.T) {
	useFixtures(t, "service-construction|pkg/tagged|pkg/services/b.NewThing")
	t.Setenv("GOFLAGS", "-tags=integration")
	analysistest.Run(t, analysistest.TestData(), Construction, "m/pkg/tagged")
}
func TestConstructionNameBoundary(t *testing.T) {
	for _, name := range []string{"New", "NewThing", "EnsureThing", "BuildThing", "CreateThing", "OpenThing", "ProvideThing"} {
		if !serviceConstructorName(name) {
			t.Errorf("missed %s", name)
		}
	}
	for _, name := range []string{"Newthing", "Renew", "InjectThing"} {
		if serviceConstructorName(name) {
			t.Errorf("unexpected %s", name)
		}
	}
}

func TestConstructionCallableOwnership(t *testing.T) {
	useFixtures(t)
	const constructors = `package builder
var NewCallback = func() {}
type BuildValue int
var CreateScalar = 1
func Newthing() {}
`
	const consumer = `package consumer
import b "m/pkg/services/callable"
var captured = b.NewCallback // want "service-construction-callable:.*NewCallback"
var detached b.BuildValue
var scalar = b.CreateScalar
func run() {
 b.NewCallback() // same exact key as captured
 _ = b.BuildValue(1) // want "service-construction-callable:.*BuildValue"
 b.Newthing()
 local := struct { NewCallback func() }{func(){}}
 local.NewCallback()
}
`
	files := map[string]string{
		"m/pkg/services/callable/b.go": constructors,
		"m/pkg/consumer/c.go":          consumer,
		"m/pkg/dot/c.go": `package dot
import . "m/pkg/services/callable"
var captured = NewCallback // want "service-construction-callable:.*NewCallback"
var converted = (BuildValue)(1) // want "service-construction-callable:.*BuildValue"
`,
		"m/pkg/external/c.go": "package external\n",
		"m/pkg/external/c_test.go": `package external_test
import b "m/pkg/services/callable"
var captured = b.NewCallback // want "service-construction-callable-test: pkg/external_test.*NewCallback"
var converted = b.BuildValue(1) // want "service-construction-callable-test: pkg/external_test.*BuildValue"
`,
	}
	for _, unit := range []string{"pkg/wire/child", "pkg/services/callable/owner", "pkg/transports/http"} {
		owner := "pkg/services/callable"
		if unit == "pkg/transports/http" {
			owner += "/transports/http"
			files["m/"+owner+"/b.go"] = constructors
		}
		files["m/"+unit+"/c.go"] = "package allowed\nimport b \"m/" + owner + "\"\nvar captured = b.NewCallback\nvar converted = b.BuildValue(1)\n"
	}
	dir, cleanup, err := analysistest.WriteFiles(files)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)
	analysistest.Run(t, dir, Construction, "m/pkg/consumer", "m/pkg/dot", "m/pkg/external",
		"m/pkg/wire/child", "m/pkg/services/callable/owner", "m/pkg/transports/http")
}

func TestConstructionCallableDebtAndStale(t *testing.T) {
	useFixtures(t,
		"service-construction-callable|pkg/consumer|pkg/services/callable.NewCallback",
		"service-construction-callable|pkg/stale|pkg/services/callable.NewCallback")
	files := map[string]string{
		"m/pkg/services/callable/b.go": "package builder\nvar NewCallback = func() {}\n",
		"m/pkg/consumer/c.go":          "package consumer\nimport b \"m/pkg/services/callable\"\nvar captured = b.NewCallback\n",
		"m/pkg/stale/c.go":             "package stale // want \"stale baseline entry.*service-construction-callable\"\n",
	}
	dir, cleanup, err := analysistest.WriteFiles(files)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)
	analysistest.Run(t, dir, Construction, "m/pkg/consumer", "m/pkg/stale")
}
