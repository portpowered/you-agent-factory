package analyzers

import (
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// RecordingReads reserves history access for explicit recording requests.
var RecordingReads = &analysis.Analyzer{
	Name: "recordingreads",
	Doc:  "reject recording reads outside explicit recording-request declarations",
	Run:  runRecordingReads,
}

const recordingReadRule = "recording-read"

// Persist the migration even when its last debt entry is removed, so zero debt
// cannot be mistaken for an unintroduced rule and seeded again.
const recordingReadMigration = "# recording-read: established"

// Includes forwarding capabilities and full-prefix reductions. Incremental
// reducers and bounded metadata are excluded.
var recordingReadOperations = setOf(
	"QueryHistoricalRecording", "LoadReplayInput", "LoadResumeInput", "LoadReplayRecording",
	"LoadReplayRecordingForResume", "LoadReplayRecordingScope", "LoadReplayArtifact", "LoadReplay",
	"CanonicalEvents", "ReconstructWorldState", "ReconstructFactoryWorldState",
	"ReconstructCanonicalFactoryWorldState", "ReconstructRecordingScope", "CreateReplayPlan",
	"CreateReplayPlanScope", "ObserveReplay", "ObserveReplayScope", "ValidateReconnectReplay",
	"ValidateReconnectReplayFrom", "QuerySimpleDashboard", "QuerySimpleDashboardScope",
	"QueryWorkstationRequests", "QueryWorkstationRequestsScope", "BuildArtifact", "BuildPortableArtifact",
	"BuildPortableArtifactScope", "ReadArtifact", "ReadPortableArtifact", "ReadPortableArtifactScope",
	"DecodeArtifact", "DecodePortableArtifact", "ExportArtifact", "ExportPortableArtifact",
	"ExportPortableArtifactScope", "LoadWorkerRecording", "ReduceWorkerRecording", "ReplayWorkerRecording",
	"BuildWorkerPortableRecording", "ExportWorkerPortableRecording", "EncodeWorkerPortableRecording",
	"DecodeWorkerPortableRecording", "DecodeWorkerPortableRecordingWithDiagnostics", "ReplayWorkerPortableRecording",
	"ResolveWorkerWorkAttribution", "ReadWorkerFactoryHistory", "ReadWorkerCapturedActivity",
	"ReadWorkerCapturedArtifact", "LookupWorkerSessionCapture", "ListWorkerSessionCaptures",
	"ReadWorkerContinuationSource", "ReadWorkerRestartRecipe", "WorkerRecordingProjection",
)

// Keys name an exact package and declaration, never a service-wide exemption.
var recordingReadOwners = setOf(
	// Public recording inspection, replay, export and Worker log delivery.
	"pkg/transports/http/recordings#Adapter.invokeBuildPortableArtifact",
	"pkg/transports/http/recordings#Adapter.invokeQueryHistoricalRecording",
	"pkg/transports/http/recordings#Adapter.invokeReconstructWorldState",
	"pkg/services/factory_sessions/transports/mcp#historicalRecording",
	"pkg/services/factory_sessions/transports/mcp#listFactorySessionArtifacts",
	"pkg/services/worker_sessions/internal/service#LogReader.ReadLogs",
	"pkg/services/worker_sessions/internal/service#LogReader.ReadLogsArtifact",
	"pkg/services/worker_sessions/internal/service#LogReader.capturedTranscript",
	"pkg/services/worker_sessions/internal/service#LogReader.transcriptByProvider",
	// Explicit replay/resume and startup restore activation.
	// Operator-approved startup catalog preparation, never a request handler.
	"pkg/services/recordings/internal/worker_work_attribution#Service.PrepareWorkerWorkAttribution",
	"pkg/services/factory_sessions/internal/service#Root.openForRequest",
	"pkg/services/factory_sessions/internal/service#Root.InspectHistoricalApplication",
	"pkg/services/factory_sessions/internal/service#restoreCurrentBoardHistory",
	"pkg/services/factory_sessions/internal/service#RuntimeInputLoading.loadRuntimeReplay",
	"pkg/services/factory_sessions/internal/service#RuntimeSnapshotSelection.loadReplayInputForActivation",
	"pkg/services/factory_runtime/internal#reconstructRestoredWorldStateEvents",
	"pkg/services/factory_runtime/internal/services/orchestration/runtime#reconcileRestoredDispatches",
	"pkg/services/factory_runtime/internal/services/orchestration/runtime#recordRestoredWorkRequests",
	"pkg/services/factory_runtime/internal/services/orchestration/runtime#factoryImpl.GetFactoryEvents",
	// Recording envelopes and replay codecs. Attribution is deliberately absent.
	"pkg/services/recordings/internal/contracts#collectPortableRecordingDecodePaths",
	"pkg/services/recordings/internal/canonical#ReconstructWorldState",
	"pkg/services/recordings/internal/services/historical_query/internal/service#Service.reconstructHistoricalWorldState",
	"pkg/services/recordings/internal/services/replay/internal/service#Service.ObserveReplay",
	"pkg/services/recordings/internal/services/artifacts_export/internal/service#Service.ExportPortableArtifact",
	"pkg/services/recordings/internal/services/artifacts_export/internal/service#Service.ReadPortableArtifact",
	"pkg/services/recordings/internal/services/worker_capture#WorkerRecordingCodec.BuildWorkerPortableRecording",
	"pkg/services/recordings/internal/services/worker_capture#WorkerRecordingCodec.ExportWorkerPortableRecording",
	"pkg/services/recordings/internal/services/worker_capture#WorkerRecordingCodec.ReplayWorkerPortableRecording",
	"pkg/services/recordings/internal/services/worker_capture#WorkerRecordingCodec.DecodeWorkerPortableRecording",
	"pkg/services/recordings/internal/services/worker_capture#WorkerRecordingCodec.replayWorkerRecordingSession",
	"pkg/services/recordings/internal/services/worker_capture#reducePortableHistory",
	"pkg/services/recordings/internal/services/worker_capture/internal/service#FileWriter.loadLegacy",
	"pkg/services/recordings/internal/services/worker_capture/internal/service#FileWriter.ReadWorkerCapturedActivity",
	"pkg/services/recordings/internal/services/worker_capture/internal/service#FileWriter.ReadWorkerCapturedArtifact",
	// Service-root forwarding of explicitly selected recording operations.
	"pkg/services/recordings/internal#combinedService.BuildPortableArtifact",
	"pkg/services/recordings/internal#combinedService.DecodePortableArtifact",
	"pkg/services/recordings/internal#combinedService.ExportPortableArtifact",
	"pkg/services/recordings/internal#combinedService.ReadPortableArtifact",
	"pkg/services/recordings/internal#combinedService.BuildPortableArtifactScope",
	"pkg/services/recordings/internal#combinedService.ExportPortableArtifactScope",
	"pkg/services/recordings/internal#combinedService.ReadPortableArtifactScope",
	"pkg/services/recordings/internal#combinedService.LoadReplayRecording",
	"pkg/services/recordings/internal#combinedService.LoadReplayRecordingForResume",
	"pkg/services/recordings/internal#combinedService.CreateReplayPlan",
	"pkg/services/recordings/internal#combinedService.CreateReplayPlanScope",
	"pkg/services/recordings/internal#combinedService.ObserveReplay",
	"pkg/services/recordings/internal#combinedService.ObserveReplayScope",
	"pkg/services/recordings/internal#combinedService.QueryHistoricalRecording",
	"pkg/services/recordings/internal#combinedService.LoadResumeInput",
	"pkg/services/recordings/internal#combinedService.LoadReplayInput",
	"pkg/services/recordings/internal#combinedService.LoadReplay",
	"pkg/services/recordings/internal#combinedService.BuildArtifact",
	"pkg/services/recordings/internal#combinedService.DecodeArtifact",
	"pkg/services/recordings/internal#combinedService.ExportArtifact",
	"pkg/services/recordings/internal#combinedService.ReadArtifact",
	"pkg/services/recordings/internal/services/worker_capture/internal/service#Service.LoadWorkerRecording",
)

func runRecordingReads(pass *analysis.Pass) (any, error) {
	unit, ok := unitKey(pass)
	if !ok {
		return nil, nil
	}
	var found []violation
	for _, file := range pass.Files {
		path := serviceSource(pass, unit, file)
		if ast.IsGenerated(file) || strings.HasSuffix(path, "_test.go") {
			continue
		}
		for _, decl := range file.Decls {
			name := timingDeclaration(decl)
			if recordingReadOwners[unit+"#"+name] {
				continue
			}
			ast.Inspect(decl, func(node ast.Node) bool {
				id, ok := node.(*ast.Ident)
				if !ok {
					return true
				}
				fn, ok := pass.TypesInfo.Uses[id].(*types.Func)
				if ok && recordingReadOperation(fn) {
					found = append(found, violation{rule: recordingReadRule, importer: unit,
						importee: path + "#" + name + "#" + fn.Name(), pos: id.Pos(),
						hint: "materialize ordinary facts in their owning service; reserve history for explicit recording requests"})
				}
				return true
			})
		}
	}
	reportWithBaseline(pass, unit, setOf(recordingReadRule), countedTestPolicy(found), false, recordingReadBaseline(pass, unit))
	return nil, nil
}

// Debt for an inactive platform/tag source belongs to the compilation that
// selects it. Removed sources still fail stale checks in every live unit.
func recordingReadBaseline(pass *analysis.Pass, unit string) map[string]struct{} {
	listed := baseline()
	ignored := map[string]bool{}
	for _, file := range pass.IgnoredFiles {
		ignored[sourceName(unit, file)] = true
	}
	for key := range listed {
		parts := strings.SplitN(key, "|", 3)
		if len(parts) == 3 && parts[0] == recordingReadRule && parts[1] == unit {
			path, _, _ := strings.Cut(parts[2], "#")
			if ignored[path] {
				delete(listed, key)
			}
		}
	}
	return listed
}

func recordingReadOperation(fn *types.Func) bool {
	if !recordingReadOperations[fn.Name()] {
		return false
	}
	if fn.Pkg() != nil && recordingTypeOwner(fn.Pkg().Path()) {
		return true
	}
	// Local forwarding interfaces preserve canonical parameter/result types;
	// unrelated methods with the same spelling are not recording operations.
	sig := fn.Type().(*types.Signature)
	return recordingSignatureType(sig.Params()) || recordingSignatureType(sig.Results())
}

func recordingTypeOwner(path string) bool {
	return strings.HasPrefix(path, modulePrefix) && under(strings.TrimPrefix(path, modulePrefix), "pkg/services/recordings")
}

func recordingSignatureType(t types.Type) bool {
	switch t := t.(type) {
	case *types.Alias:
		return recordingSignatureType(types.Unalias(t))
	case *types.Named:
		return t.Obj().Pkg() != nil && recordingTypeOwner(t.Obj().Pkg().Path())
	case *types.Pointer:
		return recordingSignatureType(t.Elem())
	case *types.Slice:
		return recordingSignatureType(t.Elem())
	case *types.Array:
		return recordingSignatureType(t.Elem())
	case *types.Map:
		return recordingSignatureType(t.Key()) || recordingSignatureType(t.Elem())
	case *types.Tuple:
		for i := 0; i < t.Len(); i++ {
			if recordingSignatureType(t.At(i).Type()) {
				return true
			}
		}
	}
	return false
}
