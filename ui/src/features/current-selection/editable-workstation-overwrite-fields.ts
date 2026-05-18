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
  if (draft.model !== latestDefinitionDraft.model) {
    fields.push("model");
  }
  if (draft.promptFile !== latestDefinitionDraft.promptFile) {
    fields.push("template");
  }

  return fields;
}

export function formatEditableOverwriteFieldLabels(
  overwriteFieldNames: EditableWorkstationOverwriteField[],
  messages: Pick<
    WorkstationDetailMessages,
    "modelFieldLabel" | "promptFieldLabel" | "templateFieldLabel"
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
    "modelFieldLabel" | "promptFieldLabel" | "templateFieldLabel"
  >,
) {
  switch (field) {
    case "model":
      return messages.modelFieldLabel.toLowerCase();
    case "prompt":
      return messages.promptFieldLabel.toLowerCase();
    case "template":
      return messages.templateFieldLabel.toLowerCase();
  }
}
