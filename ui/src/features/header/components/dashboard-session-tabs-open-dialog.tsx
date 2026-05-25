import {
  type FormEvent,
  useId,
} from "react";

import type {
  FactorySessionTarget,
  FactorySessionsAPIError,
} from "../../../api/factory-sessions";
import {
  Button,
  DialogContent,
  DialogHeader,
  DialogTitle,
  Input,
} from "../../../components/ui";
import { DASHBOARD_BODY_TEXT_CLASS } from "../../../components/ui/dashboard-typography";
import { cn } from "../../../lib/cn";
import {
  factorySessionTargetOptionValue,
  type FolderValidationState,
  folderValidationStatusMessage,
  selectedFactorySessionTarget,
} from "../lib/dashboard-session-tabs-utils";
import type { getHeaderControlsMessages } from "../messages/header-controls";

const SESSION_SECTION_LABEL_CLASS =
  "text-xs uppercase tracking-[0.18em] text-af-text-subtle";
const SESSION_DIALOG_ERROR_CLASS =
  "rounded-xl border border-af-danger-border bg-af-danger-surface px-3 py-2 text-sm text-af-danger-text";
const SESSION_DIALOG_STATUS_CLASS =
  "rounded-xl border border-af-accent-border bg-af-accent-surface px-3 py-2 text-sm text-af-text";
const SESSION_TARGET_PICKER_CLASS =
  "grid gap-3 rounded-2xl border border-af-border bg-af-surface-subtle p-4";
const SESSION_TARGET_BUTTON_CLASS =
  "flex min-h-16 w-full flex-col items-start justify-center rounded-xl border border-af-accent bg-af-accent px-4 py-3 text-left text-sm text-af-on-accent shadow-sm transition-colors hover:border-af-accent-hover hover:bg-af-accent-hover focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-af-focus-ring";
const SESSION_TARGET_LIST_CLASS = "grid gap-3";

export function OpenSessionDialog({
  dialogError,
  discoveredTargets,
  folderValidation,
  folderPath,
  isPending,
  messages,
  onChangeFolderPath,
  onInspectFolder,
  onOpenTarget,
  selectedTargetValue,
}: {
  dialogError: FactorySessionsAPIError | null;
  discoveredTargets: FactorySessionTarget[];
  folderValidation: FolderValidationState;
  folderPath: string;
  isPending: boolean;
  messages: ReturnType<typeof getHeaderControlsMessages>;
  onChangeFolderPath: (value: string) => void;
  onInspectFolder: (event: FormEvent<HTMLFormElement>) => void;
  onOpenTarget: (targetValue?: string) => void;
  selectedTargetValue: string;
}) {
  const folderFieldID = useId();
  const folderHelperTextID = useId();
  const dialogDescriptionID = useId();
  const hasFolderCandidate = folderPath.trim().length > 0;
  const validationStatusMessage =
    folderValidation.status === "ready"
      ? null
      : folderValidationStatusMessage(folderValidation, messages);

  return (
    <DialogContent
      aria-describedby={dialogDescriptionID}
      closeDisabled={isPending}
    >
      <DialogHeader>
        <DialogTitle>{messages.openSessionDialogTitle}</DialogTitle>
        <p
          className={cn("text-sm leading-6 text-af-text-muted", DASHBOARD_BODY_TEXT_CLASS)}
          id={dialogDescriptionID}
        >
          {messages.openSessionDialogDescription}
        </p>
      </DialogHeader>
      <form className="grid gap-4" onSubmit={onInspectFolder}>
        <div className="grid gap-2">
          <label className={SESSION_SECTION_LABEL_CLASS} htmlFor={folderFieldID}>
            {messages.sessionFolderFieldLabel}
          </label>
          <div className="flex flex-col gap-2 sm:flex-row">
            <Input
              autoFocus
              className="flex-1"
              disabled={isPending}
              id={folderFieldID}
              aria-describedby={folderHelperTextID}
              onChange={(event) => {
                onChangeFolderPath(event.target.value);
              }}
              placeholder={messages.sessionFolderFieldPlaceholder}
              value={folderPath}
            />
          </div>
          <p
            className={cn("text-sm text-af-text-muted", DASHBOARD_BODY_TEXT_CLASS)}
            id={folderHelperTextID}
          >
            {messages.sessionFolderHelperText}
          </p>
        </div>
        {validationStatusMessage ? (
          <p
            className={
              folderValidation.status === "error"
                ? SESSION_DIALOG_ERROR_CLASS
                : SESSION_DIALOG_STATUS_CLASS
            }
            aria-live="polite"
            role={folderValidation.status === "error" ? "alert" : "status"}
          >
            {validationStatusMessage}
          </p>
        ) : null}
        {dialogError && folderValidation.status !== "error" ? (
          <p className={SESSION_DIALOG_ERROR_CLASS} role="alert">
            {dialogError.message}
          </p>
        ) : null}
        {folderValidation.status !== "ready" ? (
          <div className="flex justify-end">
            <Button
              disabled={isPending || !hasFolderCandidate}
              type="submit"
            >
              {isPending
                ? messages.openSessionSubmitPendingLabel
                : messages.openSessionSubmitLabel}
            </Button>
          </div>
        ) : null}
      </form>
      {folderValidation.status === "ready" && discoveredTargets.length > 0 ? (
        <SessionTargetPicker
          isPending={isPending}
          onOpenTarget={onOpenTarget}
          selectedTargetValue={selectedTargetValue}
          targets={discoveredTargets}
        />
      ) : null}
    </DialogContent>
  );
}

function SessionTargetPicker({
  isPending,
  onOpenTarget,
  selectedTargetValue,
  targets,
}: {
  isPending: boolean;
  onOpenTarget: (targetValue?: string) => void;
  selectedTargetValue: string;
  targets: FactorySessionTarget[];
}) {
  const selectedTarget = selectedFactorySessionTarget(targets, selectedTargetValue)
    ?? (targets.length === 1 ? targets[0] : null);

  return (
    <section className={SESSION_TARGET_PICKER_CLASS}>
      <div className={SESSION_TARGET_LIST_CLASS}>
        {targets.map((target) => {
          const targetValue = factorySessionTargetOptionValue(target);
          const isSelected =
            selectedTarget != null &&
            factorySessionTargetOptionValue(selectedTarget) === targetValue;

          return (
            <button
              key={`${target.ref.kind}:${target.ref.name ?? ""}:${target.factoryDir}`}
              className={cn(
                SESSION_TARGET_BUTTON_CLASS,
                isSelected
                  ? "ring-2 ring-af-focus-ring ring-offset-2 ring-offset-af-surface-subtle"
                  : undefined,
              )}
              disabled={isPending}
              onClick={() => {
                onOpenTarget(targetValue);
              }}
              type="button"
            >
              <span className="font-semibold text-af-text">{target.label}</span>
              <span className="truncate text-xs text-af-text-subtle">
                {target.factoryDir}
              </span>
            </button>
          );
        })}
      </div>
      {selectedTarget ? (
        <div className="sr-only" aria-live="polite">
          {selectedTarget.label}
        </div>
      ) : null}
    </section>
  );
}
