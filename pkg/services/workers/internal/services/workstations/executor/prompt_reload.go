package executor

import (
	"fmt"
	"strings"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	workerprompting "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/prompting"
)

func (we *WorkstationExecutor) promptFileReader() factorydefinitions.FileReader {
	if we.FileSystem == nil {
		return nil
	}
	return we.FileSystem.ReadFile
}

func (we *WorkstationExecutor) prepareWorkstationDefinition(
	dispatch work.WorkDispatch,
	workstationName string,
	workstationDef *factorydefinitions.FactoryWorkstationConfig,
	invocationArgs *work.InvocationArguments,
	readFile factorydefinitions.FileReader,
	diagnostics *workerexecution.WorkDiagnostics,
	start time.Time,
) (*factorydefinitions.FactoryWorkstationConfig, *workerexecution.WorkResult) {
	snapshot, promptPath, err := we.workstationPromptSnapshot(workstationName, workstationDef)
	if err != nil {
		result := promptSourceFailureResult(
			dispatch,
			"workstation",
			workstationName,
			promptPath,
			err,
			diagnostics,
			we.Now().Sub(start),
		)
		return nil, &result
	}
	if we.Interpolation == nil {
		return snapshot, nil
	}
	interpolated, err := we.Interpolation.InterpolateWorkstationConfig(*snapshot, invocationArgs, readFile)
	if err != nil {
		return nil, promptPreparationFailureResult(dispatch, err, diagnostics, we.Now().Sub(start))
	}
	return &interpolated, nil
}

func (we *WorkstationExecutor) prepareWorkerDefinition(
	dispatch work.WorkDispatch,
	workerName string,
	workerDef *factorydefinitions.FactoryWorkerConfig,
	invocationArgs *work.InvocationArguments,
	readFile factorydefinitions.FileReader,
	diagnostics *workerexecution.WorkDiagnostics,
	start time.Time,
) (*factorydefinitions.FactoryWorkerConfig, *workerexecution.WorkResult) {
	snapshot, promptPath, err := we.workerPromptSnapshot(workerName, workerDef)
	if err != nil {
		result := promptSourceFailureResult(
			dispatch,
			"worker",
			workerName,
			promptPath,
			err,
			diagnostics,
			we.Now().Sub(start),
		)
		return nil, &result
	}
	if we.Interpolation == nil {
		return snapshot, nil
	}
	interpolated, err := we.Interpolation.InterpolateWorkerConfig(*snapshot, invocationArgs, readFile)
	if err != nil {
		return nil, promptPreparationFailureResult(dispatch, err, diagnostics, we.Now().Sub(start))
	}
	if strings.TrimSpace(interpolated.ModelProvider) == "" {
		interpolated.ModelProvider = interpolated.RuntimeDefaultModelProvider
	}
	if strings.TrimSpace(interpolated.Model) == "" {
		interpolated.Model = interpolated.RuntimeDefaultModel
	}
	if failed := we.resolveInvocationProvider(dispatch, &interpolated, diagnostics, start); failed != nil {
		return nil, failed
	}
	return &interpolated, nil
}

func promptPreparationFailureResult(
	dispatch work.WorkDispatch,
	err error,
	diagnostics *workerexecution.WorkDiagnostics,
	duration time.Duration,
) *workerexecution.WorkResult {
	return &workerexecution.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      workerexecution.OutcomeFailed,
		Error:        err.Error(),
		Diagnostics:  diagnostics,
		Metrics:      workerexecution.WorkMetrics{Duration: duration},
	}
}

func (we *WorkstationExecutor) workerPromptSnapshot(
	workerName string,
	workerDef *factorydefinitions.FactoryWorkerConfig,
) (*factorydefinitions.FactoryWorkerConfig, string, error) {
	snapshot := factorydefinitions.CloneWorkerConfig(*workerDef)
	source := we.workerPromptSource(workerName, workerDef)
	snapshot.PromptSourcePath = source.Path
	if err := we.refreshWorkerPrompt(&snapshot); err != nil {
		return nil, snapshot.PromptSourcePath, err
	}
	return &snapshot, snapshot.PromptSourcePath, nil
}

