import type { FactoryPreviewDiagnostic } from "../../../api/factory-preview";
import { useFactoryPreview } from "../hooks/useWorkflowPreview";
import { workflowPreviewPanelMessages } from "../messages/panel";

export interface WorkflowPreviewPanelProps {
  projectRoot: string;
  sourceKind: "WORKFLOW_NAME" | "INLINE_WORKFLOW" | "WORKFLOW_FILE";
  sourceValue?: string;
  inlineSource?: string;
  artifactRoot?: string;
}

function formatDiagnostic(diagnostic: FactoryPreviewDiagnostic): string {
  const location =
    diagnostic.line != null && diagnostic.line > 0
      ? diagnostic.column != null && diagnostic.column > 0
        ? ` (line ${diagnostic.line}, column ${diagnostic.column})`
        : ` (line ${diagnostic.line})`
      : "";
  const path =
    diagnostic.path != null && diagnostic.path.trim().length > 0
      ? `${diagnostic.path}: `
      : "";
  return `${path}${diagnostic.code}${location}: ${diagnostic.message}`;
}

export function WorkflowPreviewPanel({
  projectRoot,
  sourceKind,
  sourceValue,
  inlineSource,
  artifactRoot,
}: WorkflowPreviewPanelProps) {
  const hasRequestInput =
    (sourceValue != null && sourceValue.trim().length > 0) ||
    (inlineSource != null && inlineSource.trim().length > 0);

  const previewQuery = useFactoryPreview(
    hasRequestInput
      ? {
          sourceKind,
          projectRoot,
          sourceValue,
          inlineSource,
          artifactRoot,
        }
      : null,
    hasRequestInput,
  );

  if (!hasRequestInput) {
    return (
      <section aria-live="polite" data-testid="workflow-preview-empty">
        <p>{workflowPreviewPanelMessages.empty}</p>
      </section>
    );
  }

  if (previewQuery.isLoading) {
    return (
      <section aria-busy="true" data-testid="workflow-preview-loading">
        <p>{workflowPreviewPanelMessages.loading}</p>
      </section>
    );
  }

  if (previewQuery.isError) {
    return (
      <section
        aria-live="assertive"
        data-testid="workflow-preview-error"
        role="alert"
      >
        <p>{previewQuery.error.message}</p>
      </section>
    );
  }

  const preview = previewQuery.data;
  if (!preview) {
    return null;
  }

  const resolutionDiagnostics = preview.sourceResolution.diagnostics ?? [];
  const deniedCapabilities = preview.policyPreview.deniedCapabilities ?? [];

  return (
    <section
      aria-live="polite"
      data-testid={
        preview.valid ? "workflow-preview-success" : "workflow-preview-error"
      }
    >
      <p>
        {preview.valid
          ? workflowPreviewPanelMessages.success
          : workflowPreviewPanelMessages.error}
      </p>
      {preview.sourceResolution.sourceRef ? (
        <p>
          {workflowPreviewPanelMessages.sourceRefLabel}:{" "}
          {preview.sourceResolution.sourceRef}
        </p>
      ) : null}
      {preview.sourceResolution.sourceHash ? (
        <p>
          {workflowPreviewPanelMessages.sourceHashLabel}:{" "}
          {preview.sourceResolution.sourceHash}
        </p>
      ) : null}
      <p>
        {workflowPreviewPanelMessages.policyHashLabel}:{" "}
        {preview.policyPreview.policyHash}
      </p>

      {resolutionDiagnostics.length > 0 ? (
        <div>
          <h3>{workflowPreviewPanelMessages.sourceResolution}</h3>
          <ul>
            {resolutionDiagnostics.map((diagnostic) => (
              <li key={`${diagnostic.code}:${diagnostic.message}`}>
                {formatDiagnostic(diagnostic)}
              </li>
            ))}
          </ul>
        </div>
      ) : null}

      {preview.sourceValidationIssues.length > 0 ? (
        <ul>
          {preview.sourceValidationIssues.map((issue) => (
            <li key={`${issue.code}:${issue.message}:${issue.path ?? ""}`}>
              {formatDiagnostic(issue)}
            </li>
          ))}
        </ul>
      ) : null}

      {deniedCapabilities.length > 0 ? (
        <div>
          <h3>{workflowPreviewPanelMessages.deniedCapabilities}</h3>
          <ul>
            {deniedCapabilities.map((diagnostic) => (
              <li key={`${diagnostic.code}:${diagnostic.message}`}>
                {formatDiagnostic(diagnostic)}
              </li>
            ))}
          </ul>
        </div>
      ) : null}

      <p>
        {workflowPreviewPanelMessages.resultConstraints}:{" "}
        {workflowPreviewPanelMessages.resultConstraintsSummary(
          preview.resultConstraints.artifactUriScheme,
          preview.resultConstraints.maxEmbeddedBytes,
        )}
      </p>
    </section>
  );
}
