import {
  DASHBOARD_BODY_CODE_CLASS,
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SUPPORTING_LABEL_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../components/ui/dashboard-typography";
import { cn } from "../../lib/cn";
import type { PromptEditorDiagnostic } from "./prompt-editor-types";

const PROMPT_EDITOR_NOTICE_SUBTLE_CLASS = cn(
  "m-0 text-af-text-muted",
  DASHBOARD_SUPPORTING_TEXT_CLASS,
);

const PROMPT_EDITOR_ALERT_PANEL_CLASS =
  "grid gap-2 rounded-xl border border-af-danger-border bg-af-danger-surface p-3";

const PROMPT_EDITOR_CODE_SUBTLE_CLASS = cn(
  "text-xs text-af-text-muted",
  DASHBOARD_BODY_CODE_CLASS,
);

export type PromptEditorValidationFeedbackState =
  | { status: "loading" }
  | { errorMessage: string; status: "error" }
  | { status: "idle" }
  | { status: "ready" };

export interface PromptEditorDiagnosticsPanelLabels {
  diagnosticsHeading: string;
  diagnosticsSummary: string;
  syntaxDiagnosticLabel: string;
  validationDetail: string;
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
      <p className={PROMPT_EDITOR_NOTICE_SUBTLE_CLASS}>
        {labels.validationLoading}
      </p>
    );
  }

  if (validationState.status === "error") {
    return (
      <p
        className={cn(
          "m-0 text-af-danger-text",
          DASHBOARD_SUPPORTING_TEXT_CLASS,
        )}
        role="alert"
      >
        {labels.validationErrorPrefix} {validationState.errorMessage}
      </p>
    );
  }

  if (diagnostics.length === 0) {
    return null;
  }

  return (
    <div className={PROMPT_EDITOR_ALERT_PANEL_CLASS} id={id} role="alert">
      <p className={cn("m-0 text-af-danger-text", DASHBOARD_BODY_TEXT_CLASS)}>
        {labels.diagnosticsSummary}
      </p>
      <p
        className={cn(
          "m-0 text-af-text-subtle",
          DASHBOARD_SUPPORTING_TEXT_CLASS,
        )}
      >
        {labels.validationDetail}
      </p>
      <div className="grid gap-2">
        <h5 className={cn("m-0", DASHBOARD_SUPPORTING_LABEL_CLASS)}>
          {labels.diagnosticsHeading}
        </h5>
        <ul className="m-0 grid list-none gap-2 p-0">
          {diagnostics.map((diagnostic) => (
            <li
              className="grid gap-1 rounded-lg border border-af-danger-border bg-af-surface-raised p-2"
              key={diagnosticListItemKey(diagnostic)}
            >
              <p
                className={cn(
                  "m-0 text-af-danger-text",
                  DASHBOARD_BODY_TEXT_CLASS,
                )}
              >
                {diagnosticKindLabel(diagnostic.kind, labels)}:{" "}
                {diagnostic.message}
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
                <pre className="m-0 whitespace-pre-wrap rounded-lg border border-af-border bg-af-surface-subtle p-2 text-xs text-af-text-muted [overflow-wrap:anywhere]">
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

function diagnosticKindLabel(
  kind: string,
  labels: PromptEditorDiagnosticsPanelLabels,
) {
  return kind === "SYNTAX_ERROR"
    ? labels.syntaxDiagnosticLabel
    : labels.variableDiagnosticLabel;
}
