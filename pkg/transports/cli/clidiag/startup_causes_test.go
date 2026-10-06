package clidiag

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestStartupCausePreservesEnvelopeIdentityAndEmitsOnce(t *testing.T) {
	t.Parallel()
	cause := errors.New("unexpected EOF")
	local := NewLocalInputFailure("--resume", "./board.json", fmt.Errorf("load recording: %w", cause))
	err := WithStartupCause(local)
	if !errors.Is(err, cause) || WithStartupCause(err) != err {
		t.Fatal("startup wrapper lost identity or wrapped twice")
	}
	var typed *LocalFailure
	if !errors.As(err, &typed) || typed != local {
		t.Fatal("startup wrapper lost the original local failure")
	}
	for _, debug := range []bool{false, true} {
		var output bytes.Buffer
		inner := NewDiagnosticWriter(&output, debug)
		outer := NewDiagnosticWriter(inner, debug)
		if !WriteFailure(outer, err) || !WriteStartupCauses(outer, err) {
			t.Fatal("startup envelope/causes were not rendered")
		}
		WriteStartupCauses(inner, err)
		if debug {
			WriteDebugFailure(outer, err)
		}
		want := "{\"code\":\"CLI_LOCAL_INPUT_FAILED\",\"family\":\"BAD_REQUEST\",\"message\":\"failed to load --resume input \\\"./board.json\\\"\"}\n" +
			"cause[0]=load recording: unexpected EOF\ncause[1]=unexpected EOF\n"
		if output.String() != want {
			t.Fatalf("debug=%t output=%q, want %q", debug, output.String(), want)
		}
	}
}

type cyclicCause struct{}

func (err *cyclicCause) Error() string { return "cyclic load failure" }
func (err *cyclicCause) Unwrap() error { return err }

func TestStartupCauseBoundedJoinedCyclicDeepAndNil(t *testing.T) {
	t.Parallel()
	deep := errors.New("deep leaf")
	for index := 0; index < 32; index++ {
		deep = fmt.Errorf("layer %d: %w", index, deep)
	}
	var nilCause *LocalFailure
	for _, test := range []struct {
		name string
		err  error
		want string
	}{
		{"joined", errors.Join(errors.New("decode failed"), errors.New("read failed")), "read failed"},
		{"cyclic", &cyclicCause{}, "cyclic load failure"},
		{"deep", deep, "<cause chain truncated>"},
		{"nil", nil, ""},
		{"typed nil", nilCause, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var output bytes.Buffer
			WriteStartupCauses(&output, WithStartupCause(test.err))
			if !strings.Contains(output.String(), test.want) || strings.Count(output.String(), "\n") > maxDebugCauseDepth+1 {
				t.Fatalf("unbounded or incomplete causes: %q", output.String())
			}
		})
	}
}

func TestStartupCauseRedactsSecretsAndPrivatePaths(t *testing.T) {
	t.Parallel()
	err := errors.New(`decode failed: token="PRIVATE_TOKEN" prompt="PRIVATE_PROMPT" body="PRIVATE_BODY" open C:\private\board.json open /private/board.json open ./private/board.json open "C:\PRIVATE_FOLDER NAME\board.json" open "/PRIVATE_FOLDER NAME/board.json" https://user:PRIVATE_PASSWORD@example.test/board?token=PRIVATE_QUERY#PRIVATE_FRAGMENT`)
	var output bytes.Buffer
	WriteStartupCauses(&output, WithStartupCause(err))
	for _, forbidden := range []string{"PRIVATE_", `C:\private`, "/private/board", "./private/board"} {
		if strings.Contains(output.String(), forbidden) {
			t.Fatalf("startup causes leaked %q: %q", forbidden, output.String())
		}
	}
	if !strings.Contains(output.String(), "decode failed") || !strings.Contains(output.String(), "https://example.test/board") {
		t.Fatalf("startup causes lost safe context: %q", output.String())
	}
}

func TestTerminalFailureIncludesWrappedCauseWithoutDebug(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	WriteTerminalFailure(&output, NewLocalInputFailure("--resume", "board.json", errors.New("unexpected EOF")))
	if !strings.Contains(output.String(), "cause[0]=unexpected EOF\n") {
		t.Fatalf("terminal failure hid its cause: %q", output.String())
	}
}
