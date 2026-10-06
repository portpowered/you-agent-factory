package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
)

func TestStartupCauseCommandRendererPreservesIdentityAndPriorDiagnostic(t *testing.T) {
	t.Parallel()
	for _, alreadyRendered := range []bool{false, true} {
		var output bytes.Buffer
		writer := clidiag.NewDiagnosticWriter(&output)
		cause := errors.New("load board: unexpected EOF")
		err := clidiag.WithStartupCause(clidiag.NewLocalInputFailure("--resume", "board.json", cause))
		if alreadyRendered {
			clidiag.WriteFailure(writer, err)
		}
		result := executeCommandResult(writer, err)
		if !errors.Is(result, cause) || strings.Count(output.String(), "CLI_LOCAL_INPUT_FAILED") != 1 || strings.Count(output.String(), "cause[0]=load board: unexpected EOF") != 1 {
			t.Fatalf("prior=%t result=%v stderr=%q", alreadyRendered, result, output.String())
		}
	}
}

func TestStartupCauseCommandRendererLeavesUsageCancellationAndOtherFailures(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name string
		err  error
		want string
	}{
		{"uncoded startup", clidiag.WithStartupCause(errors.New("decode board failed")), "cause[0]=decode board failed"},
		{"ordinary failure", errors.New("private ordinary cause"), "CLI_COMMAND_FAILED"},
		{"usage", clidiag.WithStartupCause(clidiag.NewUsageError("you run", errors.New("invalid flag"))), "Run 'you run --help'"},
		{"cancellation", clidiag.WithStartupCause(context.Canceled), "Error: context canceled"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var output bytes.Buffer
			result := executeCommandResult(clidiag.NewDiagnosticWriter(&output), test.err)
			if !errors.Is(result, test.err) && !errors.Is(result, context.Canceled) {
				t.Fatalf("renderer lost identity: %v", result)
			}
			if !strings.Contains(output.String(), test.want) {
				t.Fatalf("renderer output=%q, want %q", output.String(), test.want)
			}
			if test.name != "uncoded startup" && strings.Contains(output.String(), "cause[") {
				t.Fatalf("renderer added causes outside startup: %q", output.String())
			}
		})
	}
}
