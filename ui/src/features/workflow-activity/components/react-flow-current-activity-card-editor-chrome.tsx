import { type ReactNode, useCallback, useState } from "react";

import { DashboardActionRow } from "../../../components/ui";
import { cn } from "../../../lib/cn";
import {
  FactoryGraphEditorModeToggle,
  FactoryGraphEditorStatus,
  type FactoryGraphEditorTool,
} from "../../factory-graph-editor/components/factory-graph-editor-controls";
import { getFactoryGraphEditorMessages } from "../../factory-graph-editor/messages/editor";
import {
  applyFactoryGraphAddEntityDraft,
  type CanonicalFactoryDefinition,
  createFactoryGraphAddEntityDraft,
  type FactoryGraphAddEntityDraft,
  type FactoryGraphAddEntityFieldErrors,
  type FactoryGraphAddEntityKind,
  type useFactoryGraphDraftState,
  validateFactoryGraphAddEntityDraft,
} from "../../factory-graph-editor/public";

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

const FACTORY_GRAPH_HEADER_ACTIONS_CLASS = "min-w-0 justify-end";
const FACTORY_GRAPH_HEADER_ACTIONS_COMPACT_CLASS = "gap-1.5";
const FACTORY_GRAPH_HEADER_ACTIONS_SECTIONS_COMPACT_CLASS = "gap-1.5";
const STATUS_PILL_COMPACT_CLASS = "px-2.5 py-0.5 text-[0.7rem]";
const MODE_TOGGLE_COMPACT_CLASS =
  "size-8 rounded-md border-af-border bg-transparent text-af-text-muted hover:border-af-border-strong hover:bg-af-overlay hover:text-af-text";

export function CurrentActivityGraphHeaderActions({
  compact = false,
  editorMode,
  editorUnavailableClassifierWorkstationName,
  headerActions,
  hasChanges,
  isDefinitionLoading,
  loadErrorMessage,
  locale,
  onToggle,
}: {
  compact?: boolean;
  editorMode: boolean;
  editorUnavailableClassifierWorkstationName?: string;
  headerActions?: ReactNode;
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
    <DashboardActionRow
      actions={
        <>
          <FactoryGraphEditorModeToggle
            className={compact ? MODE_TOGGLE_COMPACT_CLASS : undefined}
            disabled={!editorMode && editorUnavailableReason !== undefined}
            editorMode={editorMode}
            locale={locale}
            onClick={onToggle}
            tooltipOverride={editorUnavailableReason}
          />
          {headerActions}
        </>
      }
      actionsClassName={
        compact
          ? FACTORY_GRAPH_HEADER_ACTIONS_SECTIONS_COMPACT_CLASS
          : undefined
      }
      className={cn(
        FACTORY_GRAPH_HEADER_ACTIONS_CLASS,
        compact && FACTORY_GRAPH_HEADER_ACTIONS_COMPACT_CLASS,
      )}
      statuses={
        <FactoryGraphEditorStatus
          className={compact ? STATUS_PILL_COMPACT_CLASS : undefined}
          editorMode={editorMode}
          editorUnavailableReason={editorUnavailableReason}
          hasChanges={hasChanges}
          isDefinitionLoading={isDefinitionLoading}
          locale={locale}
          loadErrorMessage={loadErrorMessage}
        />
      }
      statusesClassName={
        compact
          ? FACTORY_GRAPH_HEADER_ACTIONS_SECTIONS_COMPACT_CLASS
          : undefined
      }
    />
  );
}
