package wire

import (
	"context"
	"reflect"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/providers"
)

func TestProvideProvidersServicePrefersAgyCommandRunnerWithInjectedPTYHost(t *testing.T) {
	t.Parallel()

	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout: []byte(`{"event":"result","result":{"conversation_id":"wire-agy-selection","status":"SUCCESS","response":"WIRE_OK","duration_seconds":1,"num_turns":1,"usage":{"input_tokens":1,"output_tokens":1,"thinking_tokens":0,"cache_read_tokens":0,"total_tokens":2}}}` + "\n"),
	})
	host := &recordingAgyPTYHost{}
	workDir := t.TempDir()
	service, err := provideProvidersService(serviceedges.Edges{
		ProviderCommandRunner: runner,
		AgyPTYHost:            host,
	})
	if err != nil {
		t.Fatalf("provideProvidersService() error = %v", err)
	}

	result, err := service.Execute(context.Background(), providers.ExecuteRequest{
		Provider:         providers.IDAntigravity,
		AttemptID:        "agy-command-selection",
		Model:            "gemini-3.6-flash-high",
		SkipPermissions:  true,
		WorkingDirectory: workDir,
		UserMessage:      "wire selection",
	})
	if err != nil {
		t.Fatalf("Execute(antigravity) error = %v", err)
	}
	if result.Content != "WIRE_OK" {
		t.Fatalf("Execute(antigravity) content = %q, want WIRE_OK", result.Content)
	}
	if host.allocated {
		t.Fatal("injected Agy PTY host was selected; command runner must own canonical print dispatch")
	}

	wantArgs := []string{
		"-p", "wire selection",
		"--output-format", "stream-json",
		"--add-dir", workDir,
		"--disable-slash-commands",
		"--model", "gemini-3.6-flash-high",
		"--dangerously-skip-permissions",
		"--print-timeout", "5m",
	}
	request := runner.LastRequest()
	if request.Command != "agy" {
		t.Fatalf("provider command = %q, want agy", request.Command)
	}
	if request.WorkDir != workDir {
		t.Fatalf("provider workdir = %q, want %q", request.WorkDir, workDir)
	}
	if !reflect.DeepEqual(request.Args, wantArgs) {
		t.Fatalf("provider argv = %#v, want %#v", request.Args, wantArgs)
	}
}
