import type { ReactFlowInstance, XYPosition } from "@xyflow/react";
import {
  type MutableRefObject,
  type PointerEvent,
  useCallback,
  useRef,
} from "react";

import { isTouchPanePointerDown } from "../../lib/selection/factory-graph-editor-react-flow-interaction";

export function useFactoryGraphTouchPanePan(
  flowInstanceRef: MutableRefObject<ReactFlowInstance | null>,
) {
  const sessionRef = useRef<{
    pointerId: number;
    startClient: XYPosition;
    startViewport: { x: number; y: number; zoom: number };
  } | null>(null);

  const onPointerDownCapture = useCallback(
    (event: PointerEvent<HTMLDivElement>) => {
      if (!isTouchPanePointerDown(event)) {
        return;
      }
      if (!event.isPrimary) {
        sessionRef.current = null;
        return;
      }

      const instance = flowInstanceRef.current;
      if (!instance) {
        return;
      }
      sessionRef.current = {
        pointerId: event.pointerId,
        startClient: { x: event.clientX, y: event.clientY },
        startViewport: instance.getViewport(),
      };
      event.currentTarget.setPointerCapture?.(event.pointerId);
      event.preventDefault();
      event.stopPropagation();
    },
    [flowInstanceRef],
  );
  const onPointerMoveCapture = useCallback(
    (event: PointerEvent<HTMLDivElement>) => {
      const session = sessionRef.current;
      if (!session || session.pointerId !== event.pointerId) {
        return;
      }

      event.preventDefault();
      event.stopPropagation();
      void flowInstanceRef.current?.setViewport({
        x: session.startViewport.x + event.clientX - session.startClient.x,
        y: session.startViewport.y + event.clientY - session.startClient.y,
        zoom: session.startViewport.zoom,
      });
    },
    [flowInstanceRef],
  );
  const onPointerEndCapture = useCallback(
    (event: PointerEvent<HTMLDivElement>) => {
      if (sessionRef.current?.pointerId !== event.pointerId) {
        return;
      }

      sessionRef.current = null;
      if (event.currentTarget.hasPointerCapture?.(event.pointerId)) {
        event.currentTarget.releasePointerCapture(event.pointerId);
      }
    },
    [],
  );

  return {
    onPointerCancelCapture: onPointerEndCapture,
    onPointerDownCapture,
    onPointerMoveCapture,
    onPointerUpCapture: onPointerEndCapture,
  };
}
