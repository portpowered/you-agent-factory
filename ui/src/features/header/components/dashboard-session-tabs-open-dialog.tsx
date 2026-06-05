import { type FormEvent, useId } from "react";

import type {
  FactorySessionsAPIError,
  FactorySessionTarget,
} from "../../../api/factory-sessions";
import {
  AlertPanel,
  Button,
  DashboardText,
  DialogContent,
  DialogHeader,
  DialogTitle,
  Input,
  StandardListSelection,
  StandardListSelectionItem,
  SurfacePanel,
} from "../../../components/ui";
import {
  type FolderValidationState,
  factorySessionTargetOptionValue,
  folderValidationStatusMessage,
  selectedFactorySessionTarget,
} from "../lib/dashboard-session-tabs-utils";
import type { getHeaderControlsMessages } from "../messages/header-controls";

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
        <DashboardText
          className="text-sm leading-6 text-on-surface-variant"
          id={dialogDescriptionID}
        >
          {messages.openSessionDialogDescription}
        </DashboardText>
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
          <DashboardText
            className="text-sm text-on-surface-variant"
            id={folderHelperTextID}
          >
            {messages.sessionFolderHelperText}
          </DashboardText>
        </div>
        {validationStatusMessage ? (
          <AlertPanel
            aria-live="polite"
            role={folderValidation.status === "error" ? "alert" : "status"}
            tone={folderValidation.status === "error" ? "danger" : "info"}
          >
            {validationStatusMessage}
          </AlertPanel>
        ) : null}
        {dialogError && folderValidation.status !== "error" ? (
          <AlertPanel role="alert" tone="danger">
            {dialogError.message}
          </AlertPanel>
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
    <SurfacePanel
      aria-labelledby="init-new-factory-confirmation-title"
      asChild
      radius="2xl"
      surface="low"
    >
      <section className="grid gap-3">
        <DashboardText
          className="text-sm leading-6 text-on-surface"
          id="init-new-factory-confirmation-title"
        >
          {description}
        </DashboardText>
        <DashboardText className="break-all font-mono text-xs text-on-surface-subtle">
          {folderPath}
        </DashboardText>
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
    </SurfacePanel>
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
    <SurfacePanel asChild className="grid gap-3" radius="2xl" surface="low">
      <section>
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
    </SurfacePanel>
  );
}
