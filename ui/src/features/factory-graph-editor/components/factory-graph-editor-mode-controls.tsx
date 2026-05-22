import { buttonVariants } from "../../../components/ui";
import { cn } from "../../../lib/cn";
import { getFactoryGraphEditorMessages } from "../messages/editor";
import { FactoryGraphEditorTooltipButton } from "./factory-graph-editor-tooltip-button";

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
  className,
  disabled = false,
  editorMode,
  locale,
  onClick,
  tooltipOverride,
}: {
  className?: string;
  disabled?: boolean;
  editorMode: boolean;
  locale?: string;
  onClick: () => void;
  tooltipOverride?: string;
}) {
  const messages = getFactoryGraphEditorMessages(locale);
  const label =
    tooltipOverride ??
    (editorMode ? messages.modeLeaveEditor : messages.modeEnterEditor);

  return (
    <FactoryGraphEditorTooltipButton
      aria-label={label}
      aria-pressed={editorMode}
      className={buttonVariants({
        className: cn(
          "shrink-0",
          disabled && "cursor-not-allowed opacity-60",
          className,
        ),
        size: "icon",
        tone: editorMode ? "secondary" : "outline",
      })}
      disabled={disabled}
      onClick={onClick}
      tooltip={label}
      type="button"
    >
      <EditModeIcon />
    </FactoryGraphEditorTooltipButton>
  );
}

export function FactoryGraphEditorStatus({
  className,
  editorMode,
  editorUnavailableReason,
  hasChanges,
  isDefinitionLoading,
  locale,
  loadErrorMessage,
}: {
  className?: string;
  editorMode: boolean;
  editorUnavailableReason?: string;
  hasChanges: boolean;
  isDefinitionLoading: boolean;
  locale?: string;
  loadErrorMessage?: string;
}) {
  const messages = getFactoryGraphEditorMessages(locale);
  if (editorUnavailableReason) {
    return (
      <p
        aria-live="polite"
        className={cn(
          STATUS_PILL_CLASS,
          className,
          "border-af-danger/30 bg-af-danger/8 text-af-danger-ink",
        )}
        role="status"
      >
        {messages.modeUnavailablePrefix}: {editorUnavailableReason}
      </p>
    );
  }

  if (!editorMode) {
    return (
      <p
        className={cn(
          STATUS_PILL_CLASS,
          className,
          "border-af-overlay/12 bg-af-overlay/6 text-af-ink/76",
        )}
      >
        {messages.modeObserve}
      </p>
    );
  }

  if (isDefinitionLoading) {
    return (
      <p
        aria-live="polite"
        className={cn(
          STATUS_PILL_CLASS,
          className,
          "border-af-accent/24 bg-af-accent/10 text-af-accent",
        )}
      >
        {messages.modeLoadingDefinition}
      </p>
    );
  }

  if (loadErrorMessage) {
    return (
      <p
        aria-live="polite"
        className={cn(
          STATUS_PILL_CLASS,
          className,
          "border-af-danger/30 bg-af-danger/8 text-af-danger-ink",
        )}
        role="status"
      >
        {messages.modeUnavailablePrefix}: {loadErrorMessage}
      </p>
    );
  }

  return (
    <p
      aria-live="polite"
      className={cn(
        STATUS_PILL_CLASS,
        className,
        hasChanges
          ? "border-af-warning/30 bg-af-warning/10 text-af-warning-ink"
          : "border-af-accent/24 bg-af-accent/10 text-af-accent",
      )}
      role="status"
    >
      {hasChanges ? messages.modeUnsavedChanges : messages.modeActive}
    </p>
  );
}
