import { useEffect } from "react";

import type {
  DashboardLayoutTouchSession,
  TouchListLike,
  TouchPoint,
} from "./dashboardLayoutTouch";

type SessionRef = { current: DashboardLayoutTouchSession | null };
type CompleteSession = (cancelled: boolean) => void;
type UpdateSession = (point: TouchPoint) => void;

export function useDashboardLayoutTouchListeners({
  completeSession,
  sessionRef,
  updateSession,
}: {
  completeSession: CompleteSession;
  sessionRef: SessionRef;
  updateSession: UpdateSession;
}) {
  useEffect(() => {
    const touchMove = createTouchMoveHandler(sessionRef, updateSession);
    const touchEnd = createTouchEndHandler(
      sessionRef,
      updateSession,
      completeSession,
    );
    const touchCancel = createTouchCancelHandler(sessionRef, completeSession);
    const pointerMove = createPointerMoveHandler(sessionRef, updateSession);
    const pointerUp = createPointerUpHandler(
      sessionRef,
      updateSession,
      completeSession,
    );
    const pointerCancel = createPointerCancelHandler(
      sessionRef,
      completeSession,
    );

    document.addEventListener("touchmove", touchMove, { passive: false });
    document.addEventListener("touchend", touchEnd, { passive: false });
    document.addEventListener("touchcancel", touchCancel, { passive: false });
    document.addEventListener("pointermove", pointerMove, { passive: false });
    document.addEventListener("pointerup", pointerUp, { passive: false });
    document.addEventListener("pointercancel", pointerCancel, {
      passive: false,
    });

    return () => {
      document.removeEventListener("touchmove", touchMove);
      document.removeEventListener("touchend", touchEnd);
      document.removeEventListener("touchcancel", touchCancel);
      document.removeEventListener("pointermove", pointerMove);
      document.removeEventListener("pointerup", pointerUp);
      document.removeEventListener("pointercancel", pointerCancel);
      sessionRef.current = null;
    };
  }, [completeSession, sessionRef, updateSession]);
}

export function findTouchPoint(
  event: Pick<globalThis.TouchEvent, "changedTouches" | "touches">,
  identifier?: number,
): TouchPoint | undefined {
  return (
    findTouchInList(event.touches, identifier) ??
    findTouchInList(event.changedTouches, identifier)
  );
}

function createTouchMoveHandler(
  sessionRef: SessionRef,
  updateSession: UpdateSession,
) {
  return (event: globalThis.TouchEvent) => {
    const session = sessionRef.current;
    if (session?.source !== "touch") {
      return;
    }

    const point = findTouchPoint(event, session.identifier);
    if (!point) {
      return;
    }

    event.preventDefault();
    event.stopPropagation();
    updateSession(point);
  };
}

function createTouchEndHandler(
  sessionRef: SessionRef,
  updateSession: UpdateSession,
  completeSession: CompleteSession,
) {
  return (event: globalThis.TouchEvent) => {
    const session = sessionRef.current;
    if (session?.source !== "touch") {
      return;
    }

    const point = findTouchPoint(event, session.identifier);
    if (point) {
      updateSession(point);
    }
    event.preventDefault();
    event.stopPropagation();
    completeSession(false);
  };
}

function createTouchCancelHandler(
  sessionRef: SessionRef,
  completeSession: CompleteSession,
) {
  return (event: globalThis.TouchEvent) => {
    if (sessionRef.current?.source !== "touch") {
      return;
    }

    event.preventDefault();
    event.stopPropagation();
    completeSession(true);
  };
}

function createPointerMoveHandler(
  sessionRef: SessionRef,
  updateSession: UpdateSession,
) {
  return (event: globalThis.PointerEvent) => {
    const session = sessionRef.current;
    if (
      session?.source !== "pointer" ||
      event.pointerId !== session.identifier
    ) {
      return;
    }

    event.preventDefault();
    event.stopPropagation();
    updateSession({
      clientX: event.clientX,
      clientY: event.clientY,
      identifier: event.pointerId,
    });
  };
}

function createPointerUpHandler(
  sessionRef: SessionRef,
  updateSession: UpdateSession,
  completeSession: CompleteSession,
) {
  return (event: globalThis.PointerEvent) => {
    const session = sessionRef.current;
    if (
      session?.source !== "pointer" ||
      event.pointerId !== session.identifier
    ) {
      return;
    }

    event.preventDefault();
    event.stopPropagation();
    updateSession({
      clientX: event.clientX,
      clientY: event.clientY,
      identifier: event.pointerId,
    });
    completeSession(false);
  };
}

function createPointerCancelHandler(
  sessionRef: SessionRef,
  completeSession: CompleteSession,
) {
  return (event: globalThis.PointerEvent) => {
    const session = sessionRef.current;
    if (
      session?.source !== "pointer" ||
      event.pointerId !== session.identifier
    ) {
      return;
    }

    event.preventDefault();
    event.stopPropagation();
    completeSession(true);
  };
}

function findTouchInList(
  list: TouchList | undefined,
  identifier?: number,
): TouchPoint | undefined {
  if (!list) {
    return undefined;
  }

  const touchList = list as unknown as TouchListLike;
  for (let index = 0; index < touchList.length; index += 1) {
    const touch = touchList.item?.(index) ?? touchList[index];
    if (
      touch &&
      (identifier === undefined || touch.identifier === identifier)
    ) {
      return touch;
    }
  }

  return undefined;
}
