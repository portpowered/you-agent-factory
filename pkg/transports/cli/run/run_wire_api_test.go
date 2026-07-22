package run

import (
	"strings"
	"testing"
)

func TestNormalizeInvocationOutputMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr string
	}{
		{
			name: "empty defaults to primary",
			raw:  "",
			want: InvocationOutputPrimaryResult,
		},
		{
			name: "primary literal accepted",
			raw:  "primary",
			want: InvocationOutputPrimaryResult,
		},
		{
			name: "response-stream accepted",
			raw:  "response-stream",
			want: InvocationOutputResponseStream,
		},
		{
			name:    "unknown rejected",
			raw:     "sse",
			wantErr: "unsupported --output value",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := NormalizeInvocationOutputMode(tc.raw)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("NormalizeInvocationOutputMode(%q) error = %v, want %q", tc.raw, err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeInvocationOutputMode(%q): %v", tc.raw, err)
			}
			if got != tc.want {
				t.Fatalf("mode = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestValidateInvocationOutputMode_RejectsUnsupportedRunShapes(t *testing.T) {
	t.Parallel()

	text := "Plan the sprint"
	tests := []struct {
		name           string
		cfg            RunConfig
		invocationMode bool
		wantCode       string
	}{
		{
			name: "continuous unsupported",
			cfg: RunConfig{
				InvocationOutputMode:     InvocationOutputResponseStream,
				Continuously:             true,
				InvocationPositionalText: &text,
			},
			invocationMode: true,
			wantCode:       "INVOCATION_OUTPUT_UNSUPPORTED",
		},
		{
			name: "non-invocation run unsupported",
			cfg: RunConfig{
				InvocationOutputMode: InvocationOutputResponseStream,
			},
			invocationMode: false,
			wantCode:       "INVOCATION_OUTPUT_UNSUPPORTED",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateInvocationOutputMode(tc.cfg, tc.invocationMode)
			if err == nil {
				t.Fatal("expected validation error")
			}
			invocationErr, ok := err.(*InvocationError)
			if !ok {
				t.Fatalf("error = %#v, want InvocationError", err)
			}
			if invocationErr.Code != tc.wantCode {
				t.Fatalf("code = %q, want %q", invocationErr.Code, tc.wantCode)
			}
		})
	}
}

func TestValidateInvocationOutputMode_AllowsSupportedInvocation(t *testing.T) {
	t.Parallel()

	text := "Plan the sprint"
	err := validateInvocationOutputMode(RunConfig{
		InvocationOutputMode:     InvocationOutputResponseStream,
		InvocationPositionalText: &text,
	}, true)
	if err != nil {
		t.Fatalf("validateInvocationOutputMode: %v", err)
	}
}

func TestValidateInvocationOutputMode_AllowsReplayInvocation(t *testing.T) {
	t.Parallel()

	text := "Plan the sprint"
	err := validateInvocationOutputMode(RunConfig{
		InvocationOutputMode:     InvocationOutputResponseStream,
		InvocationPositionalText: &text,
		ReplayPath:               "/tmp/replay.json",
	}, true)
	if err != nil {
		t.Fatalf("validateInvocationOutputMode replay: %v", err)
	}
}

func TestValidateInvocationOutputMode_AllowsJSONResponseStream(t *testing.T) {
	t.Parallel()

	text := "Plan the sprint"
	err := validateInvocationOutputMode(RunConfig{
		InvocationOutputMode:     InvocationOutputResponseStream,
		JSONOutput:               true,
		InvocationPositionalText: &text,
	}, true)
	if err != nil {
		t.Fatalf("validateInvocationOutputMode with JSON: %v", err)
	}
}
