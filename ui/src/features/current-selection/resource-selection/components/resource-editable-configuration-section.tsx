// biome-ignore lint/nursery/noExcessiveLinesPerFile: resource editable fields, validation feedback, and runtime context stay colocated in one section.
import { type ReactNode, useId, useState } from "react";

import {
  DashboardActionButton,
  DashboardActionRow,
  DisclosureButton,
  Select,
} from "../../../../components/ui";
import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SUPPORTING_LABEL_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../../../components/ui/dashboard-typography";
import { formatList } from "../../../../components/ui/formatters";
import { cn } from "../../../../lib/cn";
import { EDITABLE_RESOURCE_TYPES } from "../../../current-factory-definition/lib/resource-editable-values";
import {
  CURRENT_SELECTION_FIELD_PANEL_CLASS,
  CURRENT_SELECTION_WARNING_PANEL_CLASS,
  CurrentSelectionSectionHeader,
  HISTORY_TOGGLE_CLASS,
} from "../../base/components/detail-card-shared";
import { formatEditableResourceOverwriteFieldLabels } from "../editing/editable-resource-overwrite-fields";
import type {
  EditableResourceConfigurationState,
  EditableResourceOverwriteField,
  ResourceDetailCardProps,
  ResourceDetailState,
} from "../lib/detail-card-types";
import type { getResourceDetailMessages } from "../messages/resource-detail";

