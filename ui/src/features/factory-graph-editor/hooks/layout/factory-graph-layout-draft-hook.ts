import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import type { CurrentFactoryDocument } from "../../lib/draft/factory-graph-draft-types";
import {
  addFactoryLayoutEdgeWaypoint,
  factoryLayoutEdgeWaypoints,
  moveFactoryLayoutEdgeWaypoint,
} from "../../lib/layout/factory-graph-layout-edge-waypoints";
import {
  createMoveFactoryLayoutNodeCommand,
  createMoveFactoryLayoutNodesCommand,
  createResetFactoryLayoutCommand,
  createUpdateFactoryLayoutEdgeWaypointsCommand,
  createUpdateFactoryLayoutViewportCommand,
  type FactoryLayoutCommand,
} from "../../lib/layout/history/factory-graph-layout-commands";
import {
  canRedoFactoryLayoutHistory,
  canUndoFactoryLayoutHistory,
  clearFactoryLayoutHistoryState,
  type FactoryLayoutHistoryState,
  pruneFactoryLayoutHistoryForNodeIds,
  pushFactoryLayoutHistoryCommand,
  redoFactoryLayoutHistory,
  undoFactoryLayoutHistory,
} from "../../lib/layout/history/factory-graph-layout-history";
import {
  type FactoryLayout,
  type FactoryLayoutPoint,
  type FactoryLayoutViewport,
  factoryLayoutFromDefinition,
  factoryLayoutNodePosition,
  hasFactoryLayoutChanges,
  moveFactoryLayoutNode,
  moveFactoryLayoutNodesByDelta,
  updateFactoryLayoutViewport,
} from "../../lib/layout/factory-graph-layout-operations";

export interface FactoryGraphLayoutDraftDerivedState {
  adoptSavedLayout: (layout: FactoryLayout) => void;
  baseLayout: FactoryLayout;
  canRedoLayout: boolean;
  canUndoLayout: boolean;
  hasChanges: boolean;
  layout: FactoryLayout;
  layoutDirty: boolean;
  addEdgeWaypoint: (
    edgeId: string,
    position: FactoryLayoutPoint,
    insertIndex?: number,
  ) => void;
  moveEdgeWaypoint: (
    edgeId: string,
    waypointIndex: number,
    position: FactoryLayoutPoint,
  ) => void;
  moveNode: (nodeId: string, position: FactoryLayoutPoint) => void;
  moveNodesByDelta: (
    nodeIds: readonly string[],
    delta: FactoryLayoutPoint,
    resolvedPositionsByNodeId: ReadonlyMap<string, FactoryLayoutPoint>,
  ) => void;
  pruneLayoutHistoryForNodeIds: (nodeIds: readonly string[]) => void;
  redoLayout: () => void;
  replaceLayout: (layout: FactoryLayout) => void;
  resetLayout: (options?: { recordHistory?: boolean }) => void;
  undoLayout: () => void;
  updateViewport: (viewport: FactoryLayoutViewport) => void;
}

interface FactoryGraphLayoutSessionState {
  baseLayout: FactoryLayout;
  layout: FactoryLayout;
}

interface LayoutDraftStoreState {
  history: FactoryLayoutHistoryState;
  sessionState: FactoryGraphLayoutSessionState | null;
}

