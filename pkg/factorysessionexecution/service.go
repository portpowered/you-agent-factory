package factorysessionexecution

import (
	"context"
	"fmt"
	"strings"

	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
)

// Service is the shared durable factory-session execution contract consumed by
// API, CLI, MCP, and UI transports. Live-session open and invocation remain on
// the separate factorysessions compatibility surface. All methods are
// cancellation-aware; transports must not mutate runtime state directly.
type Service interface {
	StartAsync(ctx context.Context, req StartRequest) (AsyncStartResult, error)
	StartSync(ctx context.Context, req StartRequest) (SyncStartResult, error)
	GetSession(ctx context.Context, sessionID string) (SessionReadResult, error)
	Pause(ctx context.Context, sessionID string, req ControlRequest) (LifecycleControlResult, error)
	Resume(ctx context.Context, sessionID string, req ControlRequest) (LifecycleControlResult, error)
	Cancel(ctx context.Context, sessionID string, req ControlRequest) (LifecycleControlResult, error)
	Terminate(ctx context.Context, sessionID string, req ControlRequest) (LifecycleControlResult, error)
	Approve(ctx context.Context, sessionID string, req ApproveRequest) (LifecycleControlResult, error)
	RetryDispatch(ctx context.Context, sessionID string, req RetryDispatchRequest) (LifecycleControlResult, error)
	GetResult(ctx context.Context, sessionID string, req ResultRequest) (ResultReadResult, error)
	ListDispatches(ctx context.Context, sessionID string) (ListDispatchesResult, error)
	GetDispatch(ctx context.Context, sessionID, dispatchID string) (DispatchDetail, error)
	ListArtifacts(ctx context.Context, sessionID string) (ListArtifactsResult, error)
	GetArtifact(ctx context.Context, sessionID, artifactID string) (ArtifactDetail, error)
	ReadEvents(ctx context.Context, sessionID string, req EventReconnectRequest) (EventReadResult, error)
	ListSessions(ctx context.Context, req ListSessionsRequest) (ListSessionsResult, error)
}

// InspectionLinksForSession builds API-relative inspection links for one durable session.
func InspectionLinksForSession(sessionID string, includeEvents bool) InspectionLinks {
	base := fmt.Sprintf("/factory-sessions/%s", sessionID)
	links := InspectionLinks{
		Session:    base,
		Status:     base,
		Results:    base + "/results",
		Dispatches: base + "/dispatches",
		Artifacts:  base + "/artifacts",
	}
	if includeEvents {
		links.Events = base + "/events"
	}
	return links
}

// LifecycleControlLinksForSession builds post-control inspection links for one durable session.
func LifecycleControlLinksForSession(sessionID string, includeEvents bool) LifecycleControlLinks {
	inspection := InspectionLinksForSession(sessionID, includeEvents)
	return LifecycleControlLinks{
		Session:    inspection.Session,
		Status:     inspection.Status,
		Results:    inspection.Results,
		Dispatches: inspection.Dispatches,
		Artifacts:  inspection.Artifacts,
		Events:     inspection.Events,
	}
}
// StartSourceContext supplies filesystem roots for durable start source resolution.
type StartSourceContext struct {
	ProjectRoot string
}

// ResolveStartSource resolves one normalized start request through the JavaScript
// orchestrator source contract used by durable Factory Session start paths.
func ResolveStartSource(req StartRequest, ctx StartSourceContext) (ResolvedSource, error) {
	projectRoot := strings.TrimSpace(ctx.ProjectRoot)
	if projectRoot == "" {
		return ResolvedSource{}, NewValidationError("projectRoot", "projectRoot is required")
	}

	sourceCtx, err := workflowsource.DefaultContext(projectRoot)
	if err != nil {
		return ResolvedSource{}, NewValidationError("projectRoot", err.Error())
	}

	resolution := workflowsource.Resolve(startSourceRequest(req.Source), sourceCtx)
	if !resolution.Found {
		message := "workflow source could not be resolved"
		if len(resolution.Diagnostics) > 0 && strings.TrimSpace(resolution.Diagnostics[0].Message) != "" {
			message = resolution.Diagnostics[0].Message
		}
		return ResolvedSource{}, NewValidationError("source", message)
	}

	resolved := ResolvedSource{
		Kind:       resolution.ResolvedKind,
		SourceRef:  resolution.SourceRef,
		SourceHash: resolution.SourceHash,
		Dialect:    resolution.Dialect,
		Metadata: map[string]string{
			"project": sourceCtx.ProjectRoot,
		},
	}
	if stage := resolutionOrderForLookupStage(resolution.LookupStage); stage != "" {
		resolved.ResolutionOrder = []string{stage}
	}
	return resolved, nil
}

func startSourceRequest(source Source) workflowsource.Request {
	switch source.Kind {
	case workflowsource.KindFactoryID:
		return workflowsource.Request{
			Kind:  source.Kind,
			Value: source.FactoryID,
		}
	case workflowsource.KindFactoryInline:
		return workflowsource.Request{
			Kind:  source.Kind,
			Value: string(source.FactoryInline),
		}
	case workflowsource.KindWorkflowFile:
		return workflowsource.Request{
			Kind:  source.Kind,
			Value: source.WorkflowFile,
		}
	case workflowsource.KindWorkflowName:
		return workflowsource.Request{
			Kind:  source.Kind,
			Value: source.WorkflowName,
		}
	case workflowsource.KindInlineWorkflow:
		inline := source.InlineWorkflow
		if inline == nil {
			return workflowsource.Request{Kind: source.Kind}
		}
		return workflowsource.Request{
			Kind:         source.Kind,
			Value:        inline.InlineSource,
			InlineSource: inline.InlineSource,
		}
	default:
		return workflowsource.Request{Kind: source.Kind}
	}
}

func resolutionOrderForLookupStage(stage workflowsource.LookupStage) string {
	switch stage {
	case workflowsource.LookupStageProjectClaude, workflowsource.LookupStageExplicitSourceKind:
		return "PROJECT_CLAUDE_WORKFLOWS"
	case workflowsource.LookupStageGlobalUser:
		return "USER_YOU_AGENT_FACTORY_WORKFLOWS"
	case workflowsource.LookupStagePackageRelative:
		return "PACKAGE_RELATIVE_WORKFLOW_DIRECTORIES"
	case workflowsource.LookupStageNamedJavaScript:
		return "BUILTIN_GLOBAL_JAVASCRIPT_FACTORIES"
	case workflowsource.LookupStageExplicitFactory:
		return "EXPLICIT_FACTORY_LOOKUP"
	default:
		return ""
	}
}
