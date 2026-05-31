import type { ReactNode } from "react";

import { DashboardActionButton, DashboardActionRow } from "../../../../components/ui";
import { cn } from "../../../../lib/cn";

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
    <DashboardActionRow
      actions={
        <>
          {resetSlot}
          <DashboardActionButton
            className={
              emphasizeSave
                ? cn(
                    "border-af-warning-border bg-af-warning-surface text-af-warning-text",
                    "hover:border-af-warning-border hover:bg-af-warning-surface hover:text-af-warning-text",
                  )
                : undefined
            }
            disabled={!canSave}
            executing={isSaving}
            onClick={onSave}
            type="button"
          >
            {label}
          </DashboardActionButton>
        </>
      }
    />
  );
}
