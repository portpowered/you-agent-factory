import type { ReactNode } from "react";
import { createContext, useContext } from "react";

import { DETAIL_CARD_WIDE_CLASS } from "../../../components/dashboard/widget-board";
import { DashboardWidgetFrame } from "../../../components/ui";
import { DASHBOARD_SUPPORTING_TEXT_CLASS } from "../../../components/ui/dashboard-typography";
import { cn } from "../../../lib/cn";
import { useCurrentSelectionShellMessages } from "./current-selection-locale";
import type { SelectionDetailLayoutProps } from "./detail-card-types";
import { useSelectionHistoryStore } from "../state/selectionHistoryStore";

const SELECTION_HISTORY_ACTIONS_CLASS = "flex items-center gap-2";
const SELECTION_HISTORY_BUTTON_CLASS = cn(
  "inline-flex h-9 items-center justify-center rounded-lg border border-af-border bg-af-surface-subtle px-3 text-af-text-muted transition hover:border-af-border-strong hover:bg-af-overlay hover:text-af-text focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-af-focus-ring disabled:cursor-not-allowed disabled:border-af-border disabled:bg-af-surface-subtle disabled:text-af-text-disabled",
  DASHBOARD_SUPPORTING_TEXT_CLASS,
);
const CurrentSelectionHeaderActionContext = createContext<ReactNode>(null);

export function CurrentSelectionHeaderActionProvider({
  children,
  headerAction,
}: {
  children: ReactNode;
  headerAction: ReactNode;
}) {
  return (
    <CurrentSelectionHeaderActionContext.Provider value={headerAction}>
      {children}
    </CurrentSelectionHeaderActionContext.Provider>
  );
}

export function SelectionDetailLayout({
  widgetId = "current-selection",
  children,
  headerAction,
}: SelectionDetailLayoutProps) {
  const messages = useCurrentSelectionShellMessages();
  const sharedHeaderAction = useContext(CurrentSelectionHeaderActionContext);
  const canRedo = useSelectionHistoryStore((state) => state.future.length > 0);
  const canUndo = useSelectionHistoryStore((state) => state.past.length > 0);
  const redoSelection = useSelectionHistoryStore((state) => state.redo);
  const undoSelection = useSelectionHistoryStore((state) => state.undo);

  return (
    <DashboardWidgetFrame
      className={DETAIL_CARD_WIDE_CLASS}
      headerAction={
        <div className={SELECTION_HISTORY_ACTIONS_CLASS}>
          {sharedHeaderAction}
          {headerAction}
          <button
            aria-label={messages.undoAccessibleLabel}
            className={SELECTION_HISTORY_BUTTON_CLASS}
            disabled={!canUndo}
            onClick={() => undoSelection()}
            type="button"
          >
            {messages.undoLabel}
          </button>
          <button
            aria-label={messages.redoAccessibleLabel}
            className={SELECTION_HISTORY_BUTTON_CLASS}
            disabled={!canRedo}
            onClick={() => redoSelection()}
            type="button"
          >
            {messages.redoLabel}
          </button>
        </div>
      }
      title={messages.title}
      widgetId={widgetId}
    >
      {children}
    </DashboardWidgetFrame>
  );
}
