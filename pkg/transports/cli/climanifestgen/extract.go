package climanifestgen

import (
	"fmt"

	"github.com/portpowered/infinite-you/pkg/services/work/transports/cli/climanifest"
)

// ExtractRepresentativeFamily returns manifest metadata for exactly the
// representative root/session-show command IDs declared in commands.json.
func ExtractRepresentativeFamily(manifest climanifest.Manifest) (climanifest.Manifest, error) {
	return extractFamily(manifest, "representative", RepresentativeFamilyCommandIDs)
}

// ExtractSessionFamily returns manifest metadata for the complete canonical
// Factory Session parent and runnable leaves.
func ExtractSessionFamily(manifest climanifest.Manifest) (climanifest.Manifest, error) {
	return extractFamily(manifest, "session", SessionFamilyCommandIDs)
}

// ExtractWorkFamily returns manifest metadata for exactly the work
// inspection/control command IDs declared in commands.json.
func ExtractWorkFamily(manifest climanifest.Manifest) (climanifest.Manifest, error) {
	return extractFamily(manifest, "work", WorkFamilyCommandIDs)
}

// ExtractWorkersFamily returns customer-facing worker integration commands.
func ExtractWorkersFamily(manifest climanifest.Manifest) (climanifest.Manifest, error) {
	return extractFamily(manifest, "workers", WorkersFamilyCommandIDs)
}

// ExtractWorkerSessionsFamily returns customer-facing worker-session
// observation commands.
func ExtractWorkerSessionsFamily(manifest climanifest.Manifest) (climanifest.Manifest, error) {
	return extractFamily(manifest, "worker sessions", WorkerSessionsFamilyCommandIDs)
}

// ExtractRunSubmitFamily returns manifest metadata for exactly the run/submit
// command IDs declared in commands.json.
func ExtractRunSubmitFamily(manifest climanifest.Manifest) (climanifest.Manifest, error) {
	return extractFamily(manifest, "run/submit", RunSubmitFamilyCommandIDs)
}

// ExtractFactoryConfigInitFamily returns manifest metadata for exactly the
// factory/config/init command IDs declared in commands.json.
func ExtractFactoryConfigInitFamily(manifest climanifest.Manifest) (climanifest.Manifest, error) {
	return extractFamily(manifest, "factory/config/init", FactoryConfigInitFamilyCommandIDs)
}

// ExtractMCPFamily returns canonical metadata for you mcp and you mcp serve.
func ExtractMCPFamily(manifest climanifest.Manifest) (climanifest.Manifest, error) {
	return extractFamily(manifest, "canonical MCP", MCPFamilyCommandIDs)
}

// ExtractServeFamily returns canonical metadata for you serve and you serve acp.
func ExtractServeFamily(manifest climanifest.Manifest) (climanifest.Manifest, error) {
	return extractFamily(manifest, "canonical serve", ServeFamilyCommandIDs)
}

func extractFamily(
	manifest climanifest.Manifest,
	familyLabel string,
	commandIDs []string,
) (climanifest.Manifest, error) {
	if manifest.RootPath == "" {
		return climanifest.Manifest{}, fmt.Errorf("CLI command manifest missing rootPath")
	}
	if len(manifest.Commands) == 0 {
		return climanifest.Manifest{}, fmt.Errorf("CLI command manifest missing commands")
	}

	commands := make(map[string]climanifest.Command, len(commandIDs))
	for _, id := range commandIDs {
		record, ok := manifest.Commands[id]
		if !ok {
			return climanifest.Manifest{}, fmt.Errorf(
				"production manifest missing %s-family command %q",
				familyLabel,
				id,
			)
		}
		commands[id] = record
	}

	return climanifest.Manifest{
		FormatVersion: manifest.FormatVersion,
		RootPath:      manifest.RootPath,
		Commands:      commands,
	}, nil
}

// ExtractModelsDocsFamily returns manifest metadata for exactly the models/docs
// command IDs declared in commands.json.
func ExtractModelsDocsFamily(manifest climanifest.Manifest) (climanifest.Manifest, error) {
	if manifest.RootPath == "" {
		return climanifest.Manifest{}, fmt.Errorf("CLI command manifest missing rootPath")
	}
	if len(manifest.Commands) == 0 {
		return climanifest.Manifest{}, fmt.Errorf("CLI command manifest missing commands")
	}

	commands := make(map[string]climanifest.Command, len(ModelsDocsFamilyCommandIDs))
	for _, id := range ModelsDocsFamilyCommandIDs {
		record, ok := manifest.Commands[id]
		if !ok {
			return climanifest.Manifest{}, fmt.Errorf("production manifest missing models/docs-family command %q", id)
		}
		commands[id] = record
	}

	return climanifest.Manifest{
		FormatVersion: manifest.FormatVersion,
		RootPath:      manifest.RootPath,
		Commands:      commands,
	}, nil
}

// ExtractProvidersFamily returns manifest metadata for exactly the providers
// capability-discovery command IDs declared in commands.json.
func ExtractProvidersFamily(manifest climanifest.Manifest) (climanifest.Manifest, error) {
	return extractFamily(manifest, "providers", ProvidersFamilyCommandIDs)
}

// ExtractMetricsFamily returns manifest metadata for the local runtime-metrics
// inspection command.
func ExtractMetricsFamily(manifest climanifest.Manifest) (climanifest.Manifest, error) {
	return extractFamily(manifest, "metrics", MetricsFamilyCommandIDs)
}
