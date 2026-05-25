import { DashboardStatusPill } from "../../../components/ui";
import { cn } from "../../../lib/cn";
import { getFactoryGraphEditorMessages } from "../messages/editor";
import { FactoryGraphEditorTooltipActionButton } from "./factory-graph-editor-tooltip-button";

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
    <FactoryGraphEditorTooltipActionButton
      aria-label={label}
      aria-pressed={editorMode}
      className={cn("shrink-0", disabled && "cursor-not-allowed", className)}
      disabled={disabled}
      iconOnly
      onClick={onClick}
      tooltip={label}
      tone={editorMode ? "secondary" : "outline"}
      type="button"
    >
      <EditModeIcon />
    </FactoryGraphEditorTooltipActionButton>
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
      <DashboardStatusPill
        aria-live="polite"
        className={className}
        role="status"
        tone="danger"
      >
        {messages.modeUnavailablePrefix}: {editorUnavailableReason}
      </DashboardStatusPill>
    );
  }

  if (!editorMode) {
    return (
      <DashboardStatusPill className={className} tone="neutral">
        {messages.modeObserve}
      </DashboardStatusPill>
    );
  }

  if (isDefinitionLoading) {
    return (
      <DashboardStatusPill
        aria-live="polite"
        className={className}
        tone="active"
      >
        {messages.modeLoadingDefinition}
      </DashboardStatusPill>
    );
  }

  if (loadErrorMessage) {
    return (
      <DashboardStatusPill
        aria-live="polite"
        className={className}
        role="status"
        tone="danger"
      >
        {messages.modeUnavailablePrefix}: {loadErrorMessage}
      </DashboardStatusPill>
    );
  }

  return (
    <DashboardStatusPill
      aria-live="polite"
      className={className}
      role="status"
      tone={hasChanges ? "warning" : "active"}
    >
      {hasChanges ? messages.modeUnsavedChanges : messages.modeActive}
    </DashboardStatusPill>
  );
}
