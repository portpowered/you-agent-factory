// Package providerprofiles contains the repository-owned runtime profile
// identities shared by offline package generation and Providers composition.
// The profiles are names for typed runtime implementations; they do not
// perform discovery or launch external processes.
package providerprofiles

const ACPAgentImplementationKind = "acp_agent"

var acpProfileIDs = []string{
	"copilot-acp",
	"cursor-acp",
	"droid-acp",
	"fast-agent-acp",
	"gemini-acp",
	"grok-build-acp",
	"iflow-acp",
	"kilocode-acp",
	"kimi-acp",
	"kiro-acp",
	"mux-acp",
	"openclaw-acp",
	"opencode-acp",
	"pi-acp",
	"pool-acp",
	"qoder-acp",
	"qwen-acp",
	"reasonix-acp",
	"trae-acp",
	"zeroclaw-acp",
}

// RegisteredACPProfileIDs returns a detached, deterministic list of runtime
// profiles registered by the ACP implementation owner.
func RegisteredACPProfileIDs() []string {
	return append([]string(nil), acpProfileIDs...)
}
