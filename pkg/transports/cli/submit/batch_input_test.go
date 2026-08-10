package submit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"strings"
	"testing"
	"time"

	workservice "github.com/portpowered/infinite-you/pkg/services/work"
)

type factoryRequestBatchPreparationFunc func(context.Context, []byte) (workservice.PreparedFactoryRequestBatch, error)

func (prepare factoryRequestBatchPreparationFunc) PrepareFactoryRequestBatch(
	ctx context.Context,
	data []byte,
) (workservice.PreparedFactoryRequestBatch, error) {
	return prepare(ctx, data)
}

type testFactoryRequestBatchPreparation struct{}

func (testFactoryRequestBatchPreparation) PrepareFactoryRequestBatch(
	_ context.Context,
	data []byte,
) (workservice.PreparedFactoryRequestBatch, error) {
	var request workservice.WorkRequest
	if err := json.Unmarshal(data, &request); err != nil {
		return workservice.PreparedFactoryRequestBatch{}, err
	}
	return workservice.PreparedFactoryRequestBatch{
		Request:       request,
		CanonicalJSON: append([]byte(nil), data...),
	}, nil
}

type batchInputFileSystemFake struct {
	files     map[string][]byte
	statError map[string]error
	readError map[string]error
}

func (f batchInputFileSystemFake) Stat(path string) (fs.FileInfo, error) {
	if err := f.statError[path]; err != nil {
		return nil, err
	}
	if _, ok := f.files[path]; !ok {
		return nil, fs.ErrNotExist
	}
	return batchInputFileInfo{name: path}, nil
}

func (f batchInputFileSystemFake) ReadFile(path string) ([]byte, error) {
	if err := f.readError[path]; err != nil {
		return nil, err
	}
	data, ok := f.files[path]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return append([]byte(nil), data...), nil
}

type batchInputFileInfo struct{ name string }

func (i batchInputFileInfo) Name() string     { return i.name }
func (batchInputFileInfo) Size() int64        { return 0 }
func (batchInputFileInfo) Mode() fs.FileMode  { return 0 }
func (batchInputFileInfo) ModTime() time.Time { return time.Time{} }
func (batchInputFileInfo) IsDir() bool        { return false }
func (batchInputFileInfo) Sys() any           { return nil }

