import { type ReactNode, useCallback, useState } from "react";

import { DashboardActionRow } from "../../../components/ui";
import { cn } from "../../../lib/cn";
import { buildDocTargetPathFromFileName } from "../../current-factory-definition/lib/doc-editable-values";
import {
  FactoryGraphEditorModeToggle,
  FactoryGraphEditorStatus,
  type FactoryGraphEditorTool,
} from "../../factory-graph-editor/components/controls/factory-graph-editor-controls";
import type { EditableFactoryGraphViewModel } from "../../factory-graph-editor/hooks/use-editable-factory-graph-types";
import type { CanonicalFactoryDefinition } from "../../factory-graph-editor/lib/draft/factory-graph-draft-types";
import {
  createFactoryGraphAddEntityDraft,
  type FactoryGraphAddEntityDraft,
  type FactoryGraphAddEntityFieldErrors,
  type FactoryGraphAddEntityKind,
  validateFactoryGraphAddEntityDraft,
} from "../../factory-graph-editor/lib/editor/factory-graph-editor-additions";
import {
  type FactoryGraphEditorDirtyState,
  hasAnyFactoryGraphEditorChanges,
} from "../../factory-graph-editor/lib/editor-runtime/factory-graph-editor-dirty-state";
import { getFactoryGraphEditorMessages } from "../../factory-graph-editor/messages/editor";
import { useGraphEditorPlaceAddedNode } from "./graph-editor-placement-context";

export function useFactoryGraphAddEntityController({
  currentFactoryDefinition,
  editableGraph,
  onDocAdded,
  setActiveTool,
}: {
  currentFactoryDefinition: CanonicalFactoryDefinition | null;
  editableGraph: EditableFactoryGraphViewModel;
  onDocAdded?: (targetPath: string) => void;
  setActiveTool: (tool: FactoryGraphEditorTool) => void;
}) {
  const [addMenuOpen, setAddMenuOpen] = useState(false);
  const [addEntityDraft, setAddEntityDraft] =
    useState<FactoryGraphAddEntityDraft | null>(null);
  const [addEntityErrors, setAddEntityErrors] =
    useState<FactoryGraphAddEntityFieldErrors>({});
  const placeAddedNode = useGraphEditorPlaceAddedNode();

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

    const addResult = editableGraph.actions.addNode(addEntityDraft);
    if (!addResult.ok) {
      setAddEntityErrors(addResult.fieldErrors ?? { name: addResult.message });
      return;
    }

    placeAddedNode?.(addEntityDraft);
    if (addEntityDraft.kind === "doc") {
      onDocAdded?.(
        buildDocTargetPathFromFileName(addEntityDraft.fileName.trim()),
      );
    }
    setActiveTool(null);
    setAddEntityDraft(null);
    setAddEntityErrors({});
  }, [
    addEntityDraft,
    currentFactoryDefinition,
    editableGraph.actions,
    onDocAdded,
    placeAddedNode,
    setActiveTool,
  ]);

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
  "size-8 rounded-md border-outline bg-transparent text-on-surface-variant hover:border-outline-variant hover:bg-af-overlay hover:text-on-surface";
const MODE_TOGGLE_COMPACT_DIRTY_CLASS = "size-8 rounded-md";

function CurrentActivityGraphHeaderDirtySummary({
  className,
  dirtyState,
  dirtySummary,
  hasChanges,
  locale,
}: {
  className?: string;
  dirtyState?: FactoryGraphEditorDirtyState;
  dirtySummary?: string | null;
  hasChanges: boolean;
  locale?: string;
}) {
  const messages = getFactoryGraphEditorMessages(locale);
  const hasDirtyIndicator = dirtyState
    ? hasAnyFactoryGraphEditorChanges(dirtyState)
    : Boolean(dirtySummary) || hasChanges;

  if (!hasDirtyIndicator) {
    return null;
  }

  return (
    <span className={cn("text-on-surface-variant", className)}>
      {dirtySummary ??
        (dirtyState
          ? messages.dirtyStateSummary(dirtyState)
          : messages.modeUnsavedChanges)}
    </span>
  );
}

export function CurrentActivityGraphHeaderActions({
  compact = false,
  dirtyState,
  dirtySummary,
  editorMode,
  editorUnavailableClassifierWorkstationName,
  headerActions,
  hasChanges,
  isDefinitionLoading,
  loadErrorMessage,
  locale,
  onToggle,
  showModeToggle = true,
}: {
  compact?: boolean;
  dirtyState?: FactoryGraphEditorDirtyState;
  dirtySummary?: string | null;
  editorMode: boolean;
  editorUnavailableClassifierWorkstationName?: string;
  headerActions?: ReactNode;
  hasChanges: boolean;
  isDefinitionLoading: boolean;
  loadErrorMessage?: string;
  locale?: string;
  onToggle: () => void;
  showModeToggle?: boolean;
}) {
  const messages = getFactoryGraphEditorMessages(locale);
  const editorUnavailableReason =
    editorUnavailableClassifierWorkstationName === undefined
      ? undefined
      : messages.modeClassifierRoutesUnavailable(
          editorUnavailableClassifierWorkstationName,
        );
  const hasDirtyIndicator = dirtyState
    ? hasAnyFactoryGraphEditorChanges(dirtyState)
    : Boolean(dirtySummary) || hasChanges;

  return (
    <DashboardActionRow
      actions={
        <>
          {showModeToggle ? (
            <FactoryGraphEditorModeToggle
              className={
                compact
                  ? hasDirtyIndicator
                    ? MODE_TOGGLE_COMPACT_DIRTY_CLASS
                    : MODE_TOGGLE_COMPACT_CLASS
                  : undefined
              }
              disabled={!editorMode && editorUnavailableReason !== undefined}
              editorMode={editorMode}
              hasChanges={hasDirtyIndicator}
              locale={locale}
              onClick={onToggle}
              tooltipOverride={editorUnavailableReason}
            />
          ) : null}
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
        showModeToggle ? (
          <FactoryGraphEditorStatus
            className={compact ? STATUS_PILL_COMPACT_CLASS : undefined}
            dirtyState={dirtyState}
            editorMode={editorMode}
            editorUnavailableReason={editorUnavailableReason}
            hasChanges={hasDirtyIndicator}
            isDefinitionLoading={isDefinitionLoading}
            locale={locale}
            loadErrorMessage={loadErrorMessage}
          />
        ) : editorMode ? (
          <CurrentActivityGraphHeaderDirtySummary
            className={compact ? STATUS_PILL_COMPACT_CLASS : undefined}
            dirtyState={dirtyState}
            dirtySummary={dirtySummary}
            hasChanges={hasDirtyIndicator}
            locale={locale}
          />
        ) : null
      }
      statusesClassName={
        compact
          ? FACTORY_GRAPH_HEADER_ACTIONS_SECTIONS_COMPACT_CLASS
          : undefined
      }
    />
  );
}
