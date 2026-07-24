import type { FitViewOptions, ReactFlowInstance } from "@xyflow/react";
import {
  type MutableRefObject,
  type RefObject,
  useEffect,
  useRef,
} from "react";

import type { FactoryLayoutViewport } from "../../../factory-graph-editor/lib/layout/factory-graph-layout-operations";

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
  skipNextViewportMoveEndRef,
  viewportResetKey,
}: {
  canonicalLayoutViewport?: FactoryLayoutViewport | null;
  fitViewOptions: FitViewOptions;
  flowInstanceRef?: MutableRefObject<ReactFlowInstance | null>;
  skipNextViewportMoveEndRef?: RefObject<boolean>;
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

    if (skipNextViewportMoveEndRef) {
      skipNextViewportMoveEndRef.current = true;
    }

    if (canonicalLayoutViewport) {
      void instance.setViewport(canonicalLayoutViewport, { duration: 0 });
      return;
    }

    void instance.fitView(fitViewOptions);
  }, [
    canonicalLayoutViewport,
    fitViewOptions,
    flowInstanceRef,
    skipNextViewportMoveEndRef,
    viewportResetKey,
  ]);
}
