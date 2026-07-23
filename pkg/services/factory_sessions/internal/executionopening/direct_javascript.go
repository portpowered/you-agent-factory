package executionopening

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
)

type directJavaScriptRunOperation struct {
	build             roles.ExecutionServiceBuilder
	runSync           roles.DirectJavaScriptSyncRunner
	generateSessionID factorysessions.SessionIDGenerator
}

// NewDirectJavaScriptRunOperation constructs the Factory Sessions-owned raw
// JavaScript invocation boundary.
func NewDirectJavaScriptRunOperation(
	build roles.ExecutionServiceBuilder,
	runSync roles.DirectJavaScriptSyncRunner,
	generateSessionID factorysessions.SessionIDGenerator,
) (roles.DirectJavaScriptRunOperation, error) {
	if build == nil {
		return nil, errors.New("session execution builder is required")
	}
	if runSync == nil {
		return nil, errors.New("direct JavaScript sync runner is required")
	}
	if generateSessionID == nil {
		return nil, errors.New("Factory Session ID generator is required")
	}
	return &directJavaScriptRunOperation{build: build, runSync: runSync, generateSessionID: generateSessionID}, nil
}

func (*directJavaScriptRunOperation) Supports(sourcePath string) bool {
	switch strings.ToLower(filepath.Ext(strings.TrimSpace(sourcePath))) {
	case ".js", ".mjs", ".cjs":
		return true
	default:
		return false
	}
}

func (o *directJavaScriptRunOperation) Run(
	ctx context.Context,
	request factorysessions.DirectJavaScriptRunRequest,
) (resultErr error) {
	if o == nil || o.build == nil || o.runSync == nil {
		return errors.New("direct JavaScript run operation is unavailable")
	}
	sourcePath, err := filepath.Abs(strings.TrimSpace(request.SourcePath))
	if err != nil {
		return fmt.Errorf("resolve workflow source: %w", err)
	}
	if !o.Supports(sourcePath) {
		return fmt.Errorf("workflow source %q is not a supported JavaScript file", request.SourcePath)
	}
	childMode := factorysessions.ChildExecutorModeLive
	if request.MockWorkersEnabled {
		childMode = factorysessions.ChildExecutorModeFake
	}
	execution, err := o.build(
		ctx,
		string(factorysessions.ExecutionProviderJavaScriptRuntime),
		filepath.Dir(sourcePath),
		"",
		childMode,
	)
	if err != nil {
		return fmt.Errorf("open direct JavaScript execution: %w", err)
	}
	defer func() { resultErr = errors.Join(resultErr, execution.Close()) }()

	requestID := "run-" + strings.TrimSpace(o.generateSessionID())
	if requestID == "run-" {
		return errors.New("Factory Session ID generator returned an empty identity")
	}
	return o.runSync(ctx, execution, factorysessions.StartRequest{
		RequestID: requestID,
		Source: factorysessions.Source{
			Kind:         factoryruntime.WorkflowSourceKindWorkflowFile,
			WorkflowFile: sourcePath,
		},
		Runtime: &factorysessions.RuntimeOptions{ChildExecutorMode: childMode},
	}, request.JSONOutput, request.Output)
}

var _ roles.DirectJavaScriptRunOperation = (*directJavaScriptRunOperation)(nil)
