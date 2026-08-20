import { SurfacePanel } from "@you-agent-factory/components/layout";
import {
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@you-agent-factory/components/overlays";
import { Button, Text } from "@you-agent-factory/components/primitives";
import { type FormEvent, useId } from "react";
import type {
  FactorySessionsAPIError,
  FactorySessionTarget,
} from "../../../api/factory-sessions";
import { AlertPanel, AlertPanelText } from "../../../components/ui/alert-panel";
import { Input } from "../../../components/ui/input";
import {
  StandardListSelection,
  StandardListSelectionItem,
} from "../../../components/ui/standard-list-selection";
import {
  type FolderValidationState,
  factorySessionTargetOptionValue,
  folderValidationStatusMessage,
  initNewFactoryNestedPath,
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
  onCloseAutoFocus,
  onCreateNewFactory,
  onInspectFolder,
  onOpenTarget,
  onSelectTarget,
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
  onCloseAutoFocus: (event: Event) => void;
  onCreateNewFactory: () => void;
  onInspectFolder: (event: FormEvent<HTMLFormElement>) => void;
  onOpenTarget: (targetValue?: string) => void;
  onSelectTarget: (targetValue: string) => void;
  selectedTargetValue: string;
}) {
  const folderFieldID = useId();
  const folderHelperTextID = useId();
  const folderErrorTextID = useId();
  const dialogDescriptionID = useId();
  const hasFolderCandidate = folderPath.trim().length > 0;
  const showInspectSubmit =
    folderValidation.status !== "ready" &&
    folderValidation.status !== "init_ready";
  const configLoadFailure =
    dialogError?.code === "FACTORY_SESSION_CONFIG_LOAD_FAILED"
      ? dialogError
      : null;
  const validationStatusMessage =
    folderValidation.status === "ready" ||
    folderValidation.status === "init_ready"
      ? null
      : folderValidationStatusMessage(folderValidation, messages);
  const folderLabel = messages.sessionFolderFieldLabel;
  const folderPlaceholder = messages.sessionFolderFieldPlaceholder;
  const folderHelperText = messages.sessionFolderHelperText;

  return (
    <DialogContent
      aria-describedby={dialogDescriptionID}
      closeLabel={messages.closeDialogLabel}
      closeDisabled={isPending}
      onCloseAutoFocus={onCloseAutoFocus}
    >
      <DialogHeader>
        <DialogTitle>{messages.openSessionDialogTitle}</DialogTitle>
        <DialogDescription
          className="text-sm leading-6 text-on-surface-variant"
          id={dialogDescriptionID}
        >
          {messages.openSessionDialogDescription}
        </DialogDescription>
      </DialogHeader>
      <form className="grid gap-4" onSubmit={onInspectFolder}>
        <div className="grid gap-2">
          <label
            className="text-xs uppercase tracking-[0.18em] text-on-surface-subtle"
            htmlFor={folderFieldID}
          >
            {folderLabel}
          </label>
          <div className="flex flex-col gap-2 sm:flex-row">
            <Input
              autoFocus
              className="flex-1"
              disabled={isPending || isValidatePending}
              id={folderFieldID}
              aria-describedby={folderHelperTextID}
              aria-errormessage={
                folderValidation.status === "error"
                  ? folderErrorTextID
                  : undefined
              }
              aria-invalid={folderValidation.status === "error"}
              onChange={(event) => {
                onChangeFolderPath(event.target.value);
              }}
              placeholder={folderPlaceholder}
              value={folderPath}
            />
          </div>
          <Text
            className="text-sm text-on-surface-variant"
            id={folderHelperTextID}
          >
            {folderHelperText}
          </Text>
        </div>
        {validationStatusMessage ? (
          <AlertPanel
            aria-live="polite"
            id={
              folderValidation.status === "error"
                ? folderErrorTextID
                : undefined
            }
            role={folderValidation.status === "error" ? "alert" : "status"}
            tone={folderValidation.status === "error" ? "danger" : "info"}
          >
            {validationStatusMessage}
          </AlertPanel>
        ) : null}
        {configLoadFailure ? (
          <ConfigLoadFailedPanel error={configLoadFailure} />
        ) : null}
        {dialogError &&
        folderValidation.status !== "error" &&
        configLoadFailure == null ? (
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
        <NewFactoryConfirmation
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
          messages={messages}
          onOpenTarget={onOpenTarget}
          onSelectTarget={onSelectTarget}
          selectedTargetValue={selectedTargetValue}
          targets={discoveredTargets}
        />
      ) : null}
    </DialogContent>
  );
}

