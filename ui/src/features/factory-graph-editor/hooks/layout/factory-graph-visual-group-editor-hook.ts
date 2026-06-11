import { useCallback, useMemo, useState } from "react";

import type { FactoryGraphEditorTool } from "../../components/controls/factory-graph-editor-controls";
import {
  factoryLayoutGroupById,
  factoryLayoutGroupContainsNode,
  factoryLayoutGroups,
  type FactoryLayoutGroupCanvasNodeOption,
  type FactoryLayoutGroupColorToken,
} from "../../lib/layout/factory-graph-layout-groups";
import type {
  FactoryLayout,
  FactoryLayoutPoint,
} from "../../lib/layout/factory-graph-layout-operations";
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

export function useFactoryGraphVisualGroupEditor(input: {
  activeTool: FactoryGraphEditorTool;
  addNodeToVisualGroup: (groupId: string, nodeId: string) => void;
  canInteractWithEditor: boolean;
  canvasNodeOptions: readonly FactoryLayoutGroupCanvasNodeOption[];
  createVisualGroup: (center: FactoryLayoutPoint) => { id: string } | null;
  editorMode: boolean;
  layout: FactoryLayout;
  locale?: string | null;
  removeNodeFromVisualGroup: (groupId: string, nodeId: string) => void;
  renameVisualGroup: (groupId: string, label: string) => void;
  setVisualGroupColor: (
    groupId: string,
    color: FactoryLayoutGroupColorToken,
  ) => void;
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
    const createdGroup = input.createVisualGroup(center);
    if (createdGroup) {
      setSelectedGroupId(createdGroup.id);
    }
    return createdGroup;
  }, [canEditVisualGroups, input]);

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

  return {
    canEditVisualGroups,
    clearSelectedVisualGroup,
    groups,
    groupAriaLabel: messages.visualGroupAriaLabel,
    handleCreateVisualGroup,
    handleRenameSelectedGroup,
    handleSelectVisualGroup,
    handleSetSelectedGroupColor,
    selectedGroup,
    selectedGroupId,
    visualGroupControls:
      selectedGroup && canEditVisualGroups
        ? {
            canvasNodeOptions: input.canvasNodeOptions,
            colorLabel: messages.visualGroupColorLabel,
            colorOptionLabel: messages.visualGroupColorOptionLabel,
            emptyLabelError: messages.visualGroupEmptyLabelError,
            group: selectedGroup,
            isNodeMember: (nodeId: string) =>
              factoryLayoutGroupContainsNode(selectedGroup, nodeId),
            labelFieldLabel: messages.visualGroupLabelFieldLabel,
            membershipEmptyLabel: messages.visualGroupMembershipEmptyLabel,
            membershipLabel: messages.visualGroupMembershipLabel,
            membershipNodeLabel: messages.visualGroupMembershipNodeLabel,
            membershipStaleNodeLabel:
              messages.visualGroupMembershipStaleNodeLabel,
            onRenameGroup: handleRenameSelectedGroup,
            onSetGroupColor: handleSetSelectedGroupColor,
            onToggleNodeMembership: handleToggleSelectedGroupNodeMembership,
            selectedGroupLabel: messages.visualGroupSelectedLabel,
            staleMemberNodeIds,
          }
        : null,
  };
}
