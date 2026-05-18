import type { ReactNode } from "react";

import { DashboardMutationDialog } from "../../components/dashboard";
import {
  Button,
  Popover,
  PopoverContent,
  PopoverTrigger,
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "../../components/ui";
import { cx } from "../../lib/cx";

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
  "pointer-events-auto absolute bottom-4 left-1/2 z-20 flex -translate-x-1/2 items-center gap-2 rounded-full border border-af-overlay/12 bg-af-surface/94 px-3 py-2 shadow-af-panel backdrop-blur-[16px] max-[720px]:bottom-3 max-[720px]:w-[calc(100%-1.5rem)] max-[720px]:justify-between";
const STATUS_PILL_CLASS =
  "inline-flex items-center gap-2 rounded-full border px-3 py-1 text-xs font-semibold";
const MENU_LIST_CLASS = "grid gap-1";
const MENU_ACTION_CLASS =
  "grid w-full gap-1 rounded-[1rem] border border-transparent px-3 py-2 text-left transition hover:border-af-accent/20 hover:bg-af-overlay/6 focus-visible:outline-2 focus-visible:outline-af-accent disabled:cursor-not-allowed disabled:opacity-55";
const MENU_ACTION_LABEL_CLASS = "text-sm font-semibold text-af-ink";
const MENU_ACTION_DESCRIPTION_CLASS = "text-xs leading-5 text-af-ink/68";
const VISIBILITY_PANEL_CLASS =
  "pointer-events-auto absolute right-7 top-24 z-20 grid gap-3 rounded-[1.25rem] border border-af-overlay/12 bg-af-surface/94 p-3 shadow-af-panel backdrop-blur-[16px] max-[720px]:left-4 max-[720px]:right-4 max-[720px]:top-20";
const TOOLTIP_COPY = {
  add: "Add supported graph entities",
  connect: "Create compatible graph connections",
  delete: "Remove nodes or edges from the draft",
} as const;
const NOTICE_TONE_CLASS: Record<FactoryGraphEditorNoticeTone, string> = {
  danger: "border-af-danger/28 bg-af-danger/8 text-af-danger-ink",
  neutral: "border-af-overlay/14 bg-af-overlay/6 text-af-ink/82",
  warning: "border-af-warning/30 bg-af-warning/10 text-af-warning-ink",
};

function EditModeIcon() {
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
      <path d="M12 20h9" />
      <path d="M16.5 3.5a2.12 2.12 0 1 1 3 3L7 19l-4 1 1-4Z" />
    </svg>
  );
}

