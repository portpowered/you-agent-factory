import { useId } from "react";

import { AlertPanel, AlertPanelText, Text } from "../../../../../components/ui";
import { formatList } from "../../../../../components/ui/formatters";
import {
  isModelProviderWorkerType,
  isScriptWorkerType,
} from "../../../../current-factory-definition/public";
import { CurrentSelectionExpandableSection } from "../../../base/components/detail/current-selection-expandable-section";
import { mergeDetailCardSaveFieldErrors } from "../../../base/components/save/detail-card-factory-save-feedback";
import {
  CurrentSelectionDetailFeedback,
  CurrentSelectionFormField,
  CurrentSelectionFormFields,
} from "../../../base/public";
import { formatEditableWorkerOverwriteFieldLabels } from "../../editing/editable-worker-overwrite-fields";
import type {
  EditableWorkerOverwriteField,
  EditableWorkerSaveState,
  EditableWorkerSaveValidationErrors,
  EditableWorkerValidationErrors,
  WorkerDetailCardProps,
} from "../../lib/detail-card-types";
import type { getWorkerDetailMessages } from "../../messages/worker-detail";
import type {
  ReadyWorkerEditableConfigurationState,
  ReadyWorkerEditableValidationErrors,
  WorkerEditableConfigurationMessages,
} from "./fields/primitives/worker-editable-configuration-field-types";
import { WorkerEditableConfigurationHostedFields } from "./fields/worker-editable-configuration-hosted-fields";
import { WorkerEditableConfigurationModelFields } from "./fields/worker-editable-configuration-model-fields";
import { WorkerEditableConfigurationScriptFields } from "./fields/worker-editable-configuration-script-fields";
import { WorkerEditableConfigurationSharedFields } from "./fields/worker-editable-configuration-shared-fields";

export function WorkerEditableConfigurationSection({
  messages,
  saveState,
  state,
  workerName,
}: {
  messages: ReturnType<typeof getWorkerDetailMessages>;
  saveState?: EditableWorkerSaveState;
  state?: WorkerDetailCardProps["editableConfigurationState"];
  workerName: string;
}) {
  const sectionId = useId();
  const contentId = `${sectionId}-content`;
  const headingId = `${sectionId}-heading`;

  return (
    <CurrentSelectionExpandableSection
      contentId={contentId}
      defaultExpanded
      headingId={headingId}
      title={messages.editableConfigurationHeading}
      toggleLabel={(expanded) =>
        expanded
          ? messages.editableConfigurationCollapseActionLabel
          : messages.editableConfigurationExpandActionLabel
      }
    >
      {state?.status === "loading" ? (
        <CurrentSelectionDetailFeedback>
          {messages.editableConfigurationLoading}
        </CurrentSelectionDetailFeedback>
      ) : null}
      {state?.status === "error" ? (
        <CurrentSelectionDetailFeedback role="alert" tone="danger">
          {messages.editableConfigurationErrorPrefix} {state.errorMessage}
        </CurrentSelectionDetailFeedback>
      ) : null}
      {state?.status === "empty" ? (
        <CurrentSelectionDetailFeedback>
          {state.message || messages.editableConfigurationEmpty}
        </CurrentSelectionDetailFeedback>
      ) : null}
      {state?.status === "ready" ? (
        <WorkerEditableConfigurationReadyForm
          messages={messages}
          saveState={saveState}
          state={state}
          workerName={workerName}
        />
      ) : null}
    </CurrentSelectionExpandableSection>
  );
}

