import { DashboardMutationDialog } from "../../components/dashboard";
import { Button } from "../../components/ui";
import { cx } from "../../lib/cx";

export type FactoryGraphEditorTool = "add" | "connect" | "delete" | null;

const TOOLBAR_SHELL_CLASS =
  "pointer-events-auto absolute bottom-4 left-1/2 z-20 flex -translate-x-1/2 items-center gap-2 rounded-full border border-af-overlay/12 bg-af-surface/94 px-3 py-2 shadow-af-panel backdrop-blur-[16px] max-[720px]:bottom-3 max-[720px]:w-[calc(100%-1.5rem)] max-[720px]:justify-between";
const STATUS_PILL_CLASS =
  "inline-flex items-center gap-2 rounded-full border px-3 py-1 text-xs font-semibold";

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
    <Button
      aria-label={label}
      aria-pressed={editorMode}
      className="shrink-0"
      onClick={onClick}
      size="icon"
      title={label}
      tone={editorMode ? "secondary" : "outline"}
      type="button"
    >
      <EditModeIcon />
    </Button>
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
  canInteract,
  visible,
  onSelectTool,
}: {
  activeTool: FactoryGraphEditorTool;
  canInteract: boolean;
  visible: boolean;
  onSelectTool: (tool: FactoryGraphEditorTool) => void;
}) {
  if (!visible) {
    return null;
  }

  return (
    <section
      aria-label="Factory graph editor tools"
      className={TOOLBAR_SHELL_CLASS}
    >
      {(
        [
          ["add", "Add"],
          ["delete", "Delete"],
          ["connect", "Connect"],
        ] as const
      ).map(([tool, label]) => (
        <Button
          aria-pressed={activeTool === tool}
          disabled={!canInteract}
          key={tool}
          onClick={() =>
            onSelectTool(activeTool === tool ? null : tool)
          }
          size="sm"
          tone={activeTool === tool ? "secondary" : "outline"}
          type="button"
        >
          {label}
        </Button>
      ))}
    </section>
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
