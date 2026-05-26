import type { ReactNode } from "react";

import {
  DashboardActionButton,
  DashboardActionRow,
} from "../../../components/ui";

const CURRENT_SELECTION_HEADER_ACTIONS_CLASS = "w-full justify-end";
const CURRENT_SELECTION_HEADER_ACTIONS_GROUP_CLASS = "w-full justify-end";
const CURRENT_SELECTION_ICON_CLASS = "size-4";

export function CurrentSelectionHeaderActions({
  canRedo,
  canUndo,
  headerActions,
  onRedo,
  onUndo,
  redoLabel,
  undoLabel,
}: {
  canRedo: boolean;
  canUndo: boolean;
  headerActions?: ReactNode;
  onRedo: () => void;
  onUndo: () => void;
  redoLabel: string;
  undoLabel: string;
}) {
  return (
    <DashboardActionRow
      actions={
        <>
          <DashboardActionButton
            aria-label={undoLabel}
            disabled={!canUndo}
            iconOnly
            onClick={onUndo}
            type="button"
          >
            <UndoIcon />
          </DashboardActionButton>
          <DashboardActionButton
            aria-label={redoLabel}
            disabled={!canRedo}
            iconOnly
            onClick={onRedo}
            type="button"
          >
            <RedoIcon />
          </DashboardActionButton>
          {headerActions}
        </>
      }
      actionsClassName={CURRENT_SELECTION_HEADER_ACTIONS_GROUP_CLASS}
      className={CURRENT_SELECTION_HEADER_ACTIONS_CLASS}
    />
  );
}

function UndoIcon() {
  return (
    <svg
      aria-hidden="true"
      className={CURRENT_SELECTION_ICON_CLASS}
      fill="none"
      viewBox="0 0 16 16"
    >
      <path
        d="M6.5 3 2.5 7l4 4"
        stroke="currentColor"
        strokeLinecap="round"
        strokeLinejoin="round"
        strokeWidth="1.5"
      />
      <path
        d="M3 7h6.25a3.75 3.75 0 1 1 0 7.5H7.5"
        stroke="currentColor"
        strokeLinecap="round"
        strokeLinejoin="round"
        strokeWidth="1.5"
      />
    </svg>
  );
}

function RedoIcon() {
  return (
    <svg
      aria-hidden="true"
      className={CURRENT_SELECTION_ICON_CLASS}
      fill="none"
      viewBox="0 0 16 16"
    >
      <path
        d="m9.5 3 4 4-4 4"
        stroke="currentColor"
        strokeLinecap="round"
        strokeLinejoin="round"
        strokeWidth="1.5"
      />
      <path
        d="M13 7H6.75a3.75 3.75 0 1 0 0 7.5H8.5"
        stroke="currentColor"
        strokeLinecap="round"
        strokeLinejoin="round"
        strokeWidth="1.5"
      />
    </svg>
  );
}
