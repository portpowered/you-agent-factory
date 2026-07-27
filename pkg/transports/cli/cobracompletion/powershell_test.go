package cobracompletion_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/cobracompletion"
	"github.com/spf13/cobra"
)

func TestRegisterPowerShellFilesystemDelegationPatchesGeneratedScript(t *testing.T) {
	root := &cobra.Command{Use: "you"}
	root.AddCommand(&cobra.Command{Use: "run"})
	var output bytes.Buffer
	root.SetOut(&output)
	if err := cobracompletion.RegisterPowerShellFilesystemDelegation(root); err != nil {
		t.Fatalf("RegisterPowerShellFilesystemDelegation() error = %v", err)
	}

	root.SetArgs([]string{"completion", "powershell"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	script := output.String()
	for _, want := range []string{
		"Get-ChildItem -LiteralPath $SearchParent",
		"$CompletionText = $Flag + \"=\" + $CompletionText",
		"'ProviderItem'",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("generated PowerShell completion lacks %q", want)
		}
	}
}

func TestRegisterPowerShellFilesystemDelegationRejectsMissingRoot(t *testing.T) {
	err := cobracompletion.RegisterPowerShellFilesystemDelegation(nil)
	if err == nil || !strings.Contains(err.Error(), "root command is required") {
		t.Fatalf("error = %v, want missing-root diagnostic", err)
	}
}
