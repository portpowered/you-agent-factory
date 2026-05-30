import { useEffect, useId, useState } from "react";

import { DisclosureButton } from "../../../../components/ui/disclosure-button";
import {
  Collapsible,
  CollapsibleContent,
} from "../../../../components/ui/collapsible";
import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SUPPORTING_LABEL_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../../../components/ui/dashboard-typography";
import { cn } from "../../../../lib/cn";
import type { getWorkstationDetailMessages } from "../messages/workstation-detail";
import {
  CURRENT_SELECTION_ALERT_PANEL_CLASS,
  CURRENT_SELECTION_CODE_SUBTLE_CLASS,
  CURRENT_SELECTION_FIELD_PANEL_CLASS,
  CURRENT_SELECTION_NOTICE_SUBTLE_CLASS,
  HISTORY_TOGGLE_CLASS,
} from "../../base/components/detail-card-shared";
import type {
  EditableWorkstationPromptHelpState,
  WorkstationDetailCardProps,
} from "../lib/detail-card-types";
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
          ariaDescribedBy={describedBy || undefined}
          ariaInvalid={Boolean(state.validationErrors.prompt)}
          autocompleteState={state.promptHelpState}
          className={cn(
            "bg-transparent",
            state.promptDiagnostics.length > 0
              ? "border-af-danger-border focus-visible:border-af-danger focus-visible:ring-af-focus-ring"
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
      <p className={CURRENT_SELECTION_NOTICE_SUBTLE_CLASS}>
        {messages.editableConfigurationPromptHelpLoading}
      </p>
    );
  }

  if (state.promptHelpState.status === "error") {
    return (
      <p
        className={cn(
          "m-0 text-af-danger-text",
          DASHBOARD_SUPPORTING_TEXT_CLASS,
        )}
        role="alert"
      >
        {messages.editableConfigurationPromptHelpErrorPrefix}{" "}
        {state.promptHelpState.errorMessage}
      </p>
    );
  }

  if (state.promptHelpState.status === "empty") {
    return (
      <p className={CURRENT_SELECTION_NOTICE_SUBTLE_CLASS}>
        {state.promptHelpState.message}
      </p>
    );
  }

  return (
    <EditableConfigurationReadyPromptHelp
      messages={messages}
      promptHelpState={state.promptHelpState}
      workstationSelectionKey={state.initialValues.workstationName}
    />
  );
}

function EditableConfigurationReadyPromptHelp({
  messages,
  promptHelpState,
  workstationSelectionKey,
}: {
  messages: ReturnType<typeof getWorkstationDetailMessages>;
  promptHelpState: Extract<EditableWorkstationPromptHelpState, { status: "ready" }>;
  workstationSelectionKey: string;
}) {
  const [expanded, setExpanded] = useState(false);
  const sectionId = useId();
  const contentId = `${sectionId}-prompt-help-content`;

  useEffect(() => {
    setExpanded(false);
  }, [workstationSelectionKey]);

  return (
    <div className={CURRENT_SELECTION_FIELD_PANEL_CLASS}>
      <div className="flex flex-wrap items-start justify-between gap-2">
        <p className={cn("m-0 min-w-0 flex-1", CURRENT_SELECTION_NOTICE_SUBTLE_CLASS)}>
          {messages.editableConfigurationPromptAutocompleteSummary(
            promptHelpState.contract.availableVariables.length,
            promptHelpState.contract.inputCount,
          )}
        </p>
        <DisclosureButton
          aria-label={
            expanded
              ? messages.editableConfigurationPromptHelpCollapseActionLabel
              : messages.editableConfigurationPromptHelpExpandActionLabel
          }
          className={HISTORY_TOGGLE_CLASS}
          controlsID={contentId}
          expanded={expanded}
          onClick={() => setExpanded((current) => !current)}
          type="button"
        >
          {expanded ? messages.collapseAction : messages.expandAction}
        </DisclosureButton>
      </div>
      <Collapsible onOpenChange={setExpanded} open={expanded}>
        <CollapsibleContent className="grid gap-2 pt-2" id={contentId}>
          <p
            className={cn(
              "m-0 text-af-text-subtle",
              DASHBOARD_SUPPORTING_TEXT_CLASS,
            )}
          >
            {messages.editableConfigurationPromptAutocompleteDetail}
          </p>
          {promptHelpState.contract.availableVariables.length > 0 ? (
            <PromptContractList
              heading={
                messages.editableConfigurationPromptAvailableVariablesHeading
              }
              items={promptHelpState.contract.availableVariables.map(
                (variable) => ({
                  detail: variable.description,
                  example: variable.example,
                  key: `${variable.category}:${variable.path}:${variable.example}`,
                  label: variable.path,
                }),
              )}
            />
          ) : null}
          {promptHelpState.contract.unavailableAccessPatterns.length > 0 ? (
            <PromptContractList
              heading={
                messages.editableConfigurationPromptUnavailableAccessHeading
              }
              items={promptHelpState.contract.unavailableAccessPatterns.map(
                (pattern) => ({
                  detail: pattern.reason,
                  example: pattern.example,
                  key: `${pattern.path}:${pattern.example}:${pattern.reason}`,
                  label: pattern.path,
                }),
              )}
            />
          ) : null}
        </CollapsibleContent>
      </Collapsible>
    </div>
  );
}

