// Package provider owns stable model-provider runtime identities.
package modelprovider

// ID identifies the CLI command used for model inference dispatch.
type ID string

const (
	Claude   ID = "claude"
	Codex    ID = "codex"
	Gemini   ID = "gemini"
	Kiro     ID = "kiro-cli"
	Cursor   ID = "agent"
	OpenCode ID = "opencode"
	Pi       ID = "pi"
	Agy      ID = "agy"
)

// Supported returns the canonical internal provider commands used by runtime dispatch.
func Supported() []ID {
	return []ID{Claude, Codex, Gemini, Kiro, Cursor, OpenCode, Pi, Agy}
}
