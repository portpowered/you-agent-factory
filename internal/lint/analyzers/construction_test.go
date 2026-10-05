package analyzers

import (
	"fmt"
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
	useFixtures(t, "service-construction|pkg/listed|pkg/services/b.NewThing", "service-construction|pkg/ctorstale|pkg/services/b.NewThing",
		"construction-recorded-site|pkg/listed|pkg/listed/l.go#service-construction#pkg/services/b.NewThing::count=1")
	analysistest.Run(t, analysistest.TestData(), Construction, "m/pkg/listed", "m/pkg/ctorstale")
}
func TestConstructionDefersTaggedStale(t *testing.T) {
	useFixtures(t, "service-construction|pkg/tagged|pkg/services/b.NewThing",
		"construction-recorded-site|pkg/tagged|pkg/tagged/debt.go#service-construction#pkg/services/b.NewThing::count=1")
	old := Construction.Flags.Lookup("check-stale").Value.String()
	t.Cleanup(func() { _ = Construction.Flags.Set("check-stale", old) })
	_ = Construction.Flags.Set("check-stale", "false")
	analysistest.Run(t, analysistest.TestData(), Construction, "m/pkg/tagged")
}

func TestConstructionTaggedDebtStrict(t *testing.T) {
	useFixtures(t, "service-construction|pkg/tagged|pkg/services/b.NewThing",
		"construction-recorded-site|pkg/tagged|pkg/tagged/debt.go#service-construction#pkg/services/b.NewThing::count=1")
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
		"construction-recorded-site|pkg/consumer|pkg/consumer/c.go#service-construction-callable#pkg/services/callable.NewCallback::count=1",
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

func TestConstructionRecordedSites(t *testing.T) {
	for _, tc := range []struct {
		name, source, filename string
	}{
		{"exact", "var captured = b.NewThing", "a.go"},
		{"line motion", "\n\n\nvar captured = b.NewThing", "a.go"},
		{"extra", `var captured = b.NewThing // want "construction-recorded-site:.*count=2"
var another = b.NewThing`, "a.go"},
		{"neighbor", `var captured = b.NewThing // want "construction-recorded-site:.*b.go"`, "b.go"},
		{"removed", "var _ = b.Unrelated", "a.go"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const unit = "pkg/sites"
			useFixtures(t, "service-construction|"+unit+"|pkg/services/b.NewThing",
				"construction-recorded-site|"+unit+"|"+unit+"/a.go#service-construction#pkg/services/b.NewThing::count=1")
			pkg := "package sites"
			if tc.name == "extra" || tc.name == "neighbor" || tc.name == "removed" {
				pkg += ` // want "stale baseline entry.*construction-recorded-site"`
			}
			if tc.name == "removed" {
				pkg += ` "stale baseline entry.*service-construction"`
			}
			files := map[string]string{
				"m/pkg/services/b/b.go":         "package b\nfunc NewThing() {}\nvar Unrelated = 1\n",
				"m/" + unit + "/" + tc.filename: fmt.Sprintf("%s\nimport b \"m/pkg/services/b\"\n%s\n", pkg, tc.source),
			}
			dir, cleanup, err := analysistest.WriteFiles(files)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(cleanup)
			analysistest.Run(t, dir, Construction, "m/"+unit)
		})
	}
}

func TestConstructionRecordedTestSites(t *testing.T) {
	for _, external := range []bool{false, true} {
		t.Run(fmt.Sprint(external), func(t *testing.T) {
			unit, pkg := "pkg/sites", "sites"
			if external {
				unit += "_test"
				pkg += "_test"
			}
			useFixtures(t, "service-construction-test|"+unit+"|pkg/services/b.NewThing",
				"construction-recorded-site-test|"+unit+"|pkg/sites/a_test.go#service-construction-test#pkg/services/b.NewThing::count=1")
			testPackage := "package " + pkg + ` // want "stale baseline entry.*construction-recorded-site-test"`
			files := map[string]string{
				"m/pkg/services/b/b.go": "package b\nfunc NewThing() {}\n",
				"m/pkg/sites/a_test.go": testPackage + `
import b "m/pkg/services/b"
var captured = b.NewThing // want "construction-recorded-site-test:.*count=2"
var another = b.NewThing
`,
			}
			if external {
				files["m/pkg/sites/a.go"] = "package sites\n"
			}
			dir, cleanup, err := analysistest.WriteFiles(files)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(cleanup)
			analysistest.Run(t, dir, Construction, "m/pkg/sites")
		})
	}
}

func TestConstructionRecordedInactiveSources(t *testing.T) {
	useFixtures(t,
		"service-construction|pkg/sites|pkg/services/b.NewThing",
		"construction-recorded-site|pkg/sites|pkg/sites/a.go#service-construction#pkg/services/b.NewThing::count=1",
		"construction-recorded-site|pkg/sites|pkg/sites/tagged.go#service-construction#pkg/services/b.NewThing::count=1")
	files := map[string]string{
		"m/pkg/services/b/b.go": "package b\nfunc NewThing() {}\n",
		"m/pkg/sites/a.go":      "package sites\nimport b \"m/pkg/services/b\"\nvar captured = b.NewThing\n",
		"m/pkg/sites/tagged.go": "//go:build boundaryinactive\n\npackage sites\nimport b \"m/pkg/services/b\"\nvar tagged = b.NewThing\n",
	}
	dir, cleanup, err := analysistest.WriteFiles(files)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)
	analysistest.Run(t, dir, Construction, "m/pkg/sites")
}
