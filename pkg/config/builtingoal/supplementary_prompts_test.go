package builtingoal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteSupplementaryWorkstationPromptFiles_MaterializesReviewGoalSummarizerPrompt(t *testing.T) {
	workstationDir := filepath.Join(t.TempDir(), "review-goal")
	if err := os.MkdirAll(workstationDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	if err := WriteSupplementaryWorkstationPromptFiles(workstationDir, "review-goal"); err != nil {
		t.Fatalf("WriteSupplementaryWorkstationPromptFiles: %v", err)
	}

	promptPath := filepath.Join(workstationDir, "prompts", "summarizer.md")
	data, err := os.ReadFile(promptPath)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", promptPath, err)
	}
	if got := strings.TrimSpace(string(data)); got != strings.TrimSpace(summarizerPrompt) {
		t.Fatalf("materialized summarizer prompt = %q, want authored source", got)
	}
}

func TestWriteSupplementaryWorkstationPromptFiles_OmitsUnknownWorkstations(t *testing.T) {
	workstationDir := t.TempDir()
	if err := WriteSupplementaryWorkstationPromptFiles(workstationDir, "plan-goal"); err != nil {
		t.Fatalf("WriteSupplementaryWorkstationPromptFiles: %v", err)
	}
	entries, err := os.ReadDir(workstationDir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("unexpected files for plan-goal supplementary prompts: %#v", entries)
	}
}

func TestSafeSupplementaryPromptPath_RejectsEscape(t *testing.T) {
	workstationDir := t.TempDir()
	_, err := safeSupplementaryPromptPath(workstationDir, "../escape.md")
	if err == nil {
		t.Fatal("expected escape validation error")
	}
}
