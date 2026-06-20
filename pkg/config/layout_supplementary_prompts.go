package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/portpowered/infinite-you/pkg/config/builtingoal"
)

func writeSupplementaryWorkstationPromptFiles(workstationDir, workstationName string) error {
	supplementaryPrompts := builtingoal.SupplementaryWorkstationPromptFiles(workstationName)
	for promptFile, promptContent := range supplementaryPrompts {
		promptPath, err := safePromptFilePath(workstationDir, promptFile)
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
