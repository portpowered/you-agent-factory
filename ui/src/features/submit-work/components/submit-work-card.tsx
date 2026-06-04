import { Plus } from "lucide-react";
import { type ReactNode, useState } from "react";

import {
  Button,
  DashboardIconButtonShell,
  DashboardWidgetFrame,
  Input,
  Popover,
  PopoverContent,
  PopoverTrigger,
  Select,
} from "../../../components/ui";
import { cn } from "../../../lib/cn";
import { getSubmitWorkMessages } from "../messages/submit-work";
import { SubmissionItemsList } from "./submit-work-items-list";

export interface SubmitWorkDraft {
  items: SubmitWorkDraftItem[];
  requestName: string;
  workTypeName: string;
}

export interface SubmitWorkDraftTextItem {
  id: string;
  text: string;
  type: "text";
}

export interface SubmitWorkDraftFileItem {
  fileName?: string;
  id: string;
  mediaType?: string;
  stagedFileRef?: string;
  stagingError?: string;
  stagingStatus: "failure" | "idle" | "ready" | "staging";
  type: "audio" | "document" | "image" | "video";
  url?: string;
}

export type SubmitWorkDraftItem =
  | SubmitWorkDraftFileItem
  | SubmitWorkDraftTextItem;
export type SubmitWorkDraftItemType = SubmitWorkDraftItem["type"];

export interface SubmitWorkValidationErrors {
  submissionItems?: string;
  requestName?: string;
  workTypeName?: string;
}

export interface SubmitWorkStatus {
  kind: "error" | "guidance" | "submitting" | "success" | "validation-error";
  message: string;
}

export interface SubmitWorkCardProps {
  draft: SubmitWorkDraft;
  headerAction?: ReactNode;
  isSubmitting?: boolean;
  locale?: string;
  onAddItem: (type: SubmitWorkDraftItemType) => void;
  onItemTextChange: (itemId: string, value: string) => void;
  onRemoveItem: (itemId: string) => void;
  onRequestNameChange: (value: string) => void;
  onStageFileItems: (itemId: string, files: File[]) => void;
  onSubmit: () => void;
  onWorkTypeNameChange: (value: string) => void;
  status: SubmitWorkStatus;
  submitWorkTypeNames: string[];
  validationErrors?: SubmitWorkValidationErrors;
  widgetId?: string;
}

const STATUS_TONE_CLASS_BY_KIND: Record<SubmitWorkStatus["kind"], string> = {
  error: "text-on-error-container",
  guidance: "text-on-surface-subtle",
  submitting: "text-on-surface",
  success: "text-on-success-container",
  "validation-error": "text-on-error-container",
};
const ADDABLE_ITEM_TYPES: SubmitWorkDraftItemType[] = [
  "text",
  "image",
  "video",
  "audio",
  "document",
];

