import { useState, type DragEvent } from "react";

import { buttonVariants } from "../../../components/ui";
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
  onStageFileItem,
  validationTextClassName,
}: {
  disabled: boolean;
  fieldLabelClassName: string;
  helpTextClassName: string;
  inputID: string;
  item: SubmitWorkDraftFileItem;
  messages: SubmitWorkMessages;
  onStageFileItem: (file: File) => void;
  validationTextClassName: string;
}) {
  const [isDragActive, setIsDragActive] = useState(false);
  const typeLabel = messages.addItemOptionLabel(item.type);
  const inputLabel = messages.fileItemInputLabel(typeLabel);
  const stateDescription = fileItemDescription(messages, item, typeLabel, isDragActive);

  return (
    <div className="grid gap-3">
      <input
        className="sr-only"
        disabled={disabled}
        id={inputID}
        onChange={(event) => {
          const nextFile = event.target.files?.[0];
          if (!nextFile) {
            return;
          }
          onStageFileItem(nextFile);
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
          if (!hasDraggedFiles(event)) {
            return;
          }
          event.preventDefault();
          setIsDragActive(true);
        }}
        onDragLeave={(event) => {
          if (!hasDraggedFiles(event)) {
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
          if (!hasDraggedFiles(event)) {
            return;
          }
          event.preventDefault();
          event.dataTransfer.dropEffect = "copy";
          setIsDragActive(true);
        }}
        onDrop={(event) => {
          if (!hasDraggedFiles(event)) {
            return;
          }
          event.preventDefault();
          setIsDragActive(false);
          const nextFile = event.dataTransfer.files?.[0];
          if (!nextFile) {
            return;
          }
          onStageFileItem(nextFile);
        }}
      >
        <div className="flex flex-wrap items-center gap-3">
          <span className={fieldLabelClassName}>{inputLabel}</span>
          <span className={helpTextClassName}>{stateDescription}</span>
        </div>
        <div className="flex flex-wrap items-center gap-3">
          <span
            className={buttonVariants({
              className: "pointer-events-none",
              size: "sm",
              tone: item.stagingStatus === "failure" ? "destructive" : "outline",
            })}
          >
            {item.stagingStatus === "ready"
              ? messages.replaceFileAction
              : messages.chooseFileAction}
          </span>
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
