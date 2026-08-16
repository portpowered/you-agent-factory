import type { Dispatch, SetStateAction } from "react";
import { useCallback, useEffect, useState } from "react";

import type { AgentBentoLayoutItem } from "../../components/agent-bento";
import { getDashboardWidgetRemovalMessages } from "../../messages/dashboard-widget-removal";
import { restoreDashboardWidgetToLayout } from "../dashboardLayoutMutations";
import type { DashboardLayoutStorageWriteResult } from "../storage/dashboardLayoutStorage";

export const DASHBOARD_WIDGET_UNDO_TIMEOUT_MS = 8_000;

export type DashboardWidgetUndoStatus =
  | "available"
  | "dismissed"
  | "expired"
  | "failed-to-persist"
  | "restored";

export interface DashboardWidgetRemovalCandidate {
  originalIndex: number;
  removedItem: AgentBentoLayoutItem;
  triggerElement: HTMLElement | null;
  widgetTitle: string;
}

export interface DashboardWidgetUndoState
  extends DashboardWidgetRemovalCandidate {
  status: DashboardWidgetUndoStatus;
  storageWriteFailed: boolean;
}

export interface UseDashboardWidgetRemovalOptions {
  dashboardLayout: AgentBentoLayoutItem[];
  dirtyCardInstanceIDs: ReadonlySet<string>;
  getWidgetTitle: (widgetType: string) => string;
  persistDashboardLayout: (
    layout: AgentBentoLayoutItem[],
  ) => DashboardLayoutStorageWriteResult | undefined;
  removeDashboardWidget: (
    widgetInstanceID: string,
  ) => DashboardLayoutStorageWriteResult | undefined;
}

export interface UseDashboardWidgetRemovalResult {
  cancelRemoval: () => void;
  confirmRemoval: () => void;
  dismissUndo: () => void;
  handleDialogCloseAutoFocus: (event: Event) => void;
  handleDialogOpenChange: (open: boolean) => void;
  pendingRemoval: DashboardWidgetRemovalCandidate | null;
  requestRemoval: (widgetInstanceID: string) => void;
  undoRemoval: () => void;
  undoState: DashboardWidgetUndoState | null;
}

type FocusRequest =
  | { kind: "restored"; widgetInstanceID: string }
  | { kind: "survivor"; widgetInstanceIDs: string[] }
  | { element: HTMLElement | null; kind: "trigger" };

function useDashboardWidgetUndoExpiry(
  undoState: DashboardWidgetUndoState | null,
  setUndoState: Dispatch<SetStateAction<DashboardWidgetUndoState | null>>,
): void {
  const undoStatus = undoState?.status;
  const undoWidgetInstanceID = undoState?.removedItem.id;

  useEffect(() => {
    if (undoStatus !== "available") {
      return;
    }

    const timeoutID = setTimeout(() => {
      setUndoState((currentState) =>
        currentState?.status === "available" &&
        currentState.removedItem.id === undoWidgetInstanceID
          ? { ...currentState, status: "expired" }
          : currentState,
      );
    }, DASHBOARD_WIDGET_UNDO_TIMEOUT_MS);

    return () => clearTimeout(timeoutID);
  }, [setUndoState, undoStatus, undoWidgetInstanceID]);
}

