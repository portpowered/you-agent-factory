import { DASHBOARD_SUPPORTING_TEXT_CLASS } from "../../../../components/ui/dashboard-typography";
import { cn } from "../../../../lib/cn";
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
