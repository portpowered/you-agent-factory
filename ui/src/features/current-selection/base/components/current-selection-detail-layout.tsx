import type { ReactNode } from "react";
import { createContext, useContext } from "react";

import { DETAIL_CARD_WIDE_CLASS } from "../../../../components/ui/widget-frame";
import { DashboardWidgetFrame } from "../../../../components/ui";
import { useSelectionHistoryStore } from "../state/selectionHistoryStore";
import { CurrentSelectionHeaderActions } from "./current-selection-header-actions";
import { useCurrentSelectionShellMessages } from "./current-selection-locale";
import type { SelectionDetailLayoutProps } from "./detail-card-types";

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
        <CurrentSelectionHeaderActions
          canRedo={canRedo}
          canUndo={canUndo}
          headerActions={
            <>
              {headerAction}
              {sharedHeaderAction}
            </>
          }
          onRedo={() => redoSelection()}
          onUndo={() => undoSelection()}
          redoLabel={messages.redoAccessibleLabel}
          undoLabel={messages.undoAccessibleLabel}
        />
      }
      title={messages.title}
      widgetId={widgetId}
    >
      {children}
    </DashboardWidgetFrame>
  );
}
