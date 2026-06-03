import { type FormEvent, useId } from "react";

import type {
  FactorySessionsAPIError,
  FactorySessionTarget,
} from "../../../api/factory-sessions";
import {
  Button,
  DialogContent,
  DialogHeader,
  DialogTitle,
  Input,
  StandardListSelection,
  StandardListSelectionItem,
} from "../../../components/ui";
import { DASHBOARD_BODY_TEXT_CLASS } from "../../../components/ui/dashboard-typography";
import { cn } from "../../../lib/cn";
import {
  type FolderValidationState,
  factorySessionTargetOptionValue,
  folderValidationStatusMessage,
  selectedFactorySessionTarget,
} from "../lib/dashboard-session-tabs-utils";
import type { getHeaderControlsMessages } from "../messages/header-controls";

const SESSION_DIALOG_ERROR_CLASS =
  "rounded-xl border border-af-danger-border bg-error-container px-3 py-2 text-sm text-on-error-container";
const SESSION_DIALOG_STATUS_CLASS =
  "rounded-xl border border-primary bg-primary-container px-3 py-2 text-sm text-on-surface";
const SESSION_TARGET_PICKER_CLASS =
  "grid gap-3 rounded-2xl border border-outline bg-surface-container-low p-4";

export function OpenSessionDialog({
  dialogError,
  discoveredTargets,
  folderValidation,
  folderPath,
  isPending,
  isValidatePending,
  messages,
  onCancelInitConfirmation,
  onChangeFolderPath,
  onCreateNewFactory,
  onInspectFolder,
  onOpenTarget,
  selectedTargetValue,
}: {
  dialogError: FactorySessionsAPIError | null;
  discoveredTargets: FactorySessionTarget[];
  folderValidation: FolderValidationState;
  folderPath: string;
  isPending: boolean;
  isValidatePending: boolean;
  messages: ReturnType<typeof getHeaderControlsMessages>;
  onCancelInitConfirmation: () => void;
  onChangeFolderPath: (value: string) => void;
  onCreateNewFactory: () => void;
  onInspectFolder: (event: FormEvent<HTMLFormElement>) => void;
  onOpenTarget: (targetValue?: string) => void;
  selectedTargetValue: string;
}) {
  const folderFieldID = useId();
  const folderHelperTextID = useId();
  const dialogDescriptionID = useId();
  const hasFolderCandidate = folderPath.trim().length > 0;
  const showInspectSubmit =
    folderValidation.status !== "ready" &&
    folderValidation.status !== "init_ready";
  const validationStatusMessage =
    folderValidation.status === "ready" ||
    folderValidation.status === "init_ready"
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
          className={cn(
            "text-sm leading-6 text-on-surface-variant",
            DASHBOARD_BODY_TEXT_CLASS,
          )}
          id={dialogDescriptionID}
        >
          {messages.openSessionDialogDescription}
        </p>
      </DialogHeader>
      <form className="grid gap-4" onSubmit={onInspectFolder}>
        <div className="grid gap-2">
          <label
            className="text-xs uppercase tracking-[0.18em] text-on-surface-subtle"
            htmlFor={folderFieldID}
          >
            {messages.sessionFolderFieldLabel}
          </label>
          <div className="flex flex-col gap-2 sm:flex-row">
            <Input
              autoFocus
              className="flex-1"
              disabled={isPending || isValidatePending}
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
            className={cn(
              "text-sm text-on-surface-variant",
              DASHBOARD_BODY_TEXT_CLASS,
            )}
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
        {showInspectSubmit ? (
          <div className="flex justify-end">
            <Button
              disabled={isPending || isValidatePending || !hasFolderCandidate}
              type="submit"
            >
              {isValidatePending
                ? messages.openSessionSubmitPendingLabel
                : messages.openSessionSubmitLabel}
            </Button>
          </div>
        ) : null}
      </form>
      {folderValidation.status === "init_ready" ? (
        <InitNewFactoryConfirmation
          folderPath={folderValidation.folderPath}
          isPending={isPending}
          messages={messages}
          onCancel={onCancelInitConfirmation}
          onConfirm={onCreateNewFactory}
        />
      ) : null}
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

function InitNewFactoryConfirmation({
  folderPath,
  isPending,
  messages,
  onCancel,
  onConfirm,
}: {
  folderPath: string;
  isPending: boolean;
  messages: ReturnType<typeof getHeaderControlsMessages>;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  const description =
    messages.openSessionInitNewFactoryDescriptionTemplate.replace(
      "{{folderPath}}",
      folderPath,
    );

  return (
    <section
      aria-labelledby="init-new-factory-confirmation-title"
      className={SESSION_TARGET_PICKER_CLASS}
    >
      <p
        className={cn(
          "text-sm leading-6 text-on-surface",
          DASHBOARD_BODY_TEXT_CLASS,
        )}
        id="init-new-factory-confirmation-title"
      >
        {description}
      </p>
      <p
        className={cn(
          "break-all font-mono text-xs text-on-surface-subtle",
          DASHBOARD_BODY_TEXT_CLASS,
        )}
      >
        {folderPath}
      </p>
      <div className="flex flex-col-reverse justify-end gap-2 sm:flex-row">
        <Button
          disabled={isPending}
          onClick={onCancel}
          tone="outline"
          type="button"
        >
          {messages.openSessionCancelCreateFactoryLabel}
        </Button>
        <Button
          aria-busy={isPending}
          disabled={isPending}
          onClick={onConfirm}
          type="button"
        >
          {isPending
            ? messages.openSessionCreateFactoryPendingLabel
            : messages.openSessionCreateFactoryLabel}
        </Button>
      </div>
    </section>
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
  const selectedTarget =
    selectedFactorySessionTarget(targets, selectedTargetValue) ??
    (targets.length === 1 ? targets[0] : null);

  return (
    <section className={SESSION_TARGET_PICKER_CLASS}>
      <StandardListSelection
        disabled={isPending}
        selectionAnnouncement={selectedTarget?.label}
      >
        {targets.map((target) => {
          const targetValue = factorySessionTargetOptionValue(target);
          const isSelected =
            selectedTarget != null &&
            factorySessionTargetOptionValue(selectedTarget) === targetValue;

          return (
            <StandardListSelectionItem
              key={`${target.ref.kind}:${target.ref.name ?? ""}:${target.factoryDir}`}
              onClick={() => {
                onOpenTarget(targetValue);
              }}
              selected={isSelected}
            >
              <span className="flex min-h-16 w-full flex-col items-start justify-center px-1 py-1 text-sm">
                <span className="font-semibold text-on-surface">
                  {target.label}
                </span>
                <span className="truncate text-xs text-on-surface-subtle">
                  {target.factoryDir}
                </span>
              </span>
            </StandardListSelectionItem>
          );
        })}
      </StandardListSelection>
    </section>
  );
}
