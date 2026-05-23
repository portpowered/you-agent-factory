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
import { Button, DialogContent, DialogDescription, DialogHeader, DialogTitle, Input } from "../../../components/ui";
import { cn } from "../../../lib/cn";
import { DASHBOARD_BODY_TEXT_CLASS } from "../../../components/ui/dashboard-typography";
import {
  type FolderValidationState,
  folderValidationStatusMessage,
} from "../lib/dashboard-session-tabs-utils";
import {
  type getHeaderControlsMessages,
} from "../messages/header-controls";

const SESSION_SECTION_LABEL_CLASS =
  "text-xs uppercase tracking-[0.18em] text-af-ink/52";
const SESSION_DIALOG_ERROR_CLASS =
  "rounded-xl border border-af-danger/32 bg-af-danger/8 px-3 py-2 text-sm text-af-ink";
const SESSION_DIALOG_STATUS_CLASS =
  "rounded-xl border border-af-accent/24 bg-af-accent/8 px-3 py-2 text-sm text-af-ink";
const SESSION_TARGET_LIST_CLASS = "grid gap-2 sm:grid-cols-2";
const SESSION_TARGET_BUTTON_CLASS =
  "flex min-h-11 flex-col items-start justify-center rounded-xl border border-af-overlay/12 bg-af-overlay/4 px-3 py-2 text-left text-sm text-af-ink/82 transition-colors hover:border-af-accent/30 hover:bg-af-overlay/8 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-af-accent/25";

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
}: {
  dialogError: FactorySessionsAPIError | null;
  discoveredTargets: FactorySessionTarget[];
  folderValidation: FolderValidationState;
  folderPath: string;
  isPending: boolean;
  messages: ReturnType<typeof getHeaderControlsMessages>;
  onChangeFolderPath: (value: string) => void;
  onInspectFolder: (event: FormEvent<HTMLFormElement>) => void;
  onOpenTarget: (target: FactorySessionTarget) => void;
}) {
  const folderFieldID = useId();
  const folderHelperTextID = useId();
  const folderPickerInputRef = useRef<HTMLInputElement | null>(null);
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
            className={cn("text-sm text-af-ink/68", DASHBOARD_BODY_TEXT_CLASS)}
            id={folderHelperTextID}
          >
            {messages.sessionFolderHelperText}
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
          <Button disabled={isPending} type="submit">
            {isPending
              ? messages.openSessionSubmitPendingLabel
              : messages.openSessionSubmitLabel}
          </Button>
        </div>
      </form>
      {folderValidation.status === "ready" && discoveredTargets.length > 0 ? (
        <SessionTargetPicker
          isPending={isPending}
          messages={messages}
          onOpenTarget={onOpenTarget}
          targets={discoveredTargets}
        />
      ) : null}
    </DialogContent>
  );
}

function SessionTargetPicker({
  isPending,
  messages,
  onOpenTarget,
  targets,
}: {
  isPending: boolean;
  messages: ReturnType<typeof getHeaderControlsMessages>;
  onOpenTarget: (target: FactorySessionTarget) => void;
  targets: FactorySessionTarget[];
}) {
  return (
    <section aria-label={messages.targetPickerTitle} className="grid gap-3">
      <div className="grid gap-1">
        <p className={SESSION_SECTION_LABEL_CLASS}>{messages.targetPickerTitle}</p>
        <p className={cn("text-sm text-af-ink/72", DASHBOARD_BODY_TEXT_CLASS)}>
          {messages.targetPickerHint}
        </p>
      </div>
      <div className={SESSION_TARGET_LIST_CLASS}>
        {targets.map((target) => (
          <button
            className={SESSION_TARGET_BUTTON_CLASS}
            disabled={isPending}
            key={`${target.ref.kind}:${target.ref.name ?? ""}:${target.factoryDir}`}
            onClick={() => {
              onOpenTarget(target);
            }}
            type="button"
          >
            <span className="font-semibold text-af-ink">{target.label}</span>
            <span className="truncate text-xs text-af-ink/58">
              {target.project || target.factoryDir}
            </span>
          </button>
        ))}
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
