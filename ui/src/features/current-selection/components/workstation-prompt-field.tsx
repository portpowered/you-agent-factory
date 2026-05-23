import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SUPPORTING_LABEL_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../../components/ui/dashboard-typography";
import { cn } from "../../../lib/cn";
import type { WorkstationDetailCardProps } from "./detail-card-types";
import type { getWorkstationDetailMessages } from "../messages";
import { WorkstationPromptEditor } from "./workstation-prompt-editor";

export function EditableConfigurationPromptInput({
  messages,
  state,
}: {
  messages: ReturnType<typeof getWorkstationDetailMessages>;
  state: Extract<
    NonNullable<WorkstationDetailCardProps["editableConfigurationState"]>,
    { status: "ready" }
  >;
}) {
  const diagnosticsId = "editable-workstation-prompt-diagnostics";
  const errorId = "editable-workstation-prompt-error";
  const describedBy = [
    state.validationErrors.prompt ? errorId : null,
    state.promptDiagnostics.length > 0 ? diagnosticsId : null,
  ]
    .filter(Boolean)
    .join(" ");

  return (
    <div className="grid gap-2">
      <div>
        <WorkstationPromptEditor
          ariaLabel={messages.promptFieldLabel}
          aria-describedby={describedBy || undefined}
          ariaInvalid={Boolean(state.validationErrors.prompt)}
          autocompleteState={state.promptHelpState}
          className={cn(
            "bg-transparent",
            state.promptDiagnostics.length > 0
              ? "border-af-danger/45 focus-visible:border-af-danger focus-visible:ring-af-danger/20"
              : undefined,
            DASHBOARD_BODY_TEXT_CLASS,
          )}
          diagnostics={state.promptDiagnostics}
          hasDiagnostics={state.promptDiagnostics.length > 0}
          loadingMessage={messages.editableConfigurationPromptEditorLoading}
          onChange={state.onPromptChange}
          startupErrorMessage={messages.editableConfigurationPromptEditorError}
          value={state.draft.prompt}
        />
      </div>
      <EditableConfigurationPromptAutocompleteFeedback
        messages={messages}
        state={state}
      />
      <EditableConfigurationPromptValidationFeedback
        diagnosticsId={diagnosticsId}
        messages={messages}
        state={state}
      />
    </div>
  );
}

function EditableConfigurationPromptAutocompleteFeedback({
  messages,
  state,
}: {
  messages: ReturnType<typeof getWorkstationDetailMessages>;
  state: Extract<
    NonNullable<WorkstationDetailCardProps["editableConfigurationState"]>,
    { status: "ready" }
  >;
}) {
  if (state.promptHelpState.status === "loading") {
    return (
      <p className={cn("m-0 text-af-ink/70", DASHBOARD_SUPPORTING_TEXT_CLASS)}>
        {messages.editableConfigurationPromptHelpLoading}
      </p>
    );
  }

  if (state.promptHelpState.status === "error") {
    return (
      <p
        className={cn("m-0 text-af-danger-ink", DASHBOARD_SUPPORTING_TEXT_CLASS)}
        role="alert"
      >
        {messages.editableConfigurationPromptHelpErrorPrefix}{" "}
        {state.promptHelpState.errorMessage}
      </p>
    );
  }

  if (state.promptHelpState.status === "empty") {
    return (
      <p className={cn("m-0 text-af-ink/70", DASHBOARD_SUPPORTING_TEXT_CLASS)}>
        {state.promptHelpState.message}
      </p>
    );
  }

  return (
    <div className="grid gap-1 rounded-xl border border-af-overlay/10 bg-af-overlay/4 p-3">
      <p className={cn("m-0 text-af-ink/72", DASHBOARD_SUPPORTING_TEXT_CLASS)}>
        {messages.editableConfigurationPromptAutocompleteSummary(
          state.promptHelpState.contract.availableVariables.length,
          state.promptHelpState.contract.inputCount,
        )}
      </p>
      <p className={cn("m-0 text-af-ink/62", DASHBOARD_SUPPORTING_TEXT_CLASS)}>
        {messages.editableConfigurationPromptAutocompleteDetail}
      </p>
    </div>
  );
}

function EditableConfigurationPromptValidationFeedback({
  diagnosticsId,
  messages,
  state,
}: {
  diagnosticsId: string;
  messages: ReturnType<typeof getWorkstationDetailMessages>;
  state: Extract<
    NonNullable<WorkstationDetailCardProps["editableConfigurationState"]>,
    { status: "ready" }
  >;
}) {
  if (state.promptValidationState.status === "loading") {
    return (
      <p className={cn("m-0 text-af-ink/70", DASHBOARD_SUPPORTING_TEXT_CLASS)}>
        {messages.editableConfigurationPromptValidationLoading}
      </p>
    );
  }

  if (state.promptValidationState.status === "error") {
    return (
      <p
        className={cn(
          "m-0 text-af-danger-ink",
          DASHBOARD_SUPPORTING_TEXT_CLASS,
        )}
        role="alert"
      >
        {messages.editableConfigurationPromptValidationErrorPrefix}{" "}
        {state.promptValidationState.errorMessage}
      </p>
    );
  }

  if (state.promptDiagnostics.length === 0) {
    return null;
  }

  return (
    <div
      className="grid gap-2 rounded-xl border border-af-danger/20 bg-af-danger/6 p-3"
      id={diagnosticsId}
      role="alert"
    >
      <p className={cn("m-0 text-af-danger-ink", DASHBOARD_BODY_TEXT_CLASS)}>
        {messages.editableConfigurationPromptDiagnosticsSummary}
      </p>
      <p className={cn("m-0 text-af-ink/62", DASHBOARD_SUPPORTING_TEXT_CLASS)}>
        {messages.editableConfigurationPromptValidationDetail}
      </p>
      <div className="grid gap-2">
        <h5 className={cn("m-0", DASHBOARD_SUPPORTING_LABEL_CLASS)}>
          {messages.editableConfigurationPromptDiagnosticsHeading}
        </h5>
        <ul className="m-0 grid list-none gap-2 p-0">
          {state.promptDiagnostics.map((diagnostic) => (
            <li
              className="grid gap-1 rounded-lg border border-af-danger/18 bg-af-overlay/4 p-2"
              key={[
                diagnostic.kind,
                diagnostic.path ?? "",
                diagnostic.sourceText ?? "",
                diagnostic.startOffset ?? "",
                diagnostic.endOffset ?? "",
                diagnostic.message,
              ].join(":")}
            >
              <p
                className={cn("m-0 text-af-danger-ink", DASHBOARD_BODY_TEXT_CLASS)}
              >
                {diagnosticLabel(diagnostic.kind, messages)}: {diagnostic.message}
              </p>
              {diagnostic.path ? (
                <code className="text-xs text-af-ink/72">{diagnostic.path}</code>
              ) : null}
              {diagnostic.sourceText ? (
                <pre className="m-0 whitespace-pre-wrap rounded-lg border border-af-overlay/8 bg-af-overlay/6 p-2 text-xs text-af-ink/78 [overflow-wrap:anywhere]">
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

function diagnosticLabel(
  kind: string,
  messages: ReturnType<typeof getWorkstationDetailMessages>,
) {
  return kind === "SYNTAX_ERROR"
    ? messages.editableConfigurationPromptSyntaxDiagnosticLabel
    : messages.editableConfigurationPromptVariableDiagnosticLabel;
}