export function ResourceEditableConfigurationSection({
  detailState,
  messages,
  resourceName,
  state,
  tokenCount,
}: {
  detailState: Extract<ResourceDetailState, { status: "ready" }>;
  messages: ReturnType<typeof getResourceDetailMessages>;
  resourceName: string;
  state?: ResourceDetailCardProps["editableConfigurationState"];
  tokenCount?: number | null;
}) {
  const [expanded, setExpanded] = useState(true);
  const sectionId = useId();
  const contentId = `${sectionId}-content`;
  const headingId = `${sectionId}-heading`;

  return (
    <div className="grid gap-4 [&_h4]:m-0">
      <section
        aria-labelledby={headingId}
        className="grid gap-2.5 [&_h4]:m-0"
      >
        <CurrentSelectionSectionHeader
          action={
            <DisclosureButton
              aria-label={
                expanded
                  ? messages.editableConfigurationCollapseActionLabel
                  : messages.editableConfigurationExpandActionLabel
              }
              className={HISTORY_TOGGLE_CLASS}
              controlsID={contentId}
              expanded={expanded}
              onClick={() => setExpanded((current) => !current)}
              type="button"
            >
              {expanded ? messages.collapseAction : messages.expandAction}
            </DisclosureButton>
          }
          headingId={headingId}
          title={messages.editableConfigurationHeading}
        />
        {expanded ? (
          <div className="grid gap-2.5" id={contentId}>
            {state?.status === "loading" ? (
              <p
                className={cn(
                  "m-0 text-af-text-muted",
                  DASHBOARD_BODY_TEXT_CLASS,
                )}
              >
                {messages.editableConfigurationLoading}
              </p>
            ) : null}
            {state?.status === "error" ? (
              <p
                className={cn(
                  "m-0 text-af-danger-text",
                  DASHBOARD_BODY_TEXT_CLASS,
                )}
                role="alert"
              >
                {messages.editableConfigurationErrorPrefix}{" "}
                {state.errorMessage}
              </p>
            ) : null}
            {state?.status === "empty" ? (
              <p
                className={cn(
                  "m-0 text-af-text-muted",
                  DASHBOARD_BODY_TEXT_CLASS,
                )}
              >
                {state.message || messages.editableConfigurationEmpty}
              </p>
            ) : null}
            {state?.status === "ready" ? (
              <ResourceEditableConfigurationReadyForm
                messages={messages}
                resourceName={resourceName}
                state={state}
              />
            ) : null}
          </div>
        ) : null}
      </section>

      <ResourceRuntimeContextSection
        detailState={detailState}
        messages={messages}
        tokenCount={tokenCount}
      />
    </div>
  );
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: resource editable form keeps validation feedback and field wiring colocated.
function ResourceEditableConfigurationReadyForm({
  messages,
  resourceName,
  state,
}: {
  messages: ReturnType<typeof getResourceDetailMessages>;
  resourceName: string;
  state: Extract<EditableResourceConfigurationState, { status: "ready" }>;
}) {
  const validationErrors = state.validationErrors;

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
        <p
          className={cn("m-0 text-af-danger-text", DASHBOARD_BODY_TEXT_CLASS)}
          role="alert"
        >
          {validationErrors.contract}
        </p>
      ) : null}
      <ResourceEditableConfigurationDraftStatus messages={messages} state={state} />
      <ResourceEditableConfigurationField
        errorMessage={validationErrors.name}
        fieldId="editable-resource-name"
        input={
          <input
            aria-describedby={
              validationErrors.name ? "editable-resource-name-error" : undefined
            }
            aria-invalid={validationErrors.name ? "true" : undefined}
            className="w-full rounded-lg border border-af-border bg-af-surface px-3 py-2 text-sm text-af-text"
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
            <input
              aria-describedby={
                validationErrors.capacity
                  ? "editable-resource-capacity-error"
                  : undefined
              }
              aria-invalid={validationErrors.capacity ? "true" : undefined}
              className="w-full rounded-lg border border-af-border bg-af-surface px-3 py-2 text-sm text-af-text"
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
            <Select
              aria-describedby={
                validationErrors.type ? "editable-resource-type-error" : undefined
              }
              aria-invalid={validationErrors.type ? "true" : undefined}
              aria-label={messages.typeFieldLabel}
              id="editable-resource-type"
              onChange={(event) => {
                const nextValue = event.target.value;
                state.onTypeChange(
                  nextValue.length > 0
                    ? (nextValue as NonNullable<typeof state.draft.type>)
                    : null,
                );
              }}
              value={state.draft.type ?? ""}
            >
              <option value="">{messages.notConfiguredValue}</option>
              {EDITABLE_RESOURCE_TYPES.map((resourceType) => (
                <option key={resourceType} value={resourceType}>
                  {messages.localizeResourceType(resourceType)}
                </option>
              ))}
            </Select>
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
      {state.isDirty || state.overwriteFieldNames.length > 0 ? (
        <DashboardActionRow
          actions={
            <DashboardActionButton
              onClick={state.onResetToLatest}
              type="button"
            >
              {messages.resetToLatestAction}
            </DashboardActionButton>
          }
        />
      ) : null}
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
            <input
              aria-describedby={
                validationErrors.model ? "editable-resource-model-error" : undefined
              }
              aria-invalid={validationErrors.model ? "true" : undefined}
              className="w-full rounded-lg border border-af-border bg-af-surface px-3 py-2 text-sm text-af-text"
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
            <input
              aria-describedby={
                validationErrors.backend
                  ? "editable-resource-backend-error"
                  : undefined
              }
              aria-invalid={validationErrors.backend ? "true" : undefined}
              className="w-full rounded-lg border border-af-border bg-af-surface px-3 py-2 text-sm text-af-text"
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
            <input
              aria-describedby={
                validationErrors.loadPolicy
                  ? "editable-resource-load-policy-error"
                  : undefined
              }
              aria-invalid={validationErrors.loadPolicy ? "true" : undefined}
              className="w-full rounded-lg border border-af-border bg-af-surface px-3 py-2 text-sm text-af-text"
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
          <input
            aria-describedby={
              validationErrors.provider
                ? "editable-resource-provider-error"
                : undefined
            }
            aria-invalid={validationErrors.provider ? "true" : undefined}
            className="w-full rounded-lg border border-af-border bg-af-surface px-3 py-2 text-sm text-af-text"
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
  const { workerNames, workstationNames } = detailState;

  return (
    <>
      {tokenCount !== null && tokenCount !== undefined ? (
        <section
          aria-labelledby="resource-runtime-heading"
          className="grid gap-2"
        >
          <CurrentSelectionSectionHeader
            headingId="resource-runtime-heading"
            title={messages.summaryHeading}
          />
          <div className="grid gap-0.5">
            <span className={DASHBOARD_SUPPORTING_LABEL_CLASS}>
              {messages.tokenCountFieldLabel}
            </span>
            <span className={cn("m-0 text-af-text", DASHBOARD_BODY_TEXT_CLASS)}>
              {String(tokenCount)}
            </span>
          </div>
        </section>
      ) : null}

      <section
        aria-labelledby="resource-referencing-workers-heading"
        className="grid gap-2"
      >
        <CurrentSelectionSectionHeader
          headingId="resource-referencing-workers-heading"
          title={messages.referencingWorkersHeading}
        />
        {workerNames.length > 0 ? (
          <p className={cn("m-0 text-af-text", DASHBOARD_BODY_TEXT_CLASS)}>
            {formatList(workerNames)}
          </p>
        ) : (
          <p
            className={cn(
              "m-0 text-af-text-muted",
              DASHBOARD_SUPPORTING_TEXT_CLASS,
            )}
          >
            {messages.referencingWorkersEmpty}
          </p>
        )}
      </section>

      <section
        aria-labelledby="resource-referencing-workstations-heading"
        className="grid gap-2"
      >
        <CurrentSelectionSectionHeader
          headingId="resource-referencing-workstations-heading"
          title={messages.referencingWorkstationsHeading}
        />
        {workstationNames.length > 0 ? (
          <p className={cn("m-0 text-af-text", DASHBOARD_BODY_TEXT_CLASS)}>
            {formatList(workstationNames)}
          </p>
        ) : (
          <p
            className={cn(
              "m-0 text-af-text-muted",
              DASHBOARD_SUPPORTING_TEXT_CLASS,
            )}
          >
            {messages.referencingWorkstationsEmpty}
          </p>
        )}
      </section>
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
    <div className={CURRENT_SELECTION_WARNING_PANEL_CLASS}>
      <p
        className={cn("m-0 text-af-warning-text", DASHBOARD_BODY_TEXT_CLASS)}
        role="alert"
      >
        {messages.editableConfigurationOverwriteWarning(formattedFields)}
      </p>
      <p
        className={cn(
          "m-0 text-af-text-subtle",
          DASHBOARD_SUPPORTING_TEXT_CLASS,
        )}
      >
        {messages.editableConfigurationOverwriteWarningDetail}
      </p>
    </div>
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
    <p
      className={cn(
        "m-0 text-af-warning-text",
        DASHBOARD_SUPPORTING_TEXT_CLASS,
      )}
    >
      {messages.editableConfigurationServerFieldChangedHint}
    </p>
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
    <div className={CURRENT_SELECTION_WARNING_PANEL_CLASS}>
      <p
        className={cn("m-0 text-af-warning-text", DASHBOARD_BODY_TEXT_CLASS)}
        role="alert"
      >
        {messages.editableConfigurationSharedImpactWarning(
          state.draft.name.trim() || resourceName,
          formatList(workerNames),
          formatList(workstationNames),
        )}
      </p>
      <p
        className={cn(
          "m-0 text-af-text-subtle",
          DASHBOARD_SUPPORTING_TEXT_CLASS,
        )}
      >
        {messages.editableConfigurationSharedImpactWarningDetail}
      </p>
    </div>
  );
}

