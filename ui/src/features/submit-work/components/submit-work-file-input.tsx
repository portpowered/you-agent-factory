import { useState, type DragEvent } from "react";

import { Button } from "../../../components/ui";
import { cn } from "../../../lib/cn";
import type { SubmitWorkMessages } from "../messages/submit-work";
import type { SubmitWorkDraftFileItem } from "./submit-work-card";

export function FileSubmissionItemEditor({
  disabled,
  fieldLabelClassName,
  helpTextClassName,
  inputID,
  item,
  messages,
  onStageFileItems,
  validationTextClassName,
}: {
  disabled: boolean;
  fieldLabelClassName: string;
  helpTextClassName: string;
  inputID: string;
  item: SubmitWorkDraftFileItem;
  messages: SubmitWorkMessages;
  onStageFileItems: (files: File[]) => void;
  validationTextClassName: string;
}) {
  const [isDragActive, setIsDragActive] = useState(false);
  const typeLabel = messages.addItemOptionLabel(item.type);
  const inputLabel = messages.fileItemInputLabel(typeLabel);
  const stateDescription = fileItemDescription(messages, item, typeLabel, isDragActive);
  const canStageFiles = !disabled;

  return (
    <div className="grid gap-3">
      <input
        className="sr-only"
        disabled={disabled}
        id={inputID}
        multiple
        onChange={(event) => {
          if (!canStageFiles) {
            event.currentTarget.value = "";
            return;
          }
          const nextFiles = Array.from(event.target.files ?? []);
          if (nextFiles.length === 0) {
            return;
          }
          onStageFileItems(nextFiles);
          event.currentTarget.value = "";
        }}
        type="file"
      />
      <label
        className={cn(
          "grid gap-3 rounded-lg border border-dashed p-3 transition-colors",
          isDragActive
            ? "border-af-accent-border bg-af-accent-surface"
            : "border-af-border bg-af-surface",
          disabled ? "cursor-not-allowed text-af-text-disabled" : "cursor-pointer",
        )}
        htmlFor={inputID}
        onDragEnter={(event) => {
          if (!canHandleDraggedFiles(event, canStageFiles)) {
            return;
          }
          event.preventDefault();
          setIsDragActive(true);
        }}
        onDragLeave={(event) => {
          if (!canHandleDraggedFiles(event, canStageFiles)) {
            return;
          }
          event.preventDefault();
          const nextTarget = event.relatedTarget;
          if (nextTarget instanceof Node && event.currentTarget.contains(nextTarget)) {
            return;
          }
          setIsDragActive(false);
        }}
        onDragOver={(event) => {
          if (!canHandleDraggedFiles(event, canStageFiles)) {
            return;
          }
          event.preventDefault();
          event.dataTransfer.dropEffect = "copy";
          setIsDragActive(true);
        }}
        onDrop={(event) => {
          if (!canHandleDraggedFiles(event, canStageFiles)) {
            return;
          }
          event.preventDefault();
          setIsDragActive(false);
          const nextFiles = Array.from(event.dataTransfer.files ?? []);
          if (nextFiles.length === 0) {
            return;
          }
          onStageFileItems(nextFiles);
        }}
      >
        <div className="flex flex-wrap items-center gap-3">
          <span className={fieldLabelClassName}>{inputLabel}</span>
          <span className={helpTextClassName}>{stateDescription}</span>
        </div>
        <div className="flex flex-wrap items-center gap-3">
          <Button
            asChild
            className="pointer-events-none"
            size="sm"
            tone={item.stagingStatus === "failure" ? "destructive" : "outline"}
          >
            <span>
              {item.stagingStatus === "ready"
                ? messages.replaceFileAction
                : messages.chooseFileAction}
            </span>
          </Button>
          {(item.fileName ?? "").length > 0 ? (
            <span className={helpTextClassName}>
              {messages.fileItemMetadata(item.fileName ?? "", item.mediaType ?? "")}
            </span>
          ) : null}
        </div>
      </label>
      {item.stagingStatus === "failure" && item.stagingError ? (
        <p className={validationTextClassName}>{item.stagingError}</p>
      ) : null}
    </div>
  );
}

function fileItemDescription(
  messages: SubmitWorkMessages,
  item: SubmitWorkDraftFileItem,
  typeLabel: string,
  isDragActive: boolean,
): string {
  if (isDragActive) {
    return messages.fileItemDragActive(typeLabel);
  }
  switch (item.stagingStatus) {
    case "staging":
      return messages.fileItemStaging(item.fileName ?? typeLabel);
    case "ready":
      return messages.fileItemReady(item.fileName ?? typeLabel, item.mediaType ?? "");
    case "failure":
      return messages.fileItemFailure(typeLabel);
    default:
      return messages.fileItemPlaceholder(typeLabel);
  }
}

function hasDraggedFiles(event: Pick<DragEvent, "dataTransfer">): boolean {
  const dragTypes = event.dataTransfer?.types;
  if (!dragTypes) {
    return false;
  }
  return Array.from(dragTypes).includes("Files");
}

function canHandleDraggedFiles(
  event: Pick<DragEvent, "dataTransfer">,
  canStageFiles: boolean,
): boolean {
  return canStageFiles && hasDraggedFiles(event);
}
