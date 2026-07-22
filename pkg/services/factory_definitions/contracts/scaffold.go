package factorycontracts

import "io"

type ScaffoldType string

const (
	DefaultScaffoldType ScaffoldType = "default"
	RalphScaffoldType   ScaffoldType = "ralph"

	DefaultFactoryInputType = "task"
	RalphFactoryInputType   = "request"

	DefaultStarterExecutor = "codex"
)

type ScaffoldConfig struct {
	Dir         string
	Type        string
	Executor    string
	JSON        bool
	Output      io.Writer
	Verbose     bool
	Debug       bool
	Diagnostics io.Writer
}

type ScaffoldResult struct {
	ScaffoldType string `json:"scaffoldType"`
	TargetDir    string `json:"targetDir"`
}

type ScaffoldInitializer func(ScaffoldConfig) error

func SupportedStarterExecutors() []string {
	return []string{"codex", "claude"}
}
