package initcmd

import (
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// ScaffoldType names a supported init scaffold.
type ScaffoldType string

const (
	// DefaultScaffoldType is the existing single-step task-processing scaffold.
	DefaultScaffoldType ScaffoldType = "default"
	// RalphScaffoldType is the minimal PRD-to-execution scaffold.
	RalphScaffoldType ScaffoldType = "ralph"

	// DefaultFactoryInputType is the work type created by the default scaffold.
	DefaultFactoryInputType = "task"
	// RalphFactoryInputType is the request intake work type for the Ralph scaffold.
	RalphFactoryInputType = "request"
)

type scaffoldDefinition struct {
	inputWorkType string
	files         map[string]string
}

var supportedScaffoldTypes = []ScaffoldType{
	DefaultScaffoldType,
	RalphScaffoldType,
}

func resolveScaffoldDefinition(rawType string) (ScaffoldType, scaffoldDefinition, error) {
	scaffoldType := DefaultScaffoldType
	if rawType != "" {
		scaffoldType = ScaffoldType(rawType)
	}

	switch scaffoldType {
	case DefaultScaffoldType:
		return scaffoldType, defaultScaffoldDefinition(), nil
	case RalphScaffoldType:
		return scaffoldType, ralphScaffoldDefinition(), nil
	default:
		return "", scaffoldDefinition{}, fmt.Errorf(
			"unsupported scaffold type %q (supported: %s)",
			rawType,
			supportedScaffoldTypesString(),
		)
	}
}

func supportedScaffoldTypesString() string {
	parts := make([]string, 0, len(supportedScaffoldTypes))
	for _, scaffoldType := range supportedScaffoldTypes {
		parts = append(parts, string(scaffoldType))
	}
	return strings.Join(parts, ", ")
}

func defaultScaffoldDefinition() scaffoldDefinition {
	return scaffoldDefinition{
		inputWorkType: DefaultFactoryInputType,
		files: map[string]string{
			interfaces.FactoryConfigFile: `{
  "name": "factory",
  "workTypes": [
    {
      "name": "task",
      "states": [
        { "name": "init", "type": "INITIAL" },
        { "name": "complete", "type": "TERMINAL" },
        { "name": "failed", "type": "FAILED" }
      ]
    }
  ],
  "workers": [
    { "name": "processor" }
  ],
  "workstations": [
    {
      "name": "process",
      "worker": "processor",
      "inputs": [{ "workType": "task", "state": "init" }],
      "outputs": [{ "workType": "task", "state": "complete" }],
      "onFailure": [{ "workType": "task", "state": "failed" }]
    }
  ]
}
`,
			factoryWorkersDirName + "/README.md": `# Workers

Worker configuration files go here.
Each subdirectory contains an AGENTS.md defining a worker type with its execution settings.
`,
			factoryWorkstationsDirName + "/README.md": `# Workstations

Workstation configuration files go here.
Each subdirectory contains an AGENTS.md defining the workstation prompt template.
`,
			factoryInputsDirName + "/README.md": `# Inputs

Use the default starter inbox for local task submissions:
  inputs/task/default/                 - Markdown or JSON task submissions

Seed your starter work by adding files to this inbox, then run the starter to process them.
The file watcher monitors this directory tree and automatically watches new subdirectories.
`,
			factoryWorkersDirName + "/processor/" + factoryAgentsFileName: `---
type: MODEL_WORKER
modelProvider: CODEX
executorProvider: SCRIPT_WRAP
resources: ["agent-slot"]
timeout: 1h
skipPermissions: true
---`,
			factoryWorkstationsDirName + "/process/" + factoryAgentsFileName: `---
type: MODEL_WORKSTATION
---

You are processing work item {{ (index .Inputs 0).WorkID }} of type {{ (index .Inputs 0).WorkTypeID }}.

The customer has asked you to perform the following request:

{{ (index .Inputs 0).Payload }}
`,
		},
	}
}
