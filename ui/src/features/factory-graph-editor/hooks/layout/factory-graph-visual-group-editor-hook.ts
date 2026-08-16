import { useCallback, useMemo, useState } from "react";

import type { FactoryGraphEditorTool } from "../../components/controls/factory-graph-editor-controls";
import type {
  FactoryLayout,
  FactoryLayoutPoint,
} from "../../lib/layout/factory-graph-layout-operations";
import { isValidFactoryLayoutGroupBounds } from "../../lib/layout/factory-graph-layout-validation";
import {
  type FactoryLayoutGroupCanvasNodeOption,
  type FactoryLayoutGroupColorToken,
  type FactoryLayoutGroupNodeGeometry,
  factoryLayoutGroupById,
  factoryLayoutGroupContainsNode,
  factoryLayoutGroups,
} from "../../lib/layout/visual-groups/factory-graph-layout-groups";
import { getFactoryGraphEditorMessages } from "../../messages/editor";

function canEditFactoryGraphVisualGroups(input: {
  activeTool: FactoryGraphEditorTool;
  canInteractWithEditor: boolean;
  editorMode: boolean;
}): boolean {
  return (
    input.editorMode &&
    input.canInteractWithEditor &&
    input.activeTool !== "delete" &&
    input.activeTool !== "add"
  );
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: visual group editor wiring stays colocated with selection state.
export function useFactoryGraphVisualGroupEditor(input: {
  activeTool: FactoryGraphEditorTool;
  addNodeToVisualGroup: (groupId: string, nodeId: string) => void;
  canInteractWithEditor: boolean;
  canvasNodeOptions: readonly FactoryLayoutGroupCanvasNodeOption[];
  createVisualGroup: (
    center: FactoryLayoutPoint,
    options?: {
      nodeGeometryById?: ReadonlyMap<string, FactoryLayoutGroupNodeGeometry>;
      nodeIds?: readonly string[];
    },
  ) => { id: string } | null;
  deleteVisualGroup: (groupId: string) => void;
  editorMode: boolean;
  fitVisualGroup?: (
    groupId: string,
    nodeGeometryById: ReadonlyMap<string, FactoryLayoutGroupNodeGeometry>,
  ) => void;
  layout: FactoryLayout;
  locale?: string | null;
  moveVisualGroupByDelta: (
    groupId: string,
    delta: FactoryLayoutPoint,
    resolvedNodePositions?: ReadonlyMap<string, FactoryLayoutPoint>,
  ) => void;
  removeNodeFromVisualGroup: (groupId: string, nodeId: string) => void;
  renameVisualGroup: (groupId: string, label: string) => void;
  resizeVisualGroup: (
    groupId: string,
    bounds: NonNullable<FactoryLayout["groups"]>[number]["bounds"],
  ) => void;
  setVisualGroupColor: (
    groupId: string,
    color: FactoryLayoutGroupColorToken,
  ) => void;
  nodeGeometryById?: ReadonlyMap<string, FactoryLayoutGroupNodeGeometry>;
  selectedNodeIds?: readonly string[];
  resolveViewportCenter: () => FactoryLayoutPoint | null;
}) {
  const messages = getFactoryGraphEditorMessages(input.locale);
  const [selectedGroupId, setSelectedGroupId] = useState<string | null>(null);
  const canEditVisualGroups = canEditFactoryGraphVisualGroups(input);
  const groups = useMemo(
    () => factoryLayoutGroups(input.layout),
    [input.layout],
  );
  const selectedGroup = useMemo(
    () =>
      selectedGroupId
        ? factoryLayoutGroupById(input.layout, selectedGroupId)
        : undefined,
    [input.layout, selectedGroupId],
  );

  const handleCreateVisualGroup = useCallback(() => {
    if (!canEditVisualGroups) {
      return null;
    }

    const center = input.resolveViewportCenter();
    if (!center) {
      return null;
    }
    const selectedNodeIds = input.selectedNodeIds ?? [];
    const createdGroup = input.createVisualGroup(
      center,
      selectedNodeIds.length > 0
        ? {
            nodeGeometryById: input.nodeGeometryById,
            nodeIds: selectedNodeIds,
          }
        : undefined,
    );
    if (createdGroup) {
      setSelectedGroupId(createdGroup.id);
    }
    return createdGroup;
  }, [canEditVisualGroups, input]);

  const handleFitSelectedGroup = useCallback(() => {
    if (!selectedGroupId || !canEditVisualGroups || !input.fitVisualGroup) {
      return;
    }

    input.fitVisualGroup(selectedGroupId, input.nodeGeometryById ?? new Map());
  }, [canEditVisualGroups, input, selectedGroupId]);

  const handleSelectVisualGroup = useCallback(
    (groupId: string) => {
      if (!canEditVisualGroups) {
        return;
      }

      setSelectedGroupId((current) => (current === groupId ? null : groupId));
    },
    [canEditVisualGroups],
  );

  const handleRenameSelectedGroup = useCallback(
    (label: string) => {
      if (!selectedGroupId || !canEditVisualGroups) {
        return;
      }

      input.renameVisualGroup(selectedGroupId, label);
    },
    [canEditVisualGroups, input, selectedGroupId],
  );

  const handleSetSelectedGroupColor = useCallback(
    (color: FactoryLayoutGroupColorToken) => {
      if (!selectedGroupId || !canEditVisualGroups) {
        return;
      }

      input.setVisualGroupColor(selectedGroupId, color);
    },
    [canEditVisualGroups, input, selectedGroupId],
  );

  const handleToggleSelectedGroupNodeMembership = useCallback(
    (nodeId: string, selected: boolean) => {
      if (!selectedGroupId || !canEditVisualGroups) {
        return;
      }

      if (selected) {
        input.addNodeToVisualGroup(selectedGroupId, nodeId);
        return;
      }

      input.removeNodeFromVisualGroup(selectedGroupId, nodeId);
    },
    [canEditVisualGroups, input, selectedGroupId],
  );

  const clearSelectedVisualGroup = useCallback(() => {
    setSelectedGroupId(null);
  }, []);

  const handleMoveVisualGroup = useCallback(
    (
      groupId: string,
      delta: FactoryLayoutPoint,
      startMemberPositions: ReadonlyMap<string, FactoryLayoutPoint>,
    ) => {
      if (!canEditVisualGroups) {
        return;
      }

      input.moveVisualGroupByDelta(groupId, delta, startMemberPositions);
    },
    [canEditVisualGroups, input],
  );

  const handleResizeVisualGroup = useCallback(
    (
      groupId: string,
      bounds: NonNullable<FactoryLayout["groups"]>[number]["bounds"],
    ) => {
      if (!canEditVisualGroups) {
        return;
      }

      input.resizeVisualGroup(groupId, bounds);
    },
    [canEditVisualGroups, input],
  );

  const handleDeleteSelectedGroup = useCallback(() => {
    if (!selectedGroupId || !canEditVisualGroups) {
      return;
    }

    input.deleteVisualGroup(selectedGroupId);
    setSelectedGroupId(null);
  }, [canEditVisualGroups, input, selectedGroupId]);

  const canvasNodeOptionIds = useMemo(
    () => new Set(input.canvasNodeOptions.map((option) => option.id)),
    [input.canvasNodeOptions],
  );
  const staleMemberNodeIds = useMemo(() => {
    if (!selectedGroup) {
      return [];
    }

    return (selectedGroup.nodeIds ?? []).filter(
      (nodeId) => !canvasNodeOptionIds.has(nodeId),
    );
  }, [canvasNodeOptionIds, selectedGroup]);
  const selectedGroupBoundsError =
    selectedGroup && !isValidFactoryLayoutGroupBounds(selectedGroup.bounds)
      ? messages.visualGroupInvalidBoundsError
      : null;

  return {
    canEditVisualGroups,
    clearSelectedVisualGroup,
    groups,
    groupAriaLabel: messages.visualGroupAriaLabel,
    groupOutlineAriaLabel: messages.visualGroupOutlineAriaLabel,
    handleCreateVisualGroup,
    handleDeleteSelectedGroup,
    handleFitSelectedGroup,
    handleMoveVisualGroup,
    handleRenameSelectedGroup,
    handleResizeVisualGroup,
    handleSelectVisualGroup,
    handleSetSelectedGroupColor,
    resizeHandleAriaLabel: messages.visualGroupResizeHandleLabel,
    selectedGroup,
    selectedGroupId,
    visualGroupControls:
      selectedGroup && canEditVisualGroups
        ? {
            canvasNodeOptions: input.canvasNodeOptions,
            colorLabel: messages.visualGroupColorLabel,
            colorOptionLabel: messages.visualGroupColorOptionLabel,
            deleteGroupLabel: messages.visualGroupDeleteLabel,
            fitGroupLabel: messages.visualGroupFitLabel,
            boundsError: selectedGroupBoundsError,
            emptyLabelError: messages.visualGroupEmptyLabelError,
            group: selectedGroup,
            isNodeMember: (nodeId: string) =>
              factoryLayoutGroupContainsNode(selectedGroup, nodeId),
            labelFieldLabel: messages.visualGroupLabelFieldLabel,
            membershipEmptyLabel: messages.visualGroupMembershipEmptyLabel,
            membershipLabel: messages.visualGroupMembershipLabel,
            membershipNodeKindLabel: messages.kindLabel,
            membershipNodeLabel: messages.visualGroupMembershipNodeLabel,
            membershipStaleNodeLabel:
              messages.visualGroupMembershipStaleNodeLabel,
            onDeleteGroup: handleDeleteSelectedGroup,
            onFitGroup: handleFitSelectedGroup,
            onRenameGroup: handleRenameSelectedGroup,
            onSetGroupColor: handleSetSelectedGroupColor,
            onToggleNodeMembership: handleToggleSelectedGroupNodeMembership,
            groupAriaLabel: messages.visualGroupAriaLabel(selectedGroup),
            staleMemberNodeIds,
          }
        : null,
  };
}
