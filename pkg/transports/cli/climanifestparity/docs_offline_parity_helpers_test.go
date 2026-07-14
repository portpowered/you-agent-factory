package climanifestparity_test

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestcobra"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
)

type generatedVsLegacyDocsCase struct {
	name       string
	argv       []string
	wantStdout func(t *testing.T) string
	wantErr    string
}

func assertGeneratedVsLegacyDocsArgvParity(
	t *testing.T,
	registry *commandregistry.Registry,
	invokeFlags climanifestcobra.ModelsInvokeFlagBindings,
	tc generatedVsLegacyDocsCase,
) {
	t.Helper()

	var legacyStdout bytes.Buffer
	var legacyStderr bytes.Buffer
	legacy := cli.NewLegacyDocsFamilyCommand()
	legacy.SetOut(&legacyStdout)
	legacy.SetErr(&legacyStderr)
	legacy.SetArgs(tc.argv)
	legacyErr := legacy.Execute()

	var generatedStdout bytes.Buffer
	var generatedStderr bytes.Buffer
	generated, err := cli.NewGeneratedDocsFamilyParityCommand(registry, invokeFlags)
	if err != nil {
		t.Fatalf("NewGeneratedDocsFamilyParityCommand() error = %v", err)
	}
	generated.SetOut(&generatedStdout)
	generated.SetErr(&generatedStderr)
	generated.SetArgs(tc.argv)
	generatedErr := generated.Execute()

	if tc.wantErr != "" {
		if legacyErr == nil || !strings.Contains(legacyErr.Error(), tc.wantErr) {
			t.Fatalf("legacy error = %v, want substring %q", legacyErr, tc.wantErr)
		}
		if generatedErr == nil || !strings.Contains(generatedErr.Error(), tc.wantErr) {
			t.Fatalf("generated error = %v, want substring %q", generatedErr, tc.wantErr)
		}
		if legacyStdout.String() != "" || generatedStdout.String() != "" {
			t.Fatalf("unsupported topic wrote stdout legacy=%q generated=%q", legacyStdout.String(), generatedStdout.String())
		}
		return
	}

	if legacyErr != nil {
		t.Fatalf("legacy execute %v: %v", tc.argv, legacyErr)
	}
	if generatedErr != nil {
		t.Fatalf("generated execute %v: %v", tc.argv, generatedErr)
	}

	wantStdout := tc.wantStdout(t)
	if legacyStdout.String() != wantStdout {
		t.Fatalf("legacy stdout mismatch\nwant:\n%s\ngot:\n%s", wantStdout, legacyStdout.String())
	}
	if generatedStdout.String() != wantStdout {
		t.Fatalf("generated stdout mismatch\nwant:\n%s\ngot:\n%s", wantStdout, generatedStdout.String())
	}
}

func legacyDocsIndexStdout(t *testing.T) string {
	t.Helper()
	var stdout bytes.Buffer
	legacyRoot := cli.NewLegacyDocsFamilyCommand()
	legacyRoot.SetOut(&stdout)
	legacyRoot.SetErr(io.Discard)
	legacyRoot.SetArgs([]string{"docs"})
	if err := legacyRoot.Execute(); err != nil {
		t.Fatalf("legacy execute docs: %v", err)
	}
	return stdout.String()
}
