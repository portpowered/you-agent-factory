package loading

import (
	"fmt"
	factoryeffects "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/effects"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

type pathRequiredToolChecker struct {
	lookPath     factoryeffects.RequiredToolPathLookup
	versionProbe factoryeffects.RequiredToolVersionProbe
}

// NewPathRequiredToolChecker constructs the Factory Definitions external-tool
// availability adapter used by the application composition root.
func NewPathRequiredToolChecker(
	lookPath factoryeffects.RequiredToolPathLookup,
	versionProbe factoryeffects.RequiredToolVersionProbe,
) (factorydefinitions.RequiredToolChecker, error) {
	if lookPath == nil {
		return nil, fmt.Errorf("required-tool executable path lookup is required")
	}
	if versionProbe == nil {
		return nil, fmt.Errorf("required-tool version probe is required")
	}
	return pathRequiredToolChecker{
		lookPath:     lookPath,
		versionProbe: versionProbe,
	}, nil
}

func (p pathRequiredToolChecker) Check(
	tool factorydefinitions.RequiredToolConfig,
) factorydefinitions.RequiredToolCheckResult {
	command := strings.TrimSpace(tool.Command)
	if command == "" {
		return factorydefinitions.RequiredToolCheckResult{}
	}

	resolvedPath, err := p.lookPath(command)
	if err != nil {
		return factorydefinitions.RequiredToolCheckResult{
			FailureKind: factorydefinitions.RequiredToolFailureKindMissing,
			Err: fmt.Errorf(
				"required tool %q command %q was not found on PATH",
				tool.Name,
				tool.Command,
			),
		}
	}
	if len(tool.VersionArgs) == 0 {
		return factorydefinitions.RequiredToolCheckResult{ResolvedPath: resolvedPath}
	}

	output, err := p.versionProbe(resolvedPath, tool.VersionArgs...)
	if err == nil {
		return factorydefinitions.RequiredToolCheckResult{ResolvedPath: resolvedPath}
	}
	message := fmt.Sprintf(
		"required tool %q command %q failed version probe %q: %v",
		tool.Name,
		tool.Command,
		strings.Join(tool.VersionArgs, " "),
		err,
	)
	if trimmed := strings.TrimSpace(string(output)); trimmed != "" {
		message += fmt.Sprintf(" (%s)", trimmed)
	}
	return factorydefinitions.RequiredToolCheckResult{
		ResolvedPath: resolvedPath,
		FailureKind:  factorydefinitions.RequiredToolFailureKindVersionProbe,
		Err:          fmt.Errorf("%s", message),
	}
}
