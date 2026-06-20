package builtingoal

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// WriteSupplementaryWorkstationPromptFiles materializes extra prompt files under an
// existing workstation directory without changing factory topology.
func WriteSupplementaryWorkstationPromptFiles(workstationDir, workstationName string) error {
	for promptFile, promptContent := range SupplementaryWorkstationPromptFiles(workstationName) {
		promptPath, err := safeSupplementaryPromptPath(workstationDir, promptFile)
		if err != nil {
			return fmt.Errorf("resolve workstation %q supplementary prompt file: %w", workstationName, err)
		}
		if err := os.MkdirAll(filepath.Dir(promptPath), 0o755); err != nil {
			return fmt.Errorf("create workstation %q supplementary prompt directory: %w", workstationName, err)
		}
		if err := os.WriteFile(promptPath, []byte(promptContent), 0o644); err != nil {
			return fmt.Errorf("write workstation %q supplementary prompt file: %w", workstationName, err)
		}
	}
	return nil
}

func safeSupplementaryPromptPath(workstationDir, promptFile string) (string, error) {
	cleaned := filepath.Clean(strings.TrimSpace(promptFile))
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("prompt file %q must be relative to the workstation directory", promptFile)
	}
	promptPath := filepath.Join(workstationDir, cleaned)
	relative, err := filepath.Rel(workstationDir, promptPath)
	if err != nil {
		return "", err
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("prompt file %q cannot escape the workstation directory", promptFile)
	}
	return promptPath, nil
}
