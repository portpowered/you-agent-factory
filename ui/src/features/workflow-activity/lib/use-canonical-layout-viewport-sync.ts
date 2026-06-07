import type { FitViewOptions, ReactFlowInstance } from "@xyflow/react";
import { useEffect, useRef, type MutableRefObject } from "react";

import type { FactoryLayoutViewport } from "../../factory-graph-editor/lib/factory-graph-layout-operations";

export function canonicalLayoutViewportKey(
  viewport: FactoryLayoutViewport | null | undefined,
): string {
  if (!viewport) {
    return "none";
  }

  return `${viewport.x}:${viewport.y}:${viewport.zoom}`;
}

export function useCanonicalLayoutViewportSync({
  canonicalLayoutViewport,
  fitViewOptions,
  flowInstanceRef,
  viewportResetKey,
}: {
  canonicalLayoutViewport?: FactoryLayoutViewport | null;
  fitViewOptions: FitViewOptions;
  flowInstanceRef?: MutableRefObject<ReactFlowInstance | null>;
  viewportResetKey: string;
}) {
  const lastAppliedKeyRef = useRef<string | null>(null);

  useEffect(() => {
    const instance = flowInstanceRef?.current;
    if (!instance) {
      return;
    }

    const nextKey = `${viewportResetKey}:${canonicalLayoutViewportKey(canonicalLayoutViewport)}`;
    if (lastAppliedKeyRef.current === nextKey) {
      return;
    }
    lastAppliedKeyRef.current = nextKey;

    if (canonicalLayoutViewport) {
      void instance.setViewport(canonicalLayoutViewport, { duration: 0 });
      return;
    }

    void instance.fitView(fitViewOptions);
  }, [
    canonicalLayoutViewport,
    fitViewOptions,
    flowInstanceRef,
    viewportResetKey,
  ]);
}
