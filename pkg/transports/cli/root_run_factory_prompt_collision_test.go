package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	factorydefinitionfixtures "github.com/portpowered/infinite-you/internal/testutil/factorydefinitionfixtures"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestRunCommand_SelectedFactoryHelpRejectsReservedInvocationFlagBeforeRendering(t *testing.T) {
	for _, selection := range []string{"named", "factory"} {
		selection := selection
		t.Run(selection, func(t *testing.T) {
			err, stdout, stderr := executeSelectedFactoryHelpFixture(t, selection, "model")
			if err == nil {
				t.Fatal("reserved Factory flag help error = nil, want non-zero failure")
			}
			diagnostic := err.Error() + "\n" + stdout + "\n" + stderr
			for _, want := range []string{
				"cli.composition.long-name-collision",
				"model",
				"you.run.flag.model",
				"child-model",
				"worker-provider",
				"research-model",
			} {
				if !strings.Contains(diagnostic, want) {
					t.Fatalf("help diagnostic missing %q:\n%s", want, diagnostic)
				}
			}
			if strings.Contains(stdout, "Factory invocation help") || strings.Contains(stdout, "--model") {
				t.Fatalf("colliding Factory help rendered unusable signature:\n%s", stdout)
			}
		})
	}
}

func TestRunCommand_SelectedFactoryHelpAcceptsPrefixedInvocationFlag(t *testing.T) {
	for _, selection := range []string{"named", "factory"} {
		selection := selection
		t.Run(selection, func(t *testing.T) {
			err, stdout, stderr := executeSelectedFactoryHelpFixture(t, selection, "child-model")
			if err != nil {
				t.Fatalf("prefixed Factory help error = %v\nstderr:\n%s", err, stderr)
			}
			for _, want := range []string{"Factory invocation help", "--child-model"} {
				if !strings.Contains(stdout, want) {
					t.Fatalf("prefixed Factory help missing %q:\n%s", want, stdout)
				}
			}
		})
	}
}

func executeSelectedFactoryHelpFixture(t *testing.T, selection, externalName string) (error, string, string) {
	t.Helper()
	workingDirectory := t.TempDir()
	homeDirectory := t.TempDir()
	originalWorkingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(workingDirectory); err != nil {
		t.Fatalf("Chdir(%q): %v", workingDirectory, err)
	}
	t.Cleanup(func() {
		if chdirErr := os.Chdir(originalWorkingDirectory); chdirErr != nil {
			t.Errorf("restore working directory: %v", chdirErr)
		}
	})
	t.Setenv("HOME", homeDirectory)
	t.Setenv("USERPROFILE", homeDirectory)

	factoryDir, err := factorydefinitionfixtures.SeedNamedFactory(
		filepath.Join(workingDirectory, "factory", "collision"),
		reservedInvocationHelpPayload(externalName),
	)
	if err != nil {
		t.Fatalf("SeedNamedFactory(collision): %v", err)
	}
	root := newLegacyTestRootCommandWithCatalog(transportNamedFactoryCatalog{"collision": factoryDir})
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	args := []string{"run"}
	if selection == "named" {
		args = append(args, "--named", "collision")
	} else {
		args = append(args, "--factory", filepath.Join(factoryDir, interfaces.FactoryConfigFile))
	}
	root.SetArgs(append(args, "--help"))

	err = root.Execute()
	return err, stdout.String(), stderr.String()
}

func reservedInvocationHelpPayload(externalName string) []byte {
	return bytes.Replace(
		portableFactoryPayloadWithInvocationSignature(),
		[]byte(`"name": "mode",`),
		[]byte(`"name": "mode", "externalName": "`+externalName+`",`),
		1,
	)
}
