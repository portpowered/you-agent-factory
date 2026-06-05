import type { ReactNode } from "react";

import { DashboardLabel, DashboardText, Input } from "../../../../components/ui";
import { CurrentSelectionFormField } from "../../base/public";
import type {
  EditableWorkstationOverwriteField,
  WorkstationDetailCardProps,
} from "../lib/detail-card-types";
import type { getWorkstationDetailMessages } from "../messages/workstation-detail";
import { EditableConfigurationServerChangedHint } from "./editable-configuration-server-changed-hint";

type ReadyEditableConfigurationState = Extract<
  NonNullable<WorkstationDetailCardProps["editableConfigurationState"]>,
  { status: "ready" }
>;

export function EditableConfigurationCronFields({
  messages,
  state,
}: {
  messages: ReturnType<typeof getWorkstationDetailMessages>;
  state: ReadyEditableConfigurationState;
}) {
  if (state.draft.behavior !== "CRON" || state.draft.cron == null) {
    return null;
  }

  const cron = state.draft.cron;

  return (
    <>
      <EditableConfigurationCronField
        errorMessage={state.validationErrors.cronSchedule}
        fieldId="editable-workstation-cron-schedule"
        hint={messages.cronScheduleFieldHint}
        input={
          <Input
            aria-describedby={
              state.validationErrors.cronSchedule
                ? "editable-workstation-cron-schedule-error editable-workstation-cron-schedule-hint"
                : "editable-workstation-cron-schedule-hint"
            }
            aria-invalid={
              state.validationErrors.cronSchedule ? "true" : undefined
            }
            id="editable-workstation-cron-schedule"
            onChange={(event) => state.onCronScheduleChange(event.target.value)}
            value={cron.schedule}
          />
        }
        label={messages.cronScheduleFieldLabel}
        messages={messages}
        overwriteFieldName="cronSchedule"
        state={state}
      />
      <EditableConfigurationCronTriggerAtStartField
        messages={messages}
        state={state}
      />
      <EditableConfigurationCronField
        errorMessage={state.validationErrors.cronJitter}
        fieldId="editable-workstation-cron-jitter"
        hint={messages.cronJitterFieldHint}
        input={
          <Input
            aria-describedby={
              state.validationErrors.cronJitter
                ? "editable-workstation-cron-jitter-error editable-workstation-cron-jitter-hint"
                : "editable-workstation-cron-jitter-hint"
            }
            aria-invalid={
              state.validationErrors.cronJitter ? "true" : undefined
            }
            id="editable-workstation-cron-jitter"
            onChange={(event) => state.onCronJitterChange(event.target.value)}
            value={cron.jitter}
          />
        }
        label={messages.cronJitterFieldLabel}
        messages={messages}
        overwriteFieldName="cronJitter"
        state={state}
      />
      <EditableConfigurationCronField
        errorMessage={state.validationErrors.cronExpiryWindow}
        fieldId="editable-workstation-cron-expiry-window"
        hint={messages.cronExpiryWindowFieldHint}
        input={
          <Input
            aria-describedby={
              state.validationErrors.cronExpiryWindow
                ? "editable-workstation-cron-expiry-window-error editable-workstation-cron-expiry-window-hint"
                : "editable-workstation-cron-expiry-window-hint"
            }
            aria-invalid={
              state.validationErrors.cronExpiryWindow ? "true" : undefined
            }
            id="editable-workstation-cron-expiry-window"
            onChange={(event) =>
              state.onCronExpiryWindowChange(event.target.value)
            }
            value={cron.expiryWindow}
          />
        }
        label={messages.cronExpiryWindowFieldLabel}
        messages={messages}
        overwriteFieldName="cronExpiryWindow"
        state={state}
      />
    </>
  );
}

function EditableConfigurationCronField({
  errorMessage,
  fieldId,
  hint,
  input,
  label,
  messages,
  overwriteFieldName,
  state,
}: {
  errorMessage?: string;
  fieldId: string;
  hint: string;
  input: ReactNode;
  label: string;
  messages: ReturnType<typeof getWorkstationDetailMessages>;
  overwriteFieldName: Extract<
    EditableWorkstationOverwriteField,
    "cronExpiryWindow" | "cronJitter" | "cronSchedule"
  >;
  state: ReadyEditableConfigurationState;
}) {
  return (
    <CurrentSelectionFormField>
      <DashboardLabel as="label" htmlFor={fieldId}>
        {label}
      </DashboardLabel>
      {input}
      <DashboardText
        className="m-0 text-on-surface-subtle"
        id={`${fieldId}-hint`}
        variant="supporting"
      >
        {hint}
      </DashboardText>
      <EditableConfigurationServerChangedHint
        fieldName={overwriteFieldName}
        messages={messages}
        state={state}
      />
      {errorMessage ? (
        <DashboardText
          className="m-0 text-on-error-container"
          id={`${fieldId}-error`}
          role="alert"
          variant="supporting"
        >
          {errorMessage}
        </DashboardText>
      ) : null}
    </CurrentSelectionFormField>
  );
}

function EditableConfigurationCronTriggerAtStartField({
  messages,
  state,
}: {
  messages: ReturnType<typeof getWorkstationDetailMessages>;
  state: ReadyEditableConfigurationState;
}) {
  const cron = state.draft.cron;
  if (!cron) {
    return null;
  }

  const fieldId = "editable-workstation-cron-trigger-at-start";

  return (
    <CurrentSelectionFormField>
      <DashboardText
        as="label"
        className="inline-flex items-center gap-2 text-on-surface"
        htmlFor={fieldId}
      >
        <input
          checked={cron.triggerAtStart}
          className="size-4 rounded border border-outline"
          id={fieldId}
          onChange={(event) =>
            state.onCronTriggerAtStartChange(event.target.checked)
          }
          type="checkbox"
        />
        {messages.cronTriggerAtStartFieldLabel}
      </DashboardText>
      <EditableConfigurationServerChangedHint
        fieldName="cronTriggerAtStart"
        messages={messages}
        state={state}
      />
      {state.validationErrors.cronTriggerAtStart ? (
        <DashboardText
          className="m-0 text-on-error-container"
          id={`${fieldId}-error`}
          role="alert"
          variant="supporting"
        >
          {state.validationErrors.cronTriggerAtStart}
        </DashboardText>
      ) : null}
    </CurrentSelectionFormField>
  );
}
