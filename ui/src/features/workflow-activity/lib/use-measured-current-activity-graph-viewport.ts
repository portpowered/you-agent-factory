import type { MutableRefObject } from "react";
import { useLayoutEffect, useState } from "react";

export interface MeasuredCurrentActivityGraphViewport {
  height: number;
  ready: boolean;
  width: number;
}

function isJsdomRuntime() {
  return (
    typeof navigator !== "undefined" &&
    navigator.userAgent.toLowerCase().includes("jsdom")
  );
}

export function useMeasuredCurrentActivityGraphViewport(
  graphViewportRef?: MutableRefObject<HTMLElement | null>,
) {
  const [graphViewport, setGraphViewport] =
    useState<MeasuredCurrentActivityGraphViewport>(() => ({
      height: 0,
      ready: typeof window === "undefined" || isJsdomRuntime(),
      width: 0,
    }));

  useLayoutEffect(() => {
    const graphViewport = graphViewportRef?.current;
    if (!graphViewport) {
      setGraphViewport({ height: 0, ready: false, width: 0 });
      return;
    }

    if (isJsdomRuntime()) {
      const rect = graphViewport.getBoundingClientRect();
      setGraphViewport({
        height: rect.height,
        ready: true,
        width: rect.width,
      });
      return;
    }

    let animationFrameID = 0;
    let disposed = false;

    const updateReadyState = (width: number, height: number) => {
      const ready = width > 0 && height > 0;
      setGraphViewport((current) => {
        if (
          current.ready === ready &&
          current.width === width &&
          current.height === height
        ) {
          return current;
        }

        return {
          height,
          ready,
          width,
        };
      });
      if (ready && animationFrameID !== 0) {
        cancelAnimationFrame(animationFrameID);
        animationFrameID = 0;
      }
      return ready;
    };
    const measureGraphViewport = () => {
      const rect = graphViewport.getBoundingClientRect();
      return updateReadyState(rect.width, rect.height);
    };
    const scheduleAnimationFrameMeasure = () => {
      if (animationFrameID !== 0 || disposed) {
        return;
      }

      animationFrameID = requestAnimationFrame(() => {
        animationFrameID = 0;
        if (disposed) {
          return;
        }
        if (!measureGraphViewport()) {
          scheduleAnimationFrameMeasure();
        }
      });
    };

    if (!measureGraphViewport()) {
      scheduleAnimationFrameMeasure();
    }

    if (typeof ResizeObserver === "undefined") {
      return () => {
        disposed = true;
        if (animationFrameID !== 0) {
          cancelAnimationFrame(animationFrameID);
        }
      };
    }

    const resizeObserver = new ResizeObserver((entries) => {
      const entry = entries[0];
      if (!entry) {
        if (!measureGraphViewport()) {
          scheduleAnimationFrameMeasure();
        }
        return;
      }

      updateReadyState(entry.contentRect.width, entry.contentRect.height);
    });

    resizeObserver.observe(graphViewport);

    return () => {
      disposed = true;
      if (animationFrameID !== 0) {
        cancelAnimationFrame(animationFrameID);
      }
      resizeObserver.disconnect();
    };
  }, [graphViewportRef]);

  return graphViewport;
}
