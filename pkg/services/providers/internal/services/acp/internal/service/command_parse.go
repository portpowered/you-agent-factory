package service

import (
	"strings"

	"github.com/mattn/go-shellwords"
)

// parseACPCommand preserves Windows executable paths while retaining the
// existing shell-style parsing for portable ACP launch commands.
func parseACPCommand(command string) ([]string, error) {
	first := strings.TrimSpace(command)
	first = strings.TrimPrefix(first, `"`)
	if len(first) >= 3 && isASCIIAlpha(first[0]) && first[1] == ':' && (first[2] == '\\' || first[2] == '/') {
		command = strings.ReplaceAll(command, `\`, `\\`)
	}
	return shellwords.Parse(command)
}

func isASCIIAlpha(value byte) bool {
	return value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z'
}