func TestResolveBatchInput_RejectsUnsupportedModes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  BatchConfig
		want string
	}{
		{
			name: "no args interactive tty",
			cfg: BatchConfig{Context: context.Background(),
				StdinIsTTY: func() bool { return true },
			},
			want: "batch input required",
		},
		{
			name: "nonexistent path without json prefix",
			cfg: BatchConfig{Context: context.Background(),
				Args: []string{"missing.json"}, FileSystem: batchInputFileSystemFake{},
			},
			want: "batch file not found",
		},
		{
			name: "file flag missing path",
			cfg:  BatchConfig{Context: context.Background(), FileFlag: "missing.json", FileSystem: batchInputFileSystemFake{}},
			want: "batch file not found",
		},
		{
			name: "too many args",
			cfg:  BatchConfig{Context: context.Background(), Args: []string{"a.json", "b.json"}},
			want: "at most one positional",
		},
		{
			name: "empty piped stdin",
			cfg: BatchConfig{Context: context.Background(),
				Stdin:      strings.NewReader("   \n"),
				StdinIsTTY: func() bool { return false },
			},
			want: "stdin input is empty",
		},
		{
			name: "empty explicit stdin dash",
			cfg: BatchConfig{Context: context.Background(),
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

func TestResolveBatchInput_RequiresInjectedFileSystemForFileSource(t *testing.T) {
	t.Parallel()

	_, err := resolveBatchInput(BatchConfig{Context: context.Background(), Args: []string{"batch.json"}})
	if err == nil || !strings.Contains(err.Error(), "batch input file system is required") {
		t.Fatalf("error = %v, want required injected file system", err)
	}
}

func TestResolveBatchInput_RequiresProcessStdinForStdinSource(t *testing.T) {
	t.Parallel()

	_, err := resolveBatchInput(BatchConfig{Context: context.Background(), Args: []string{"-"}})
	if err == nil || !strings.Contains(err.Error(), "process stdin reader is required") {
		t.Fatalf("error = %v, want required process stdin", err)
	}
}

func TestResolveBatchInput_PreservesStatAndReadFailureStages(t *testing.T) {
	t.Parallel()

	statFailure := errors.New("stat denied")
	_, err := resolveBatchInput(BatchConfig{Context: context.Background(),
		Args: []string{"batch.json"},
		FileSystem: batchInputFileSystemFake{
			statError: map[string]error{"batch.json": statFailure},
		},
	})
	if !errors.Is(err, statFailure) || !strings.Contains(err.Error(), "batch file batch.json") {
		t.Fatalf("stat error = %v, want staged batch-file failure", err)
	}

	readFailure := errors.New("read denied")
	_, err = resolveBatchInput(BatchConfig{Context: context.Background(),
		FileFlag: "batch.json",
		FileSystem: batchInputFileSystemFake{
			files:     map[string][]byte{"batch.json": []byte(`{}`)},
			readError: map[string]error{"batch.json": readFailure},
		},
	})
	if !errors.Is(err, readFailure) || !strings.Contains(err.Error(), "read batch.json") {
		t.Fatalf("read error = %v, want staged read failure", err)
	}
}

func TestResolveBatchInput_ReadsPipedStdinWithNoArgs(t *testing.T) {
	t.Parallel()

	json := validBatchJSON("batch-stdin-pipe", "alpha")
	cfg := BatchConfig{Context: context.Background(),
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
	cfg := BatchConfig{Context: context.Background(),
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

func TestResolveBatchInput_ReadsInlineJSONPositional(t *testing.T) {
	t.Parallel()

	json := validBatchJSON("batch-inline", "alpha")
	cfg := BatchConfig{Context: context.Background(), Args: []string{json}}

	resolved, err := resolveBatchInput(cfg)
	if err != nil {
		t.Fatalf("resolveBatchInput: %v", err)
	}
	if resolved.source != batchSourceInline {
		t.Fatalf("source = %q, want %q", resolved.source, batchSourceInline)
	}
	if resolved.label != "inline JSON" {
		t.Fatalf("label = %q, want inline JSON", resolved.label)
	}
	if !bytes.Contains(resolved.data, []byte("batch-inline")) {
		t.Fatalf("data = %q, want inline batch JSON", resolved.data)
	}
}

func TestResolveBatchInput_InlineJSONIgnoresLeadingWhitespace(t *testing.T) {
	t.Parallel()

	json := "  \t\n" + validBatchJSON("batch-inline-ws", "alpha")
	cfg := BatchConfig{Context: context.Background(), Args: []string{json}}

	resolved, err := resolveBatchInput(cfg)
	if err != nil {
		t.Fatalf("resolveBatchInput: %v", err)
	}
	if resolved.source != batchSourceInline {
		t.Fatalf("source = %q, want %q", resolved.source, batchSourceInline)
	}
}

func TestResolveBatchInput_NonexistentJSONLookingPathParsesInlineNotFile(t *testing.T) {
	t.Parallel()

	json := validBatchJSON("batch-inline-not-a-file", "alpha")
	cfg := BatchConfig{Context: context.Background(), Args: []string{json}}

	resolved, err := resolveBatchInput(cfg)
	if err != nil {
		t.Fatalf("resolveBatchInput: %v", err)
	}
	if resolved.source != batchSourceInline {
		t.Fatalf("source = %q, want inline not filesystem", resolved.source)
	}
}

func TestResolveBatchInput_ReadsFileFlag(t *testing.T) {
	t.Parallel()

	path := "batch-file-flag.json"
	cfg := BatchConfig{Context: context.Background(),
		FileFlag: path,
		FileSystem: batchInputFileSystemFake{files: map[string][]byte{
			path: []byte(validBatchJSON("batch-file-flag", "alpha")),
		}},
	}

	resolved, err := resolveBatchInput(cfg)
	if err != nil {
		t.Fatalf("resolveBatchInput: %v", err)
	}
	if resolved.source != batchSourceFile {
		t.Fatalf("source = %q, want %q", resolved.source, batchSourceFile)
	}
	if resolved.label != path {
		t.Fatalf("label = %q, want %q", resolved.label, path)
	}
	if !bytes.Contains(resolved.data, []byte("batch-file-flag")) {
		t.Fatalf("data = %q, want file contents", resolved.data)
	}
}

func TestResolveBatchInput_FileFlagStdinDash(t *testing.T) {
	t.Parallel()

	json := validBatchJSON("batch-file-stdin", "alpha")
	cfg := BatchConfig{Context: context.Background(),
		FileFlag: "-",
		Stdin:    strings.NewReader(json),
	}

	resolved, err := resolveBatchInput(cfg)
	if err != nil {
		t.Fatalf("resolveBatchInput: %v", err)
	}
	if resolved.source != batchSourceStdin {
		t.Fatalf("source = %q, want %q", resolved.source, batchSourceStdin)
	}
}

func TestResolveBatchInput_FileFlagWinsOverPositional(t *testing.T) {
	t.Parallel()

	flagPath := "batch-flag-wins.json"
	posPath := "batch-pos-loses.json"
	cfg := BatchConfig{Context: context.Background(),
		FileFlag: flagPath,
		Args:     []string{posPath},
		FileSystem: batchInputFileSystemFake{files: map[string][]byte{
			flagPath: []byte(validBatchJSON("batch-flag-wins", "alpha")),
			posPath:  []byte(validBatchJSON("batch-pos-loses", "beta")),
		}},
	}

	resolved, err := resolveBatchInput(cfg)
	if err != nil {
		t.Fatalf("resolveBatchInput: %v", err)
	}
	if resolved.source != batchSourceFile {
		t.Fatalf("source = %q, want %q", resolved.source, batchSourceFile)
	}
	if resolved.label != flagPath {
		t.Fatalf("label = %q, want flag path %q", resolved.label, flagPath)
	}
	if !bytes.Contains(resolved.data, []byte("batch-flag-wins")) {
		t.Fatalf("data = %q, want --file contents not positional", resolved.data)
	}
}

func TestResolveBatchInput_FileFlagIgnoresStdin(t *testing.T) {
	t.Parallel()

	path := "batch-file-flag-ignores-stdin.json"
	cfg := BatchConfig{Context: context.Background(),
		FileFlag: path,
		Stdin:    strings.NewReader(`{"requestId":"wrong","type":"FACTORY_REQUEST_BATCH","works":[]}`),
		FileSystem: batchInputFileSystemFake{files: map[string][]byte{
			path: []byte(validBatchJSON("batch-file-flag-ignores-stdin", "alpha")),
		}},
	}

	resolved, err := resolveBatchInput(cfg)
	if err != nil {
		t.Fatalf("resolveBatchInput: %v", err)
	}
	if resolved.source != batchSourceFile {
		t.Fatalf("source = %q, want %q", resolved.source, batchSourceFile)
	}
	if !bytes.Contains(resolved.data, []byte("batch-file-flag-ignores-stdin")) {
		t.Fatalf("data = %q, want --file contents", resolved.data)
	}
}

func TestResolveBatchInput_FilePathIgnoresStdin(t *testing.T) {
	t.Parallel()

	path := "batch-file-wins.json"
	cfg := BatchConfig{Context: context.Background(),
		Args:  []string{path},
		Stdin: strings.NewReader(`{"requestId":"wrong","type":"FACTORY_REQUEST_BATCH","works":[]}`),
		FileSystem: batchInputFileSystemFake{files: map[string][]byte{
			path: []byte(validBatchJSON("batch-file-wins", "alpha")),
		}},
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