export function FactoryGraphEditorModeToggle({
  editorMode,
  onClick,
}: {
  editorMode: boolean;
  onClick: () => void;
}) {
  const label = editorMode
    ? "Leave factory graph editor"
    : "Enter factory graph editor";

  return (
    <TooltipProvider delayDuration={120}>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            aria-label={label}
            aria-pressed={editorMode}
            className="shrink-0"
            onClick={onClick}
            size="icon"
            tone={editorMode ? "secondary" : "outline"}
            type="button"
          >
            <EditModeIcon />
          </Button>
        </TooltipTrigger>
        <TooltipContent>{label}</TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}

export function FactoryGraphEditorStatus({
  editorMode,
  hasChanges,
  isDefinitionLoading,
  loadErrorMessage,
}: {
  editorMode: boolean;
  hasChanges: boolean;
  isDefinitionLoading: boolean;
  loadErrorMessage?: string;
}) {
  if (!editorMode) {
    return (
      <p
        className={cx(
          STATUS_PILL_CLASS,
          "border-af-overlay/12 bg-af-overlay/6 text-af-ink/76",
        )}
      >
        Observe mode
      </p>
    );
  }

  if (isDefinitionLoading) {
    return (
      <p
        aria-live="polite"
        className={cx(
          STATUS_PILL_CLASS,
          "border-af-accent/24 bg-af-accent/10 text-af-accent",
        )}
      >
        Loading editor definition
      </p>
    );
  }

  if (loadErrorMessage) {
    return (
      <p
        aria-live="polite"
        className={cx(
          STATUS_PILL_CLASS,
          "border-af-danger/30 bg-af-danger/8 text-af-danger-ink",
        )}
        role="status"
      >
        Editor unavailable: {loadErrorMessage}
      </p>
    );
  }

  return (
    <p
      aria-live="polite"
      className={cx(
        STATUS_PILL_CLASS,
        hasChanges
          ? "border-af-warning/30 bg-af-warning/10 text-af-warning-ink"
          : "border-af-accent/24 bg-af-accent/10 text-af-accent",
      )}
      role="status"
    >
      {hasChanges ? "Unsaved graph changes" : "Editor mode active"}
    </p>
  );
}

export function FactoryGraphEditorToolbar({
  activeTool,
  addMenuActions = [],
  canInteract,
  hasPendingChanges = false,
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
  onAddAction?: (actionID: string) => void;
  onAddMenuOpenChange?: (open: boolean) => void;
  visible: boolean;
  onSelectTool: (tool: FactoryGraphEditorTool) => void;
  openAddMenu?: boolean;
}) {
  if (!visible) {
    return null;
  }

  return (
    <section
      aria-label="Factory graph editor tools"
      className={TOOLBAR_SHELL_CLASS}
    >
      <TooltipProvider delayDuration={120}>
        <FactoryGraphEditorToolbarButton
          active={activeTool === "add"}
          description={TOOLTIP_COPY.add}
          disabled={!canInteract}
          label="Add"
          onClick={() => onSelectTool(activeTool === "add" ? null : "add")}
          tone={activeTool === "add" ? "secondary" : "outline"}
        />
        <FactoryGraphEditorAddMenu
          actions={addMenuActions}
          canInteract={canInteract}
          onAction={onAddAction}
          onOpenChange={onAddMenuOpenChange}
          open={openAddMenu}
        />
        <FactoryGraphEditorToolbarButton
          active={activeTool === "delete"}
          description={TOOLTIP_COPY.delete}
          disabled={!canInteract}
          label="Delete"
          onClick={() =>
            onSelectTool(activeTool === "delete" ? null : "delete")
          }
          tone={activeTool === "delete" ? "secondary" : "outline"}
        />
        <FactoryGraphEditorToolbarButton
          active={activeTool === "connect"}
          description={TOOLTIP_COPY.connect}
          disabled={!canInteract}
          label="Connect"
          onClick={() =>
            onSelectTool(activeTool === "connect" ? null : "connect")
          }
          tone={activeTool === "connect" ? "secondary" : "outline"}
        />
      </TooltipProvider>
      <p
        aria-live="polite"
        className={cx(
          STATUS_PILL_CLASS,
          hasPendingChanges
            ? "border-af-warning/30 bg-af-warning/10 text-af-warning-ink"
            : "border-af-overlay/12 bg-af-overlay/4 text-af-ink/70",
        )}
      >
        {hasPendingChanges ? "Draft changes pending" : "No draft changes"}
      </p>
    </section>
  );
}

export function FactoryGraphEditorVisibilityPanel({
  onToggle,
  options,
  visible,
}: {
  onToggle: (key: FactoryGraphEditorVisibilityOption["key"]) => void;
  options: FactoryGraphEditorVisibilityOption[];
  visible: boolean;
}) {
  if (!visible || options.length === 0) {
    return null;
  }

  return (
    <section
      aria-label="Factory graph density controls"
      className={VISIBILITY_PANEL_CLASS}
    >
      <div className="grid gap-1">
        <p className="m-0 text-sm font-semibold text-af-ink">Dense graph</p>
        <p className="m-0 text-xs leading-5 text-af-ink/68">
          Collapse worker or resource lanes to focus on the rest of the
          topology while editing.
        </p>
      </div>
      <div className="grid gap-2">
        {options.map((option) => (
          <button
            aria-label={`${option.visible ? "Hide" : "Show"} ${option.label.toLowerCase()} lane`}
            aria-pressed={option.visible}
            className={cx(
              "flex items-center justify-between gap-3 rounded-[1rem] border px-3 py-2 text-left transition focus-visible:outline-2 focus-visible:outline-af-accent",
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
                {option.visible ? "Visible" : "Collapsed"}
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
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          aria-pressed={active}
          disabled={disabled}
          onClick={onClick}
          size="sm"
          tone={tone}
          type="button"
        >
          {label}
        </Button>
      </TooltipTrigger>
      <TooltipContent>{description}</TooltipContent>
    </Tooltip>
  );
}

function FactoryGraphEditorAddMenu({
  actions,
  canInteract,
  onAction,
  onOpenChange,
  open,
}: {
  actions: FactoryGraphEditorMenuAction[];
  canInteract: boolean;
  onAction?: (actionID: string) => void;
  onOpenChange?: (open: boolean) => void;
  open: boolean;
}) {
  if (actions.length === 0 || onAction === undefined) {
    return null;
  }

  return (
    <Popover onOpenChange={onOpenChange} open={open}>
      <PopoverTrigger asChild>
        <Button
          aria-label="Open add entity menu"
          disabled={!canInteract}
          size="icon"
          tone="ghost"
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
        </Button>
      </PopoverTrigger>
      <PopoverContent
        align="start"
        aria-label="Add graph entity menu"
        className="grid gap-2"
      >
        <div className="grid gap-1">
          <p className="m-0 text-sm font-semibold text-af-ink">
            Add graph entity
          </p>
          <p className="m-0 text-xs leading-5 text-af-ink/68">
            Choose a supported entity to add to the current draft.
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
      className={cx(
        "grid gap-1 rounded-[1.25rem] border p-4",
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

export function FactoryGraphEditorLeaveDialog({
  canSave,
  isOpen,
  isSaving,
  onCancel,
  onDiscard,
  onSave,
}: {
  canSave: boolean;
  isOpen: boolean;
  isSaving: boolean;
  onCancel: () => void;
  onDiscard: () => void;
  onSave: () => void;
}) {
  if (!isOpen) {
    return null;
  }

  return (
    <DashboardMutationDialog
      closeDisabled={isSaving}
      description="This graph editor session still has local topology changes."
      onClose={onCancel}
      title="Leave graph editor with unsaved changes?"
      footer={
        <>
          <Button
            disabled={isSaving}
            onClick={onCancel}
            tone="outline"
            type="button"
          >
            Keep editing
          </Button>
          <Button
            disabled={isSaving}
            onClick={onDiscard}
            tone="ghost"
            type="button"
          >
            Discard changes
          </Button>
          <Button
            disabled={!canSave || isSaving}
            onClick={onSave}
            type="button"
          >
            {isSaving ? "Saving..." : "Save changes"}
          </Button>
        </>
      }
    >
      <p className="m-0 text-sm text-af-ink/78">
        Save to keep the pending factory topology, discard to revert to the
        latest server-backed graph, or keep editing.
      </p>
    </DashboardMutationDialog>
  );
}
