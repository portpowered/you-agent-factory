// biome-ignore lint/style/noExcessiveLinesPerFile: coordinates layout draft session state with typed undo history and visual groups.
import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import type { CurrentFactoryDocument } from "../../lib/draft/factory-graph-draft-types";
import {
  addFactoryLayoutEdgeWaypoint,
  factoryLayoutEdgeWaypoints,
  moveFactoryLayoutEdgeWaypoint,
  removeFactoryLayoutEdgeWaypoint,
} from "../../lib/layout/factory-graph-layout-edge-waypoints";
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
import {
  createCreateFactoryLayoutGroupCommand,
  createDeleteFactoryLayoutGroupCommand,
  createMoveFactoryLayoutNodeCommand,
  createMoveFactoryLayoutNodesCommand,
  createMoveFactoryLayoutVisualGroupCommand,
  createResetFactoryLayoutCommand,
  createUpdateFactoryLayoutEdgeWaypointsCommand,
  createUpdateFactoryLayoutGroupCommand,
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
  addFactoryLayoutGroup,
  addNodeToFactoryLayoutGroup,
  createFactoryLayoutGroup,
  createFactoryLayoutGroupId,
  defaultFactoryLayoutGroupBounds,
  type FactoryLayoutGroup,
  type FactoryLayoutGroupColorToken,
  moveFactoryLayoutGroupByDelta,
  removeFactoryLayoutGroup,
  removeNodeFromFactoryLayoutGroup,
  resizeFactoryLayoutGroup,
  updateFactoryLayoutGroup,
} from "../../lib/layout/visual-groups/factory-graph-layout-groups";

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
  removeEdgeWaypoint: (edgeId: string, waypointIndex: number) => void;
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
  createVisualGroup: (center: FactoryLayoutPoint) => FactoryLayoutGroup | null;
  renameVisualGroup: (groupId: string, label: string) => void;
  setVisualGroupColor: (
    groupId: string,
    color: FactoryLayoutGroupColorToken,
  ) => void;
  addNodeToVisualGroup: (groupId: string, nodeId: string) => void;
  removeNodeFromVisualGroup: (groupId: string, nodeId: string) => void;
  moveVisualGroupByDelta: (
    groupId: string,
    delta: FactoryLayoutPoint,
    resolvedNodePositions?: ReadonlyMap<string, FactoryLayoutPoint>,
  ) => void;
  resizeVisualGroup: (
    groupId: string,
    bounds: FactoryLayoutGroup["bounds"],
  ) => void;
  deleteVisualGroup: (groupId: string) => void;
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
        baseLayout:
          currentStore.sessionState?.baseLayout ?? createDefaultLayoutState(),
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
            baseLayout:
              currentStore.sessionState?.baseLayout ?? documentBaseLayout,
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
    (edgeId: string, updater: (layout: FactoryLayout) => FactoryLayout) => {
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
    (edgeId: string, position: FactoryLayoutPoint, insertIndex?: number) => {
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
    (edgeId: string, waypointIndex: number, position: FactoryLayoutPoint) => {
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
  const removeEdgeWaypoint = useCallback(
    (edgeId: string, waypointIndex: number) => {
      commitEdgeWaypointUpdate(edgeId, (currentLayout) =>
        removeFactoryLayoutEdgeWaypoint(currentLayout, edgeId, waypointIndex),
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
  const pruneLayoutHistoryForNodeIds = useCallback(
    (nodeIds: readonly string[]) => {
      setStore((currentStore) => ({
        ...currentStore,
        history: pruneFactoryLayoutHistoryForNodeIds(
          currentStore.history,
          new Set(nodeIds),
        ),
      }));
    },
    [],
  );
  const createVisualGroup = useCallback(
    (center: FactoryLayoutPoint): FactoryLayoutGroup | null => {
      let createdGroup: FactoryLayoutGroup | null = null;
      commitLayoutUpdate(({ currentLayout }) => {
        const groupId = createFactoryLayoutGroupId(currentLayout);
        const group = createFactoryLayoutGroup({
          bounds: defaultFactoryLayoutGroupBounds(center),
          id: groupId,
          layout: currentLayout,
        });
        createdGroup = group;
        const nextLayout = addFactoryLayoutGroup(currentLayout, group);
        return {
          command: createCreateFactoryLayoutGroupCommand({ group }),
          layout: nextLayout,
        };
      });
      return createdGroup;
    },
    [commitLayoutUpdate],
  );
  const renameVisualGroup = useCallback(
    (groupId: string, label: string) => {
      commitLayoutUpdate(({ currentLayout }) => {
        const nextLayout = updateFactoryLayoutGroup(
          currentLayout,
          groupId,
          (group) => ({
            ...group,
            label,
          }),
        );
        const updatedGroup = nextLayout.groups?.find(
          (group) => group.id === groupId,
        );
        return {
          command:
            updatedGroup === undefined
              ? null
              : createUpdateFactoryLayoutGroupCommand({
                  groupId,
                  layout: currentLayout,
                  to: updatedGroup,
                }),
          layout: nextLayout,
        };
      });
    },
    [commitLayoutUpdate],
  );
  const setVisualGroupColor = useCallback(
    (groupId: string, color: FactoryLayoutGroupColorToken) => {
      commitLayoutUpdate(({ currentLayout }) => {
        const nextLayout = updateFactoryLayoutGroup(
          currentLayout,
          groupId,
          (group) => ({
            ...group,
            color,
          }),
        );
        const updatedGroup = nextLayout.groups?.find(
          (group) => group.id === groupId,
        );
        return {
          command:
            updatedGroup === undefined
              ? null
              : createUpdateFactoryLayoutGroupCommand({
                  groupId,
                  layout: currentLayout,
                  to: updatedGroup,
                }),
          layout: nextLayout,
        };
      });
    },
    [commitLayoutUpdate],
  );
  const addNodeToVisualGroup = useCallback(
    (groupId: string, nodeId: string) => {
      commitLayoutUpdate(({ currentLayout }) => {
        const nextLayout = addNodeToFactoryLayoutGroup(
          currentLayout,
          groupId,
          nodeId,
        );
        const updatedGroup = nextLayout.groups?.find(
          (group) => group.id === groupId,
        );
        return {
          command:
            updatedGroup === undefined
              ? null
              : createUpdateFactoryLayoutGroupCommand({
                  groupId,
                  layout: currentLayout,
                  to: updatedGroup,
                }),
          layout: nextLayout,
        };
      });
    },
    [commitLayoutUpdate],
  );
  const removeNodeFromVisualGroup = useCallback(
    (groupId: string, nodeId: string) => {
      commitLayoutUpdate(({ currentLayout }) => {
        const nextLayout = removeNodeFromFactoryLayoutGroup(
          currentLayout,
          groupId,
          nodeId,
        );
        const updatedGroup = nextLayout.groups?.find(
          (group) => group.id === groupId,
        );
        return {
          command:
            updatedGroup === undefined
              ? null
              : createUpdateFactoryLayoutGroupCommand({
                  groupId,
                  layout: currentLayout,
                  to: updatedGroup,
                }),
          layout: nextLayout,
        };
      });
    },
    [commitLayoutUpdate],
  );
  const moveVisualGroupByDelta = useCallback(
    (
      groupId: string,
      delta: FactoryLayoutPoint,
      resolvedNodePositions: ReadonlyMap<
        string,
        FactoryLayoutPoint
      > = new Map(),
    ) => {
      commitLayoutUpdate(({ currentLayout }) => {
        const nextLayout = moveFactoryLayoutGroupByDelta(
          currentLayout,
          groupId,
          delta,
          resolvedNodePositions,
        );
        return {
          command: createMoveFactoryLayoutVisualGroupCommand({
            delta,
            groupId,
            layout: currentLayout,
            resolvedNodePositions,
          }),
          layout: nextLayout,
        };
      });
    },
    [commitLayoutUpdate],
  );
  const resizeVisualGroup = useCallback(
    (groupId: string, bounds: FactoryLayoutGroup["bounds"]) => {
      commitLayoutUpdate(({ currentLayout }) => {
        const nextLayout = resizeFactoryLayoutGroup(
          currentLayout,
          groupId,
          bounds,
        );
        const updatedGroup = nextLayout.groups?.find(
          (group) => group.id === groupId,
        );
        return {
          command:
            updatedGroup === undefined
              ? null
              : createUpdateFactoryLayoutGroupCommand({
                  groupId,
                  layout: currentLayout,
                  to: updatedGroup,
                }),
          layout: nextLayout,
        };
      });
    },
    [commitLayoutUpdate],
  );
  const deleteVisualGroup = useCallback(
    (groupId: string) => {
      commitLayoutUpdate(({ currentLayout }) => ({
        command: createDeleteFactoryLayoutGroupCommand({
          groupId,
          layout: currentLayout,
        }),
        layout: removeFactoryLayoutGroup(currentLayout, groupId),
      }));
    },
    [commitLayoutUpdate],
  );

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
    removeEdgeWaypoint,
    moveNode,
    moveNodesByDelta,
    pruneLayoutHistoryForNodeIds,
    redoLayout,
    replaceLayout,
    resetLayout,
    undoLayout,
    updateViewport,
    createVisualGroup,
    renameVisualGroup,
    setVisualGroupColor,
    addNodeToVisualGroup,
    removeNodeFromVisualGroup,
    moveVisualGroupByDelta,
    resizeVisualGroup,
    deleteVisualGroup,
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