function ResourceEditableConfigurationDraftStatus({
  messages,
  state,
}: {
  messages: ReturnType<typeof getResourceDetailMessages>;
  state: Extract<EditableResourceConfigurationState, { status: "ready" }>;
}) {
  return (
    <div className={CURRENT_SELECTION_FIELD_PANEL_CLASS}>
      <p
        className={cn(
          "m-0",
          state.hasValidationErrors
            ? "text-af-danger-text"
            : "text-af-text-muted",
          DASHBOARD_BODY_TEXT_CLASS,
        )}
        role={state.hasValidationErrors ? "alert" : "status"}
      >
        {state.hasValidationErrors
          ? messages.editableConfigurationValidationStatus
          : state.isDirty
            ? messages.editableConfigurationDirtyStatus
            : messages.editableConfigurationDraftNote}
      </p>
      {state.hasValidationErrors ? (
        <p
          className={cn(
            "m-0 text-af-text-subtle",
            DASHBOARD_SUPPORTING_TEXT_CLASS,
          )}
        >
          {messages.editableConfigurationSaveDisabledValidationDetail}
        </p>
      ) : (
        <p
          className={cn(
            "m-0 text-af-text-subtle",
            DASHBOARD_SUPPORTING_TEXT_CLASS,
          )}
        >
          {messages.editableConfigurationDraftNote}
        </p>
      )}
    </div>
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
    <div className={CURRENT_SELECTION_FIELD_PANEL_CLASS}>
      <label className={DASHBOARD_SUPPORTING_LABEL_CLASS} htmlFor={fieldId}>
        {label}
      </label>
      {input}
      {supportingContent}
      {errorMessage ? (
        <p
          className={cn(
            "m-0 text-af-danger-text",
            DASHBOARD_SUPPORTING_TEXT_CLASS,
          )}
          id={`${fieldId}-error`}
        >
          {errorMessage}
        </p>
      ) : null}
    </div>
  );
}
