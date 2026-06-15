// Package reference holds canonical CLI reference markdown and the embedded
// filesystem used by the packaged you docs surface.
package reference

import "embed"

// PackagedTopics is the embedded markdown for every topic registered in
// pkg/cli/docs. Authoritative content lives only in this directory.
var (
	//go:embed agents.md authoring-factories.md batch-inputs.md config.md guards.md mcp-hosts.md mock-workers.md models.md orchestrators.md packaged-tts.md record-replay.md relationships.md resources.md sessions.md templates.md work.md workers.md workstations.md
	PackagedTopics embed.FS
)
