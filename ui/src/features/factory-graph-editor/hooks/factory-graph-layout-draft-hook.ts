import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import type { CurrentFactoryDocument } from "../lib/factory-graph-draft-types";
import {
  type FactoryLayout,
  type FactoryLayoutPoint,
  factoryLayoutFromDefinition,
  hasFactoryLayoutChanges,
  moveFactoryLayoutNode,
  moveFactoryLayoutNodesByDelta,
} from "../lib/factory-graph-layout-operations";

export interface FactoryGraphLayoutDraftDerivedState {
  adoptSavedLayout: (layout: FactoryLayout) => void;
  baseLayout: FactoryLayout;
  hasChanges: boolean;
  layout: FactoryLayout;
  moveNode: (nodeId: string, position: FactoryLayoutPoint) => void;
  moveNodesByDelta: (
    nodeIds: readonly string[],
    delta: FactoryLayoutPoint,
    resolvedPositionsByNodeId: ReadonlyMap<string, FactoryLayoutPoint>,
  ) => void;
  replaceLayout: (layout: FactoryLayout) => void;
  resetLayout: () => void;
}

interface FactoryGraphLayoutSessionState {
  baseLayout: FactoryLayout;
  layout: FactoryLayout;
}

interface UseFactoryGraphLayoutDraftStateOptions {
  currentFactoryDocument?: CurrentFactoryDocument;
  factoryDocumentScopeKey?: string | null;
}

export function useFactoryGraphLayoutDraftState(
  options: UseFactoryGraphLayoutDraftStateOptions,
): FactoryGraphLayoutDraftDerivedState {
  const currentFactoryDocument = options.currentFactoryDocument;
  const factoryDocumentScopeKey = options.factoryDocumentScopeKey ?? null;
  const lastFactoryDocumentScopeKeyRef = useRef<string | null>(null);
  const [sessionState, setSessionState] =
    useState<FactoryGraphLayoutSessionState | null>(null);
  const documentBaseLayout = useMemo(
    () => factoryLayoutFromDefinition(currentFactoryDocument),
    [currentFactoryDocument],
  );

  useEffect(() => {
    const previousScopeKey = lastFactoryDocumentScopeKeyRef.current;
    const scopeChanged =
      previousScopeKey !== null && previousScopeKey !== factoryDocumentScopeKey;
    lastFactoryDocumentScopeKeyRef.current = factoryDocumentScopeKey;

    if (scopeChanged || !currentFactoryDocument) {
      setSessionState(
        currentFactoryDocument
          ? createLayoutSessionState(documentBaseLayout)
          : null,
      );
      return;
    }

    setSessionState((currentState) => {
      if (!currentState) {
        return createLayoutSessionState(documentBaseLayout);
      }

      if (
        hasFactoryLayoutChanges(currentState.baseLayout, currentState.layout)
      ) {
        return currentState;
      }

      return createLayoutSessionState(documentBaseLayout);
    });
  }, [currentFactoryDocument, documentBaseLayout, factoryDocumentScopeKey]);

  const baseLayout = sessionState?.baseLayout ?? documentBaseLayout;
  const layout = sessionState?.layout ?? documentBaseLayout;
  const replaceLayout = useCallback((nextLayout: FactoryLayout) => {
    setSessionState((currentState) => ({
      baseLayout: currentState?.baseLayout ?? createDefaultLayoutState(),
      layout: structuredClone(nextLayout),
    }));
  }, []);
  const resetLayout = useCallback(() => {
    setSessionState(createLayoutSessionState(documentBaseLayout));
  }, [documentBaseLayout]);
  const adoptSavedLayout = useCallback((savedLayout: FactoryLayout) => {
    setSessionState(createLayoutSessionState(savedLayout));
  }, []);
  const moveNode = useCallback((nodeId: string, position: FactoryLayoutPoint) => {
    setSessionState((currentState) => {
      const currentLayout = currentState?.layout ?? documentBaseLayout;
      const nextLayout = moveFactoryLayoutNode(
        currentLayout,
        nodeId,
        position,
      );

      return {
        baseLayout: currentState?.baseLayout ?? documentBaseLayout,
        layout: nextLayout,
      };
    });
  }, [documentBaseLayout]);
  const moveNodesByDelta = useCallback(
    (
      nodeIds: readonly string[],
      delta: FactoryLayoutPoint,
      resolvedPositionsByNodeId: ReadonlyMap<string, FactoryLayoutPoint>,
    ) => {
      setSessionState((currentState) => {
        const currentLayout = currentState?.layout ?? documentBaseLayout;
        const nextLayout = moveFactoryLayoutNodesByDelta(
          currentLayout,
          nodeIds,
          delta,
          resolvedPositionsByNodeId,
        );

        return {
          baseLayout: currentState?.baseLayout ?? documentBaseLayout,
          layout: nextLayout,
        };
      });
    },
    [documentBaseLayout],
  );

  return {
    adoptSavedLayout,
    baseLayout,
    hasChanges: hasFactoryLayoutChanges(baseLayout, layout),
    layout,
    moveNode,
    moveNodesByDelta,
    replaceLayout,
    resetLayout,
  };
}

function createLayoutSessionState(
  layout: FactoryLayout,
): FactoryGraphLayoutSessionState {
  const clonedLayout = structuredClone(layout);
  return {
    baseLayout: clonedLayout,
    layout: structuredClone(clonedLayout),
  };
}

function createDefaultLayoutState(): FactoryLayout {
  return factoryLayoutFromDefinition(null);
}
