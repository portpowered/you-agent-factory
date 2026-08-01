package wire

import "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"

// These aliases expose only the construction boundary needed by the parent
// composition graph. Models peers continue to depend on models.Service.
type PullMetric = effects.PullMetric
type PullMetricsRecorder = effects.PullMetricsRecorder
type HostDiagnosticLogger = effects.HostDiagnosticLogger
type HostMetricsRecorder = effects.HostMetricsRecorder
type LocalRuntimeHooks = effects.LocalRuntimeHooks
type ProcessDependencies = effects.ProcessDependencies

type AssetHTTPDoer = effects.AssetHTTPDoer
type AssetMakeDirectories = effects.AssetMakeDirectories
type AssetInspectPath = effects.AssetInspectPath
type AssetResolveHomeDirectory = effects.AssetResolveHomeDirectory
type AssetWriteFile = effects.AssetWriteFile
type AssetRenamePath = effects.AssetRenamePath
type AssetRemovePath = effects.AssetRemovePath
type AssetReadFile = effects.AssetReadFile
type AssetReadDirectory = effects.AssetReadDirectory
type AssetCreateFile = effects.AssetCreateFile
type AssetOpenFile = effects.AssetOpenFile

type HostProcessStartSpec = effects.HostProcessStartSpec
type HostManagedProcess = effects.HostManagedProcess
type HostProcessLauncher = effects.HostProcessLauncher
type HostHTTPDoer = effects.HostHTTPDoer
type HostTimer = effects.HostTimer
type HostClock = effects.HostClock

type RuntimeHTTPDoer = effects.RuntimeHTTPDoer
type RuntimeTempFile = effects.RuntimeTempFile
type RuntimeInspectFile = effects.RuntimeInspectFile
type RuntimeTempDirectory = effects.RuntimeTempDirectory
type RuntimeCreateTempFile = effects.RuntimeCreateTempFile

type InvocationArtifactFileSystem = effects.InvocationArtifactFileSystem
type InvocationArtifactExporter = effects.InvocationArtifactExporter