function useDashboardWidgetFocus(
  focusRequest: FocusRequest | null,
  dashboardLayout: readonly AgentBentoLayoutItem[],
  setFocusRequest: Dispatch<SetStateAction<FocusRequest | null>>,
): void {
  useEffect(() => {
    if (!focusRequest || typeof document === "undefined") {
      return;
    }

    const timeoutID = setTimeout(() => {
      const target = resolveFocusTarget(focusRequest, dashboardLayout);
      target?.focus();
      setFocusRequest(null);
    }, 0);

    return () => clearTimeout(timeoutID);
  }, [dashboardLayout, focusRequest, setFocusRequest]);
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: removal state, persistence, and focus callbacks form one public controller boundary.
export function useDashboardWidgetRemoval({
  dashboardLayout,
  dirtyCardInstanceIDs,
  getWidgetTitle,
  persistDashboardLayout,
  removeDashboardWidget,
}: UseDashboardWidgetRemovalOptions): UseDashboardWidgetRemovalResult {
  const [pendingRemoval, setPendingRemoval] =
    useState<DashboardWidgetRemovalCandidate | null>(null);
  const [undoState, setUndoState] = useState<DashboardWidgetUndoState | null>(
    null,
  );
  const [focusRequest, setFocusRequest] = useState<FocusRequest | null>(null);

  const commitRemoval = useCallback(
    (candidate: DashboardWidgetRemovalCandidate) => {
      const writeResult = removeDashboardWidget(candidate.removedItem.id);
      setUndoState({
        ...candidate,
        status: "available",
        storageWriteFailed: writeResult?.persisted === false,
      });
      setFocusRequest({
        kind: "survivor",
        widgetInstanceIDs: getNearestSurvivingWidgetIDs(
          candidate.removedItem.id,
          candidate.removedItem,
          dashboardLayout,
        ),
      });
    },
    [dashboardLayout, removeDashboardWidget],
  );

  const requestRemoval = useCallback(
    (widgetInstanceID: string) => {
      const originalIndex = dashboardLayout.findIndex(
        (item) => item.id === widgetInstanceID,
      );
      const item = dashboardLayout[originalIndex];
      if (!item || item.widgetType === "add-widget") {
        return;
      }

      const candidate: DashboardWidgetRemovalCandidate = {
        originalIndex,
        removedItem: { ...item },
        triggerElement: getActiveHTMLElement(),
        widgetTitle: getWidgetTitle(item.widgetType),
      };

      if (dirtyCardInstanceIDs.has(widgetInstanceID)) {
        setPendingRemoval(candidate);
        return;
      }

      commitRemoval(candidate);
    },
    [commitRemoval, dashboardLayout, dirtyCardInstanceIDs, getWidgetTitle],
  );

  const cancelRemoval = useCallback(() => {
    if (!pendingRemoval) {
      return;
    }

    setFocusRequest({
      element: pendingRemoval.triggerElement,
      kind: "trigger",
    });
    setPendingRemoval(null);
  }, [pendingRemoval]);

  const confirmRemoval = useCallback(() => {
    if (!pendingRemoval) {
      return;
    }

    setPendingRemoval(null);
    commitRemoval(pendingRemoval);
  }, [commitRemoval, pendingRemoval]);

  const undoRemoval = useCallback(() => {
    if (undoState?.status !== "available") {
      return;
    }

    const restoredLayout = restoreDashboardWidgetToLayout(
      dashboardLayout,
      undoState.removedItem,
      undoState.originalIndex,
    );
    const writeResult = persistDashboardLayout(restoredLayout);
    setUndoState((currentState) =>
      currentState && currentState.status === "available"
        ? {
            ...currentState,
            status:
              writeResult?.persisted === false
                ? "failed-to-persist"
                : "restored",
          }
        : currentState,
    );
    setFocusRequest({
      kind: "restored",
      widgetInstanceID: undoState.removedItem.id,
    });
  }, [dashboardLayout, persistDashboardLayout, undoState]);

  const dismissUndo = useCallback(() => {
    setUndoState((currentState) =>
      currentState && currentState.status === "available"
        ? { ...currentState, status: "dismissed" }
        : currentState,
    );
  }, []);

  const handleDialogOpenChange = useCallback(
    (open: boolean) => (open ? undefined : cancelRemoval()),
    [cancelRemoval],
  );

  const handleDialogCloseAutoFocus = useCallback(
    (event: Event) => focusRequest && event.preventDefault(),
    [focusRequest],
  );

  useDashboardWidgetUndoExpiry(undoState, setUndoState);
  useDashboardWidgetFocus(focusRequest, dashboardLayout, setFocusRequest);

  return {
    cancelRemoval,
    confirmRemoval,
    dismissUndo,
    handleDialogCloseAutoFocus,
    handleDialogOpenChange,
    pendingRemoval,
    requestRemoval,
    undoRemoval,
    undoState,
  };
}

function getActiveHTMLElement(): HTMLElement | null {
  if (
    typeof document === "undefined" ||
    !(document.activeElement instanceof HTMLElement)
  ) {
    return null;
  }

  return document.activeElement;
}

function getNearestSurvivingWidgetIDs(
  removedWidgetInstanceID: string,
  removedItem: AgentBentoLayoutItem,
  layout: readonly AgentBentoLayoutItem[],
): string[] {
  return layout
    .filter(
      (item) =>
        item.id !== removedWidgetInstanceID &&
        item.widgetType !== "add-widget" &&
        item.widgetType !== "session-controls" &&
        !item.hidden,
    )
    .sort((left, right) => {
      const leftDistance = gridDistance(left, removedItem);
      const rightDistance = gridDistance(right, removedItem);
      return leftDistance - rightDistance;
    })
    .map((item) => item.id);
}

function gridDistance(
  left: Pick<AgentBentoLayoutItem, "x" | "y">,
  right: Pick<AgentBentoLayoutItem, "x" | "y">,
): number {
  return Math.abs(left.x - right.x) + Math.abs(left.y - right.y);
}

function resolveFocusTarget(
  focusRequest: FocusRequest,
  dashboardLayout: readonly AgentBentoLayoutItem[],
): HTMLElement | null {
  const visibleWidgetInstanceIDs = new Set(
    dashboardLayout
      .filter((item) => item.widgetType !== "add-widget" && !item.hidden)
      .map((item) => item.id),
  );
  if (focusRequest.kind === "trigger") {
    return focusRequest.element?.isConnected ? focusRequest.element : null;
  }

  const removeButtons = Array.from(
    document.querySelectorAll<HTMLElement>(
      "[data-dashboard-widget-remove='true']",
    ),
  );
  const findRemoveButton = (widgetInstanceID: string) =>
    visibleWidgetInstanceIDs.has(widgetInstanceID)
      ? (removeButtons.find(
          (button) =>
            button.dataset.dashboardWidgetInstanceId === widgetInstanceID,
        ) ?? null)
      : null;

  if (focusRequest.kind === "restored") {
    return (
      findRemoveButton(focusRequest.widgetInstanceID) ?? findAddWidgetControl()
    );
  }

  for (const widgetInstanceID of focusRequest.widgetInstanceIDs) {
    const button = findRemoveButton(widgetInstanceID);
    if (button) {
      return button;
    }
  }

  return findAddWidgetControl();
}

function findAddWidgetControl(): HTMLElement | null {
  return document.querySelector<HTMLElement>(
    "[data-dashboard-add-widget-control='true']",
  );
}

export function dashboardWidgetRemovalStatusMessage(
  undoState: DashboardWidgetUndoState,
  locale?: string | null,
): string {
  const messages = getDashboardWidgetRemovalMessages(locale);
  if (undoState.status === "failed-to-persist") {
    return messages.failedToPersist(undoState.widgetTitle);
  }
  if (undoState.status === "expired") {
    return messages.undoExpired(undoState.widgetTitle);
  }
  if (undoState.status === "dismissed") {
    return messages.undoDismissed;
  }
  if (undoState.status === "restored") {
    return messages.restored(undoState.widgetTitle);
  }
  if (undoState.storageWriteFailed) {
    return messages.removedWithStorageWarning(undoState.widgetTitle);
  }
  return messages.removed(undoState.widgetTitle);
}
