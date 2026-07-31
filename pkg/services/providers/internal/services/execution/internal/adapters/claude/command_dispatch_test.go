package claude_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
	claude "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/claude"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestCommandEffectRendersProviderNeutralReasoningEffort(t *testing.T) {
	t.Parallel()

	runner := testutil.NewProviderCommandRunner()
	effect := claude.NewCommandEffect(workers.AdaptCommandRunner(runner))
	if effect == nil {
		t.Fatal("NewCommandEffect() returned nil")
	}

	_, err := effect.Execute(context.Background(), providers.ExecuteRequest{
		Provider:        providers.IDClaude,
		AttemptID:       "claude-xhigh-dispatch",
		Model:           "claude-model",
		ReasoningEffort: "xhigh",
		UserMessage:     "perform work",
	}, func([]byte) error { return nil })
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	want := []string{
		"-p",
		"--model", "claude-model",
		"--effort", "xhigh",
		"--output-format", "stream-json",
		"--include-partial-messages",
		"perform work",
	}
	if got := runner.LastRequest().Args; !reflect.DeepEqual(got, want) {
		t.Fatalf("command args = %#v, want %#v", got, want)
	}
}

func TestCommandEffectRejectsUnsupportedReasoningEffort(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		effort string
		want   string
	}{
		{name: "provider unsupported", effort: "minimal", want: `does not support reasoning effort "minimal"`},
		{name: "globally unsupported", effort: "extreme", want: `unsupported reasoning effort "extreme"`},
	} {
		t.Run(test.name, func(t *testing.T) {
			runner := testutil.NewProviderCommandRunner()
			effect := claude.NewCommandEffect(workers.AdaptCommandRunner(runner))
			_, err := effect.Execute(context.Background(), providers.ExecuteRequest{
				Provider:        providers.IDClaude,
				AttemptID:       "claude-invalid-effort-dispatch",
				ReasoningEffort: test.effort,
				UserMessage:     "perform work",
			}, func([]byte) error { return nil })
			var failure execution.AttemptFailure
			if !errors.As(err, &failure) ||
				failure.NativeError == nil ||
				!strings.Contains(failure.NativeError.Error(), test.want) {
				t.Fatalf("Execute() error = %v, want %q", err, test.want)
			}
			if got := runner.Requests(); len(got) != 0 {
				t.Fatalf("runner requests = %#v, want none", got)
			}
		})
	}
}
