import type { EditableDocDraft } from "../../../current-factory-definition/lib/doc-editable-values";
import type { EditableDocOverwriteField } from "../lib/detail-card-types";

export function resolveEditableDocOverwriteFields(
  sessionStartDraft: EditableDocDraft,
  currentDraft: EditableDocDraft,
  latestDefinitionDraft: EditableDocDraft,
): EditableDocOverwriteField[] {
  const fields: EditableDocOverwriteField[] = [];

  if (
    sessionStartDraft.fileName !== currentDraft.fileName &&
    latestDefinitionDraft.fileName !== currentDraft.fileName
  ) {
    fields.push("fileName");
  }

  if (
    sessionStartDraft.inlineContent !== currentDraft.inlineContent &&
    latestDefinitionDraft.inlineContent !== currentDraft.inlineContent
  ) {
    fields.push("inlineContent");
  }

  return fields;
}

export function formatEditableDocOverwriteFieldLabels(
  fields: EditableDocOverwriteField[],
  messages: {
    editableConfigurationFileNameFieldLabel: string;
    editableConfigurationInlineContentFieldLabel: string;
  },
): string {
  return fields
    .map((field) =>
      field === "fileName"
        ? messages.editableConfigurationFileNameFieldLabel
        : messages.editableConfigurationInlineContentFieldLabel,
    )
    .join(", ");
}
