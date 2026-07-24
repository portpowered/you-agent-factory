import { OptionalEnumSelect } from "@you-agent-factory/components/forms";
import { type ReactNode, useId } from "react";

import {
  AlertPanel,
  AlertPanelText,
  FormWarning,
  Input,
  Label,
  Text,
} from "../../../../components/ui";
import { formatList } from "../../../../components/ui/formatters";
import { EDITABLE_RESOURCE_TYPES } from "../../../current-factory-definition/lib/resource-editable-values";
import { CurrentSelectionExpandableSection } from "../../base/components/detail/current-selection-expandable-section";
import { mergeDetailCardSaveFieldErrors } from "../../base/components/save/detail-card-factory-save-feedback";
import {
  CurrentSelectionDetailFeedback,
  CurrentSelectionFormField,
} from "../../base/public";
import { formatEditableResourceOverwriteFieldLabels } from "../editing/editable-resource-overwrite-fields";
import type {
  EditableResourceConfigurationState,
  EditableResourceOverwriteField,
  EditableResourceSaveState,
  EditableResourceSaveValidationErrors,
  ResourceDetailCardProps,
  ResourceDetailState,
} from "../lib/detail-card-types";
import type { EditableResourceValidationErrors } from "../lib/resource-editable-validation";
import type { getResourceDetailMessages } from "../messages/resource-detail";
import {
  ResourceReferencingWorkersSection,
  ResourceReferencingWorkstationsSection,
  ResourceSummarySection,
} from "./resource-detail-context-section";

export function ResourceEditableConfigurationSection({
  detailState,
  messages,
  resourceName,
  saveState,
  state,
  tokenCount,
}: {
  detailState: Extract<ResourceDetailState, { status: "ready" }>;
  messages: ReturnType<typeof getResourceDetailMessages>;
  resourceName: string;
  saveState?: EditableResourceSaveState;
  state?: ResourceDetailCardProps["editableConfigurationState"];
  tokenCount?: number | null;
}) {
  const sectionId = useId();
  const contentId = `${sectionId}-content`;
  const headingId = `${sectionId}-heading`;

  return (
    <>
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
          <ResourceEditableConfigurationReadyForm
            messages={messages}
            resourceName={resourceName}
            saveState={saveState}
            state={state}
          />
        ) : null}
      </CurrentSelectionExpandableSection>

      <ResourceRuntimeContextSection
        detailState={detailState}
        messages={messages}
        tokenCount={tokenCount}
      />
    </>
  );
}

function ResourceEditableConfigurationReadyForm({
  messages,
  resourceName,
  saveState,
  state,
}: {
  messages: ReturnType<typeof getResourceDetailMessages>;
  resourceName: string;
  saveState?: EditableResourceSaveState;
  state: Extract<EditableResourceConfigurationState, { status: "ready" }>;
}) {
  const validationErrors = mergeDetailCardSaveFieldErrors<
    EditableResourceValidationErrors & Record<string, string | undefined>,
    EditableResourceSaveValidationErrors
  >(state.validationErrors, saveState);

  return (
    <form className="grid gap-3" onSubmit={(event) => event.preventDefault()}>
      <ResourceEditableConfigurationSharedImpactWarning
        messages={messages}
        resourceName={resourceName}
        state={state}
      />
      <ResourceEditableConfigurationOverwriteWarning
        messages={messages}
        overwriteFieldNames={state.overwriteFieldNames}
      />
      {validationErrors.contract ? (
        <CurrentSelectionDetailFeedback role="alert" tone="danger">
          {validationErrors.contract}
        </CurrentSelectionDetailFeedback>
      ) : null}
      <ResourceEditableConfigurationDraftStatus
        messages={messages}
        state={state}
      />
      <ResourceEditableConfigurationField
        errorMessage={validationErrors.name}
        fieldId="editable-resource-name"
        input={
          <Input
            aria-describedby={
              validationErrors.name ? "editable-resource-name-error" : undefined
            }
            aria-invalid={validationErrors.name ? "true" : undefined}
            id="editable-resource-name"
            onChange={(event) => state.onNameChange(event.target.value)}
            type="text"
            value={state.draft.name}
          />
        }
        label={messages.nameFieldLabel}
        supportingContent={
          <ResourceEditableConfigurationServerChangedHint
            fieldName="name"
            messages={messages}
            state={state}
          />
        }
      />
      <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
        <ResourceEditableConfigurationField
          errorMessage={validationErrors.capacity}
          fieldId="editable-resource-capacity"
          input={
            <Input
              aria-describedby={
                validationErrors.capacity
                  ? "editable-resource-capacity-error"
                  : undefined
              }
              aria-invalid={validationErrors.capacity ? "true" : undefined}
              id="editable-resource-capacity"
              inputMode="numeric"
              onChange={(event) => state.onCapacityChange(event.target.value)}
              type="text"
              value={state.draft.capacityText}
            />
          }
          label={messages.capacityFieldLabel}
          supportingContent={
            <ResourceEditableConfigurationServerChangedHint
              fieldName="capacity"
              messages={messages}
              state={state}
            />
          }
        />
        <ResourceEditableConfigurationField
          errorMessage={validationErrors.type}
          fieldId="editable-resource-type"
          input={
            <OptionalEnumSelect
              aria-describedby={
                validationErrors.type
                  ? "editable-resource-type-error"
                  : undefined
              }
              aria-invalid={validationErrors.type ? "true" : undefined}
              aria-label={messages.typeFieldLabel}
              emptyOptionLabel={messages.notConfiguredValue}
              id="editable-resource-type"
              onValueChange={(nextValue) =>
                state.onTypeChange(
                  nextValue as NonNullable<typeof state.draft.type> | null,
                )
              }
              options={EDITABLE_RESOURCE_TYPES.map((resourceType) => ({
                label: messages.localizeResourceType(resourceType),
                value: resourceType,
              }))}
              value={state.draft.type}
            />
          }
          label={messages.typeFieldLabel}
          supportingContent={
            <ResourceEditableConfigurationServerChangedHint
              fieldName="type"
              messages={messages}
              state={state}
            />
          }
        />
      </div>
      <ResourceTypeSpecificFields
        messages={messages}
        state={state}
        validationErrors={validationErrors}
      />
    </form>
  );
}

