package acp_test

import (
	"context"
	"os"
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestRootBuiltACPCommandsRejectInvalidMutationsWithoutPersistingSettings(t *testing.T) {
	home := t.TempDir()
	working := t.TempDir()
	process := support.BuildProcess(t, serviceedges.Edges{
		OperatorSettingsIDGenerator: func() string { return "must-not-be-used" },
		ProvidersExecutableLocator:  availableExecutableLocator{},
	})

	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{name: "missing name", args: []string{"workers", "acp", "add", "--argument", "agent acp"}, want: "required flag"},
		{name: "non canonical name", args: []string{"workers", "acp", "add", "--name", "Cursor ACP", "--argument", "agent acp"}, want: "lowercase"},
		{name: "unsupported transport", args: []string{"workers", "acp", "add", "--name", "custom-acp", "--transport", "tcp", "--argument", "agent acp"}, want: "must be stdio"},
		{name: "empty command", args: []string{"workers", "acp", "add", "--name", "custom-acp", "--argument", ""}, want: "command is required"},
		{name: "missing delete name", args: []string{"workers", "acp", "delete"}, want: "required flag"},
	} {
		t.Run(test.name, func(t *testing.T) {
			inputs := support.FakeInputs(context.Background(), append([]string{"you"}, test.args...))
			inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
			inputs.Input.WorkingDirectory = working
			err := process.Execute(inputs.Input)
			if err == nil || !strings.Contains(strings.ToLower(err.Error()+inputs.Stderr()), strings.ToLower(test.want)) {
				t.Fatalf("execute %v error = %v, stderr=%q; want %q", test.args, err, inputs.Stderr(), test.want)
			}
		})
	}

	if _, err := os.Stat(operatorACPConfigPath(home)); !os.IsNotExist(err) {
		t.Fatalf("invalid ACP mutations changed operator settings: stat error = %v", err)
	}
}

func operatorACPConfigPath(home string) string {
	return home + string(os.PathSeparator) + ".you-agent-factory" + string(os.PathSeparator) + "config.json"
}
