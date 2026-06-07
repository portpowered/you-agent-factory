import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import type { CurrentFactoryDocument } from "../lib/factory-graph-draft-types";
import {
  type FactoryLayout,
  type FactoryLayoutPoint,
  type FactoryLayoutViewport,
  factoryLayoutFromDefinition,
  hasFactoryLayoutChanges,
  moveFactoryLayoutNode,
  moveFactoryLayoutNodesByDelta,
  updateFactoryLayoutViewport,
} from "../lib/factory-graph-layout-operations";

export interface FactoryGraphLayoutDraftDerivedState {
  adoptSavedLayout: (layout: FactoryLayout) => void;
  baseLayout: FactoryLayout;
  hasChanges: boolean;
  layout: FactoryLayout;
  layoutDirty: boolean;
  moveNode: (nodeId: string, position: FactoryLayoutPoint) => void;
  moveNodesByDelta: (
    nodeIds: readonly string[],
    delta: FactoryLayoutPoint,
    resolvedPositionsByNodeId: ReadonlyMap<string, FactoryLayoutPoint>,
  ) => void;
  replaceLayout: (layout: FactoryLayout) => void;
  resetLayout: () => void;
  updateViewport: (viewport: FactoryLayoutViewport) => void;
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
  const updateViewport = useCallback((viewport: FactoryLayoutViewport) => {
    setSessionState((currentState) => {
      const currentLayout = currentState?.layout ?? documentBaseLayout;
      const nextLayout = updateFactoryLayoutViewport(currentLayout, viewport);

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

  const layoutDirty = hasFactoryLayoutChanges(baseLayout, layout);

  return {
    adoptSavedLayout,
    baseLayout,
    hasChanges: layoutDirty,
    layout,
    layoutDirty,
    moveNode,
    moveNodesByDelta,
    replaceLayout,
    resetLayout,
    updateViewport,
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
