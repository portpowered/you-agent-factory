import { useEffect, useId, useRef, useState } from "react";
import {
  CURRENT_SELECTION_WORKSTATION_PROMPT_MODEL_PATH,
  MonacoPromptEditor,
  PromptEditorDiagnosticsPanel,
} from "../../../../components/prompt-editor";
import { VerticalResizableWidth } from "../../../../components/prompt-editor/vertical-resizable-width";
import { ExpandablePanelTrigger } from "../../../../components/ui";
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
import {
  CURRENT_SELECTION_CODE_SUBTLE_CLASS,
  CURRENT_SELECTION_FIELD_PANEL_CLASS,
  CURRENT_SELECTION_NOTICE_SUBTLE_CLASS,
} from "../../base/components/detail-card-shared";
import type {
  EditableWorkstationPromptHelpState,
  WorkstationDetailCardProps,
} from "../lib/detail-card-types";
import type { getWorkstationDetailMessages } from "../messages/workstation-detail";

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
    <div className="grid min-w-0 gap-2">
      <VerticalResizableWidth
        resizeHandleLabel={
          messages.editableConfigurationPromptResizeHandleLabel
        }
      >
        <MonacoPromptEditor
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
          height="100%"
          loadingMessage={messages.editableConfigurationPromptEditorLoading}
          modelPath={CURRENT_SELECTION_WORKSTATION_PROMPT_MODEL_PATH}
          onChange={state.onPromptChange}
          startupErrorMessage={messages.editableConfigurationPromptEditorError}
          value={state.draft.prompt}
        />
      </VerticalResizableWidth>
      <EditableConfigurationPromptFeedback
        diagnosticsId={diagnosticsId}
        messages={messages}
        state={state}
      />
    </div>
  );
}

