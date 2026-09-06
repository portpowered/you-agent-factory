package localai

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestConfinedVibeVoiceTokenizerAcceptsOnlyOneVerifiedLayoutEntry(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	modelFile := filepath.Join(root, "model.gguf")
	tokenizerFile := filepath.Join(root, vibeVoiceTokenizerName)
	tests := []struct {
		name      string
		files     []string
		want      string
		wantError bool
	}{
		{
			name:  "unique absolute entry",
			files: []string{modelFile, tokenizerFile},
			want:  tokenizerFile,
		},
		{
			name:  "unique relative entry",
			files: []string{modelFile, vibeVoiceTokenizerName},
			want:  tokenizerFile,
		},
		{
			name:      "missing",
			files:     []string{modelFile},
			wantError: true,
		},
		{
			name:      "ambiguous",
			files:     []string{modelFile, tokenizerFile, filepath.Join(root, "voices", vibeVoiceTokenizerName)},
			wantError: true,
		},
		{
			name:      "absolute escape",
			files:     []string{modelFile, filepath.Join(filepath.Dir(root), "outside", vibeVoiceTokenizerName)},
			wantError: true,
		},
		{
			name:      "traversal escape",
			files:     []string{modelFile, filepath.FromSlash("../" + vibeVoiceTokenizerName)},
			wantError: true,
		},
		{
			name:      "malformed blank",
			files:     []string{modelFile, ""},
			wantError: true,
		},
		{
			name:      "wrong tokenizer name",
			files:     []string{modelFile, filepath.Join(root, "tokenizer.json")},
			wantError: true,
		},
	}
	for _, testCase := range tests {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			got, err := confinedVibeVoiceTokenizer(modelFile, testCase.files, nil)
			if testCase.wantError {
				if !errors.Is(err, errVibeVoiceLayout) {
					t.Fatalf("confinedVibeVoiceTokenizer() error = %v, want typed layout error", err)
				}
				return
			}
			if err != nil || got != testCase.want {
				t.Fatalf("confinedVibeVoiceTokenizer() = %q, %v, want %q", got, err, testCase.want)
			}
		})
	}
}

func TestConfinedVibeVoiceTokenizerRejectsSymlinkEscape(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	outside := t.TempDir()
	outsideTokenizer := filepath.Join(outside, vibeVoiceTokenizerName)
	if err := os.WriteFile(outsideTokenizer, []byte("outside"), 0o600); err != nil {
		t.Fatalf("write outside tokenizer fixture: %v", err)
	}
	linkedTokenizer := filepath.Join(root, vibeVoiceTokenizerName)
	if err := os.Symlink(outsideTokenizer, linkedTokenizer); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}
	_, err := confinedVibeVoiceTokenizer(filepath.Join(root, "model.gguf"), []string{
		filepath.Join(root, "model.gguf"), linkedTokenizer,
	}, filepath.EvalSymlinks)
	if !errors.Is(err, errVibeVoiceLayout) {
		t.Fatalf("confinedVibeVoiceTokenizer() error = %v, want symlink escape rejection", err)
	}
}

func TestConfinedVibeVoiceTokenizerRejectsInjectedResolvedEscape(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	modelFile := filepath.Join(root, "model.gguf")
	tokenizerFile := filepath.Join(root, vibeVoiceTokenizerName)
	if err := os.WriteFile(modelFile, []byte("model"), 0o600); err != nil {
		t.Fatalf("write model fixture: %v", err)
	}
	if err := os.WriteFile(tokenizerFile, []byte("tokenizer"), 0o600); err != nil {
		t.Fatalf("write tokenizer fixture: %v", err)
	}
	outside := filepath.Join(filepath.Dir(root), "outside", vibeVoiceTokenizerName)
	resolve := func(path string) (string, error) {
		if filepath.Clean(path) == filepath.Clean(tokenizerFile) {
			return outside, nil
		}
		return path, nil
	}

	_, err := confinedVibeVoiceTokenizer(modelFile, []string{modelFile, tokenizerFile}, resolve)
	if !errors.Is(err, errVibeVoiceLayout) {
		t.Fatalf("confinedVibeVoiceTokenizer() error = %v, want injected resolved escape rejection", err)
	}
}