export function SubmitWorkCard({
  draft,
  headerAction,
  isSubmitting = false,
  locale,
  onAddItem,
  onItemTextChange,
  onRemoveItem,
  onRequestNameChange,
  onStageFileItems,
  onSubmit,
  onWorkTypeNameChange,
  status,
  submitWorkTypeNames,
  validationErrors,
  widgetId = "submit-work",
}: SubmitWorkCardProps) {
  const messages = getSubmitWorkMessages(locale);
  const [isAddItemMenuOpen, setIsAddItemMenuOpen] = useState(false);
  const hasConfiguredWorkTypes = submitWorkTypeNames.length > 0;
  const hasIncompleteFileItems = draft.items.some(
    (item) => item.type !== "text" && item.stagingStatus !== "ready",
  );
  const hasSelectedWorkType = draft.workTypeName.length > 0;
  const hasValidRequestName = draft.requestName.trim().length > 0;
  const controlsDisabled = !hasConfiguredWorkTypes || isSubmitting;
  const canSubmit =
    hasConfiguredWorkTypes &&
    !hasIncompleteFileItems &&
    hasSelectedWorkType &&
    hasValidRequestName &&
    !isSubmitting;
  const requestNameID = `${widgetId}-request-name`;
  const requestNameErrorID = `${widgetId}-request-name-error`;
  const submissionItemsID = `${widgetId}-submission-items`;
  const workTypeID = `${widgetId}-work-type`;
  const workTypeErrorID = `${widgetId}-work-type-error`;
  const statusID = `${widgetId}-status`;
  const shouldRenderStatus =
    status.kind !== "guidance" ||
    (status.message !== messages.statusMessages.emptyGuidance &&
      status.message !== messages.statusMessages.ready);

  return (
    <DashboardWidgetFrame
      headerAction={
        <SubmitWorkHeaderControls
          controlsDisabled={controlsDisabled}
          headerAction={headerAction}
          isAddItemMenuOpen={isAddItemMenuOpen}
          messages={messages}
          onAddItem={onAddItem}
          onAddItemMenuOpenChange={setIsAddItemMenuOpen}
          onWorkTypeNameChange={onWorkTypeNameChange}
          submitWorkTypeNames={submitWorkTypeNames}
          validationErrors={validationErrors}
          widgetId={widgetId}
          workTypeID={workTypeID}
          workTypeName={draft.workTypeName}
          workTypeErrorID={workTypeErrorID}
        />
      }
      title={messages.cardTitle}
      widgetId={widgetId}
    >
      <form
        className="grid h-full min-h-0 content-start gap-3"
        onSubmit={(event) => {
          event.preventDefault();
          onSubmit();
        }}
      >
        <div className="grid min-h-0 content-start gap-4 overflow-y-auto pr-1">
          <div className="grid gap-2">
            <label
              className="af-dashboard-supporting-label"
              htmlFor={requestNameID}
            >
              {messages.requestNameLabel}
            </label>
            <Input
              aria-describedby={
                validationErrors?.requestName ? requestNameErrorID : undefined
              }
              aria-invalid={validationErrors?.requestName ? "true" : undefined}
              className="af-dashboard-body-text"
              disabled={controlsDisabled}
              id={requestNameID}
              onChange={(event) => onRequestNameChange(event.target.value)}
              placeholder={messages.requestNamePlaceholder}
              type="text"
              value={draft.requestName}
            />
            {validationErrors?.requestName ? (
              <p className=  "text-on-error-container af-dashboard-supporting-text" id={requestNameErrorID}>
                {validationErrors.requestName}
              </p>
            ) : null}
          </div>

          <div className="grid gap-2">
            <div className="sr-only" id={submissionItemsID}>
              {messages.submissionItemsLabel}
            </div>
            <SubmissionItemsList
              controlsDisabled={controlsDisabled}
              draft={draft}
              locale={locale}
              messages={messages}
              onItemTextChange={onItemTextChange}
              onRemoveItem={onRemoveItem}
              onStageFileItems={onStageFileItems}
              submissionItemsID={submissionItemsID}
              widgetId={widgetId}
            />
            {validationErrors?.submissionItems ? (
              <p className=  "text-on-error-container af-dashboard-supporting-text">
                {validationErrors.submissionItems}
              </p>
            ) : null}
          </div>
        </div>

        <div className="grid gap-3">
          {shouldRenderStatus ? (
            <p
              className={cn(
                "min-w-0 flex-1 max-w-xl leading-relaxed text-on-surface-variant af-dashboard-supporting-text",
                STATUS_TONE_CLASS_BY_KIND[status.kind],
              )}
              id={statusID}
              role={
                status.kind === "error" || status.kind === "validation-error"
                  ? "alert"
                  : "status"
              }
            >
              {status.message}
            </p>
          ) : null}
          <Button
            aria-busy={isSubmitting ? "true" : undefined}
            disabled={!canSubmit}
            tone={canSubmit ? "default" : "outline"}
            type="submit"
          >
            {isSubmitting ? messages.submittingAction : messages.submitAction}
          </Button>
        </div>
      </form>
    </DashboardWidgetFrame>
  );
}