function ResourceTypeSpecificFields({
  messages,
  state,
  validationErrors,
}: {
  messages: ReturnType<typeof getResourceDetailMessages>;
  state: Extract<EditableResourceConfigurationState, { status: "ready" }>;
  validationErrors: Extract<
    EditableResourceConfigurationState,
    { status: "ready" }
  >["validationErrors"];
}) {
  if (state.draft.type === "MODEL") {
    return (
      <div className="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-3">
        <ResourceEditableConfigurationField
          errorMessage={validationErrors.model}
          fieldId="editable-resource-model"
          input={
            <Input
              aria-describedby={
                validationErrors.model
                  ? "editable-resource-model-error"
                  : undefined
              }
              aria-invalid={validationErrors.model ? "true" : undefined}
              id="editable-resource-model"
              onChange={(event) => state.onModelChange(event.target.value)}
              type="text"
              value={state.draft.model}
            />
          }
          label={messages.modelFieldLabel}
          supportingContent={
            <ResourceEditableConfigurationServerChangedHint
              fieldName="model"
              messages={messages}
              state={state}
            />
          }
        />
        <ResourceEditableConfigurationField
          errorMessage={validationErrors.backend}
          fieldId="editable-resource-backend"
          input={
            <Input
              aria-describedby={
                validationErrors.backend
                  ? "editable-resource-backend-error"
                  : undefined
              }
              aria-invalid={validationErrors.backend ? "true" : undefined}
              id="editable-resource-backend"
              onChange={(event) => state.onBackendChange(event.target.value)}
              type="text"
              value={state.draft.backend}
            />
          }
          label={messages.backendFieldLabel}
          supportingContent={
            <ResourceEditableConfigurationServerChangedHint
              fieldName="backend"
              messages={messages}
              state={state}
            />
          }
        />
        <ResourceEditableConfigurationField
          errorMessage={validationErrors.loadPolicy}
          fieldId="editable-resource-load-policy"
          input={
            <Input
              aria-describedby={
                validationErrors.loadPolicy
                  ? "editable-resource-load-policy-error"
                  : undefined
              }
              aria-invalid={validationErrors.loadPolicy ? "true" : undefined}
              id="editable-resource-load-policy"
              onChange={(event) => state.onLoadPolicyChange(event.target.value)}
              type="text"
              value={state.draft.loadPolicy}
            />
          }
          label={messages.loadPolicyFieldLabel}
          supportingContent={
            <ResourceEditableConfigurationServerChangedHint
              fieldName="loadPolicy"
              messages={messages}
              state={state}
            />
          }
        />
      </div>
    );
  }

  if (state.draft.type === "PROVIDER_QUOTA") {
    return (
      <ResourceEditableConfigurationField
        errorMessage={validationErrors.provider}
        fieldId="editable-resource-provider"
        input={
          <Input
            aria-describedby={
              validationErrors.provider
                ? "editable-resource-provider-error"
                : undefined
            }
            aria-invalid={validationErrors.provider ? "true" : undefined}
            id="editable-resource-provider"
            onChange={(event) => state.onProviderChange(event.target.value)}
            type="text"
            value={state.draft.provider}
          />
        }
        label={messages.providerFieldLabel}
        supportingContent={
          <ResourceEditableConfigurationServerChangedHint
            fieldName="provider"
            messages={messages}
            state={state}
          />
        }
      />
    );
  }

  return null;
}

