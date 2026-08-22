import type {
  PointerEvent as ReactPointerEvent,
  TouchEvent as ReactTouchEvent,
} from "react";
import { useCallback, useRef } from "react";
import type { Layout } from "react-grid-layout";
import { cloneLayout, getLayoutItem } from "react-grid-layout/core";
import {
  findTouchPoint,
  useDashboardLayoutTouchListeners,
} from "./dashboardLayoutTouchListeners";
import { applyDashboardTouchLayoutOperation } from "./dashboardLayoutTouchOperations";

export type DashboardTouchLayoutMode = "move" | "resize";
export type DashboardTouchResizeHandle = "e" | "s" | "se";

const TOUCH_MOVE_THRESHOLD_PX = 3;
const BENTO_CARD_SELECTOR = "[data-bento-instance-id]";
const BENTO_DRAG_HANDLE_SELECTOR = "[data-bento-drag-handle='true']";
const BENTO_RESIZE_HANDLE_SELECTOR = "[data-bento-touch-resize-handle]";
const BENTO_TOUCH_CANCEL_SELECTOR =
  "button,a,input,select,textarea,[contenteditable='true']";

interface DashboardLayoutTouchOptions {
  columns: number;
  enabled: boolean;
  layout: Layout;
  margin: readonly [number, number];
  onCommitLayout: (layout: Layout) => void;
  onPreviewLayout: (layout: Layout) => void;
  rowHeight: number;
  width: number;
}

interface DashboardLayoutTouchInteraction {
  handle?: DashboardTouchResizeHandle;
  itemID: string;
  mode: DashboardTouchLayoutMode;
}

export interface DashboardLayoutTouchSession {
  currentLayout: Layout;
  identifier: number;
  interaction: DashboardLayoutTouchInteraction;
  moved: boolean;
  origin: { x: number; y: number };
  source: "pointer" | "touch";
  startLayout: Layout;
}

export interface TouchPoint {
  clientX: number;
  clientY: number;
  identifier: number;
}

export interface TouchListLike {
  [index: number]: TouchPoint | undefined;
  item?: (index: number) => TouchPoint | null;
  length: number;
}

type BeginSession = (
  target: EventTarget | null,
  point: TouchPoint,
  source: "pointer" | "touch",
) => boolean;

export interface DashboardLayoutTouchHandlers {
  onPointerDownCapture: (event: ReactPointerEvent<HTMLElement>) => void;
  onTouchStartCapture: (event: ReactTouchEvent<HTMLElement>) => void;
}

export interface DashboardTouchLayoutOperationOptions {
  columns: number;
  deltaX: number;
  deltaY: number;
  handle?: DashboardTouchResizeHandle;
  itemID: string;
  layout: Layout;
  margin: readonly [number, number];
  mode: DashboardTouchLayoutMode;
  rowHeight: number;
  width: number;
}

