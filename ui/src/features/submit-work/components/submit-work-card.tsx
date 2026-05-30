import { useState, type ReactNode } from "react";
import { Plus } from "lucide-react";

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
import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SUPPORTING_LABEL_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../../components/ui/dashboard-typography";
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
}

export type SubmitWorkDraftItem = SubmitWorkDraftFileItem | SubmitWorkDraftTextItem;
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

const FORM_CLASS = "grid h-full min-h-0 content-start gap-3";
const FORM_FIELDS_CLASS = "grid min-h-0 content-start gap-4 overflow-y-auto pr-1";
const FIELD_GROUP_CLASS = "grid gap-2";
const ACTION_ROW_CLASS = "grid gap-3";
const HELP_TEXT_CLASS = cn(
  "max-w-xl leading-relaxed text-af-text-muted",
  DASHBOARD_SUPPORTING_TEXT_CLASS,
);
const VALIDATION_TEXT_CLASS = cn(
  "text-af-danger-text",
  DASHBOARD_SUPPORTING_TEXT_CLASS,
);
const STATUS_TONE_CLASS_BY_KIND: Record<SubmitWorkStatus["kind"], string> = {
  error: "text-af-danger-text",
  guidance: "text-af-text-subtle",
  submitting: "text-af-text",
  success: "text-af-success-text",
  "validation-error": "text-af-danger-text",
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
        className={FORM_CLASS}
        onSubmit={(event) => {
          event.preventDefault();
          onSubmit();
        }}
      >
        <div className={FORM_FIELDS_CLASS}>
          <div className={FIELD_GROUP_CLASS}>
            <label
              className={DASHBOARD_SUPPORTING_LABEL_CLASS}
              htmlFor={requestNameID}
            >
              {messages.requestNameLabel}
            </label>
            <Input
              aria-describedby={
                validationErrors?.requestName ? requestNameErrorID : undefined
              }
              aria-invalid={validationErrors?.requestName ? "true" : undefined}
              className={DASHBOARD_BODY_TEXT_CLASS}
              disabled={controlsDisabled}
              id={requestNameID}
              onChange={(event) => onRequestNameChange(event.target.value)}
              placeholder={messages.requestNamePlaceholder}
              type="text"
              value={draft.requestName}
            />
            {validationErrors?.requestName ? (
              <p className={VALIDATION_TEXT_CLASS} id={requestNameErrorID}>
                {validationErrors.requestName}
              </p>
            ) : null}
          </div>

          <div className={FIELD_GROUP_CLASS}>
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
              <p className={VALIDATION_TEXT_CLASS}>
                {validationErrors.submissionItems}
              </p>
            ) : null}
          </div>
        </div>

        <div className={ACTION_ROW_CLASS}>
          {shouldRenderStatus ? (
            <p
              className={cn(
                "min-w-0 flex-1",
                HELP_TEXT_CLASS,
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
            className="w-full justify-center self-start"
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
          className={cn("min-h-9 py-2 text-xs", DASHBOARD_BODY_TEXT_CLASS)}
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
        <p className={HELP_TEXT_CLASS} id={menuDescriptionID}>
          {messages.addItemMenuDescription}
        </p>
        <div className="grid gap-2">
          {ADDABLE_ITEM_TYPES.map((itemType) => {
            const typeLabel = itemTypeLabel(messages, itemType);
            return (
              <Button
                aria-label={typeLabel}
                className="justify-start border-af-border text-left font-medium"
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
