import { formatList } from "../../components/ui/formatters";
import type { EditableWorkstationDraft } from "../current-factory-definition/workstation-editable-values";
import type { EditableWorkstationOverwriteField } from "./detail-card-types";
import type { WorkstationDetailMessages } from "./messages";

export function resolveEditableWorkstationOverwriteFields(
  draft: EditableWorkstationDraft,
  latestDefinitionDraft: EditableWorkstationDraft,
): EditableWorkstationOverwriteField[] {
  const fields: EditableWorkstationOverwriteField[] = [];

  if (draft.prompt !== latestDefinitionDraft.prompt) {
    fields.push("prompt");
  }
  if (draft.workerName !== latestDefinitionDraft.workerName) {
    fields.push("worker");
  }
  if (draft.runnerName !== latestDefinitionDraft.runnerName) {
    fields.push("runner");
  }

  return fields;
}

export function formatEditableOverwriteFieldLabels(
  overwriteFieldNames: EditableWorkstationOverwriteField[],
  messages: Pick<
    WorkstationDetailMessages,
    "promptFieldLabel" | "runnerFieldLabel" | "workerFieldLabel"
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
    "promptFieldLabel" | "runnerFieldLabel" | "workerFieldLabel"
  >,
) {
  switch (field) {
    case "prompt":
      return messages.promptFieldLabel.toLowerCase();
    case "runner":
      return messages.runnerFieldLabel.toLowerCase();
    case "worker":
      return messages.workerFieldLabel.toLowerCase();
  }
}
