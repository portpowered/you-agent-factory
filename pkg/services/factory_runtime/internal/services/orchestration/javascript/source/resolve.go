package workflowsource

import "fmt"

// Resolve applies the shared workflow source lookup contract.
func Resolve(req Request, ctx Context) Resolution {
	resolution, handled := resolveSource(req, ctx)
	resolution.ArtifactRoot = resolveArtifactRoot(ctx.ProjectRoot, req.ArtifactRoot, ctx.resolveSymlinks)
	if resolution.ArtifactRoot.Diagnostic != nil {
		resolution.Found = false
		if resolution.Diagnostics == nil {
			resolution.Diagnostics = []Diagnostic{*resolution.ArtifactRoot.Diagnostic}
		} else {
			resolution.Diagnostics = append(resolution.Diagnostics, *resolution.ArtifactRoot.Diagnostic)
		}
	}
	if handled {
		return resolution
	}

	return Resolution{
		RequestKind:  req.Kind,
		RequestValue: req.Value,
		ResolvedKind: req.Kind,
		Diagnostics: []Diagnostic{{
			Code:    CodeUnsupportedKind,
			Message: fmt.Sprintf("unsupported workflow source kind %q", req.Kind),
		}},
		ArtifactRoot: resolution.ArtifactRoot,
		Found:        false,
	}
}

func resolveSource(req Request, ctx Context) (Resolution, bool) {
	switch req.Kind {
	case KindWorkflowName:
		return lookupWorkflowByName(ctx, req.Value, req.AllowFactoryLookup)
	case KindWorkflowFile:
		return resolveWorkflowFile(ctx, req.Value)
	case KindFactoryID:
		return resolveFactoryID(ctx, req.Value)
	case KindInlineWorkflow:
		return resolveInlineSource(req.Kind, req.Value, req.InlineSource)
	case KindFactoryInline:
		return resolveFactoryInline(req.Value)
	default:
		return Resolution{}, false
	}
}
