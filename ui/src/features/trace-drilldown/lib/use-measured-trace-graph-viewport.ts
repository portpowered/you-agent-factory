import { useLayoutEffect, useRef, useState } from "react";

export function useMeasuredTraceGraphViewport() {
  const graphViewportRef = useRef<HTMLDivElement | null>(null);
  const [graphViewportReady, setGraphViewportReady] = useState(false);

  useLayoutEffect(() => {
    const graphViewport = graphViewportRef.current;
    if (!graphViewport) {
      return;
    }

    const updateReadyState = (width: number, height: number) => {
      setGraphViewportReady(width > 0 && height > 0);
    };
    const measureGraphViewport = () => {
      const rect = graphViewport.getBoundingClientRect();
      updateReadyState(rect.width, rect.height);
    };

    measureGraphViewport();

    if (typeof ResizeObserver === "undefined") {
      const animationFrameID = requestAnimationFrame(measureGraphViewport);
      return () => {
        cancelAnimationFrame(animationFrameID);
      };
    }

    const resizeObserver = new ResizeObserver((entries) => {
      const entry = entries[0];
      if (!entry) {
        measureGraphViewport();
        return;
      }

      updateReadyState(entry.contentRect.width, entry.contentRect.height);
    });

    resizeObserver.observe(graphViewport);

    return () => {
      resizeObserver.disconnect();
    };
  }, []);

  return { graphViewportReady, graphViewportRef };
}
