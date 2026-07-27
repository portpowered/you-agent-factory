import { SurfacePanel } from "@you-agent-factory/components/layout";
import { Code, Label } from "@you-agent-factory/components/primitives";
import { AlertPanel, AlertPanelText } from "../../components/ui/alert-panel";
import { CodePanel } from "../../components/ui/code-panel";
import { cn } from "../../lib/cn";
import { formatSyntaxDiagnosticMessage } from "./prompt-editor-diagnostic-message";
import type { PromptEditorDiagnostic } from "./prompt-editor-types";

const PROMPT_EDITOR_DIAGNOSTICS_RESERVED_PANEL_CLASS =
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
          PROMPT_EDITOR_DIAGNOSTICS_RESERVED_PANEL_CLASS,
          "border-transparent bg-transparent",
        )}
        id={id}
      />
    );
  }

  if (validationState.status === "error") {
    return (
      <AlertPanel className="min-h-24" id={id} role="alert" tone="danger">
        <AlertPanelText variant="supporting">
          {labels.validationErrorPrefix} {validationState.errorMessage}
        </AlertPanelText>
      </AlertPanel>
    );
  }

  if (diagnostics.length === 0) {
    return (
      <div
        aria-hidden="true"
        className={cn(
          PROMPT_EDITOR_DIAGNOSTICS_RESERVED_PANEL_CLASS,
          "border-transparent bg-transparent",
        )}
        id={id}
      />
    );
  }

  return (
    <AlertPanel className="min-h-24" id={id} role="alert" tone="danger">
      <AlertPanelText>{labels.diagnosticsSummary}</AlertPanelText>
      <div className="grid gap-2">
        <Label as="h5" className="m-0">
          {labels.diagnosticsHeading}
        </Label>
        <ul className="m-0 grid list-none gap-2 p-0">
          {diagnostics.map((diagnostic) => (
            <SurfacePanel
              asChild
              className="grid gap-1 border-af-danger-border"
              key={diagnosticListItemKey(diagnostic)}
              padding="compact"
              radius="lg"
            >
              <li>
                <AlertPanelText>
                  {formatDiagnosticListMessage(diagnostic, labels)}
                </AlertPanelText>
                {diagnostic.path ? (
                  <Code className="text-xs text-on-surface-variant [overflow-wrap:anywhere]">
                    {diagnostic.path}
                  </Code>
                ) : null}
                {diagnostic.sourceText ? (
                  <CodePanel
                    className="text-xs text-on-surface-variant"
                    surface="low"
                  >
                    {diagnostic.sourceText}
                  </CodePanel>
                ) : null}
              </li>
            </SurfacePanel>
          ))}
        </ul>
      </div>
    </AlertPanel>
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
