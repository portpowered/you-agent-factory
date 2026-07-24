import type { ReactNode } from "react";

import { ActionRow, DashboardActionButton } from "../../../../../components/ui";

export interface EditableConfigurationSaveRowProps {
  busyLabel: string;
  canSave: boolean;
  isSaving: boolean;
  onSave: () => void;
  resetSlot?: ReactNode;
  saveLabel: string;
}

export function EditableConfigurationSaveRow({
  busyLabel,
  canSave,
  isSaving,
  onSave,
  resetSlot,
  saveLabel,
}: EditableConfigurationSaveRowProps) {
  const emphasizeSave = canSave && !isSaving;
  const label = isSaving ? busyLabel : saveLabel;

  return (
    <ActionRow
      actions={
        <>
          {resetSlot}
          <DashboardActionButton
            disabled={!canSave}
            executing={isSaving}
            onClick={onSave}
            tone={emphasizeSave ? "warning" : "outline"}
            type="button"
          >
            {label}
          </DashboardActionButton>
        </>
      }
    />
  );
}
