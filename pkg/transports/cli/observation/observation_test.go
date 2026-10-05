package observation

import (
	"reflect"
	"strings"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	"github.com/spf13/cobra"
)

func TestObservationRoundTripDetachesPrivateCLIRepresentation(t *testing.T) {
	root := &cobra.Command{Use: "you"}
	run := &cobra.Command{Use: "run [prompt]", Args: cobra.MaximumNArgs(1)}
	run.Flags().Bool("verbose", false, "verbose output")
	root.AddCommand(run)

	snapshot, err := CaptureSnapshot(root)
	if err != nil {
		t.Fatalf("CaptureSnapshot() error = %v", err)
	}
	original := Result{
		Snapshot: snapshot,
		Parse: platformprocess.CLIParseResult{
			CommandPath: "you run", Positionals: []string{"hello"},
			Flags: []platformprocess.CLIParsedFlag{{Name: "verbose", Changed: true, Value: "true"}},
		},
		ResolvedInputs: []resolvedinput.Observation{{
			InputID: "you.flag.verbose", Kind: resolvedinput.ValueKindBool,
			Provenance: resolvedinput.SourceCLIFlag, Changed: true, Value: true,
		}},
	}
	edge, err := Encode(original)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	decoded, err := Decode(edge)
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if !reflect.DeepEqual(decoded, original) {
		t.Fatalf("round trip = %#v, want %#v", decoded, original)
	}

	root.AddCommand(&cobra.Command{Use: "later"})
	if decoded.Snapshot.Commands.RootPath != "you" || len(decoded.Snapshot.Commands.Commands) != 2 {
		t.Fatalf("decoded observation changed with private tree mutation: %#v", decoded.Snapshot.Commands)
	}
}

func TestCaptureObserverDecodesNeutralEdge(t *testing.T) {
	var target Result
	observer := Capture(&target)
	if err := observer(platformprocess.CLIObservation{
		CommandIdentityJSON: `{"formatVersion":"cli-command-identity/v1","rootPath":"you","commands":[]}`,
		CommandInputsJSON:   `{"formatVersion":"cli-command-inputs/v1","arguments":[],"flags":[],"relationships":[]}`,
		CommandTree:         "you\tyou\t\n",
	}); err != nil {
		t.Fatalf("Capture observer error = %v", err)
	}
	if target.Snapshot.Commands.RootPath != "you" || target.Snapshot.CommandTree != "you\tyou\t\n" {
		t.Fatalf("captured observation = %#v", target)
	}
}

func TestResolvedInputObservationRejectsInvalidEdgeValues(t *testing.T) {
	_, err := Decode(platformprocess.CLIObservation{
		CommandIdentityJSON: `{"formatVersion":"cli-command-identity/v1","rootPath":"you","commands":[]}`,
		CommandInputsJSON:   `{"formatVersion":"cli-command-inputs/v1","arguments":[],"flags":[],"relationships":[]}`,
		ResolvedInputsJSON:  `{`,
	})
	if err == nil || !strings.Contains(err.Error(), "decode resolved CLI inputs observation") {
		t.Fatalf("Decode() error = %v, want resolved-input diagnostic", err)
	}

	_, err = Encode(Result{ResolvedInputs: []resolvedinput.Observation{{Value: func() {}}}})
	if err == nil || !strings.Contains(err.Error(), "encode resolved CLI inputs observation") {
		t.Fatalf("Encode() error = %v, want resolved-input diagnostic", err)
	}
}

func TestParseObservationDetachesArgumentsAndFindsFlags(t *testing.T) {
	command := &cobra.Command{Use: "run"}
	command.Flags().String("target", "default", "target")
	if err := command.Flags().Set("target", "selected"); err != nil {
		t.Fatal(err)
	}
	args := []string{"input"}
	result := CaptureParseResult(command, args)
	args[0] = "mutated"
	flag, found := Flag(result, "target")
	if result.CommandPath != "run" || result.Positionals[0] != "input" || !found || flag.Value != "selected" || !flag.Changed {
		t.Fatalf("parse result = %#v, flag=%#v", result, flag)
	}
	if _, found := Flag(result, "missing"); found {
		t.Fatal("missing flag found")
	}
	if result := CaptureParseResult(nil, []string{"input"}); result.CommandPath != "" || result.Positionals[0] != "input" {
		t.Fatalf("nil command result = %#v", result)
	}
}

func TestCaptureAppendRetainsOnlyValidObservations(t *testing.T) {
	var results []Result
	observer := CaptureAppend(&results)
	valid, err := Encode(Result{Parse: platformprocess.CLIParseResult{CommandPath: "you run"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := observer(valid); err != nil {
		t.Fatal(err)
	}
	if err := observer(platformprocess.CLIObservation{}); err == nil {
		t.Fatal("invalid observation accepted")
	}
	if len(results) != 1 || results[0].Parse.CommandPath != "you run" {
		t.Fatalf("observations = %#v", results)
	}
	if err := CaptureAppend(nil)(valid); err != nil {
		t.Fatalf("nil capture = %v", err)
	}
}
