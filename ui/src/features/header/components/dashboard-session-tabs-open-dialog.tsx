import {
  type ChangeEvent,
  type FormEvent,
  useId,
  useRef,
} from "react";

import type {
  FactorySessionTarget,
  FactorySessionsAPIError,
} from "../../../api/factory-sessions";
import {
  Button,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  Input,
  Select,
} from "../../../components/ui";
import { DASHBOARD_BODY_TEXT_CLASS } from "../../../components/ui/dashboard-typography";
import { cn } from "../../../lib/cn";
import {
  factorySessionTargetOptionValue,
  manualFactorySessionTargetRef,
  normalizeManualFactoryName,
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
  "flex min-h-11 flex-col items-start justify-center rounded-xl border border-af-border bg-af-surface-raised px-3 py-2 text-left text-sm text-af-text transition-colors hover:border-af-border-strong hover:bg-af-surface focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-af-focus-ring";
const SESSION_LAUNCH_SUMMARY_CLASS =
  "grid gap-2 rounded-xl border border-af-border bg-af-surface px-3 py-3";

export function OpenSessionDialog({
  dialogError,
  discoveredTargets,
  folderValidation,
  folderPath,
  isPending,
  manualFactoryName,
  messages,
  onChangeFolderPath,
  onChangeManualFactoryName,
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
  manualFactoryName: string;
  messages: ReturnType<typeof getHeaderControlsMessages>;
  onChangeFolderPath: (value: string) => void;
  onChangeManualFactoryName: (value: string) => void;
  onInspectFolder: (event: FormEvent<HTMLFormElement>) => void;
  onOpenTarget: () => void;
  onSelectTarget: (value: string) => void;
  selectedTargetValue: string;
}) {
  const folderFieldID = useId();
  const folderHelperTextID = useId();
  const manualFactoryFieldID = useId();
  const manualFactoryHelperTextID = useId();
  const folderPickerInputRef = useRef<HTMLInputElement | null>(null);
  const hasFolderCandidate = folderPath.trim().length > 0;
  const validationStatusMessage = folderValidationStatusMessage(
    folderValidation,
    messages,
  );

  async function handleOpenFolderPicker() {
    const directoryHandle = await pickDirectoryHandle();
    if (directoryHandle) {
      const selectedPath = readDirectoryHandlePath(directoryHandle);
      if (selectedPath) {
        onChangeFolderPath(selectedPath);
        return;
      }
    }

    folderPickerInputRef.current?.click();
  }

  function handleSelectFolder(event: ChangeEvent<HTMLInputElement>) {
    const nextFolderPath = extractSelectedFolderPath(event.target.files);
    if (nextFolderPath) {
      onChangeFolderPath(nextFolderPath);
    }
    event.target.value = "";
  }

  return (
    <DialogContent closeDisabled={isPending}>
      <DialogHeader>
        <DialogTitle>{messages.openSessionDialogTitle}</DialogTitle>
        <DialogDescription>
          {messages.openSessionDialogDescription}
        </DialogDescription>
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
            <Button
              disabled={isPending}
              onClick={handleOpenFolderPicker}
              size="sm"
              tone="outline"
              type="button"
            >
              {messages.browseSessionFolderButtonLabel}
            </Button>
          </div>
          <p
            className={cn("text-sm text-af-text-muted", DASHBOARD_BODY_TEXT_CLASS)}
            id={folderHelperTextID}
          >
            {messages.sessionFolderHelperText}
          </p>
        </div>
        <div className="grid gap-2">
          <label
            className={SESSION_SECTION_LABEL_CLASS}
            htmlFor={manualFactoryFieldID}
          >
            {messages.manualFactoryNameFieldLabel}
          </label>
          <Input
            aria-describedby={manualFactoryHelperTextID}
            className="flex-1"
            disabled={isPending}
            id={manualFactoryFieldID}
            onChange={(event) => {
              onChangeManualFactoryName(event.target.value);
            }}
            placeholder={messages.manualFactoryNameFieldPlaceholder}
            value={manualFactoryName}
          />
          <p
            className={cn("text-sm text-af-text-muted", DASHBOARD_BODY_TEXT_CLASS)}
            id={manualFactoryHelperTextID}
          >
            {messages.manualFactoryNameHelperText}
          </p>
          <input
            {...({ directory: "", webkitdirectory: "" } as Record<string, string>)}
            aria-hidden="true"
            className="sr-only"
            disabled={isPending}
            onChange={handleSelectFolder}
            ref={folderPickerInputRef}
            tabIndex={-1}
            type="file"
          />
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
        <div className="flex justify-end">
          <Button disabled={isPending || !hasFolderCandidate} type="submit">
            {isPending
              ? messages.openSessionSubmitPendingLabel
              : messages.openSessionSubmitLabel}
          </Button>
        </div>
      </form>
      {folderValidation.status === "ready" && discoveredTargets.length > 0 ? (
        <SessionTargetPicker
          isPending={isPending}
          manualFactoryName={manualFactoryName}
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

function SessionTargetPicker({
  isPending,
  manualFactoryName,
  messages,
  onOpenTarget,
  onSelectTarget,
  selectedTargetValue,
  targets,
}: {
  isPending: boolean;
  manualFactoryName: string;
  messages: ReturnType<typeof getHeaderControlsMessages>;
  onOpenTarget: () => void;
  onSelectTarget: (value: string) => void;
  selectedTargetValue: string;
  targets: FactorySessionTarget[];
}) {
  const targetSelectID = useId();
  const normalizedManualFactoryName = normalizeManualFactoryName(manualFactoryName);
  const manualOverrideTargetRef = manualFactorySessionTargetRef(manualFactoryName);
  const selectedTarget = selectedFactorySessionTarget(targets, selectedTargetValue);
  const manualOverrideTarget =
    manualOverrideTargetRef == null
      ? null
      : targets.find(
          (target) =>
            target.ref.kind === manualOverrideTargetRef.kind &&
            target.ref.name === manualOverrideTargetRef.name,
        ) ?? null;
  const launchTarget = manualOverrideTarget ?? selectedTarget;
  const launchTargetName =
    normalizedManualFactoryName !== ""
      ? normalizedManualFactoryName
      : launchTarget?.label ?? null;

  return (
    <section
      aria-label={messages.targetPickerTitle}
      className={SESSION_TARGET_PICKER_CLASS}
    >
      <div className="grid gap-1">
        <p className={SESSION_SECTION_LABEL_CLASS}>{messages.targetPickerTitle}</p>
        <p className={cn("text-sm text-af-text-muted", DASHBOARD_BODY_TEXT_CLASS)}>
          {messages.targetPickerHint}
        </p>
      </div>
      {targets.length > 1 ? (
        <div className="grid gap-2">
          <label className={SESSION_SECTION_LABEL_CLASS} htmlFor={targetSelectID}>
            {messages.selectSessionTargetLabel}
          </label>
          <Select
            disabled={isPending}
            id={targetSelectID}
            onChange={(event) => {
              onSelectTarget(event.target.value);
            }}
            value={selectedTargetValue}
          >
            <option value="">{messages.selectSessionTargetPlaceholder}</option>
            {targets.map((target) => (
              <option
                key={`${target.ref.kind}:${target.ref.name ?? ""}:${target.factoryDir}`}
                value={factorySessionTargetOptionValue(target)}
              >
                {target.label}
              </option>
            ))}
          </Select>
        </div>
      ) : null}
      {normalizedManualFactoryName !== "" ? (
        <p
          aria-live="polite"
          className={SESSION_DIALOG_STATUS_CLASS}
          role="status"
        >
          {messages.manualFactoryNamePrecedenceTemplate
            .replace("{{factoryName}}", normalizedManualFactoryName)
            .replace("{{detectedTarget}}", selectedTarget?.label ?? messages.selectSessionTargetPlaceholder)}
        </p>
      ) : null}
      {launchTarget ? (
        <div className={SESSION_LAUNCH_SUMMARY_CLASS}>
          <div className={SESSION_TARGET_BUTTON_CLASS}>
            <span className="font-semibold text-af-text">{launchTarget.label}</span>
            <span className="truncate text-xs text-af-text-subtle">
              {launchTarget.project || launchTarget.factoryDir}
            </span>
          </div>
          {launchTargetName ? (
            <p className={cn("text-sm text-af-text-muted", DASHBOARD_BODY_TEXT_CLASS)}>
              {messages.openSessionLaunchSummaryTemplate
                .replace("{{folderPath}}", launchTarget.folderPath)
                .replace("{{factoryName}}", launchTargetName)}
            </p>
          ) : null}
        </div>
      ) : (
        <p className={SESSION_DIALOG_STATUS_CLASS} role="status">
          {messages.selectSessionTargetPrompt}
        </p>
      )}
      <div className="flex justify-end">
        <Button
          disabled={isPending || launchTarget == null}
          onClick={onOpenTarget}
          type="button"
        >
          {isPending
            ? messages.openSessionTargetPendingLabel
            : messages.openSessionTargetLabel}
        </Button>
      </div>
    </section>
  );
}

function extractSelectedFolderPath(
  files: FileList | File[] | null,
): string | null {
  const firstFile = firstSelectedFile(files);
  if (!firstFile) {
    return null;
  }

  const absolutePath = readSelectedFilePath(firstFile);
  if (absolutePath) {
    return dirname(absolutePath);
  }
  return null;
}

async function pickDirectoryHandle(): Promise<FileSystemDirectoryHandle | null> {
  const browserWithDirectoryPicker = globalThis as typeof globalThis & {
    showDirectoryPicker?: () => Promise<FileSystemDirectoryHandle>;
  };
  if (typeof browserWithDirectoryPicker.showDirectoryPicker !== "function") {
    return null;
  }

  try {
    return await browserWithDirectoryPicker.showDirectoryPicker();
  } catch (error) {
    if (isDirectoryPickerAbortError(error)) {
      return null;
    }
    throw error;
  }
}

function firstSelectedFile(files: FileList | File[] | null): File | null {
  if (!files) {
    return null;
  }

  if (Array.isArray(files)) {
    return files[0] ?? null;
  }

  return files.item(0);
}

function readSelectedFilePath(file: File): string | null {
  const pathValue = (file as File & { path?: string }).path;
  return typeof pathValue === "string" && isAbsolutePath(pathValue)
    ? pathValue
    : null;
}

function readDirectoryHandlePath(handle: FileSystemDirectoryHandle): string | null {
  const pathValue = (handle as FileSystemDirectoryHandle & { path?: string }).path;
  if (typeof pathValue === "string" && isAbsolutePath(pathValue)) {
    return pathValue;
  }
  return null;
}

function isDirectoryPickerAbortError(error: unknown): boolean {
  return error instanceof DOMException && error.name === "AbortError";
}

function isAbsolutePath(path: string): boolean {
  return path.startsWith("/") || /^[a-zA-Z]:[\\/]/.test(path);
}

function dirname(path: string): string {
  return path.replace(/[\\/][^\\/]+$/, "");
}
