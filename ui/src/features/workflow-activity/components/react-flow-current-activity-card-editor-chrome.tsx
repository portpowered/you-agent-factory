import { type ReactNode, useCallback, useState } from "react";

import {
  applyFactoryGraphAddEntityDraft,
  createFactoryGraphAddEntityDraft,
  type FactoryGraphAddEntityDraft,
  type FactoryGraphAddEntityFieldErrors,
  type FactoryGraphAddEntityKind,
  validateFactoryGraphAddEntityDraft,
} from "../../factory-graph-editor/factory-graph-editor-additions";
import {
  FactoryGraphEditorModeToggle,
  FactoryGraphEditorStatus,
  type FactoryGraphEditorTool,
} from "../../factory-graph-editor/factory-graph-editor-controls";
import type { CanonicalFactoryDefinition } from "../../factory-graph-editor/factory-graph-draft-types";
import type { useFactoryGraphDraftState } from "../../factory-graph-editor/factory-graph-draft";

const CURRENT_ACTIVITY_HEADER_ROW_CLASS =
  "flex items-start justify-between gap-3";

export function useFactoryGraphAddEntityController({
  currentFactoryDefinition,
  draftState,
  setActiveTool,
}: {
  currentFactoryDefinition: CanonicalFactoryDefinition | null;
  draftState: ReturnType<typeof useFactoryGraphDraftState>;
  setActiveTool: (tool: FactoryGraphEditorTool) => void;
}) {
  const [addMenuOpen, setAddMenuOpen] = useState(false);
  const [addEntityDraft, setAddEntityDraft] =
    useState<FactoryGraphAddEntityDraft | null>(null);
  const [addEntityErrors, setAddEntityErrors] =
    useState<FactoryGraphAddEntityFieldErrors>({});

  const handleAddEntityAction = useCallback(
    (actionID: string) => {
      setActiveTool("add");
      setAddEntityDraft(
        createFactoryGraphAddEntityDraft(
          actionID as FactoryGraphAddEntityKind,
          currentFactoryDefinition,
        ),
      );
      setAddEntityErrors({});
      setAddMenuOpen(false);
    },
    [currentFactoryDefinition, setActiveTool],
  );

  const handleAddEntitySubmit = useCallback(() => {
    if (addEntityDraft === null) {
      return;
    }

    const validationErrors = validateFactoryGraphAddEntityDraft(
      addEntityDraft,
      currentFactoryDefinition,
    );
    if (Object.keys(validationErrors).length > 0) {
      setAddEntityErrors(validationErrors);
      return;
    }

    draftState.updateDraft((currentDraft) =>
      applyFactoryGraphAddEntityDraft(currentDraft, addEntityDraft),
    );
    setAddEntityDraft(null);
    setAddEntityErrors({});
  }, [addEntityDraft, currentFactoryDefinition, draftState]);

  const reset = useCallback(() => {
    setAddMenuOpen(false);
    setAddEntityDraft(null);
    setAddEntityErrors({});
  }, []);

  return {
    addEntityDraft,
    addEntityErrors,
    addMenuOpen,
    handleAddEntityAction,
    handleAddEntitySubmit,
    reset,
    setAddEntityDraft,
    setAddEntityErrors,
    setAddMenuOpen,
  };
}

export function CurrentActivityGraphEditorHeader({
  editorMode,
  hasChanges,
  isDefinitionLoading,
  loadErrorMessage,
  locale,
  onToggle,
  title,
}: {
  editorMode: boolean;
  hasChanges: boolean;
  isDefinitionLoading: boolean;
  loadErrorMessage?: string;
  locale?: string;
  onToggle: () => void;
  title: ReactNode;
}) {
  return (
    <div className={CURRENT_ACTIVITY_HEADER_ROW_CLASS}>
      <div className="min-w-0">
        {title}
        <div className="mt-2">
          <FactoryGraphEditorStatus
            editorMode={editorMode}
            hasChanges={hasChanges}
            isDefinitionLoading={isDefinitionLoading}
            locale={locale}
            loadErrorMessage={loadErrorMessage}
          />
        </div>
      </div>
      <FactoryGraphEditorModeToggle
        editorMode={editorMode}
        locale={locale}
        onClick={onToggle}
      />
    </div>
  );
}

const FACTORY_GRAPH_HEADER_ACTIONS_CLASS =
  "flex min-w-0 flex-wrap items-center justify-end gap-2";

export function CurrentActivityGraphHeaderActions({
  editorMode,
  hasChanges,
  isDefinitionLoading,
  loadErrorMessage,
  locale,
  onToggle,
}: {
  editorMode: boolean;
  hasChanges: boolean;
  isDefinitionLoading: boolean;
  loadErrorMessage?: string;
  locale?: string;
  onToggle: () => void;
}) {
  return (
    <div className={FACTORY_GRAPH_HEADER_ACTIONS_CLASS}>
      <FactoryGraphEditorStatus
        editorMode={editorMode}
        hasChanges={hasChanges}
        isDefinitionLoading={isDefinitionLoading}
        locale={locale}
        loadErrorMessage={loadErrorMessage}
      />
      <FactoryGraphEditorModeToggle
        editorMode={editorMode}
        locale={locale}
        onClick={onToggle}
      />
    </div>
  );
}