function ResourceRuntimeContextSection({
  detailState,
  messages,
  tokenCount,
}: {
  detailState: Extract<ResourceDetailState, { status: "ready" }>;
  messages: ReturnType<typeof getResourceDetailMessages>;
  tokenCount?: number | null;
}) {
  const { resource, workerNames, workstationNames } = detailState;
  const typeLabel = resource.type
    ? messages.localizeResourceType(resource.type)
    : messages.notConfiguredValue;

  return (
    <>
      <ResourceSummarySection
        messages={messages}
        resource={resource}
        tokenCount={tokenCount}
        typeLabel={typeLabel}
      />
      <ResourceReferencingWorkersSection
        messages={messages}
        workerNames={workerNames}
      />
      <ResourceReferencingWorkstationsSection
        messages={messages}
        workstationNames={workstationNames}
      />
    </>
  );
}

function ResourceEditableConfigurationOverwriteWarning({
  messages,
  overwriteFieldNames,
}: {
  messages: ReturnType<typeof getResourceDetailMessages>;
  overwriteFieldNames: EditableResourceOverwriteField[];
}) {
  if (overwriteFieldNames.length === 0) {
    return null;
  }

  const formattedFields = formatEditableResourceOverwriteFieldLabels(
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

function ResourceEditableConfigurationServerChangedHint({
  fieldName,
  messages,
  state,
}: {
  fieldName: EditableResourceOverwriteField;
  messages: ReturnType<typeof getResourceDetailMessages>;
  state: Extract<EditableResourceConfigurationState, { status: "ready" }>;
}) {
  if (!state.overwriteFieldNames.includes(fieldName)) {
    return null;
  }

  return (
    <FormWarning>
      {messages.editableConfigurationServerFieldChangedHint}
    </FormWarning>
  );
}

function ResourceEditableConfigurationSharedImpactWarning({
  messages,
  resourceName,
  state,
}: {
  messages: ReturnType<typeof getResourceDetailMessages>;
  resourceName: string;
  state: Extract<EditableResourceConfigurationState, { status: "ready" }>;
}) {
  const workerNames = state.initialValues.workerNames;
  const workstationNames = state.initialValues.workstationNames;
  if (workerNames.length === 0 && workstationNames.length === 0) {
    return null;
  }

  return (
    <AlertPanel tone="warning">
      <AlertPanelText role="alert">
        {messages.editableConfigurationSharedImpactWarning(
          state.draft.name.trim() || resourceName,
          formatList(workerNames),
          formatList(workstationNames),
        )}
      </AlertPanelText>
    </AlertPanel>
  );
}

function ResourceEditableConfigurationDraftStatus({
  messages,
  state,
}: {
  messages: ReturnType<typeof getResourceDetailMessages>;
  state: Extract<EditableResourceConfigurationState, { status: "ready" }>;
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

function ResourceEditableConfigurationField({
  errorMessage,
  fieldId,
  input,
  label,
  supportingContent,
}: {
  errorMessage?: string;
  fieldId: string;
  input: ReactNode;
  label: string;
  supportingContent?: ReactNode;
}) {
  return (
    <CurrentSelectionFormField>
      <Label as="label" htmlFor={fieldId}>
        {label}
      </Label>
      {input}
      {supportingContent}
      {errorMessage ? (
        <CurrentSelectionDetailFeedback id={`${fieldId}-error`} tone="danger">
          {errorMessage}
        </CurrentSelectionDetailFeedback>
      ) : null}
    </CurrentSelectionFormField>
  );
}
