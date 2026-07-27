import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@you-agent-factory/components/overlays";
import { ActionRow } from "@you-agent-factory/components/layout";
import type { ReactNode } from "react";
import { DashboardActionButton } from "../../../../components/ui/dashboard-action-button";
import { cn } from "../../../../lib/cn";
import type { FactoryGraphNodeKind } from "../../lib/draft/factory-graph-draft-types";
import {
  EMPTY_FACTORY_GRAPH_EDITOR_TOOLBAR_SELECTION_STATE,
  type FactoryGraphEditorToolbarSelectionState,
  resolveFactoryGraphEditorToolbarDeleteAction,
  resolveFactoryGraphEditorToolbarDeleteButtonState,
} from "../../lib/selection/factory-graph-editor-toolbar-selection";
import { getFactoryGraphEditorMessages } from "../../messages/editor";

export {
  FactoryGraphEditorModeToggle,
  FactoryGraphEditorStatus,
} from "../chrome/factory-graph-editor-mode-controls";
export { FactoryGraphEditorWorkStatePhaseLegend } from "../chrome/factory-graph-editor-work-state-phase-legend";

import { FactoryGraphEditorHideShowMenu } from "../chrome/factory-graph-editor-hide-show-menu";
import { FactoryGraphEditorModeToggle } from "../chrome/factory-graph-editor-mode-controls";
import { FactoryGraphEditorTooltipActionButton } from "../chrome/factory-graph-editor-tooltip-button";
import {
  DiscardIcon,
  GroupIcon,
  RedoIcon,
  ResetLayoutIcon,
  SaveIcon,
  TrashIcon,
  UndoIcon,
} from "../factory-graph-editor-toolbar-icons";
import { FactoryGraphEditorMenuHeader } from "../menu/factory-graph-editor-menu-header";
import { FactoryGraphEditorMenuItemButton } from "../menu/factory-graph-editor-menu-item-button";
import { FactoryGraphEditorMenuItemCopy } from "../menu/factory-graph-editor-menu-item-copy";
import { FactoryGraphEditorFloatingSurface } from "../surface/factory-graph-editor-floating-surface";

export type { FactoryGraphEditorToolbarSelectionState } from "../../lib/selection/factory-graph-editor-toolbar-selection";
export {
  FactoryGraphEditorActionPopover,
  FactoryGraphEditorConfirmationDialog,
} from "../factory-graph-editor-dialogs";

export type FactoryGraphEditorTool = "add" | "connect" | "delete" | null;
export type FactoryGraphEditorVisibilityPreset =
  | "all"
  | "workflow"
  | "execution"
  | "infrastructure";

export interface FactoryGraphEditorMenuAction {
  description?: string;
  disabled?: boolean;
  id: string;
  label: string;
}

