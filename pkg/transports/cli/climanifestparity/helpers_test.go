package climanifestparity_test

import (
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/portpowered/infinite-you/pkg/transports/cli"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestparity"
	"github.com/portpowered/infinite-you/pkg/transports/cli/cliinputs"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func TestFormatMismatchReport(t *testing.T) {
	if got := climanifestparity.FormatMismatchReport(nil); got != "" {
		t.Fatalf("FormatMismatchReport(nil) = %q, want empty", got)
	}
	mismatches := []climanifestparity.Mismatch{{
		CommandID: "you",
		Field:     "path",
		Want:      "you",
		Got:       "root",
	}}
	got := climanifestparity.FormatMismatchReport(mismatches)
	if !strings.Contains(got, "you path mismatch") {
		t.Fatalf("FormatMismatchReport() = %q, want readable mismatch text", got)
	}
}

func TestFindCommandByPath(t *testing.T) {
	root := cli.NewRootCommand()
	show, err := climanifestparity.FindCommandByPath(root, "you session show")
	if err != nil {
		t.Fatalf("FindCommandByPath() error = %v", err)
	}
	if show.Name() != "show" {
		t.Fatalf("show command = %q, want show", show.Name())
	}

	if _, err := climanifestparity.FindCommandByPath(root, "other session show"); err == nil {
		t.Fatal("FindCommandByPath() error = nil, want root mismatch failure")
	}
	if _, err := climanifestparity.FindCommandByPath(root, "you missing show"); err == nil {
		t.Fatal("FindCommandByPath() error = nil, want missing segment failure")
	}
}

func TestHelpArgsForPath(t *testing.T) {
	if got := climanifestparity.HelpArgsForPath("you"); !strings.EqualFold(strings.Join(got, " "), "--help") {
		t.Fatalf("HelpArgsForPath(you) = %v, want [--help]", got)
	}
	want := []string{"session", "show", "--help"}
	if got := climanifestparity.HelpArgsForPath("you session show"); strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("HelpArgsForPath(you session show) = %v, want %v", got, want)
	}
}

func TestCompareFlagDefault_MismatchPaths(t *testing.T) {
	contract := climanifest.Flag{
		Long:            "server",
		Default:         "http://127.0.0.1:8080",
		NoOptionDefault: "",
		Visibility:      "visible",
	}
	if mismatch := climanifestparity.CompareFlagDefault("you", contract, nil); mismatch == nil {
		t.Fatal("CompareFlagDefault(nil flag) mismatch = nil, want present failure")
	}

	hidden := pflag.Flag{Name: "server", DefValue: contract.Default, Hidden: true}
	if mismatch := climanifestparity.CompareFlagDefault("you", contract, &hidden); mismatch == nil {
		t.Fatal("CompareFlagDefault(hidden) mismatch = nil, want visibility failure")
	}

	changed := pflag.NewFlagSet("test", pflag.ContinueOnError)
	var server string
	changed.StringVar(&server, "server", contract.Default, "")
	if err := changed.Set("server", "other"); err != nil {
		t.Fatalf("Set(server): %v", err)
	}
	if mismatch := climanifestparity.CompareFlagDefault("you", contract, changed.Lookup("server")); mismatch == nil {
		t.Fatal("CompareFlagDefault(changed) mismatch = nil, want changed-default failure")
	}
}

func TestCompareArgumentCardinality_MismatchPaths(t *testing.T) {
	required := climanifest.Argument{Position: 0, Required: true, MaxCardinality: 1, MinCardinality: 1}
	if mismatch := climanifestparity.CompareArgumentCardinality("you", required, nil); mismatch == nil {
		t.Fatal("CompareArgumentCardinality(required) mismatch = nil, want required failure")
	}

	optional := climanifest.Argument{Position: 0, MaxCardinality: 1, MinCardinality: 0}
	if mismatch := climanifestparity.CompareArgumentCardinality("you", optional, []string{"one", "two"}); mismatch == nil {
		t.Fatal("CompareArgumentCardinality(excess) mismatch = nil, want maxCardinality failure")
	}
}

func TestAssertLeafCommandPath(t *testing.T) {
	if mismatch := climanifestparity.AssertLeafCommandPath("you", "you session show", nil); mismatch == nil {
		t.Fatal("AssertLeafCommandPath(nil) mismatch = nil, want nil leaf failure")
	}
	leaf := &cobra.Command{Use: "show"}
	if mismatch := climanifestparity.AssertLeafCommandPath("you.session.show", "you session show", leaf); mismatch == nil {
		t.Fatal("AssertLeafCommandPath(wrong path) mismatch = nil, want path failure")
	}
}

