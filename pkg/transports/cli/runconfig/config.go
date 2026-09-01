// Package runconfig defines the parsed parameters passed from the CLI to the
// initializer without depending on run-command behavior.
package runconfig

import (
	"context"
	"io"

	"github.com/portpowered/infinite-you/pkg/initializer"
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
	InvocationInputSourceFile       InvocationInputSource = "file prompt"
	InvocationInputSourceNamed      InvocationInputSource = "named prompt"
	InvocationInputSourceWorkFile   InvocationInputSource = "work file"
)

// Config holds parameters parsed by the run command.
type Config struct {
	Workflow     string
	Continuously bool
	// Cancellation is the invocation-local authority supplied by the
	// application process for hosted administrative stop requests.
	Cancellation initializer.InvocationCancellation
	WorkFile     string
	Dir          string
	HomeDir      string

	NamedFactoryName       string
	NamedFactoryResolution *factorydefinitions.NamedFactoryResolution
	FactoryConfigPath      string

	InvocationPositionalText      *string
	InvocationStdinText           *string
	InvocationFilePath            string
	InvocationFileExplicit        bool
	InvocationNormalizedArguments *work.NormalizedArguments
	PreparedInvocationInput       *work.PreparedInvocationInput
	InvocationArguments           *work.InvocationArguments
	RunnerID                      string
	ProviderOverride              string
	ModelOverride                 string
	WorkerReasoningEffort         string
	Worktree                      string
	OperatorDefaults              operatorconfig.ResolvedDefaults
	ACPIntegrations               []operatorconfig.ACPIntegration
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
	ListenAddress                 string
	ListenExplicit                bool
	Pprof                         bool
	Port                          int
	AutoPort                      bool
	RecordPath                    string
	ReplayPath                    string
	ResumePath                    string
	DisableDefaultRecording       bool
	RecordingTargetPlanner        recordings.LiveRecordingTargetPlanner
	CanonicalSessionID            string
	FactorySessionID              string
	CanonicalSessionIDGenerator   factorysessions.SessionIDGenerator
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
	// ReplayMetadataOutput is the raw human stdout sink used for non-fatal
	// replay drift disclosure even when quiet mode suppresses normal output.
	ReplayMetadataOutput io.Writer
	ProgressOutput       io.Writer
	OpenDashboard        bool
	StartupOutput        io.Writer
	Diagnostics          io.Writer
	// DeferHomeDisclosureUntilHostReady keeps an explicitly selected listener
	// failure free of human startup output while preserving the home-before-
	// initialization boundary for ordinary auto-port hosting.
	DeferHomeDisclosureUntilHostReady bool
	Stdin                             io.Reader
	StdinIsTTY                        func() bool
	OutputIsTTY                       bool
	ProgressIsTTY                     bool
	JSONOutput                        bool
	InvocationOutputMode              string
	InvocationOutputExplicit          bool
	InvocationMetricsRecorder         InvocationMetricsRecorder

	InvocationSkipPermissionsOverride *bool
	Logger                            *zap.Logger
	// StartupPreparation is the process-owned gate for local activation. The
	// CLI installs it after resolving invocation inputs; the run transport
	// invokes it at the applicable startup boundary so human disclosure can
	// precede system initialization without contaminating machine output. The
	// writer argument lets hosted startup stage that disclosure until the
	// listener has proved ready, while still recording it before runtime work.
	StartupPreparation func(context.Context, bool, io.Writer) error
	// StartupDisclosureCommit flushes a home disclosure staged before hosted
	// startup. It is installed only when a listener failure must remain free of
	// human startup output; the run transport calls it after the host is ready.
	StartupDisclosureCommit func()
	// StartupPreflightBlocked records a missing or invalid local input selected
	// during startup preparation. The runtime still owns the authoritative
	// input load so custom replay readers remain supported.
	StartupPreflightBlocked bool
}

// InvocationMetricsRecorder is the observability role consumed by run config.
type InvocationMetricsRecorder interface {
	RecordInvocationMetric(factorysessions.InvocationMetric)
}