func (we *WorkstationExecutor) refreshWorkerPrompt(
	workerDef *factorydefinitions.FactoryWorkerConfig,
) error {
	if workerDef == nil || workerDef.PromptSourcePath == "" {
		return nil
	}
	body, err := workerprompting.ResolveAuthoredPromptSource(
		we.FileSystem,
		workerDef.PromptSourcePath,
		true,
	)
	if err != nil {
		return err
	}
	workerDef.Body = body
	return nil
}

func (we *WorkstationExecutor) workerPromptSource(
	workerName string,
	workerDef *factorydefinitions.FactoryWorkerConfig,
) factorydefinitions.PromptSource {
	if lookup, ok := we.RuntimeConfig.(factorydefinitions.RuntimePromptSourceLookup); ok {
		if source, found := lookup.WorkerPromptSource(workerName); found {
			return source
		}
	}
	if workerDef == nil {
		return factorydefinitions.PromptSource{}
	}
	return factorydefinitions.PromptSource{Path: workerDef.PromptSourcePath}
}

func (we *WorkstationExecutor) workstationPromptSnapshot(
	workstationName string,
	workstationDef *factorydefinitions.FactoryWorkstationConfig,
) (*factorydefinitions.FactoryWorkstationConfig, string, error) {
	snapshot := factorydefinitions.CloneWorkstationConfig(*workstationDef)
	source := we.workstationPromptSource(workstationName, workstationDef)
	snapshot.PromptSourcePath = source.Path
	snapshot.PromptSourceIsTemplate = source.IsTemplate
	if err := we.refreshWorkstationPrompt(&snapshot); err != nil {
		return nil, snapshot.PromptSourcePath, err
	}
	return &snapshot, snapshot.PromptSourcePath, nil
}

func (we *WorkstationExecutor) refreshWorkstationPrompt(
	workstationDef *factorydefinitions.FactoryWorkstationConfig,
) error {
	if workstationDef == nil || workstationDef.PromptSourcePath == "" {
		return nil
	}
	prompt, err := workerprompting.ResolveAuthoredPromptSource(
		we.FileSystem,
		workstationDef.PromptSourcePath,
		!workstationDef.PromptSourceIsTemplate,
	)
	if err != nil {
		return err
	}
	if workstationDef.PromptSourceIsTemplate {
		workstationDef.PromptTemplate = prompt
		return nil
	}
	workstationDef.Body = prompt
	workstationDef.PromptTemplate = prompt
	return nil
}

func (we *WorkstationExecutor) workstationPromptSource(
	workstationName string,
	workstationDef *factorydefinitions.FactoryWorkstationConfig,
) factorydefinitions.PromptSource {
	if lookup, ok := we.RuntimeConfig.(factorydefinitions.RuntimePromptSourceLookup); ok {
		if source, found := lookup.WorkstationPromptSource(workstationName); found {
			return source
		}
	}
	if workstationDef == nil {
		return factorydefinitions.PromptSource{}
	}
	return factorydefinitions.PromptSource{
		Path:       workstationDef.PromptSourcePath,
		IsTemplate: workstationDef.PromptSourceIsTemplate,
	}
}

func promptSourceFailureResult(
	dispatch work.WorkDispatch,
	role string,
	name string,
	path string,
	err error,
	diagnostics *workerexecution.WorkDiagnostics,
	duration time.Duration,
) workerexecution.WorkResult {
	return workerexecution.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      workerexecution.OutcomeFailed,
		Error: fmt.Sprintf(
			"%s %q prompt source %s: %v",
			role,
			name,
			path,
			err,
		),
		Diagnostics: diagnostics,
		Metrics: workerexecution.WorkMetrics{
			Duration: duration,
		},
	}
}
