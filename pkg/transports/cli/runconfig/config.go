// Package runconfig defines the parsed parameters passed from the CLI to the
// initializer without depending on run-command behavior.
package runconfig

import (
	"io"

	platformbrowser "github.com/portpowered/infinite-you/pkg/platform/browser"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformmetrics "github.com/portpowered/infinite-you/pkg/platform/metrics"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	operatorconfig "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingscli "github.com/portpowered/infinite-you/pkg/services/recordings/transports/cli"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/transports/cli/terminalpolicy"
	"go.uber.org/zap"
)

type InvocationInputSource string

const (
	InvocationInputSourcePositional InvocationInputSource = "positional prompt"
	InvocationInputSourceStdin      InvocationInputSource = "stdin"
	InvocationInputSourceWorkFile   InvocationInputSource = "work file"
)

// Config holds parameters parsed by the run command.
type Config struct {
	Workflow     string
	Continuously bool
	WorkFile     string
	Dir          string
	HomeDir      string

	NamedFactoryName       string
	NamedFactoryResolution *factorydefinitions.NamedFactoryResolution
	FactoryConfigPath      string

	InvocationPositionalText      *string
	InvocationStdinText           *string
	InvocationNormalizedArguments *work.NormalizedArguments
	PreparedInvocationInput       *work.PreparedInvocationInput
	RunnerID                      string
	OperatorDefaults              operatorconfig.ResolvedDefaults
	ExecutionBaseDir              string
	Bootstrap                     bool
	FactoryScaffoldInitializer    factorydefinitions.ScaffoldInitializer
	ResolveCurrentFactoryDir      factorydefinitions.CurrentFactoryDirectoryResolver
	ResolveFactoryConfigRoot      factorydefinitions.FactoryConfigRootResolver
	LoadFactoryConfigFile         factorydefinitions.FactoryConfigFileLoader
	WorkRequestFileLoader         work.RequestFileLoader
	DirectoryCreator              platformfilesystem.DirectoryCreator
	BrowserOpener                 platformbrowser.Opener
	BindHost                      string
	Port                          int
	AutoPort                      bool
	RecordPath                    string
	ReplayPath                    string
	DisableDefaultRecording       bool
	RecordingTargetPlanner        recordings.LiveRecordingTargetPlanner
	RecordingsCLI                 recordingscli.Adapter
	Clock                         recordings.RecordingClock
	RuntimeLogDir                 string
	RuntimeLogConfig              logging.RuntimeLogConfig
	RuntimeMetricsDir             string
	RuntimeMetricsConfig          platformmetrics.RuntimeMetricsConfig
	ModelCacheDir                 string
	MockWorkersEnabled            bool
	MockWorkersConfigPath         string
	WithServer                    bool
	WithSite                      bool
	Verbose                       bool
	TerminalPolicy                terminalpolicy.Policy
	SuppressDashboardRendering    bool
	CleanInvocation               bool
	JSON                          bool
	CleanInvocationInputSource    InvocationInputSource
	Output                        io.Writer
	OpenDashboard                 bool
	StartupOutput                 io.Writer
	Diagnostics                   io.Writer
	Stdin                         io.Reader
	StdinIsTTY                    func() bool
	OutputIsTTY                   bool
	JSONOutput                    bool
	InvocationOutputMode          string
	InvocationMetricsRecorder     InvocationMetricsRecorder

	InvocationSkipPermissionsOverride *bool
	Logger                            *zap.Logger
}

// InvocationMetricsRecorder is the observability role consumed by run config.
type InvocationMetricsRecorder interface {
	RecordInvocationMetric(factorysessions.InvocationMetric)
}
