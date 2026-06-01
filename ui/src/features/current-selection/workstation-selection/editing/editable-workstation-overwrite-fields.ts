import { formatList } from "../../../../components/ui/formatters";
import type {
  EditableWorkstationCronDraft,
  EditableWorkstationDraft,
} from "../../../current-factory-definition/lib/workstation-editable-values";
import type { EditableWorkstationOverwriteField } from "../lib/detail-card-types";
import type { WorkstationDetailMessages } from "../messages/workstation-detail-types";

export function resolveEditableWorkstationOverwriteFields(
  sessionStartDraft: EditableWorkstationDraft,
  draft: EditableWorkstationDraft,
  latestDefinitionDraft: EditableWorkstationDraft,
): EditableWorkstationOverwriteField[] {
  const fields: EditableWorkstationOverwriteField[] = [];

  if (
    sessionStartDraft.behavior !== latestDefinitionDraft.behavior &&
    draft.behavior !== latestDefinitionDraft.behavior
  ) {
    fields.push("behavior");
  }
  if (
    sessionStartDraft.prompt !== latestDefinitionDraft.prompt &&
    draft.prompt !== latestDefinitionDraft.prompt
  ) {
    fields.push("prompt");
  }
  if (
    sessionStartDraft.workerName !== latestDefinitionDraft.workerName &&
    draft.workerName !== latestDefinitionDraft.workerName
  ) {
    fields.push("worker");
  }
  if (
    sessionStartDraft.runnerName !== latestDefinitionDraft.runnerName &&
    draft.runnerName !== latestDefinitionDraft.runnerName
  ) {
    fields.push("runner");
  }

  fields.push(
    ...resolveEditableWorkstationCronOverwriteFields(
      sessionStartDraft,
      draft,
      latestDefinitionDraft,
    ),
  );

  return fields;
}

function resolveEditableWorkstationCronOverwriteFields(
  sessionStartDraft: EditableWorkstationDraft,
  draft: EditableWorkstationDraft,
  latestDefinitionDraft: EditableWorkstationDraft,
): EditableWorkstationOverwriteField[] {
  if (
    sessionStartDraft.behavior !== "CRON" ||
    draft.behavior !== "CRON" ||
    latestDefinitionDraft.behavior !== "CRON"
  ) {
    return [];
  }

  const sessionStartCron = sessionStartDraft.cron;
  const draftCron = draft.cron;
  const latestCron = latestDefinitionDraft.cron;
  if (!sessionStartCron || !draftCron || !latestCron) {
    return [];
  }

  const fields: EditableWorkstationOverwriteField[] = [];
  appendCronOverwriteField(
    fields,
    "cronSchedule",
    sessionStartCron,
    draftCron,
    latestCron,
    "schedule",
  );
  appendCronOverwriteField(
    fields,
    "cronTriggerAtStart",
    sessionStartCron,
    draftCron,
    latestCron,
    "triggerAtStart",
  );
  appendCronOverwriteField(
    fields,
    "cronJitter",
    sessionStartCron,
    draftCron,
    latestCron,
    "jitter",
  );
  appendCronOverwriteField(
    fields,
    "cronExpiryWindow",
    sessionStartCron,
    draftCron,
    latestCron,
    "expiryWindow",
  );
  return fields;
}

function appendCronOverwriteField(
  fields: EditableWorkstationOverwriteField[],
  field: EditableWorkstationOverwriteField,
  sessionStartCron: EditableWorkstationCronDraft,
  draftCron: EditableWorkstationCronDraft,
  latestCron: EditableWorkstationCronDraft,
  key: keyof EditableWorkstationCronDraft,
) {
  if (
    sessionStartCron[key] !== latestCron[key] &&
    draftCron[key] !== latestCron[key]
  ) {
    fields.push(field);
  }
}

export function formatEditableOverwriteFieldLabels(
  overwriteFieldNames: EditableWorkstationOverwriteField[],
  messages: Pick<
    WorkstationDetailMessages,
    | "cronExpiryWindowFieldLabel"
    | "cronJitterFieldLabel"
    | "cronScheduleFieldLabel"
    | "cronTriggerAtStartFieldLabel"
    | "kindLabel"
    | "promptFieldLabel"
    | "runnerFieldLabel"
    | "workerFieldLabel"
  >,
) {
  return formatList(
    overwriteFieldNames.map((field) => fieldLabel(field, messages)),
  );
}

function fieldLabel(
  field: EditableWorkstationOverwriteField,
  messages: Pick<
    WorkstationDetailMessages,
    | "cronExpiryWindowFieldLabel"
    | "cronJitterFieldLabel"
    | "cronScheduleFieldLabel"
    | "cronTriggerAtStartFieldLabel"
    | "kindLabel"
    | "promptFieldLabel"
    | "runnerFieldLabel"
    | "workerFieldLabel"
  >,
) {
  switch (field) {
    case "behavior":
      return messages.kindLabel.toLowerCase();
    case "prompt":
      return messages.promptFieldLabel.toLowerCase();
    case "runner":
      return messages.runnerFieldLabel.toLowerCase();
    case "worker":
      return messages.workerFieldLabel.toLowerCase();
    case "cronSchedule":
      return messages.cronScheduleFieldLabel.toLowerCase();
    case "cronTriggerAtStart":
      return messages.cronTriggerAtStartFieldLabel.toLowerCase();
    case "cronJitter":
      return messages.cronJitterFieldLabel.toLowerCase();
    case "cronExpiryWindow":
      return messages.cronExpiryWindowFieldLabel.toLowerCase();
  }
}