function EditableConfigurationPromptFeedback({
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
  const hasSyntaxDiagnostics = state.promptDiagnostics.some(
    (diagnostic) => diagnostic.kind === "SYNTAX_ERROR",
  );

  if (
    state.promptHelpState.status !== "ready" &&
    (state.promptValidationState.status === "error" ||
      hasSyntaxDiagnostics ||
      state.promptDiagnostics.length > 0)
  ) {
    return (
      <PromptEditorDiagnosticsPanel
        diagnostics={state.promptDiagnostics}
        id={diagnosticsId}
        labels={{
          diagnosticsHeading:
            messages.editableConfigurationPromptDiagnosticsHeading,
          diagnosticsSummary:
            messages.editableConfigurationPromptDiagnosticsSummary,
          validationErrorPrefix:
            messages.editableConfigurationPromptValidationErrorPrefix,
          validationLoading:
            messages.editableConfigurationPromptValidationLoading,
          variableDiagnosticLabel:
            messages.editableConfigurationPromptVariableDiagnosticLabel,
        }}
        validationState={state.promptValidationState}
      />
    );
  }

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
          "m-0 text-on-error-container",
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

  const readyPromptState: Extract<
    NonNullable<WorkstationDetailCardProps["editableConfigurationState"]>,
    { status: "ready" }
  > & {
    promptHelpState: Extract<
      EditableWorkstationPromptHelpState,
      { status: "ready" }
    >;
  } = {
    ...state,
    promptHelpState: state.promptHelpState,
  };

  return (
    <EditableConfigurationReadyPromptFeedback
      diagnosticsId={diagnosticsId}
      messages={messages}
      state={readyPromptState}
    />
  );
}

function EditableConfigurationReadyPromptFeedback({
  diagnosticsId,
  messages,
  state,
}: {
  diagnosticsId: string;
  messages: ReturnType<typeof getWorkstationDetailMessages>;
  state: Extract<
    NonNullable<WorkstationDetailCardProps["editableConfigurationState"]>,
    { status: "ready" }
  > & {
    promptHelpState: Extract<
      EditableWorkstationPromptHelpState,
      { status: "ready" }
    >;
  };
}) {
  const [expanded, setExpanded] = useState(false);
  const previousDiagnosticsCountRef = useRef(state.promptDiagnostics.length);
  const sectionId = useId();
  const contentId = `${sectionId}-prompt-feedback-content`;
  const promptHelpState = state.promptHelpState;
  const hasSyntaxDiagnostics = state.promptDiagnostics.some(
    (diagnostic) => diagnostic.kind === "SYNTAX_ERROR",
  );
  const showInlineValidationPanel =
    !expanded &&
    (hasSyntaxDiagnostics || state.promptValidationState.status === "error");

  useEffect(() => {
    if (
      previousDiagnosticsCountRef.current === 0 &&
      state.promptDiagnostics.length > 0
    ) {
      setExpanded(true);
    }

    previousDiagnosticsCountRef.current = state.promptDiagnostics.length;
  }, [state.promptDiagnostics.length]);

  return (
    <div className={CURRENT_SELECTION_FIELD_PANEL_CLASS}>
      <div className="flex flex-wrap items-start justify-between gap-2">
        <p
          className={cn(
            "m-0 min-w-0 flex-1",
            CURRENT_SELECTION_NOTICE_SUBTLE_CLASS,
          )}
        >
          {messages.editableConfigurationPromptAutocompleteSummary(
            promptHelpState.contract.availableVariables.length,
            promptHelpState.contract.inputCount,
          )}
        </p>
        <ExpandablePanelTrigger
          aria-label={
            expanded
              ? messages.editableConfigurationPromptHelpCollapseActionLabel
              : messages.editableConfigurationPromptHelpExpandActionLabel
          }
          controlsID={contentId}
          expanded={expanded}
          onClick={() => setExpanded((current) => !current)}
          type="button"
          variant="section"
        >
          {expanded ? messages.collapseAction : messages.expandAction}
        </ExpandablePanelTrigger>
      </div>
      {showInlineValidationPanel ? (
        <PromptEditorDiagnosticsPanel
          diagnostics={state.promptDiagnostics}
          id={diagnosticsId}
          labels={{
            diagnosticsHeading:
              messages.editableConfigurationPromptDiagnosticsHeading,
            diagnosticsSummary:
              messages.editableConfigurationPromptDiagnosticsSummary,
            validationErrorPrefix:
              messages.editableConfigurationPromptValidationErrorPrefix,
            validationLoading:
              messages.editableConfigurationPromptValidationLoading,
            variableDiagnosticLabel:
              messages.editableConfigurationPromptVariableDiagnosticLabel,
          }}
          validationState={state.promptValidationState}
        />
      ) : null}
      {!expanded && !showInlineValidationPanel && state.promptDiagnostics.length > 0 ? (
        <div aria-hidden="true" className="sr-only" id={diagnosticsId} />
      ) : null}
      <Collapsible onOpenChange={setExpanded} open={expanded}>
        <CollapsibleContent className="grid gap-2 pt-2" id={contentId}>
          <EditableConfigurationPromptAutocompleteDetails
            messages={messages}
            promptHelpState={promptHelpState}
          />
          <PromptEditorDiagnosticsPanel
            diagnostics={state.promptDiagnostics}
            id={diagnosticsId}
            labels={{
              diagnosticsHeading:
                messages.editableConfigurationPromptDiagnosticsHeading,
              diagnosticsSummary:
                messages.editableConfigurationPromptDiagnosticsSummary,
              validationErrorPrefix:
                messages.editableConfigurationPromptValidationErrorPrefix,
              validationLoading:
                messages.editableConfigurationPromptValidationLoading,
              variableDiagnosticLabel:
                messages.editableConfigurationPromptVariableDiagnosticLabel,
            }}
            validationState={state.promptValidationState}
          />
        </CollapsibleContent>
      </Collapsible>
    </div>
  );
}

function EditableConfigurationPromptAutocompleteDetails({
  messages,
  promptHelpState,
}: {
  messages: ReturnType<typeof getWorkstationDetailMessages>;
  promptHelpState: Extract<
    EditableWorkstationPromptHelpState,
    { status: "ready" }
  >;
}) {
  return (
    <>
      <p
        className={cn(
          "m-0 text-on-surface-subtle",
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
          heading={messages.editableConfigurationPromptUnavailableAccessHeading}
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
    </>
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
            className="grid min-w-0 gap-1 rounded-lg border border-outline bg-surface-container-high p-2"
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
                "m-0 text-on-surface-subtle [overflow-wrap:anywhere]",
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
