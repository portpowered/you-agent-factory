import type { ReactNode } from "react";
import { createContext, useContext } from "react";
import { DashboardWidgetFrame } from "../../../../bento/components/dashboard-widget-frame/dashboard-widget-frame";
import { useSelectionHistoryStore } from "../../state/selectionHistoryStore";
import type { SelectionDetailLayoutProps } from "../detail-card/detail-card-types";
import { useCurrentSelectionShellMessages } from "../presentation/current-selection-locale";
import { CurrentSelectionHeaderActions } from "./current-selection-header-actions";

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
      wide
      widgetId={widgetId}
    >
      {children}
    </DashboardWidgetFrame>
  );
}