func TestCompareDeclaredMetadata_MismatchPaths(t *testing.T) {
	record := climanifest.Command{ID: "you.session.show"}
	if mismatches := climanifestparity.CompareDeclaredOutputs(record); len(mismatches) == 0 {
		t.Fatal("CompareDeclaredOutputs(empty) mismatches = 0, want drift failures")
	}
	if mismatches := climanifestparity.CompareDeclaredExits(record); len(mismatches) == 0 {
		t.Fatal("CompareDeclaredExits(empty) mismatches = 0, want drift failures")
	}
	if mismatches := climanifestparity.CompareDeclaredSideEffects(record, []string{"network"}); len(mismatches) == 0 {
		t.Fatal("CompareDeclaredSideEffects(empty) mismatches = 0, want drift failures")
	}
	if mismatches := climanifestparity.CompareDeclaredConstraints(record); len(mismatches) == 0 {
		t.Fatal("CompareDeclaredConstraints(empty) mismatches = 0, want drift failures")
	}
	if mismatches := climanifestparity.CompareDeclaredChannels(record); len(mismatches) == 0 {
		t.Fatal("CompareDeclaredChannels(empty) mismatches = 0, want drift failures")
	}
}

func TestCompareCompletionParity_MissingInventory(t *testing.T) {
	record := climanifest.Command{
		ID: "you.session.show",
		Arguments: map[string]climanifest.Argument{
			"session-id": {Position: 0, Completion: "none"},
		},
		Flags: map[string]climanifest.Flag{
			"port": {Long: "port", Completion: "none"},
		},
	}
	mismatches := climanifestparity.CompareCompletionParity(record, nil, nil)
	if len(mismatches) < 2 {
		t.Fatalf("CompareCompletionParity() mismatches = %d, want missing inventory failures", len(mismatches))
	}
}

func TestCompareInheritedFlagDefaultsAgainstRoot_MissingRootBinding(t *testing.T) {
	root := climanifest.Command{ID: "you"}
	sessionShow := climanifest.Command{
		ID: "you.session.show",
		Flags: map[string]climanifest.Flag{
			"json": {Long: "json", Scope: "inherited", Default: "false"},
		},
	}
	mismatches := climanifestparity.CompareInheritedFlagDefaultsAgainstRoot(root, sessionShow, &cobra.Command{})
	if len(mismatches) == 0 {
		t.Fatal("CompareInheritedFlagDefaultsAgainstRoot() mismatches = 0, want missing root binding failure")
	}
}

func TestCompareDeclaredHandler_MismatchPaths(t *testing.T) {
	record := climanifest.Command{ID: "you.session.show"}
	if mismatches := climanifestparity.CompareDeclaredHandler(record, "you.session.show.handler", "getFactorySession"); len(mismatches) == 0 {
		t.Fatal("CompareDeclaredHandler(nil handler) mismatches = 0, want failure")
	}

	record.Handler = &climanifest.Handler{ID: "other", OperationID: "other"}
	if mismatches := climanifestparity.CompareDeclaredHandler(record, "you.session.show.handler", "getFactorySession"); len(mismatches) < 2 {
		t.Fatalf("CompareDeclaredHandler(wrong ids) mismatches = %d, want handler and operationId failures", len(mismatches))
	}
}

func TestOpenAPIOperationBinding(t *testing.T) {
	if _, _, ok := climanifestparity.OpenAPIOperationBinding(nil, "getFactorySession"); ok {
		t.Fatal("OpenAPIOperationBinding(nil doc) ok = true, want false")
	}

	doc := loadBundledOpenAPIContract(t)
	method, path, ok := climanifestparity.OpenAPIOperationBinding(doc, "getFactorySession")
	if !ok || method != "GET" || path != "/factory-sessions/{session_id}" {
		t.Fatalf("OpenAPIOperationBinding() = %q %q %t, want GET /factory-sessions/{session_id} true", method, path, ok)
	}
}

func TestCompareHandlerOpenAPIBinding_MismatchPaths(t *testing.T) {
	record := climanifest.Command{ID: "you.session.show"}
	if mismatches := climanifestparity.CompareHandlerOpenAPIBinding(record, nil, "GET", "/factory-sessions/{session_id}"); len(mismatches) == 0 {
		t.Fatal("CompareHandlerOpenAPIBinding(missing handler) mismatches = 0, want failure")
	}

	record.Handler = &climanifest.Handler{ID: "you.session.show.handler", OperationID: "missing"}
	paths := openapi3.NewPaths()
	if mismatches := climanifestparity.CompareHandlerOpenAPIBinding(record, &openapi3.T{Paths: paths}, "GET", "/factory-sessions/{session_id}"); len(mismatches) == 0 {
		t.Fatal("CompareHandlerOpenAPIBinding(missing operation) mismatches = 0, want failure")
	}
}

func TestInputsForCommandPath(t *testing.T) {
	inventory := cliinputs.Inventory{
		Arguments: []cliinputs.ArgumentRecord{{
			CommandJoin: cliinputs.CommandJoin{CommandPath: "you session show"},
			Position:    0,
			Name:        "session-id",
		}},
		Flags: []cliinputs.FlagRecord{{
			CommandJoin: cliinputs.CommandJoin{CommandPath: "you session show"},
			Long:        "port",
		}},
	}
	args, flags := climanifestparity.InputsForCommandPath(inventory, "you session show")
	if len(args) != 1 || len(flags) != 1 {
		t.Fatalf("InputsForCommandPath() args=%d flags=%d, want 1 each", len(args), len(flags))
	}
}

func TestNormalizeJSONOutput(t *testing.T) {
	if got := climanifestparity.NormalizeJSONOutput("  {}\n"); got != "{}" {
		t.Fatalf("NormalizeJSONOutput() = %q, want {}", got)
	}
}