interface UseFactoryGraphLayoutDraftStateOptions {
  currentFactoryDocument?: CurrentFactoryDocument;
  factoryDocumentScopeKey?: string | null;
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: coordinates layout draft session state with typed undo history.
export function useFactoryGraphLayoutDraftState(
  options: UseFactoryGraphLayoutDraftStateOptions,
): FactoryGraphLayoutDraftDerivedState {
  const currentFactoryDocument = options.currentFactoryDocument;
  const factoryDocumentScopeKey = options.factoryDocumentScopeKey ?? null;
  const lastFactoryDocumentScopeKeyRef = useRef<string | null>(null);
  const isApplyingHistoryRef = useRef(false);
  const [store, setStore] = useState<LayoutDraftStoreState>(() => ({
    history: clearFactoryLayoutHistoryState(),
    sessionState: null,
  }));
  const documentBaseLayout = useMemo(
    () => factoryLayoutFromDefinition(currentFactoryDocument),
    [currentFactoryDocument],
  );

  const commitLayoutUpdate = useCallback(
    (
      updater: (input: {
        currentLayout: FactoryLayout;
        currentState: FactoryGraphLayoutSessionState | null;
      }) => {
        command: FactoryLayoutCommand | null;
        layout: FactoryLayout;
      },
    ) => {
      setStore((currentStore) => {
        const currentState = currentStore.sessionState;
        const currentLayout = currentState?.layout ?? documentBaseLayout;
        const { command, layout } = updater({
          currentLayout,
          currentState,
        });
        const history =
          command && !isApplyingHistoryRef.current
            ? pushFactoryLayoutHistoryCommand(currentStore.history, command)
            : currentStore.history;

        return {
          history,
          sessionState: {
            baseLayout: currentState?.baseLayout ?? documentBaseLayout,
            layout,
          },
        };
      });
    },
    [documentBaseLayout],
  );

  useEffect(() => {
    const previousScopeKey = lastFactoryDocumentScopeKeyRef.current;
    const scopeChanged =
      previousScopeKey !== null && previousScopeKey !== factoryDocumentScopeKey;
    lastFactoryDocumentScopeKeyRef.current = factoryDocumentScopeKey;

    if (scopeChanged || !currentFactoryDocument) {
      setStore({
        history: clearFactoryLayoutHistoryState(),
        sessionState: currentFactoryDocument
          ? createLayoutSessionState(documentBaseLayout)
          : null,
      });
      return;
    }

    setStore((currentStore) => {
      if (!currentStore.sessionState) {
        return {
          history: clearFactoryLayoutHistoryState(),
          sessionState: createLayoutSessionState(documentBaseLayout),
        };
      }

      if (
        hasFactoryLayoutChanges(
          currentStore.sessionState.baseLayout,
          currentStore.sessionState.layout,
        )
      ) {
        return currentStore;
      }

      return {
        history: clearFactoryLayoutHistoryState(),
        sessionState: createLayoutSessionState(documentBaseLayout),
      };
    });
  }, [currentFactoryDocument, documentBaseLayout, factoryDocumentScopeKey]);

  const baseLayout = store.sessionState?.baseLayout ?? documentBaseLayout;
  const layout = store.sessionState?.layout ?? documentBaseLayout;
  const replaceLayout = useCallback((nextLayout: FactoryLayout) => {
    setStore((currentStore) => ({
      history: clearFactoryLayoutHistoryState(),
      sessionState: {
        baseLayout: currentStore.sessionState?.baseLayout ?? createDefaultLayoutState(),
        layout: structuredClone(nextLayout),
      },
    }));
  }, []);
  const resetLayout = useCallback(
    (options?: { recordHistory?: boolean }) => {
      const nextLayout = structuredClone(documentBaseLayout);
      if (options?.recordHistory === false) {
        setStore((currentStore) => ({
          history: clearFactoryLayoutHistoryState(),
          sessionState: {
            baseLayout: currentStore.sessionState?.baseLayout ?? documentBaseLayout,
            layout: nextLayout,
          },
        }));
        return;
      }

      commitLayoutUpdate(({ currentLayout }) => ({
        command: createResetFactoryLayoutCommand({
          fromLayout: currentLayout,
          toLayout: nextLayout,
        }),
        layout: nextLayout,
      }));
    },
    [commitLayoutUpdate, documentBaseLayout],
  );
  const adoptSavedLayout = useCallback((savedLayout: FactoryLayout) => {
    setStore({
      history: clearFactoryLayoutHistoryState(),
      sessionState: createLayoutSessionState(savedLayout),
    });
  }, []);
  const commitEdgeWaypointUpdate = useCallback(
    (
      edgeId: string,
      updater: (layout: FactoryLayout) => FactoryLayout,
    ) => {
      commitLayoutUpdate(({ currentLayout }) => {
        const nextLayout = updater(currentLayout);
        return {
          command: createUpdateFactoryLayoutEdgeWaypointsCommand({
            edgeId,
            layout: currentLayout,
            to: factoryLayoutEdgeWaypoints(nextLayout, edgeId) ?? null,
          }),
          layout: nextLayout,
        };
      });
    },
    [commitLayoutUpdate],
  );
  const addEdgeWaypoint = useCallback(
    (
      edgeId: string,
      position: FactoryLayoutPoint,
      insertIndex?: number,
    ) => {
      commitEdgeWaypointUpdate(edgeId, (currentLayout) =>
        addFactoryLayoutEdgeWaypoint(
          currentLayout,
          edgeId,
          position,
          insertIndex,
        ),
      );
    },
    [commitEdgeWaypointUpdate],
  );
  const moveEdgeWaypoint = useCallback(
    (
      edgeId: string,
      waypointIndex: number,
      position: FactoryLayoutPoint,
    ) => {
      commitEdgeWaypointUpdate(edgeId, (currentLayout) =>
        moveFactoryLayoutEdgeWaypoint(
          currentLayout,
          edgeId,
          waypointIndex,
          position,
        ),
      );
    },
    [commitEdgeWaypointUpdate],
  );
  const moveNode = useCallback(
    (nodeId: string, position: FactoryLayoutPoint) => {
      commitLayoutUpdate(({ currentLayout }) => ({
        command: createMoveFactoryLayoutNodeCommand({
          layout: currentLayout,
          nodeId,
          to: position,
        }),
        layout: moveFactoryLayoutNode(currentLayout, nodeId, position),
      }));
    },
    [commitLayoutUpdate],
  );
  const updateViewport = useCallback(
    (viewport: FactoryLayoutViewport) => {
      commitLayoutUpdate(({ currentLayout }) => ({
        command: createUpdateFactoryLayoutViewportCommand({
          layout: currentLayout,
          to: viewport,
        }),
        layout: updateFactoryLayoutViewport(currentLayout, viewport),
      }));
    },
    [commitLayoutUpdate],
  );
  const moveNodesByDelta = useCallback(
    (
      nodeIds: readonly string[],
      delta: FactoryLayoutPoint,
      resolvedPositionsByNodeId: ReadonlyMap<string, FactoryLayoutPoint>,
    ) => {
      commitLayoutUpdate(({ currentLayout }) => {
        const moves = nodeIds
          .map((nodeId) => {
            const fromPosition =
              factoryLayoutNodePosition(currentLayout, nodeId) ??
              resolvedPositionsByNodeId.get(nodeId);
            if (!fromPosition) {
              return null;
            }

            return {
              nodeId,
              to: {
                x: fromPosition.x + delta.x,
                y: fromPosition.y + delta.y,
              },
            };
          })
          .filter((move): move is NonNullable<typeof move> => move !== null);

        return {
          command: createMoveFactoryLayoutNodesCommand({
            layout: currentLayout,
            moves,
          }),
          layout: moveFactoryLayoutNodesByDelta(
            currentLayout,
            nodeIds,
            delta,
            resolvedPositionsByNodeId,
          ),
        };
      });
    },
    [commitLayoutUpdate],
  );
  const undoLayout = useCallback(() => {
    isApplyingHistoryRef.current = true;
    try {
      setStore((currentStore) => {
        const currentLayout =
          currentStore.sessionState?.layout ?? documentBaseLayout;
        const result = undoFactoryLayoutHistory(
          currentStore.history,
          currentLayout,
        );
        return {
          history: result.history,
          sessionState: {
            baseLayout:
              currentStore.sessionState?.baseLayout ?? documentBaseLayout,
            layout: result.layout,
          },
        };
      });
    } finally {
      isApplyingHistoryRef.current = false;
    }
  }, [documentBaseLayout]);
  const redoLayout = useCallback(() => {
    isApplyingHistoryRef.current = true;
    try {
      setStore((currentStore) => {
        const currentLayout =
          currentStore.sessionState?.layout ?? documentBaseLayout;
        const result = redoFactoryLayoutHistory(
          currentStore.history,
          currentLayout,
        );
        return {
          history: result.history,
          sessionState: {
            baseLayout:
              currentStore.sessionState?.baseLayout ?? documentBaseLayout,
            layout: result.layout,
          },
        };
      });
    } finally {
      isApplyingHistoryRef.current = false;
    }
  }, [documentBaseLayout]);
  const pruneLayoutHistoryForNodeIds = useCallback((nodeIds: readonly string[]) => {
    setStore((currentStore) => ({
      ...currentStore,
      history: pruneFactoryLayoutHistoryForNodeIds(
        currentStore.history,
        new Set(nodeIds),
      ),
    }));
  }, []);

  const layoutDirty = hasFactoryLayoutChanges(baseLayout, layout);

  return {
    adoptSavedLayout,
    baseLayout,
    canRedoLayout: canRedoFactoryLayoutHistory(store.history),
    canUndoLayout: canUndoFactoryLayoutHistory(store.history),
    hasChanges: layoutDirty,
    addEdgeWaypoint,
    layout,
    layoutDirty,
    moveEdgeWaypoint,
    moveNode,
    moveNodesByDelta,
    pruneLayoutHistoryForNodeIds,
    redoLayout,
    replaceLayout,
    resetLayout,
    undoLayout,
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