function SubmitWorkHeaderControls({
  controlsDisabled,
  headerAction,
  isAddItemMenuOpen,
  messages,
  onAddItem,
  onAddItemMenuOpenChange,
  onWorkTypeNameChange,
  submitWorkTypeNames,
  validationErrors,
  widgetId,
  workTypeErrorID,
  workTypeID,
  workTypeName,
}: {
  controlsDisabled: boolean;
  headerAction?: ReactNode;
  isAddItemMenuOpen: boolean;
  messages: ReturnType<typeof getSubmitWorkMessages>;
  onAddItem: (type: SubmitWorkDraftItemType) => void;
  onAddItemMenuOpenChange: (open: boolean) => void;
  onWorkTypeNameChange: (value: string) => void;
  submitWorkTypeNames: string[];
  validationErrors?: SubmitWorkValidationErrors;
  widgetId: string;
  workTypeErrorID: string;
  workTypeID: string;
  workTypeName: string;
}) {
  return (
    <div className="flex min-w-0 flex-wrap items-center justify-end gap-2">
      <div className="grid min-w-36 gap-1">
        <label className="sr-only" htmlFor={workTypeID}>
          {messages.workTypeLabel}
        </label>
        <Select
          aria-describedby={
            validationErrors?.workTypeName ? workTypeErrorID : undefined
          }
          aria-invalid={validationErrors?.workTypeName ? "true" : undefined}
          className={cn("min-h-9 py-2 text-xs", "af-dashboard-body-text")}
          disabled={controlsDisabled}
          id={workTypeID}
          onChange={(event) => onWorkTypeNameChange(event.target.value)}
          value={workTypeName}
        >
          <option value="">{messages.selectWorkTypePlaceholder}</option>
          {submitWorkTypeNames.map((submitWorkTypeName) => (
            <option key={submitWorkTypeName} value={submitWorkTypeName}>
              {submitWorkTypeName}
            </option>
          ))}
        </Select>
        {validationErrors?.workTypeName ? (
          <p className="sr-only" id={workTypeErrorID}>
            {validationErrors.workTypeName}
          </p>
        ) : null}
      </div>
      <AddSubmissionItemMenu
        controlsDisabled={controlsDisabled}
        isOpen={isAddItemMenuOpen}
        messages={messages}
        onAddItem={onAddItem}
        onOpenChange={onAddItemMenuOpenChange}
        widgetId={widgetId}
      />
      {headerAction}
    </div>
  );
}

function AddSubmissionItemMenu({
  controlsDisabled,
  isOpen,
  messages,
  onAddItem,
  onOpenChange,
  widgetId,
}: {
  controlsDisabled: boolean;
  isOpen: boolean;
  messages: ReturnType<typeof getSubmitWorkMessages>;
  onAddItem: (type: SubmitWorkDraftItemType) => void;
  onOpenChange: (open: boolean) => void;
  widgetId: string;
}) {
  const menuDescriptionID = `${widgetId}-add-item-menu-description`;

  return (
    <Popover onOpenChange={onOpenChange} open={isOpen}>
      <PopoverTrigger asChild>
        <DashboardIconButtonShell
          aria-label={messages.addItemAction}
          disabled={controlsDisabled}
          tone="outline"
          type="button"
        >
          <Plus
            aria-hidden="true"
            className="size-4"
            focusable="false"
            strokeWidth={1.8}
          />
        </DashboardIconButtonShell>
      </PopoverTrigger>
      <PopoverContent
        align="end"
        aria-describedby={menuDescriptionID}
        aria-label={messages.addItemMenuLabel}
        className="grid gap-3"
      >
        <p className="text-on-surface-variant af-dashboard-supporting-text" id={menuDescriptionID}>
          {messages.addItemMenuDescription}
        </p>
        <div className="grid gap-2">
          {ADDABLE_ITEM_TYPES.map((itemType) => {
            const typeLabel = itemTypeLabel(messages, itemType);
            return (
              <Button
                aria-label={typeLabel}
                className="justify-start border-outline text-left font-medium"
                key={itemType}
                onClick={() => {
                  onAddItem(itemType);
                  onOpenChange(false);
                }}
                tone="outline"
                type="button"
              >
                {typeLabel}
              </Button>
            );
          })}
        </div>
      </PopoverContent>
    </Popover>
  );
}

function itemTypeLabel(
  messages: ReturnType<typeof getSubmitWorkMessages>,
  itemType: SubmitWorkDraftItemType,
): string {
  return messages.addItemOptionLabel(itemType);
}
