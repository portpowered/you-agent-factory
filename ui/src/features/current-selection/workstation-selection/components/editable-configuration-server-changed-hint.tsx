import { DashboardText } from "../../../../components/ui";
import type {
  EditableWorkstationOverwriteField,
  WorkstationDetailCardProps,
} from "../lib/detail-card-types";
import type { getWorkstationDetailMessages } from "../messages/workstation-detail";

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
    <DashboardText
      className="m-0 text-on-warning-container"
      variant="supporting"
    >
      {messages.editableConfigurationServerFieldChangedHint}
    </DashboardText>
  );
}