function ConfigLoadFailedPanel({ error }: { error: FactorySessionsAPIError }) {
  return (
    <AlertPanel role="alert" tone="danger">
      <AlertPanelText>{error.message}</AlertPanelText>
      {error.targets && error.targets.length > 0 ? (
        <ul className="list-disc space-y-1 pl-5">
          {error.targets.map((target) => (
            <li key={`${target.code}:${target.subject?.id ?? ""}`}>
              <AlertPanelText as="span">{target.message}</AlertPanelText>
            </li>
          ))}
        </ul>
      ) : null}
    </AlertPanel>
  );
}

function NewFactoryConfirmation({
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
  const nestedFactoryPath = initNewFactoryNestedPath(folderPath);
  const description = messages.newFactoryPreviewDescriptionTemplate.replaceAll(
    "{{folderPath}}",
    folderPath,
  );
  const previewTitleID = useId();

  return (
    <SurfacePanel
      aria-labelledby={previewTitleID}
      asChild
      radius="2xl"
      surface="low"
    >
      <section className="grid gap-3">
        <Text className="font-semibold text-on-surface" id={previewTitleID}>
          {messages.newFactoryPreviewTitle}
        </Text>
        <Text className="text-sm leading-6 text-on-surface">{description}</Text>
        <Text
          className="break-all rounded-lg border border-outline-variant bg-surface-container-highest px-3 py-2 font-mono text-xs text-on-surface"
          data-factory-preview-target
        >
          {nestedFactoryPath}
        </Text>
        <div className="flex flex-col-reverse justify-end gap-2 sm:flex-row">
          <Button
            disabled={isPending}
            onClick={onCancel}
            tone="outline"
            type="button"
          >
            {messages.newFactoryCancelLabel}
          </Button>
          <Button
            aria-busy={isPending}
            disabled={isPending}
            onClick={onConfirm}
            type="button"
          >
            {isPending
              ? messages.newFactoryConfirmPendingLabel
              : messages.newFactoryConfirmLabel}
          </Button>
        </div>
      </section>
    </SurfacePanel>
  );
}

function SessionTargetPicker({
  isPending,
  messages,
  onOpenTarget,
  onSelectTarget,
  selectedTargetValue,
  targets,
}: {
  isPending: boolean;
  messages: ReturnType<typeof getHeaderControlsMessages>;
  onOpenTarget: (targetValue?: string) => void;
  onSelectTarget: (targetValue: string) => void;
  selectedTargetValue: string;
  targets: FactorySessionTarget[];
}) {
  const selectedTarget = selectedFactorySessionTarget(
    targets,
    selectedTargetValue,
  );
  const pickerTitleID = useId();

  return (
    <SurfacePanel asChild className="grid gap-3" radius="2xl" surface="low">
      <section aria-labelledby={pickerTitleID}>
        <Text className="font-semibold text-on-surface" id={pickerTitleID}>
          {messages.targetPickerTitle}
        </Text>
        <Text className="mt-1 text-sm text-on-surface-variant">
          {messages.targetPickerHint}
        </Text>
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
                  onSelectTarget(targetValue);
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
        <div className="mt-3 flex justify-end">
          <Button
            aria-busy={isPending}
            disabled={isPending || selectedTarget == null}
            onClick={() => {
              onOpenTarget();
            }}
            type="button"
          >
            {isPending
              ? messages.openSessionTargetPendingLabel
              : messages.openSessionTargetLabel}
          </Button>
        </div>
      </section>
    </SurfacePanel>
  );
}
