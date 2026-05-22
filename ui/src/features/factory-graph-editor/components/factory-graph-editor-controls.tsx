import type { ReactNode } from "react";

import { DashboardMutationDialog } from "../../../components/dashboard";
import {
  Button,
  Popover,
  PopoverContent,
  PopoverTrigger,
  buttonVariants,
} from "../../../components/ui";
import { cn } from "../../../lib/cn";
import { getFactoryGraphEditorMessages } from "../messages/editor";
export {
  FactoryGraphEditorModeToggle,
  FactoryGraphEditorStatus,
} from "./factory-graph-editor-mode-controls";
import { FactoryGraphEditorTooltipButton } from "./factory-graph-editor-tooltip-button";

export type FactoryGraphEditorTool = "add" | "connect" | "delete" | null;
export type FactoryGraphEditorNoticeTone = "danger" | "neutral" | "warning";

export interface FactoryGraphEditorMenuAction {
  description?: string;
  disabled?: boolean;
  id: string;
  label: string;
}

export interface FactoryGraphEditorVisibilityOption {
  count: number;
  key: "resources" | "workers";
  label: string;
  visible: boolean;
}

const TOOLBAR_SHELL_CLASS =
  "pointer-events-auto absolute bottom-4 left-1/2 z-20 flex -translate-x-1/2 items-center gap-2 rounded-full border border-af-overlay/12 bg-af-surface/94 px-3 py-2 shadow-af-panel backdrop-blur-[16px] max-md:bottom-3 max-md:left-4 max-md:right-4 max-md:translate-x-0 max-md:justify-between";
const MENU_LIST_CLASS = "grid gap-1";
const MENU_ACTION_CLASS =
  "grid w-full gap-1 rounded-2xl border border-transparent px-3 py-2 text-left transition hover:border-af-accent/20 hover:bg-af-overlay/6 focus-visible:outline-2 focus-visible:outline-af-accent disabled:cursor-not-allowed disabled:opacity-55";
const MENU_ACTION_LABEL_CLASS = "text-sm font-semibold text-af-ink";
const MENU_ACTION_DESCRIPTION_CLASS = "text-xs leading-5 text-af-ink/68";
const VISIBILITY_PANEL_CLASS =
  "pointer-events-auto absolute right-7 top-24 z-20 grid gap-3 rounded-2xl border border-af-overlay/12 bg-af-surface/94 p-3 shadow-af-panel backdrop-blur-[16px] max-md:left-4 max-md:right-4 max-md:top-20";
const NOTICE_TONE_CLASS: Record<FactoryGraphEditorNoticeTone, string> = {
  danger: "border-af-danger/28 bg-af-danger/8 text-af-danger-ink",
  neutral: "border-af-overlay/14 bg-af-overlay/6 text-af-ink/82",
  warning: "border-af-warning/30 bg-af-warning/10 text-af-warning-ink",
};

const STATUS_PILL_CLASS =
  "inline-flex items-center gap-2 rounded-full border px-3 py-1 text-xs font-semibold";

export function FactoryGraphEditorToolbar({
  activeTool,
  addMenuActions = [],
  canInteract,
  hasPendingChanges = false,
  locale,
  onAddAction,
  onAddMenuOpenChange,
  visible,
  onSelectTool,
  openAddMenu = false,
}: {
  activeTool: FactoryGraphEditorTool;
  addMenuActions?: FactoryGraphEditorMenuAction[];
  canInteract: boolean;
  hasPendingChanges?: boolean;
  locale?: string;
  onAddAction?: (actionID: string) => void;
  onAddMenuOpenChange?: (open: boolean) => void;
  visible: boolean;
  onSelectTool: (tool: FactoryGraphEditorTool) => void;
  openAddMenu?: boolean;
}) {
  if (!visible) {
    return null;
  }
  const messages = getFactoryGraphEditorMessages(locale);

  return (
    <section
      aria-label={messages.toolbarAriaLabel}
      className={TOOLBAR_SHELL_CLASS}
    >
      <FactoryGraphEditorToolbarButton
        active={activeTool === "add"}
        description={messages.toolbarAddDescription}
        disabled={!canInteract}
        label={messages.toolbarAddLabel}
        onClick={() => onSelectTool(activeTool === "add" ? null : "add")}
        tone={activeTool === "add" ? "secondary" : "outline"}
      />
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
        label={messages.toolbarDeleteLabel}
        onClick={() => onSelectTool(activeTool === "delete" ? null : "delete")}
        tone={activeTool === "delete" ? "secondary" : "outline"}
      />
      <FactoryGraphEditorToolbarButton
        active={activeTool === "connect"}
        description={messages.toolbarConnectDescription}
        disabled={!canInteract}
        label={messages.toolbarConnectLabel}
        onClick={() =>
          onSelectTool(activeTool === "connect" ? null : "connect")
        }
        tone={activeTool === "connect" ? "secondary" : "outline"}
      />
      <p
        aria-live="polite"
        className={cn(
          STATUS_PILL_CLASS,
          hasPendingChanges
            ? "border-af-warning/30 bg-af-warning/10 text-af-warning-ink"
            : "border-af-overlay/12 bg-af-overlay/4 text-af-ink/70",
        )}
      >
        {hasPendingChanges
          ? messages.toolbarPendingChanges
          : messages.toolbarNoPendingChanges}
      </p>
    </section>
  );
}