function WorkerEditableConfigurationReadyForm({
  messages,
  saveState,
  state,
  workerName,
}: {
  messages: WorkerEditableConfigurationMessages;
  saveState?: EditableWorkerSaveState;
  state: ReadyWorkerEditableConfigurationState;
  workerName: string;
}) {
  const validationErrors = mergeDetailCardSaveFieldErrors<
    EditableWorkerValidationErrors & Record<string, string | undefined>,
    EditableWorkerSaveValidationErrors
  >(state.validationErrors, saveState);

  return (
    <form className="grid gap-3" onSubmit={(event) => event.preventDefault()}>
      <WorkerEditableConfigurationSharedImpactWarning
        messages={messages}
        state={state}
        workerName={workerName}
      />
      <WorkerEditableConfigurationOverwriteWarning
        messages={messages}
        overwriteFieldNames={state.overwriteFieldNames}
      />
      {validationErrors.contract ? (
        <CurrentSelectionDetailFeedback role="alert" tone="danger">
          {validationErrors.contract}
        </CurrentSelectionDetailFeedback>
      ) : null}
      <WorkerEditableConfigurationDraftStatus
        messages={messages}
        state={state}
      />
      <CurrentSelectionFormFields>
        <WorkerEditableConfigurationSharedFields
          messages={messages}
          state={state}
          validationErrors={validationErrors}
        />
        <WorkerTypeSpecificFields
          messages={messages}
          state={state}
          validationErrors={validationErrors}
        />
      </CurrentSelectionFormFields>
    </form>
  );
}

function WorkerEditableConfigurationOverwriteWarning({
  messages,
  overwriteFieldNames,
}: {
  messages: WorkerEditableConfigurationMessages;
  overwriteFieldNames: EditableWorkerOverwriteField[];
}) {
  if (overwriteFieldNames.length === 0) {
    return null;
  }

  const formattedFields = formatEditableWorkerOverwriteFieldLabels(
    overwriteFieldNames,
    messages,
  );

  return (
    <AlertPanel tone="warning">
      <AlertPanelText role="alert">
        {messages.editableConfigurationOverwriteWarning(formattedFields)}
      </AlertPanelText>
    </AlertPanel>
  );
}

function WorkerEditableConfigurationSharedImpactWarning({
  messages,
  state,
  workerName,
}: {
  messages: WorkerEditableConfigurationMessages;
  state: ReadyWorkerEditableConfigurationState;
  workerName: string;
}) {
  const workstationNames = state.initialValues.workstationNames;
  if (workstationNames.length <= 1) {
    return null;
  }

  return (
    <AlertPanel tone="warning">
      <AlertPanelText role="alert">
        {messages.editableConfigurationSharedImpactWarning(
          state.draft.name.trim() || workerName,
          formatList(workstationNames),
        )}
      </AlertPanelText>
    </AlertPanel>
  );
}

function WorkerEditableConfigurationDraftStatus({
  messages,
  state,
}: {
  messages: WorkerEditableConfigurationMessages;
  state: ReadyWorkerEditableConfigurationState;
}) {
  if (!state.hasValidationErrors) {
    return null;
  }

  return (
    <CurrentSelectionFormField>
      <CurrentSelectionDetailFeedback role="alert" tone="danger">
        {messages.editableConfigurationValidationStatus}
      </CurrentSelectionDetailFeedback>
      <Text className="m-0 text-on-surface-subtle" variant="supporting">
        {messages.editableConfigurationSaveDisabledValidationDetail}
      </Text>
    </CurrentSelectionFormField>
  );
}

function WorkerTypeSpecificFields({
  messages,
  state,
  validationErrors,
}: {
  messages: WorkerEditableConfigurationMessages;
  state: ReadyWorkerEditableConfigurationState;
  validationErrors: ReadyWorkerEditableValidationErrors;
}) {
  if (isModelProviderWorkerType(state.draft.type)) {
    return (
      <WorkerEditableConfigurationModelFields
        messages={messages}
        state={state}
        validationErrors={validationErrors}
      />
    );
  }

  if (isScriptWorkerType(state.draft.type)) {
    return (
      <WorkerEditableConfigurationScriptFields
        messages={messages}
        state={state}
        validationErrors={validationErrors}
      />
    );
  }

  return (
    <WorkerEditableConfigurationHostedFields
      messages={messages}
      state={state}
      validationErrors={validationErrors}
    />
  );
}
