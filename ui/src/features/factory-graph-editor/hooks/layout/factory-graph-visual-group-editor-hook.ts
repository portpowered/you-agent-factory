import { useCallback, useMemo, useState } from "react";

import type { FactoryGraphEditorTool } from "../../components/controls/factory-graph-editor-controls";
import {
  factoryLayoutGroupById,
  factoryLayoutGroups,
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
  canInteractWithEditor: boolean;
  createVisualGroup: (center: FactoryLayoutPoint) => { id: string } | null;
  editorMode: boolean;
  layout: FactoryLayout;
  locale?: string | null;
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

  const clearSelectedVisualGroup = useCallback(() => {
    setSelectedGroupId(null);
  }, []);

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
            colorLabel: messages.visualGroupColorLabel,
            colorOptionLabel: messages.visualGroupColorOptionLabel,
            emptyLabelError: messages.visualGroupEmptyLabelError,
            group: selectedGroup,
            labelFieldLabel: messages.visualGroupLabelFieldLabel,
            onRenameGroup: handleRenameSelectedGroup,
            onSetGroupColor: handleSetSelectedGroupColor,
            selectedGroupLabel: messages.visualGroupSelectedLabel,
          }
        : null,
  };
}
