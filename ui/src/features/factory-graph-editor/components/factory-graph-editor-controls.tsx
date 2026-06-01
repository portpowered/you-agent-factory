import type { ReactNode } from "react";

import {
  Button,
  DashboardActionButton,
  DashboardActionRow,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "../../../components/ui";
import { cn } from "../../../lib/cn";
import type { FactoryGraphNodeKind } from "../lib/factory-graph-draft-types";
import { getFactoryGraphEditorMessages } from "../messages/editor";

export {
  FactoryGraphEditorModeToggle,
  FactoryGraphEditorStatus,
} from "./factory-graph-editor-mode-controls";
export { FactoryGraphEditorWorkStatePhaseLegend } from "./factory-graph-editor-work-state-phase-legend";

import { FactoryGraphEditorHideShowMenu } from "./factory-graph-editor-hide-show-menu";
import { FactoryGraphEditorTooltipActionButton } from "./factory-graph-editor-tooltip-button";

export type FactoryGraphEditorTool = "add" | "connect" | "delete" | null;
export type FactoryGraphEditorNoticeTone = "danger" | "neutral" | "warning";
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
  "flex items-center gap-2 border-l border-af-border pl-2 max-md:ml-auto";
const NOTICE_TONE_CLASS: Record<FactoryGraphEditorNoticeTone, string> = {
  danger: "border-af-danger-border bg-af-danger-surface text-af-danger-text",
  neutral: "border-af-border bg-af-surface-subtle text-af-text-muted",
  warning:
    "border-af-warning-border bg-af-warning-surface text-af-warning-text",
};

export function FactoryGraphEditorToolbar({
  activeTool,
  addMenuActions = [],
  canInteract,
  canSave = false,
  canDiscard = true,
  hasPendingChanges = false,
  hiddenNodeClasses = new Set<FactoryGraphNodeKind>(),
  hideShowMenuOpen = false,
  hideShowVisible = true,
  isSaving = false,
  locale,
  onDiscard,
  onAddAction,
  onAddMenuOpenChange,
  onHideShowMenuOpenChange,
  onSave,
  onToggleHiddenNodeClass,
  saveDisabledReason,
  visible,
  onSelectTool,
  openAddMenu = false,
}: {
  activeTool: FactoryGraphEditorTool;
  addMenuActions?: FactoryGraphEditorMenuAction[];
  canInteract: boolean;
  canSave?: boolean;
  canDiscard?: boolean;
  hasPendingChanges?: boolean;
  hiddenNodeClasses?: ReadonlySet<FactoryGraphNodeKind>;
  hideShowMenuOpen?: boolean;
  hideShowVisible?: boolean;
  isSaving?: boolean;
  locale?: string;
  onDiscard?: () => void;
  onAddAction?: (actionID: string) => void;
  onAddMenuOpenChange?: (open: boolean) => void;
  onHideShowMenuOpenChange?: (open: boolean) => void;
  onSave?: () => void;
  onToggleHiddenNodeClass?: (kind: FactoryGraphNodeKind) => void;
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
    <section
      aria-label={messages.toolbarAriaLabel}
      className="pointer-events-auto absolute bottom-4 left-1/2 z-20 flex -translate-x-1/2 items-center gap-2 rounded-full border border-af-border bg-af-surface-raised px-3 py-2 shadow-af-panel backdrop-blur-[16px] max-md:bottom-3 max-md:left-4 max-md:right-4 max-md:flex-wrap max-md:justify-start max-md:gap-1.5 max-md:translate-x-0"
    >
      {hideShowVisible && onToggleHiddenNodeClass ? (
        <FactoryGraphEditorHideShowMenu
          hiddenNodeClasses={hiddenNodeClasses}
          locale={locale}
          onOpenChange={onHideShowMenuOpenChange}
          onToggleHiddenNodeClass={onToggleHiddenNodeClass}
          open={hideShowMenuOpen}
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
                    tone={canSave && !isSaving ? "default" : "outline"}
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
    </section>
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
    <section
      aria-label={messages.visibilityPresetsAriaLabel}
      className="pointer-events-auto absolute right-7 top-7 z-20 flex flex-wrap items-center gap-2 rounded-full border border-af-border bg-af-surface-raised px-2 py-2 shadow-af-panel backdrop-blur-[16px] max-md:left-4 max-md:right-4 max-md:top-4"
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
    </section>
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
        <DashboardActionButton
          aria-label={messages.toolbarOpenAddMenuLabel}
          disabled={!canInteract}
          iconOnly
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
        </DashboardActionButton>
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
        <div className="grid gap-1">
          <p className="m-0 text-sm font-semibold text-af-text">
            {messages.toolbarVisibilityMenuTitle}
          </p>
          <p className="m-0 text-xs leading-5 text-af-text-muted">
            {messages.toolbarVisibilityMenuDescription}
          </p>
        </div>
        <div className="grid gap-1">
          {actions.map((action) => (
            <DashboardActionButton
              aria-label={action.label}
              className="min-h-0 w-full justify-start rounded-2xl border-transparent px-3 py-2 text-left [&>span]:grid [&>span]:w-full [&>span]:justify-items-start"
              disabled={action.disabled}
              key={action.id}
              onClick={() => {
                onAction(action.id);
                onOpenChange?.(false);
              }}
              tone="ghost"
              type="button"
            >
              <span className="text-sm font-semibold text-af-text">
                {action.label}
              </span>
              {action.description ? (
                <span className="text-xs leading-5 text-af-text-muted">
                  {action.description}
                </span>
              ) : null}
            </DashboardActionButton>
          ))}
        </div>
      </PopoverContent>
    </Popover>
  );
}

function ConnectIcon() {
  return (
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
      <path d="M7 7h3v3" />
      <path d="M14 14h3v3" />
      <path d="M10 7H7v10h10v-3" />
      <path d="M10 14 17 7" />
    </svg>
  );
}

function TrashIcon() {
  return (
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
      <path d="M4 7h16" />
      <path d="M9 7V5h6v2" />
      <path d="M8 7v11a1 1 0 0 0 1 1h6a1 1 0 0 0 1-1V7" />
      <path d="M10 11v5" />
      <path d="M14 11v5" />
    </svg>
  );
}

function SaveIcon() {
  return (
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
      <path d="M5 4h11l3 3v13H5z" />
      <path d="M9 4v6h6V4" />
      <path d="M9 20v-6h6v6" />
    </svg>
  );
}

export function FactoryGraphEditorActionPopover({
  children,
  description,
  onOpenChange,
  open,
  title,
  trigger,
}: {
  children: ReactNode;
  description?: string;
  onOpenChange: (open: boolean) => void;
  open: boolean;
  title: string;
  trigger: ReactNode;
}) {
  return (
    <Popover onOpenChange={onOpenChange} open={open}>
      <PopoverTrigger asChild>{trigger}</PopoverTrigger>
      <PopoverContent align="start" className="grid gap-3" sideOffset={12}>
        <div className="grid gap-1">
          <p className="m-0 text-sm font-semibold text-af-text">{title}</p>
          {description ? (
            <p className="m-0 text-xs leading-5 text-af-text-muted">
              {description}
            </p>
          ) : null}
        </div>
        {children}
      </PopoverContent>
    </Popover>
  );
}

export function FactoryGraphEditorNotice({
  children,
  title,
  tone = "neutral",
}: {
  children: ReactNode;
  title: string;
  tone?: FactoryGraphEditorNoticeTone;
}) {
  return (
    <section
      className={cn(
        "grid gap-1 rounded-2xl border p-4",
        NOTICE_TONE_CLASS[tone],
      )}
      role={tone === "danger" ? "alert" : "status"}
    >
      <h3 className="m-0 text-sm font-semibold">{title}</h3>
      <p className="m-0 text-sm leading-6">{children}</p>
    </section>
  );
}

export function FactoryGraphEditorConfirmationDialog({
  cancelLabel,
  confirmLabel,
  confirmTone = "default",
  description,
  isBusy = false,
  isOpen,
  onCancel,
  onConfirm,
  title,
}: {
  cancelLabel: string;
  confirmLabel: string;
  confirmTone?: "default" | "destructive";
  description: string;
  isBusy?: boolean;
  isOpen: boolean;
  onCancel: () => void;
  onConfirm: () => void;
  title: string;
}) {
  const handleOpenChange = (open: boolean) => {
    if (!open && !isBusy) {
      onCancel();
    }
  };

  return (
    <Dialog onOpenChange={handleOpenChange} open={isOpen}>
      <DialogContent
        closeDisabled={isBusy}
        onEscapeKeyDown={(event) => {
          if (isBusy) {
            event.preventDefault();
          }
        }}
        onInteractOutside={(event) => {
          if (isBusy) {
            event.preventDefault();
          }
        }}
      >
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button
            disabled={isBusy}
            onClick={onCancel}
            tone="outline"
            type="button"
          >
            {cancelLabel}
          </Button>
          <Button onClick={onConfirm} tone={confirmTone} type="button">
            {confirmLabel}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
