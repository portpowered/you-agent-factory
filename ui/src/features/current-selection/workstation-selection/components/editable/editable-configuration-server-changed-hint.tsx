import { FormWarning } from "@you-agent-factory/components/forms";
import type {
  EditableWorkstationOverwriteField,
  WorkstationDetailCardProps,
} from "../../lib/keys/detail-card-types";
import type { getWorkstationDetailMessages } from "../../messages/workstation-detail";

export function EditableConfigurationServerChangedHint({
  fieldName,
  messages,
  state,
}: {
  fieldName: EditableWorkstationOverwriteField;
  messages: ReturnType<typeof getWorkstationDetailMessages>;
  state: Extract<
    NonNullable<WorkstationDetailCardProps["editableConfigurationState"]>,
    { status: "ready" }
  >;
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
