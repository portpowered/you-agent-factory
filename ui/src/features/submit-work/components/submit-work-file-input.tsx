import { type DragEvent, useState } from "react";

import { Label, Text } from "@you-agent-factory/components/primitives";
import { Button } from "../../../components/ui/button";
import { cn } from "../../../lib/cn";
import { ChooseFileField } from "../../choose-file/components/choose-file-field";
import { submitWorkItemRowTypeLabel } from "../lib/submit-work-item-type-label";
import type { SubmitWorkMessages } from "../messages/submit-work";
import type { SubmitWorkDraftFileItem } from "./submit-work-card";

export function FileSubmissionItemEditor({
  disabled,
  inputID,
  item,
  locale,
  messages,
  onStageFileItems,
}: {
  disabled: boolean;
  inputID: string;
  item: SubmitWorkDraftFileItem;
  locale?: string;
  messages: SubmitWorkMessages;
  onStageFileItems: (files: File[]) => void;
}) {
  const [isDragActive, setIsDragActive] = useState(false);
  const typeLabel = submitWorkItemRowTypeLabel(item.type, locale);
  const inputLabel = messages.fileItemInputLabel(typeLabel);
  const stateDescription = fileItemDescription(
    messages,
    item,
    typeLabel,
    isDragActive,
  );
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
      <ChooseFileField
        afterControl={
          item.stagingStatus === "failure" && item.stagingError ? (
            <Text className="text-on-error-container" variant="supporting">
              {item.stagingError}
            </Text>
          ) : null
        }
        control={
          <label
            className={cn("grid gap-3 p-3", !disabled && "cursor-pointer")}
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
              if (
                nextTarget instanceof Node &&
                event.currentTarget.contains(nextTarget)
              ) {
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
              <Label>{inputLabel}</Label>
              <Text
                as="span"
                className="max-w-xl leading-relaxed text-on-surface-variant"
                variant="supporting"
              >
                {stateDescription}
              </Text>
            </div>
            <div className="flex flex-wrap items-center gap-3">
              <Button
                asChild
                className="pointer-events-none"
                size="sm"
                tone={
                  item.stagingStatus === "failure" ? "destructive" : "outline"
                }
              >
                <span>
                  {item.stagingStatus === "ready"
                    ? messages.replaceFileAction
                    : messages.chooseFileAction}
                </span>
              </Button>
              {(item.fileName ?? "").length > 0 ? (
                <Text
                  as="span"
                  className="max-w-xl leading-relaxed text-on-surface-variant"
                  variant="supporting"
                >
                  {messages.fileItemMetadata(
                    item.fileName ?? "",
                    item.mediaType ?? "",
                  )}
                </Text>
              ) : null}
            </div>
          </label>
        }
        disabled={disabled}
        dragActive={isDragActive}
      />
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
      return messages.fileItemReady(
        item.fileName ?? typeLabel,
        item.mediaType ?? "",
      );
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
