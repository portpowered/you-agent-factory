package submit

import (
	"bytes"
	"strings"
	"testing"
)

func TestResolveBatchInput_RejectsUnsupportedModes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  BatchConfig
		want string
	}{
		{
			name: "no args interactive tty",
			cfg: BatchConfig{
				StdinIsTTY: func() bool { return true },
			},
			want: "batch input required",
		},
		{
			name: "inline json",
			cfg:  BatchConfig{Args: []string{`{"requestId":"x"}`}},
			want: "inline JSON is not yet supported",
		},
		{
			name: "file flag",
			cfg:  BatchConfig{FileFlag: "./batch.json"},
			want: "--file is not yet supported",
		},
		{
			name: "too many args",
			cfg:  BatchConfig{Args: []string{"a.json", "b.json"}},
			want: "at most one positional",
		},
		{
			name: "empty piped stdin",
			cfg: BatchConfig{
				Stdin:      strings.NewReader("   \n"),
				StdinIsTTY: func() bool { return false },
			},
			want: "stdin input is empty",
		},
		{
			name: "empty explicit stdin dash",
			cfg: BatchConfig{
				Args:  []string{"-"},
				Stdin: strings.NewReader(""),
			},
			want: "stdin input is empty",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := resolveBatchInput(tc.cfg)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestResolveBatchInput_ReadsPipedStdinWithNoArgs(t *testing.T) {
	t.Parallel()

	json := validBatchJSON("batch-stdin-pipe", "alpha")
	cfg := BatchConfig{
		Stdin:      strings.NewReader(json),
		StdinIsTTY: func() bool { return false },
	}

	resolved, err := resolveBatchInput(cfg)
	if err != nil {
		t.Fatalf("resolveBatchInput: %v", err)
	}
	if resolved.source != batchSourceStdin {
		t.Fatalf("source = %q, want %q", resolved.source, batchSourceStdin)
	}
	if !bytes.Contains(resolved.data, []byte("batch-stdin-pipe")) {
		t.Fatalf("data = %q, want batch JSON", resolved.data)
	}
}

func TestResolveBatchInput_ReadsExplicitStdinDash(t *testing.T) {
	t.Parallel()

	json := validBatchJSON("batch-stdin-dash", "alpha")
	cfg := BatchConfig{
		Args:  []string{"-"},
		Stdin: strings.NewReader(json),
	}

	resolved, err := resolveBatchInput(cfg)
	if err != nil {
		t.Fatalf("resolveBatchInput: %v", err)
	}
	if resolved.source != batchSourceStdin {
		t.Fatalf("source = %q, want %q", resolved.source, batchSourceStdin)
	}
}

func TestResolveBatchInput_FilePathIgnoresStdin(t *testing.T) {
	t.Parallel()

	path := writeBatchFile(t, validBatchJSON("batch-file-wins", "alpha"))
	cfg := BatchConfig{
		Args:  []string{path},
		Stdin: strings.NewReader(`{"requestId":"wrong","type":"FACTORY_REQUEST_BATCH","works":[]}`),
	}

	resolved, err := resolveBatchInput(cfg)
	if err != nil {
		t.Fatalf("resolveBatchInput: %v", err)
	}
	if resolved.source != batchSourceFile {
		t.Fatalf("source = %q, want %q", resolved.source, batchSourceFile)
	}
	if !bytes.Contains(resolved.data, []byte("batch-file-wins")) {
		t.Fatalf("data = %q, want file contents", resolved.data)
	}
}