export function useDashboardLayoutTouch(
  options: DashboardLayoutTouchOptions,
): DashboardLayoutTouchHandlers {
  const optionsRef = useRef(options);
  optionsRef.current = options;
  const sessionRef = useRef<DashboardLayoutTouchSession | null>(null);

  const cancelSession = useCallback(() => {
    const session = sessionRef.current;
    if (!session) {
      return false;
    }

    sessionRef.current = null;
    optionsRef.current.onPreviewLayout(cloneLayout(session.startLayout));
    return true;
  }, []);

  const completeSession = useCallback((cancelled: boolean) => {
    const session = sessionRef.current;
    if (!session) {
      return;
    }

    sessionRef.current = null;
    if (
      cancelled ||
      !session.moved ||
      hasSameLayoutGeometry(session.startLayout, session.currentLayout)
    ) {
      if (cancelled) {
        optionsRef.current.onPreviewLayout(cloneLayout(session.startLayout));
      }
      return;
    }

    optionsRef.current.onCommitLayout(cloneLayout(session.currentLayout));
  }, []);

  const updateSession = useCallback((point: TouchPoint) => {
    const session = sessionRef.current;
    if (!session) {
      return;
    }

    const deltaX = point.clientX - session.origin.x;
    const deltaY = point.clientY - session.origin.y;
    if (
      !session.moved &&
      Math.hypot(deltaX, deltaY) < TOUCH_MOVE_THRESHOLD_PX
    ) {
      return;
    }

    session.moved = true;
    const currentOptions = optionsRef.current;
    const nextLayout = applyDashboardTouchLayoutOperation({
      columns: currentOptions.columns,
      deltaX,
      deltaY,
      handle: session.interaction.handle,
      itemID: session.interaction.itemID,
      layout: session.startLayout,
      margin: currentOptions.margin,
      mode: session.interaction.mode,
      rowHeight: currentOptions.rowHeight,
      width: currentOptions.width,
    });

    if (hasSameLayoutGeometry(session.currentLayout, nextLayout)) {
      return;
    }

    session.currentLayout = nextLayout;
    currentOptions.onPreviewLayout(cloneLayout(nextLayout));
  }, []);
  useDashboardLayoutTouchListeners({
    completeSession,
    sessionRef,
    updateSession,
  });

  const beginSession = useCallback(
    (
      target: EventTarget | null,
      point: TouchPoint,
      source: "pointer" | "touch",
    ) => {
      const currentOptions = optionsRef.current;
      if (!currentOptions.enabled || sessionRef.current) {
        return false;
      }

      const interaction = getTouchInteraction(target);
      if (
        !interaction ||
        !getLayoutItem(currentOptions.layout, interaction.itemID)
      ) {
        return false;
      }

      const initialLayout = cloneLayout(currentOptions.layout);
      sessionRef.current = {
        currentLayout: cloneLayout(initialLayout),
        identifier: point.identifier,
        interaction,
        moved: false,
        origin: { x: point.clientX, y: point.clientY },
        source,
        startLayout: initialLayout,
      };
      return true;
    },
    [],
  );

  const onTouchStartCapture = useCallback(
    (event: ReactTouchEvent<HTMLElement>) =>
      handleTouchStartCapture(event, beginSession, cancelSession),
    [beginSession, cancelSession],
  );

  const onPointerDownCapture = useCallback(
    (event: ReactPointerEvent<HTMLElement>) =>
      handlePointerDownCapture(event, beginSession),
    [beginSession],
  );

  return { onPointerDownCapture, onTouchStartCapture };
}

function getTouchInteraction(
  target: EventTarget | null,
): DashboardLayoutTouchInteraction | undefined {
  if (!(target instanceof Element)) {
    return undefined;
  }

  const card = target.closest<HTMLElement>(BENTO_CARD_SELECTOR);
  if (!card) {
    return undefined;
  }

  const resizeHandle = target.closest<HTMLElement>(
    BENTO_RESIZE_HANDLE_SELECTOR,
  );
  if (resizeHandle && card.contains(resizeHandle)) {
    const handle = resizeHandle.getAttribute("data-bento-touch-resize-handle");
    if (handle === "e" || handle === "s" || handle === "se") {
      return {
        handle,
        itemID: card.dataset.bentoInstanceId ?? "",
        mode: "resize",
      };
    }
  }

  if (target.closest(BENTO_TOUCH_CANCEL_SELECTOR)) {
    return undefined;
  }

  const moveHandle = target.closest(BENTO_DRAG_HANDLE_SELECTOR);
  if (!moveHandle || !card.contains(moveHandle)) {
    return undefined;
  }

  return {
    itemID: card.dataset.bentoInstanceId ?? "",
    mode: "move",
  };
}

function handleTouchStartCapture(
  event: ReactTouchEvent<HTMLElement>,
  beginSession: BeginSession,
  cancelSession: () => boolean,
) {
  if (event.touches.length !== 1) {
    if (event.touches.length > 1) {
      cancelSession();
    }
    return;
  }

  const point = findTouchPoint(event.nativeEvent);
  if (!point || !beginSession(event.target, point, "touch")) {
    return;
  }

  event.preventDefault();
  event.stopPropagation();
}

function handlePointerDownCapture(
  event: ReactPointerEvent<HTMLElement>,
  beginSession: BeginSession,
) {
  if (event.pointerType !== "touch") {
    return;
  }

  if (
    !beginSession(
      event.target,
      {
        clientX: event.clientX,
        clientY: event.clientY,
        identifier: event.pointerId,
      },
      "pointer",
    )
  ) {
    return;
  }

  event.preventDefault();
  event.stopPropagation();
}

function hasSameLayoutGeometry(left: Layout, right: Layout): boolean {
  if (left.length !== right.length) {
    return false;
  }

  return left.every((leftItem, index) => {
    const rightItem = right[index];
    return (
      rightItem?.i === leftItem.i &&
      rightItem.x === leftItem.x &&
      rightItem.y === leftItem.y &&
      rightItem.w === leftItem.w &&
      rightItem.h === leftItem.h
    );
  });
}
