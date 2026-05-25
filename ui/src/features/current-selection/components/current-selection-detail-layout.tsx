import type { ReactNode } from "react";
import { createContext, useContext } from "react";

import { DETAIL_CARD_WIDE_CLASS } from "../../../components/dashboard/widget-board";
import {
  DashboardActionButton,
  DashboardActionRow,
  DashboardWidgetFrame,
} from "../../../components/ui";
import { useCurrentSelectionShellMessages } from "./current-selection-locale";
import type { SelectionDetailLayoutProps } from "./detail-card-types";
import { useSelectionHistoryStore } from "../state/selectionHistoryStore";

const SELECTION_HISTORY_ACTIONS_CLASS = "justify-end";
const SELECTION_HISTORY_ACTIONS_GROUP_CLASS = "justify-end";
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
        <DashboardActionRow
          actions={
            <>
              {sharedHeaderAction}
              {headerAction}
              <DashboardActionButton
                aria-label={messages.undoAccessibleLabel}
                disabled={!canUndo}
                onClick={() => undoSelection()}
                type="button"
              >
                {messages.undoLabel}
              </DashboardActionButton>
              <DashboardActionButton
                aria-label={messages.redoAccessibleLabel}
                disabled={!canRedo}
                onClick={() => redoSelection()}
                type="button"
              >
                {messages.redoLabel}
              </DashboardActionButton>
            </>
          }
          actionsClassName={SELECTION_HISTORY_ACTIONS_GROUP_CLASS}
          className={SELECTION_HISTORY_ACTIONS_CLASS}
        />
      }
      title={messages.title}
      widgetId={widgetId}
    >
      {children}
    </DashboardWidgetFrame>
  );
}