function PromptContractList({
  heading,
  items,
}: {
  heading: string;
  items: Array<{
    detail: string;
    example: string;
    key: string;
    label: string;
  }>;
}) {
  return (
    <div className="grid gap-1">
      <h5 className={cn("m-0", DASHBOARD_SUPPORTING_LABEL_CLASS)}>{heading}</h5>
      <ul className="m-0 grid list-none gap-1 p-0">
        {items.map((item) => (
          <li
            className="grid min-w-0 gap-1 rounded-lg border border-af-border bg-af-surface-raised p-2"
            key={item.key}
          >
            <code
              className={cn(
                CURRENT_SELECTION_CODE_SUBTLE_CLASS,
                "[overflow-wrap:anywhere]",
              )}
            >
              {item.label}
            </code>
            <p
              className={cn(
                "m-0 text-af-text-subtle [overflow-wrap:anywhere]",
                DASHBOARD_SUPPORTING_TEXT_CLASS,
              )}
            >
              {item.detail}
            </p>
            <code
              className={cn(
                CURRENT_SELECTION_CODE_SUBTLE_CLASS,
                "[overflow-wrap:anywhere]",
              )}
            >
              {item.example}
            </code>
          </li>
        ))}
      </ul>
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
      <p className={CURRENT_SELECTION_NOTICE_SUBTLE_CLASS}>
        {messages.editableConfigurationPromptValidationLoading}
      </p>
    );
  }

  if (state.promptValidationState.status === "error") {
    return (
      <p
        className={cn(
          "m-0 text-af-danger-text",
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
      className={CURRENT_SELECTION_ALERT_PANEL_CLASS}
      id={diagnosticsId}
      role="alert"
    >
      <p className={cn("m-0 text-af-danger-text", DASHBOARD_BODY_TEXT_CLASS)}>
        {messages.editableConfigurationPromptDiagnosticsSummary}
      </p>
      <p
        className={cn(
          "m-0 text-af-text-subtle",
          DASHBOARD_SUPPORTING_TEXT_CLASS,
        )}
      >
        {messages.editableConfigurationPromptValidationDetail}
      </p>
      <div className="grid gap-2">
        <h5 className={cn("m-0", DASHBOARD_SUPPORTING_LABEL_CLASS)}>
          {messages.editableConfigurationPromptDiagnosticsHeading}
        </h5>
        <ul className="m-0 grid list-none gap-2 p-0">
          {state.promptDiagnostics.map((diagnostic) => (
            <li
              className="grid gap-1 rounded-lg border border-af-danger-border bg-af-surface-raised p-2"
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
                className={cn(
                  "m-0 text-af-danger-text",
                  DASHBOARD_BODY_TEXT_CLASS,
                )}
              >
                {diagnosticLabel(diagnostic.kind, messages)}:{" "}
                {diagnostic.message}
              </p>
              {diagnostic.path ? (
                <code
                  className={cn(
                    CURRENT_SELECTION_CODE_SUBTLE_CLASS,
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

function diagnosticLabel(
  kind: string,
  messages: ReturnType<typeof getWorkstationDetailMessages>,
) {
  return kind === "SYNTAX_ERROR"
    ? messages.editableConfigurationPromptSyntaxDiagnosticLabel
    : messages.editableConfigurationPromptVariableDiagnosticLabel;
}
