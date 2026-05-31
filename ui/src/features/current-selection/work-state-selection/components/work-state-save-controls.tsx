import { Save } from "lucide-react";

import { DashboardActionButton } from "../../../../components/ui";
import { cn } from "../../../../lib/cn";
import type { EditableWorkStateSaveState } from "../lib/detail-card-types";
import { getWorkStateDetailMessages } from "../messages/work-state-detail";

export function EditableWorkStateSaveHeaderAction({
  canSave,
  locale,
  onClick,
  saveState,
}: {
  canSave: boolean;
  locale?: string;
  onClick: () => void;
  saveState: EditableWorkStateSaveState;
}) {
  const messages = getWorkStateDetailMessages(locale);
  const emphasizeSave = canSave && saveState.status !== "submitting";

  return (
    <DashboardActionButton
      aria-label={
        saveState.status === "submitting"
          ? messages.editableConfigurationSaveBusyAction
          : messages.editableConfigurationSaveAction
      }
      className={
        emphasizeSave
          ? cn(
              "border-af-warning-border bg-af-warning-surface text-af-warning-text",
              "hover:border-af-warning-border hover:bg-af-warning-surface hover:text-af-warning-text",
            )
          : undefined
      }
      disabled={!canSave}
      executing={saveState.status === "submitting"}
      iconOnly
      onClick={onClick}
      type="button"
    >
      <Save aria-hidden="true" className="size-4" />
    </DashboardActionButton>
  );
}
