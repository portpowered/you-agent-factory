package cliinputs_test

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/cli/cliinputs"
	"github.com/portpowered/infinite-you/pkg/cli/commandidentity"
)

func TestValidateCommandJoins_RejectsUnknownCommandPath(t *testing.T) {
	index := cliinputs.NewCommandIdentityIndex([]commandidentity.CommandRecord{
		{Path: "you run", IDCandidate: "you.run"},
	})

	err := cliinputs.ValidateCommandJoins(cliinputs.Inventory{
		Arguments: []cliinputs.ArgumentRecord{
			{
				CommandJoin: cliinputs.CommandJoin{
					CommandPath:        "you missing",
					CommandIDCandidate: "you.missing",
				},
				IDCandidate: "you.missing.arg.0",
			},
		},
	}, index)
	if err == nil {
		t.Fatal("ValidateCommandJoins() error = nil, want unknown command path failure")
	}
	if got := err.Error(); !strings.Contains(got, `unknown command path "you missing"`) {
		t.Fatalf("error = %q, want unknown command path diagnostic", got)
	}
}

func TestValidateCommandJoins_RejectsMismatchedCommandIDCandidate(t *testing.T) {
	index := cliinputs.NewCommandIdentityIndex([]commandidentity.CommandRecord{
		{Path: "you run", IDCandidate: "you.run"},
	})

	err := cliinputs.ValidateCommandJoins(cliinputs.Inventory{
		Flags: []cliinputs.FlagRecord{
			{
				CommandJoin: cliinputs.CommandJoin{
					CommandPath:        "you run",
					CommandIDCandidate: "you.run.wrong",
				},
				IDCandidate: "you.run.wrong.flag.dir",
			},
		},
	}, index)
	if err == nil {
		t.Fatal("ValidateCommandJoins() error = nil, want mismatched commandIdCandidate failure")
	}
	if got := err.Error(); !strings.Contains(got, `does not match Batch 01 identity "you.run"`) {
		t.Fatalf("error = %q, want mismatched commandIdCandidate diagnostic", got)
	}
}

func TestValidateCommandJoins_RejectsDuplicateArgumentIdentityWithinCommand(t *testing.T) {
	index := cliinputs.NewCommandIdentityIndex([]commandidentity.CommandRecord{
		{Path: "you run", IDCandidate: "you.run"},
	})

	err := cliinputs.ValidateCommandJoins(cliinputs.Inventory{
		Arguments: []cliinputs.ArgumentRecord{
			{
				CommandJoin: cliinputs.CommandJoin{
					CommandPath:        "you run",
					CommandIDCandidate: "you.run",
				},
				IDCandidate: "you.run.arg.0",
			},
			{
				CommandJoin: cliinputs.CommandJoin{
					CommandPath:        "you run",
					CommandIDCandidate: "you.run",
				},
				IDCandidate: "you.run.arg.0",
			},
		},
	}, index)
	if err == nil {
		t.Fatal("ValidateCommandJoins() error = nil, want duplicate argument identity failure")
	}
	if got := err.Error(); !strings.Contains(got, `duplicate argument idCandidate "you.run.arg.0" within command "you run"`) {
		t.Fatalf("error = %q, want duplicate argument identity diagnostic", got)
	}
}

func TestValidateCommandJoins_RejectsDuplicateFlagIdentityWithinCommand(t *testing.T) {
	index := cliinputs.NewCommandIdentityIndex([]commandidentity.CommandRecord{
		{Path: "you run", IDCandidate: "you.run"},
	})

	err := cliinputs.ValidateCommandJoins(cliinputs.Inventory{
		Flags: []cliinputs.FlagRecord{
			{
				CommandJoin: cliinputs.CommandJoin{
					CommandPath:        "you run",
					CommandIDCandidate: "you.run",
				},
				IDCandidate: "you.run.flag.dir",
			},
			{
				CommandJoin: cliinputs.CommandJoin{
					CommandPath:        "you run",
					CommandIDCandidate: "you.run",
				},
				IDCandidate: "you.run.flag.dir",
			},
		},
	}, index)
	if err == nil {
		t.Fatal("ValidateCommandJoins() error = nil, want duplicate flag identity failure")
	}
	if got := err.Error(); !strings.Contains(got, `duplicate flag idCandidate "you.run.flag.dir" within command "you run"`) {
		t.Fatalf("error = %q, want duplicate flag identity diagnostic", got)
	}
}
