import {
  Collapsible,
  CollapsibleContent,
} from "@you-agent-factory/components/overlays";
import { useId, useState } from "react";
import {
  CURRENT_SELECTION_WORKSTATION_PROMPT_MODEL_PATH,
  MonacoPromptEditor,
  PromptEditorDiagnosticsPanel,
  type PromptEditorDiagnosticsPanelLabels,
} from "../../../../../components/prompt-editor";
import { VerticalResizableWidth } from "../../../../../components/prompt-editor/vertical-resizable-width";
import {
  FormError,
} from "@you-agent-factory/components/forms";
import { SurfacePanel } from "@you-agent-factory/components/layout";
import { Label, Text } from "@you-agent-factory/components/primitives";
import { ExpandablePanelTrigger } from "../../../../../components/ui/expandable-panel-trigger";
import { cn } from "../../../../../lib/cn";
import { CurrentSelectionFormField } from "../../../base/components/layout/current-selection-form-layout";
import { CurrentSelectionSubtleCode } from "../../../base/components/presentation/current-selection-supporting-text";
import { CurrentSelectionSupportingText } from "../../../base/components/presentation/current-selection-supporting-text";
import type {
  EditableWorkstationPromptHelpState,
  WorkstationDetailCardProps,
} from "../../lib/keys/detail-card-types";
import type { getWorkstationDetailMessages } from "../../messages/workstation-detail";

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

function editableConfigurationPromptDiagnosticsLabels(
  messages: ReturnType<typeof getWorkstationDetailMessages>,
): PromptEditorDiagnosticsPanelLabels {
  return {
    diagnosticsHeading: messages.editableConfigurationPromptDiagnosticsHeading,
    diagnosticsSummary: messages.editableConfigurationPromptDiagnosticsSummary,
    validationErrorPrefix:
      messages.editableConfigurationPromptValidationErrorPrefix,
    validationLoading: messages.editableConfigurationPromptValidationLoading,
    variableDiagnosticLabel:
      messages.editableConfigurationPromptVariableDiagnosticLabel,
  };
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
  const diagnosticsLabels =
    editableConfigurationPromptDiagnosticsLabels(messages);

  if (state.promptHelpState.status !== "ready") {
    return (
      <CurrentSelectionFormField>
        <EditableConfigurationNonReadyPromptHelpMessage
          messages={messages}
          promptHelpState={state.promptHelpState}
        />
        <EditableConfigurationPromptDiagnosticsReservedRegion
          diagnosticsId={diagnosticsId}
          diagnostics={state.promptDiagnostics}
          labels={diagnosticsLabels}
          validationState={state.promptValidationState}
        />
      </CurrentSelectionFormField>
    );
  }

  return (
    <EditableConfigurationReadyPromptFeedback
      key={state.initialValues.workstationName}
      diagnosticsId={diagnosticsId}
      diagnosticsLabels={diagnosticsLabels}
      messages={messages}
      state={{
        ...state,
        promptHelpState: state.promptHelpState,
      }}
    />
  );
}

function EditableConfigurationNonReadyPromptHelpMessage({
  messages,
  promptHelpState,
}: {
  messages: ReturnType<typeof getWorkstationDetailMessages>;
  promptHelpState: Exclude<
    EditableWorkstationPromptHelpState,
    { status: "ready" }
  >;
}) {
  if (promptHelpState.status === "loading") {
    return (
      <CurrentSelectionSupportingText>
        {messages.editableConfigurationPromptHelpLoading}
      </CurrentSelectionSupportingText>
    );
  }

  if (promptHelpState.status === "error") {
    return (
      <FormError>
        {messages.editableConfigurationPromptHelpErrorPrefix}{" "}
        {promptHelpState.errorMessage}
      </FormError>
    );
  }

  return (
    <CurrentSelectionSupportingText>
      {promptHelpState.message}
    </CurrentSelectionSupportingText>
  );
}

function EditableConfigurationPromptDiagnosticsReservedRegion({
  diagnostics,
  diagnosticsId,
  labels,
  validationState,
  visuallyHidden = false,
}: {
  diagnostics: Extract<
    NonNullable<WorkstationDetailCardProps["editableConfigurationState"]>,
    { status: "ready" }
  >["promptDiagnostics"];
  diagnosticsId: string;
  labels: PromptEditorDiagnosticsPanelLabels;
  validationState: Extract<
    NonNullable<WorkstationDetailCardProps["editableConfigurationState"]>,
    { status: "ready" }
  >["promptValidationState"];
  visuallyHidden?: boolean;
}) {
  return (
    <div className="min-h-24" data-prompt-diagnostics-reserved="true">
      <div className={cn(visuallyHidden && "invisible")}>
        <PromptEditorDiagnosticsPanel
          diagnostics={diagnostics}
          id={diagnosticsId}
          labels={labels}
          validationState={validationState}
        />
      </div>
    </div>
  );
}

function EditableConfigurationReadyPromptFeedback({
  diagnosticsId,
  diagnosticsLabels,
  messages,
  state,
}: {
  diagnosticsId: string;
  diagnosticsLabels: PromptEditorDiagnosticsPanelLabels;
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
  const sectionId = useId();
  const contentId = `${sectionId}-prompt-feedback-content`;
  const promptHelpState = state.promptHelpState;

  return (
    <CurrentSelectionFormField>
      <div className="flex flex-wrap items-start justify-between gap-2">
        <CurrentSelectionSupportingText className="min-w-0 flex-1">
          {messages.editableConfigurationPromptAutocompleteSummary(
            promptHelpState.contract.availableVariables.length,
            promptHelpState.contract.inputCount,
          )}
        </CurrentSelectionSupportingText>
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
      <Collapsible onOpenChange={setExpanded} open={expanded}>
        <EditableConfigurationPromptDiagnosticsReservedRegion
          diagnostics={state.promptDiagnostics}
          diagnosticsId={diagnosticsId}
          labels={diagnosticsLabels}
          validationState={state.promptValidationState}
          visuallyHidden={!expanded}
        />
        <CollapsibleContent className="grid gap-2 pt-2" id={contentId}>
          <EditableConfigurationPromptAutocompleteDetails
            messages={messages}
            promptHelpState={promptHelpState}
          />
        </CollapsibleContent>
      </Collapsible>
    </CurrentSelectionFormField>
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
      <Label as="h5" className="m-0">
        {heading}
      </Label>
      <ul className="m-0 grid list-none gap-1 p-0">
        {items.map((item) => (
          <SurfacePanel
            asChild
            className="grid min-w-0 gap-1"
            key={item.key}
            padding="compact"
            radius="lg"
          >
            <li>
              <CurrentSelectionSubtleCode className="[overflow-wrap:anywhere]">
                {item.label}
              </CurrentSelectionSubtleCode>
              <Text
                className="m-0 text-on-surface-subtle [overflow-wrap:anywhere]"
                variant="supporting"
              >
                {item.detail}
              </Text>
              <CurrentSelectionSubtleCode className="[overflow-wrap:anywhere]">
                {item.example}
              </CurrentSelectionSubtleCode>
            </li>
          </SurfacePanel>
        ))}
      </ul>
    </div>
  );
}
