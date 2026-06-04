import {
  DASHBOARD_BODY_CODE_CLASS,
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SUPPORTING_LABEL_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../components/ui/dashboard-typography";
import { cn } from "../../lib/cn";
import { formatSyntaxDiagnosticMessage } from "./prompt-editor-diagnostic-message";
import type { PromptEditorDiagnostic } from "./prompt-editor-types";

const PROMPT_EDITOR_CODE_SUBTLE_CLASS = cn(
  "text-xs text-on-surface-variant",
  DASHBOARD_BODY_CODE_CLASS,
);
const PROMPT_EDITOR_DIAGNOSTICS_PANEL_CLASS =
  "grid min-h-24 gap-2 rounded-xl border p-3";

export type PromptEditorValidationFeedbackState =
  | { status: "loading" }
  | { errorMessage: string; status: "error" }
  | { status: "idle" }
  | { status: "ready" };

export interface PromptEditorDiagnosticsPanelLabels {
  diagnosticsHeading: string;
  diagnosticsSummary: string;
  validationErrorPrefix: string;
  validationLoading: string;
  variableDiagnosticLabel: string;
}

export interface PromptEditorDiagnosticsPanelProps {
  diagnostics: PromptEditorDiagnostic[];
  id?: string;
  labels: PromptEditorDiagnosticsPanelLabels;
  validationState: PromptEditorValidationFeedbackState;
}

export function PromptEditorDiagnosticsPanel({
  diagnostics,
  id,
  labels,
  validationState,
}: PromptEditorDiagnosticsPanelProps) {
  if (validationState.status === "loading") {
    return (
      <div
        aria-hidden="true"
        className={cn(
          PROMPT_EDITOR_DIAGNOSTICS_PANEL_CLASS,
          "border-transparent bg-transparent",
        )}
        id={id}
      />
    );
  }

  if (validationState.status === "error") {
    return (
      <div
        className={cn(
          PROMPT_EDITOR_DIAGNOSTICS_PANEL_CLASS,
          "border-af-danger-border bg-af-danger-surface",
        )}
        id={id}
        role="alert"
      >
        <p
          className={cn(
            "m-0 text-af-danger-text",
            DASHBOARD_SUPPORTING_TEXT_CLASS,
          )}
        >
          {labels.validationErrorPrefix} {validationState.errorMessage}
        </p>
      </div>
    );
  }

  if (diagnostics.length === 0) {
    return (
      <div
        aria-hidden="true"
        className={cn(
          PROMPT_EDITOR_DIAGNOSTICS_PANEL_CLASS,
          "border-transparent bg-transparent",
        )}
        id={id}
      />
    );
  }

  return (
    <div
      className={cn(
        PROMPT_EDITOR_DIAGNOSTICS_PANEL_CLASS,
        "border-af-danger-border bg-af-danger-surface",
      )}
      id={id}
      role="alert"
    >
      <p className={cn("m-0 text-af-danger-text", DASHBOARD_BODY_TEXT_CLASS)}>
        {labels.diagnosticsSummary}
      </p>
      <div className="grid gap-2">
        <h5 className={cn("m-0", DASHBOARD_SUPPORTING_LABEL_CLASS)}>
          {labels.diagnosticsHeading}
        </h5>
        <ul className="m-0 grid list-none gap-2 p-0">
          {diagnostics.map((diagnostic) => (
            <li
              className="grid gap-1 rounded-lg border border-af-danger-border bg-surface-container-high p-2"
              key={diagnosticListItemKey(diagnostic)}
            >
              <p
                className={cn(
                  "m-0 text-af-danger-text",
                  DASHBOARD_BODY_TEXT_CLASS,
                )}
              >
                {formatDiagnosticListMessage(diagnostic, labels)}
              </p>
              {diagnostic.path ? (
                <code
                  className={cn(
                    PROMPT_EDITOR_CODE_SUBTLE_CLASS,
                    "[overflow-wrap:anywhere]",
                  )}
                >
                  {diagnostic.path}
                </code>
              ) : null}
              {diagnostic.sourceText ? (
                <pre className="m-0 whitespace-pre-wrap rounded-lg border border-outline bg-surface-container-low p-2 text-xs text-on-surface-variant [overflow-wrap:anywhere]">
                  {diagnostic.sourceText}
                </pre>
              ) : null}
            </li>
          ))}
        </ul>
      </div>
    </div>
  );
}

function diagnosticListItemKey(diagnostic: PromptEditorDiagnostic) {
  return [
    diagnostic.kind,
    diagnostic.path ?? "",
    diagnostic.sourceText ?? "",
    diagnostic.startOffset ?? "",
    diagnostic.endOffset ?? "",
    diagnostic.message,
  ].join(":");
}

function formatDiagnosticListMessage(
  diagnostic: PromptEditorDiagnostic,
  labels: PromptEditorDiagnosticsPanelLabels,
): string {
  if (diagnostic.kind === "SYNTAX_ERROR") {
    return formatSyntaxDiagnosticMessage(diagnostic.message);
  }

  return `${labels.variableDiagnosticLabel}: ${diagnostic.message}`;
}