export function FactoryGraphEditorVisibilityPanel({
  locale,
  onToggle,
  options,
  visible,
}: {
  locale?: string;
  onToggle: (key: FactoryGraphEditorVisibilityOption["key"]) => void;
  options: FactoryGraphEditorVisibilityOption[];
  visible: boolean;
}) {
  if (!visible || options.length === 0) {
    return null;
  }
  const messages = getFactoryGraphEditorMessages(locale);

  return (
    <section
      aria-label={messages.toolbarVisibilityAriaLabel}
      className={VISIBILITY_PANEL_CLASS}
    >
      <div className="grid gap-1">
        <p className="m-0 text-sm font-semibold text-af-ink">
          {messages.denseGraphTitle}
        </p>
        <p className="m-0 text-xs leading-5 text-af-ink/68">
          {messages.toolbarVisibilityDescription}
        </p>
      </div>
      <div className="grid gap-2">
        {options.map((option) => (
          <button
            aria-label={messages.toolbarVisibilityToggleLabel(
              option.visible,
              option.label,
            )}
            aria-pressed={option.visible}
            className={cn(
              "flex items-center justify-between gap-3 rounded-2xl border px-3 py-2 text-left transition focus-visible:outline-2 focus-visible:outline-af-accent",
              option.visible
                ? "border-af-accent/20 bg-af-accent/8 text-af-ink"
                : "border-af-overlay/12 bg-af-overlay/4 text-af-ink/72",
            )}
            key={option.key}
            onClick={() => onToggle(option.key)}
            type="button"
          >
            <span className="grid gap-0.5">
              <span className="text-sm font-semibold">{option.label}</span>
              <span className="text-xs opacity-80">
                {option.visible
                  ? messages.stateVisible
                  : messages.stateCollapsed}
              </span>
            </span>
            <span className="rounded-full border border-current/15 px-2 py-1 text-[0.65rem] font-semibold uppercase tracking-[0.08em]">
              {option.count}
            </span>
          </button>
        ))}
      </div>
    </section>
  );
}

function FactoryGraphEditorToolbarButton({
  active,
  description,
  disabled,
  label,
  onClick,
  tone,
}: {
  active: boolean;
  description: string;
  disabled: boolean;
  label: string;
  onClick: () => void;
  tone: "outline" | "secondary";
}) {
  return (
    <FactoryGraphEditorTooltipButton
      aria-pressed={active}
      className={buttonVariants({ size: "sm", tone })}
      disabled={disabled}
      onClick={onClick}
      tooltip={description}
      type="button"
    >
      {label}
    </FactoryGraphEditorTooltipButton>
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
      <PopoverTrigger
        aria-label={messages.toolbarOpenAddMenuLabel}
        className={buttonVariants({ size: "icon", tone: "ghost" })}
        disabled={!canInteract}
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
      </PopoverTrigger>
      <PopoverContent
        align="start"
        aria-label={messages.toolbarVisibilityMenuAriaLabel}
        className="grid gap-2"
      >
        <div className="grid gap-1">
          <p className="m-0 text-sm font-semibold text-af-ink">
            {messages.toolbarVisibilityMenuTitle}
          </p>
          <p className="m-0 text-xs leading-5 text-af-ink/68">
            {messages.toolbarVisibilityMenuDescription}
          </p>
        </div>
        <div className={MENU_LIST_CLASS}>
          {actions.map((action) => (
            <button
              aria-label={action.label}
              className={MENU_ACTION_CLASS}
              disabled={action.disabled}
              key={action.id}
              onClick={() => {
                onAction(action.id);
                onOpenChange?.(false);
              }}
              type="button"
            >
              <span className={MENU_ACTION_LABEL_CLASS}>{action.label}</span>
              {action.description ? (
                <span className={MENU_ACTION_DESCRIPTION_CLASS}>
                  {action.description}
                </span>
              ) : null}
            </button>
          ))}
        </div>
      </PopoverContent>
    </Popover>
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
          <p className="m-0 text-sm font-semibold text-af-ink">{title}</p>
          {description ? (
            <p className="m-0 text-xs leading-5 text-af-ink/68">
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
  if (!isOpen) {
    return null;
  }

  return (
    <DashboardMutationDialog
      closeDisabled={isBusy}
      description={description}
      onClose={onCancel}
      title={title}
      footer={
        <>
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
        </>
      }
    >
      <div />
    </DashboardMutationDialog>
  );
}
