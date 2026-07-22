package observation

import (
	"reflect"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
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
