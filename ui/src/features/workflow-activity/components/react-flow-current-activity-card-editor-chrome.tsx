import { useCallback, useState } from "react";

import { cn } from "../../../lib/cn";
import {
  applyFactoryGraphAddEntityDraft,
  createFactoryGraphAddEntityDraft,
  type CanonicalFactoryDefinition,
  type FactoryGraphAddEntityDraft,
  type FactoryGraphAddEntityFieldErrors,
  type FactoryGraphAddEntityKind,
  type useFactoryGraphDraftState,
  validateFactoryGraphAddEntityDraft,
} from "../../factory-graph-editor/public";
import {
  FactoryGraphEditorModeToggle,
  FactoryGraphEditorStatus,
  type FactoryGraphEditorTool,
} from "../../factory-graph-editor/components/factory-graph-editor-controls";
import { getFactoryGraphEditorMessages } from "../../factory-graph-editor/messages/editor";

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

const FACTORY_GRAPH_HEADER_ACTIONS_CLASS =
  "flex min-w-0 flex-wrap items-center justify-end gap-2";
const FACTORY_GRAPH_HEADER_ACTIONS_COMPACT_CLASS = "gap-1.5";
const STATUS_PILL_COMPACT_CLASS = "px-2.5 py-0.5 text-[0.7rem]";
const MODE_TOGGLE_COMPACT_CLASS =
  "size-8 rounded-md border-af-overlay/12 bg-transparent text-af-ink/72 hover:bg-af-overlay/8";

export function CurrentActivityGraphHeaderActions({
  compact = false,
  editorMode,
  editorUnavailableClassifierWorkstationName,
  hasChanges,
  isDefinitionLoading,
  loadErrorMessage,
  locale,
  onToggle,
}: {
  compact?: boolean;
  editorMode: boolean;
  editorUnavailableClassifierWorkstationName?: string;
  hasChanges: boolean;
  isDefinitionLoading: boolean;
  loadErrorMessage?: string;
  locale?: string;
  onToggle: () => void;
}) {
  const messages = getFactoryGraphEditorMessages(locale);
  const editorUnavailableReason =
    editorUnavailableClassifierWorkstationName === undefined
      ? undefined
      : messages.modeClassifierRoutesUnavailable(
          editorUnavailableClassifierWorkstationName,
        );

  return (
    <div
      className={cn(
        FACTORY_GRAPH_HEADER_ACTIONS_CLASS,
        compact && FACTORY_GRAPH_HEADER_ACTIONS_COMPACT_CLASS,
      )}
    >
      <FactoryGraphEditorStatus
        className={compact ? STATUS_PILL_COMPACT_CLASS : undefined}
        editorMode={editorMode}
        editorUnavailableReason={editorUnavailableReason}
        hasChanges={hasChanges}
        isDefinitionLoading={isDefinitionLoading}
        locale={locale}
        loadErrorMessage={loadErrorMessage}
      />
      <FactoryGraphEditorModeToggle
        className={compact ? MODE_TOGGLE_COMPACT_CLASS : undefined}
        disabled={!editorMode && editorUnavailableReason !== undefined}
        editorMode={editorMode}
        locale={locale}
        onClick={onToggle}
        tooltipOverride={editorUnavailableReason}
      />
    </div>
  );
}