export interface FactoryGraphEditorVisibilityPresetOption {
  key: FactoryGraphEditorVisibilityPreset;
  label: string;
  selected: boolean;
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: toolbar composes edit-mode toggle, hide/show, and layout history controls.
export function FactoryGraphEditorToolbar({
  activeTool: _activeTool,
  addMenuActions = [],
  canInteract,
  canRedoLayout = false,
  canSave = false,
  canDiscard = false,
  canDeleteSelection = false,
  canUndoLayout = false,
  editModeToggle,
  graphSelectionToolbarState = EMPTY_FACTORY_GRAPH_EDITOR_TOOLBAR_SELECTION_STATE,
  hiddenNodeClasses = new Set<FactoryGraphNodeKind>(),
  hideShowMenuOpen = false,
  hideShowVisible = true,
  isSaving = false,
  locale,
  onCreateVisualGroup,
  onDiscard,
  onDeleteSelection,
  onAddAction,
  onAddMenuOpenChange,
  onClearPreferences,
  onHideShowMenuOpenChange,
  onRedoLayout,
  onResetLayout,
  onSave,
  onToggleHiddenNodeClass,
  onUndoLayout,
  preferencesDirty = false,
  saveDisabledReason,
  visible,
  onSelectTool: _onSelectTool,
  openAddMenu = false,
}: {
  activeTool: FactoryGraphEditorTool;
  addMenuActions?: FactoryGraphEditorMenuAction[];
  canInteract: boolean;
  canRedoLayout?: boolean;
  canSave?: boolean;
  canDiscard?: boolean;
  canDeleteSelection?: boolean;
  canUndoLayout?: boolean;
  editModeToggle?: {
    disabled?: boolean;
    editorMode: boolean;
    hasChanges?: boolean;
    onToggle: () => void;
    tooltipOverride?: string;
  };
  graphSelectionToolbarState?: FactoryGraphEditorToolbarSelectionState;
  hiddenNodeClasses?: ReadonlySet<FactoryGraphNodeKind>;
  hideShowMenuOpen?: boolean;
  hideShowVisible?: boolean;
  isSaving?: boolean;
  locale?: string;
  onCreateVisualGroup?: () => void;
  onDiscard?: () => void;
  onDeleteSelection?: () => void;
  onAddAction?: (actionID: string) => void;
  onAddMenuOpenChange?: (open: boolean) => void;
  onClearPreferences?: () => void;
  onHideShowMenuOpenChange?: (open: boolean) => void;
  onRedoLayout?: () => void;
  onResetLayout?: () => void;
  onSave?: () => void;
  onToggleHiddenNodeClass?: (kind: FactoryGraphNodeKind) => void;
  onUndoLayout?: () => void;
  preferencesDirty?: boolean;
  saveDisabledReason?: string;
  visible: boolean;
  onSelectTool: (tool: FactoryGraphEditorTool) => void;
  openAddMenu?: boolean;
}) {
  if (!visible && !hideShowVisible) {
    return null;
  }
  const messages = getFactoryGraphEditorMessages(locale);
  const hideShowActive = hideShowMenuOpen || hiddenNodeClasses.size > 0;
  const showDraftActionRow = visible;
  const showEditorControls = visible;
  const toolbarButtonsDisabled = !showEditorControls || !canInteract;
  const discardDisabled = !canDiscard;
  const saveDisabled = !canSave;
  const deleteAction = resolveFactoryGraphEditorToolbarDeleteAction({
    canDeleteSelection,
    selectionState: graphSelectionToolbarState,
  });
  const deleteButtonState = resolveFactoryGraphEditorToolbarDeleteButtonState({
    deleteAction,
    messages,
  });
  const deleteDisabled =
    toolbarButtonsDisabled || deleteAction.kind === "disabled";

  return (
    <FactoryGraphEditorFloatingSurface
      aria-label={messages.toolbarAriaLabel}
      className="overflow-hidden px-3 py-2"
      placement="bottomToolbar"
    >
      {hideShowVisible && onToggleHiddenNodeClass ? (
        <FactoryGraphEditorHideShowMenu
          hiddenNodeClasses={hiddenNodeClasses}
          locale={locale}
          onClearPreferences={onClearPreferences}
          onOpenChange={onHideShowMenuOpenChange}
          onToggleHiddenNodeClass={onToggleHiddenNodeClass}
          open={hideShowMenuOpen}
          preferencesDirty={preferencesDirty}
          pressed={hideShowActive}
        />
      ) : null}
      <div
        aria-hidden={!showEditorControls}
        className={cn(
          "grid min-h-10 max-h-11 min-w-0 self-center overflow-hidden transition-[max-width,opacity] duration-200 ease-out motion-reduce:transition-none",
          showEditorControls
            ? "max-w-xl opacity-100"
            : "pointer-events-none max-w-0 opacity-0",
        )}
        data-toolbar-editor-controls-lane=""
        data-toolbar-editor-controls={
          showEditorControls ? "expanded" : "collapsed"
        }
      >
        <div
          className="flex min-h-10 max-h-11 min-w-max flex-nowrap items-center gap-2 overflow-hidden"
          data-toolbar-editor-controls-row=""
          data-toolbar-graph-selection={graphSelectionToolbarState.mode}
        >
          <FactoryGraphEditorAddMenu
            actions={addMenuActions}
            canInteract={!toolbarButtonsDisabled}
            locale={locale}
            onAction={onAddAction}
            onOpenChange={onAddMenuOpenChange}
            open={openAddMenu}
          />
          <FactoryGraphEditorToolbarButton
            active={false}
            description={messages.toolbarCreateGroupDescription}
            disabled={
              toolbarButtonsDisabled || onCreateVisualGroup === undefined
            }
            icon={<GroupIcon />}
            label={messages.toolbarCreateGroupLabel}
            onClick={() => onCreateVisualGroup?.()}
            tone="outline"
          />
          <FactoryGraphEditorToolbarButton
            active={deleteButtonState.active}
            description={deleteButtonState.description}
            disabled={deleteDisabled}
            icon={<TrashIcon />}
            label={deleteButtonState.label}
            onClick={() => {
              if (deleteAction.kind === "batch-delete" && onDeleteSelection) {
                onDeleteSelection();
              }
            }}
            tone={deleteButtonState.tone}
          />
          <FactoryGraphEditorToolbarButton
            active={false}
            description={messages.toolbarUndoDescription}
            disabled={toolbarButtonsDisabled || !canUndoLayout}
            icon={<UndoIcon />}
            label={messages.toolbarUndoLabel}
            onClick={() => onUndoLayout?.()}
            tone="outline"
          />
          <FactoryGraphEditorToolbarButton
            active={false}
            description={messages.toolbarRedoDescription}
            disabled={toolbarButtonsDisabled || !canRedoLayout}
            icon={<RedoIcon />}
            label={messages.toolbarRedoLabel}
            onClick={() => onRedoLayout?.()}
            tone="outline"
          />
          <FactoryGraphEditorToolbarButton
            active={false}
            description={messages.toolbarResetLayoutDescription}
            disabled={toolbarButtonsDisabled}
            icon={<ResetLayoutIcon />}
            label={messages.toolbarResetLayoutLabel}
            onClick={() => onResetLayout?.()}
            tone="outline"
          />
          {showDraftActionRow ? (
            <ActionRow
              actions={
                <>
                  <FactoryGraphEditorTooltipActionButton
                    aria-label={messages.draftActionsDiscard}
                    disabled={discardDisabled}
                    iconOnly
                    onClick={onDiscard}
                    placement="above"
                    tooltip={messages.draftActionsDiscard}
                    tone="outline"
                    type="button"
                  >
                    <DiscardIcon />
                  </FactoryGraphEditorTooltipActionButton>
                  <FactoryGraphEditorTooltipActionButton
                    aria-label={
                      isSaving
                        ? messages.draftActionsSaving
                        : messages.draftActionsSave
                    }
                    disabled={saveDisabled}
                    executing={isSaving}
                    iconOnly
                    onClick={onSave}
                    placement="above"
                    tooltip={saveDisabledReason ?? messages.draftActionsSave}
                    tone={canSave && !isSaving ? "warning" : "outline"}
                    type="button"
                  >
                    <SaveIcon />
                  </FactoryGraphEditorTooltipActionButton>
                </>
              }
              actionsClassName="flex flex-nowrap items-center gap-2 border-l border-outline pl-2 max-md:ml-auto"
              className="min-w-0 flex-1 flex-nowrap items-center overflow-hidden"
            />
          ) : null}
        </div>
      </div>
      {editModeToggle ? (
        <FactoryGraphEditorModeToggle
          className="shrink-0"
          disabled={editModeToggle.disabled}
          editorMode={editModeToggle.editorMode}
          hasChanges={editModeToggle.hasChanges}
          locale={locale}
          onClick={editModeToggle.onToggle}
          tooltipOverride={editModeToggle.tooltipOverride}
        />
      ) : null}
    </FactoryGraphEditorFloatingSurface>
  );
}

export function FactoryGraphEditorVisibilityPanel({
  locale,
  onSelectPreset,
  options,
  visible,
}: {
  locale?: string;
  onSelectPreset: (preset: FactoryGraphEditorVisibilityPreset) => void;
  options: FactoryGraphEditorVisibilityPresetOption[];
  visible: boolean;
}) {
  if (!visible || options.length === 0) {
    return null;
  }
  const messages = getFactoryGraphEditorMessages(locale);

  return (
    <FactoryGraphEditorFloatingSurface
      aria-label={messages.visibilityPresetsAriaLabel}
      className="px-2 py-2"
      placement="topRight"
    >
      {options.map((option) => (
        <DashboardActionButton
          aria-pressed={option.selected}
          className="min-w-20"
          iconOnly={false}
          key={option.key}
          onClick={() => onSelectPreset(option.key)}
          tone={option.selected ? "secondary" : "outline"}
          type="button"
        >
          {option.label}
        </DashboardActionButton>
      ))}
    </FactoryGraphEditorFloatingSurface>
  );
}

function FactoryGraphEditorToolbarButton({
  active,
  description,
  disabled,
  icon,
  label,
  onClick,
  tone,
}: {
  active: boolean;
  description: string;
  disabled: boolean;
  icon: ReactNode;
  label: string;
  onClick: () => void;
  tone: "outline" | "secondary";
}) {
  return (
    <FactoryGraphEditorTooltipActionButton
      aria-label={label}
      aria-pressed={active}
      disabled={disabled}
      iconOnly
      onClick={onClick}
      placement="above"
      tooltip={description}
      tone={tone}
      type="button"
    >
      {icon}
    </FactoryGraphEditorTooltipActionButton>
  );
}

function FactoryGraphEditorAddMenu({
  actions,
  canInteract,
  locale,
  onAction,
  onOpenChange,
  open,
}: {
  actions: FactoryGraphEditorMenuAction[];
  canInteract: boolean;
  locale?: string;
  onAction?: (actionID: string) => void;
  onOpenChange?: (open: boolean) => void;
  open: boolean;
}) {
  if (actions.length === 0 || onAction === undefined) {
    return null;
  }
  const messages = getFactoryGraphEditorMessages(locale);

  return (
    <Popover onOpenChange={onOpenChange} open={open}>
      <PopoverTrigger asChild>
        <FactoryGraphEditorTooltipActionButton
          aria-label={messages.toolbarOpenAddMenuLabel}
          disabled={!canInteract}
          iconOnly
          placement="above"
          tooltip={messages.toolbarAddDescription}
          tone={open ? "secondary" : "outline"}
          type="button"
        >
          <svg
            aria-hidden="true"
            fill="none"
            height="18"
            stroke="currentColor"
            strokeLinecap="round"
            strokeLinejoin="round"
            strokeWidth="1.8"
            viewBox="0 0 24 24"
            width="18"
          >
            <path d="M12 5v14" />
            <path d="M5 12h14" />
          </svg>
        </FactoryGraphEditorTooltipActionButton>
      </PopoverTrigger>
      <PopoverContent
        align="start"
        aria-label={messages.toolbarVisibilityMenuAriaLabel}
        avoidCollisions={false}
        // tailwind-exception: intrinsic-sizing
        className="grid max-h-[min(var(--radix-popover-content-available-height),calc(100vh-8rem))] gap-2 overflow-y-auto"
        side="top"
        sideOffset={12}
      >
        <FactoryGraphEditorMenuHeader
          description={messages.toolbarVisibilityMenuDescription}
          title={messages.toolbarVisibilityMenuTitle}
        />
        <div className="grid gap-1">
          {actions.map((action) => (
            <FactoryGraphEditorMenuItemButton
              aria-label={action.label}
              disabled={action.disabled}
              key={action.id}
              onClick={() => {
                onAction(action.id);
                onOpenChange?.(false);
              }}
              type="button"
            >
              <FactoryGraphEditorMenuItemCopy
                description={action.description}
                label={action.label}
              />
            </FactoryGraphEditorMenuItemButton>
          ))}
        </div>
      </PopoverContent>
    </Popover>
  );
}

export {
  FactoryGraphEditorNotice,
  type FactoryGraphEditorNoticeTone,
} from "../chrome/factory-graph-editor-notice";
