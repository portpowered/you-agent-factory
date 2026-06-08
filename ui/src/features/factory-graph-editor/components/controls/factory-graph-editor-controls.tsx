import type { ReactNode } from "react";

import {
  DashboardActionButton,
  DashboardActionRow,
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "../../../../components/ui";
import type { FactoryGraphNodeKind } from "../../lib/draft/factory-graph-draft-types";
import { getFactoryGraphEditorMessages } from "../../messages/editor";

export {
  FactoryGraphEditorModeToggle,
  FactoryGraphEditorStatus,
} from "../chrome/factory-graph-editor-mode-controls";
export { FactoryGraphEditorWorkStatePhaseLegend } from "../chrome/factory-graph-editor-work-state-phase-legend";

import { FactoryGraphEditorHideShowMenu } from "../chrome/factory-graph-editor-hide-show-menu";
import { FactoryGraphEditorModeToggle } from "../chrome/factory-graph-editor-mode-controls";
import { FactoryGraphEditorTooltipActionButton } from "../chrome/factory-graph-editor-tooltip-button";
import { FactoryGraphEditorMenuHeader } from "../menu/factory-graph-editor-menu-header";
import { FactoryGraphEditorMenuItemButton } from "../menu/factory-graph-editor-menu-item-button";
import { FactoryGraphEditorMenuItemCopy } from "../menu/factory-graph-editor-menu-item-copy";
import { FactoryGraphEditorFloatingSurface } from "../surface/factory-graph-editor-floating-surface";
import {
  ConnectIcon,
  RedoIcon,
  ResetLayoutIcon,
  SaveIcon,
  TrashIcon,
  UndoIcon,
} from "../factory-graph-editor-toolbar-icons";

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

const TOOLBAR_ACTIONS_CLASS =
  "flex items-center gap-2 border-l border-outline pl-2 max-md:ml-auto";
// biome-ignore lint/complexity/noExcessiveLinesPerFunction: toolbar composes edit-mode toggle, hide/show, and layout history controls.
export function FactoryGraphEditorToolbar({
  activeTool,
  addMenuActions = [],
  canInteract,
  canRedoLayout = false,
  canSave = false,
  canDiscard = true,
  canUndoLayout = false,
  editModeToggle,
  hasPendingChanges = false,
  hiddenNodeClasses = new Set<FactoryGraphNodeKind>(),
  hideShowMenuOpen = false,
  hideShowVisible = true,
  isSaving = false,
  locale,
  onDiscard,
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
  onSelectTool,
  openAddMenu = false,
}: {
  activeTool: FactoryGraphEditorTool;
  addMenuActions?: FactoryGraphEditorMenuAction[];
  canInteract: boolean;
  canRedoLayout?: boolean;
  canSave?: boolean;
  canDiscard?: boolean;
  canUndoLayout?: boolean;
  editModeToggle?: {
    disabled?: boolean;
    editorMode: boolean;
    hasChanges?: boolean;
    onToggle: () => void;
    tooltipOverride?: string;
  };
  hasPendingChanges?: boolean;
  hiddenNodeClasses?: ReadonlySet<FactoryGraphNodeKind>;
  hideShowMenuOpen?: boolean;
  hideShowVisible?: boolean;
  isSaving?: boolean;
  locale?: string;
  onDiscard?: () => void;
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

  return (
    <FactoryGraphEditorFloatingSurface
      aria-label={messages.toolbarAriaLabel}
      className="px-3 py-2"
      placement="bottomToolbar"
    >
      {editModeToggle ? (
        <FactoryGraphEditorModeToggle
          disabled={editModeToggle.disabled}
          editorMode={editModeToggle.editorMode}
          hasChanges={editModeToggle.hasChanges}
          locale={locale}
          onClick={editModeToggle.onToggle}
          tooltipOverride={editModeToggle.tooltipOverride}
        />
      ) : null}
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
      {visible ? (
        <>
          <FactoryGraphEditorAddMenu
            actions={addMenuActions}
            canInteract={canInteract}
            locale={locale}
            onAction={onAddAction}
            onOpenChange={onAddMenuOpenChange}
            open={openAddMenu}
          />
          <FactoryGraphEditorToolbarButton
            active={activeTool === "delete"}
            description={messages.toolbarDeleteDescription}
            disabled={!canInteract}
            icon={<TrashIcon />}
            label={messages.toolbarDeleteLabel}
            onClick={() =>
              onSelectTool(activeTool === "delete" ? null : "delete")
            }
            tone={activeTool === "delete" ? "secondary" : "outline"}
          />
          <FactoryGraphEditorToolbarButton
            active={activeTool === "connect"}
            description={messages.toolbarConnectDescription}
            disabled={!canInteract}
            icon={<ConnectIcon />}
            label={messages.toolbarConnectLabel}
            onClick={() =>
              onSelectTool(activeTool === "connect" ? null : "connect")
            }
            tone={activeTool === "connect" ? "secondary" : "outline"}
          />
          <FactoryGraphEditorToolbarButton
            active={false}
            description={messages.toolbarUndoDescription}
            disabled={!canInteract || !canUndoLayout}
            icon={<UndoIcon />}
            label={messages.toolbarUndoLabel}
            onClick={() => onUndoLayout?.()}
            tone="outline"
          />
          <FactoryGraphEditorToolbarButton
            active={false}
            description={messages.toolbarRedoDescription}
            disabled={!canInteract || !canRedoLayout}
            icon={<RedoIcon />}
            label={messages.toolbarRedoLabel}
            onClick={() => onRedoLayout?.()}
            tone="outline"
          />
          <FactoryGraphEditorToolbarButton
            active={false}
            description={messages.toolbarResetLayoutDescription}
            disabled={!canInteract}
            icon={<ResetLayoutIcon />}
            label={messages.toolbarResetLayoutLabel}
            onClick={() => onResetLayout?.()}
            tone="outline"
          />
          {hasPendingChanges ? (
            <DashboardActionRow
              actions={
                <>
                  <DashboardActionButton
                    disabled={!canDiscard || isSaving}
                    onClick={onDiscard}
                    tone="outline"
                    type="button"
                  >
                    {messages.draftActionsDiscard}
                  </DashboardActionButton>
                  <FactoryGraphEditorTooltipActionButton
                    aria-label={
                      isSaving
                        ? messages.draftActionsSaving
                        : messages.draftActionsSave
                    }
                    disabled={!canSave || isSaving}
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
              actionsClassName={TOOLBAR_ACTIONS_CLASS}
              className="min-w-0 flex-1"
            />
          ) : null}
        </>
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
