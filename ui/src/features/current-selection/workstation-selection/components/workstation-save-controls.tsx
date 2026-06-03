import { Save } from "lucide-react";
import { DashboardActionButton } from "../../../../components/ui";
import { cn } from "../../../../lib/cn";
import { EditableConfigurationDiscardHeaderAction } from "../../base/public";
import type { EditableWorkstationSaveState } from "../lib/detail-card-types";
import { getWorkstationDetailMessages } from "../messages/workstation-detail";

export function EditableWorkstationConfigurationHeaderActions({
  canDiscard,
  canSave,
  locale,
  onDiscard,
  onSave,
  saveState,
}: {
  canDiscard: boolean;
  canSave: boolean;
  locale?: string;
  onDiscard: () => void;
  onSave: () => void;
  saveState: EditableWorkstationSaveState;
}) {
  const isSaving = saveState.status === "submitting";

  return (
    <>
      <EditableConfigurationDiscardHeaderAction
        canDiscard={canDiscard}
        isSaving={isSaving}
        locale={locale}
        onClick={onDiscard}
      />
      <EditableWorkstationSaveHeaderAction
        canSave={canSave}
        locale={locale}
        onClick={onSave}
        saveState={saveState}
      />
    </>
  );
}

export function EditableWorkstationSaveHeaderAction({
  canSave,
  locale,
  onClick,
  saveState,
}: {
  canSave: boolean;
  locale?: string;
  onClick: () => void;
  saveState: EditableWorkstationSaveState;
}) {
  const messages = getWorkstationDetailMessages(locale);
  const emphasizeSave = canSave && saveState.status !== "submitting";

  return (
    <DashboardActionButton
      aria-label={
        saveState.status === "submitting"
          ? messages.editableConfigurationSaveBusyAction
          : messages.editableConfigurationSaveAction
      }
      aria-expanded={
        saveState.status === "confirming" || saveState.status === "submitting"
      }
      aria-haspopup="dialog"
      className={
        emphasizeSave
          ? cn(
              "border-af-warning-border bg-warning-container text-on-warning-container",
              "hover:border-af-warning hover:bg-warning hover:text-on-warning",
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
