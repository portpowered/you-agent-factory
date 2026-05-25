import { useState, type ReactNode } from "react";

import {
  Button,
  DashboardWidgetFrame,
  Input,
  Popover,
  PopoverContent,
  PopoverTrigger,
  Select,
  Textarea,
  buttonVariants,
} from "../../../components/ui";
import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SUPPORTING_LABEL_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../../components/ui/dashboard-typography";
import { cn } from "../../../lib/cn";
import { getSubmitWorkMessages } from "../messages/submit-work";
import { FileSubmissionItemEditor } from "./submit-work-file-input";

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

const FORM_CLASS = "grid h-full min-h-0 gap-4";
const FIELD_GROUP_CLASS = "grid gap-2";
const FIELD_LABEL_CLASS = DASHBOARD_SUPPORTING_LABEL_CLASS;
const ACTION_ROW_CLASS = "mt-auto flex items-start gap-3";
const HELP_TEXT_CLASS = cn(
  "max-w-xl leading-relaxed text-af-text-muted",
  DASHBOARD_SUPPORTING_TEXT_CLASS,
);
const VALIDATION_TEXT_CLASS = cn(
  "text-af-danger-text",
  DASHBOARD_SUPPORTING_TEXT_CLASS,
);
const ITEM_SHELL_CLASS =
  "grid gap-3 rounded-lg border border-af-border-subtle bg-af-panel p-3";
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

  return (
    <DashboardWidgetFrame
      headerAction={headerAction}
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
        <div className={FIELD_GROUP_CLASS}>
          <label className={FIELD_LABEL_CLASS} htmlFor={workTypeID}>
            {messages.workTypeLabel}
          </label>
          <Select
            aria-describedby={
              validationErrors?.workTypeName ? workTypeErrorID : undefined
            }
            aria-invalid={validationErrors?.workTypeName ? "true" : undefined}
            className={DASHBOARD_BODY_TEXT_CLASS}
            disabled={controlsDisabled}
            id={workTypeID}
            onChange={(event) => onWorkTypeNameChange(event.target.value)}
            value={draft.workTypeName}
          >
            <option value="">{messages.selectWorkTypePlaceholder}</option>
            {submitWorkTypeNames.map((workTypeName) => (
              <option key={workTypeName} value={workTypeName}>
                {workTypeName}
              </option>
            ))}
          </Select>
          {validationErrors?.workTypeName ? (
            <p className={VALIDATION_TEXT_CLASS} id={workTypeErrorID}>
              {validationErrors.workTypeName}
            </p>
          ) : null}
        </div>

        <div className={FIELD_GROUP_CLASS}>
          <label className={FIELD_LABEL_CLASS} htmlFor={requestNameID}>
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
          <div className={FIELD_LABEL_CLASS} id={submissionItemsID}>
            {messages.submissionItemsLabel}
          </div>
          <AddSubmissionItemMenu
            controlsDisabled={controlsDisabled}
            isOpen={isAddItemMenuOpen}
            messages={messages}
            onAddItem={onAddItem}
            onOpenChange={setIsAddItemMenuOpen}
          />
          <SubmissionItemsList
            controlsDisabled={controlsDisabled}
            draft={draft}
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

        <div className={ACTION_ROW_CLASS}>
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
          <Button
            aria-busy={isSubmitting ? "true" : undefined}
            className="ml-auto shrink-0 self-start"
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

function AddSubmissionItemMenu({
  controlsDisabled,
  isOpen,
  messages,
  onAddItem,
  onOpenChange,
}: {
  controlsDisabled: boolean;
  isOpen: boolean;
  messages: ReturnType<typeof getSubmitWorkMessages>;
  onAddItem: (type: SubmitWorkDraftItemType) => void;
  onOpenChange: (open: boolean) => void;
}) {
  return (
    <div className="flex justify-start">
      <Popover onOpenChange={onOpenChange} open={isOpen}>
        <PopoverTrigger
          aria-label={messages.addItemAction}
          className={buttonVariants({ size: "sm", tone: "outline" })}
          disabled={controlsDisabled}
          type="button"
        >
          {messages.addItemAction}
        </PopoverTrigger>
        <PopoverContent
          align="start"
          aria-label={messages.addItemMenuLabel}
          className="grid gap-3"
        >
          <p className={HELP_TEXT_CLASS}>{messages.addItemMenuDescription}</p>
          <div className="grid gap-2">
            {ADDABLE_ITEM_TYPES.map((itemType) => {
              const typeLabel = itemTypeLabel(messages, itemType);
              return (
                <button
                  aria-label={typeLabel}
                  className={buttonVariants({
                    className:
                      "justify-start rounded-lg border-af-border text-left font-medium",
                    tone: "outline",
                  })}
                  key={itemType}
                  onClick={() => {
                    onAddItem(itemType);
                    onOpenChange(false);
                  }}
                  type="button"
                >
                  {typeLabel}
                </button>
              );
            })}
          </div>
        </PopoverContent>
      </Popover>
    </div>
  );
}

function SubmissionItemsList({
  controlsDisabled,
  draft,
  messages,
  onItemTextChange,
  onRemoveItem,
  onStageFileItems,
  submissionItemsID,
  widgetId,
}: {
  controlsDisabled: boolean;
  draft: SubmitWorkDraft;
  messages: ReturnType<typeof getSubmitWorkMessages>;
  onItemTextChange: (itemId: string, value: string) => void;
  onRemoveItem: (itemId: string) => void;
  onStageFileItems: (itemId: string, files: File[]) => void;
  submissionItemsID: string;
  widgetId: string;
}) {
  return (
    <ol aria-labelledby={submissionItemsID} className="grid gap-3">
      {draft.items.map((item, index) => {
        const requestItemLabel = messages.requestItemLabel(index + 1);
        const typeLabel = itemTypeLabel(messages, item.type);

        return (
          <SubmitWorkItemShell
            itemLabel={requestItemLabel}
            itemTypeLabel={typeLabel}
            key={item.id}
            onRemove={() => onRemoveItem(item.id)}
            removeLabel={messages.removeItemLabel(typeLabel, index + 1)}
          >
            {item.type === "text" ? (
              <TextSubmissionItemEditor
                disabled={controlsDisabled}
                item={item}
                itemLabel={requestItemLabel}
                messages={messages}
                onChange={onItemTextChange}
                widgetId={widgetId}
              />
            ) : (
              <FileSubmissionItemEditorShell
                disabled={controlsDisabled}
                item={item}
                messages={messages}
                onStageFileItems={onStageFileItems}
                widgetId={widgetId}
              />
            )}
          </SubmitWorkItemShell>
        );
      })}
    </ol>
  );
}

function SubmitWorkItemShell({
  children,
  itemLabel,
  itemTypeLabel,
  onRemove,
  removeLabel,
}: {
  children: ReactNode;
  itemLabel: string;
  itemTypeLabel: string;
  onRemove: () => void;
  removeLabel: string;
}) {
  return (
    <li className={ITEM_SHELL_CLASS}>
      <div className="flex items-start justify-between gap-3">
        <div className="grid gap-1">
          <span className={FIELD_LABEL_CLASS}>{itemTypeLabel}</span>
          <span className={HELP_TEXT_CLASS}>{itemLabel}</span>
        </div>
        <button
          aria-label={removeLabel}
          className="inline-grid size-8 shrink-0 place-items-center rounded-md border border-af-border bg-transparent text-af-text-subtle transition-colors hover:border-af-danger-border hover:bg-af-danger-surface hover:text-af-danger-text focus-visible:ring-2 focus-visible:ring-af-focus-ring focus-visible:ring-offset-0"
          onClick={onRemove}
          type="button"
        >
          <svg
            aria-hidden="true"
            fill="none"
            height="16"
            viewBox="0 0 16 16"
            width="16"
            xmlns="http://www.w3.org/2000/svg"
          >
            <path
              d="m4 4 8 8M12 4 4 12"
              stroke="currentColor"
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth="1.7"
            />
          </svg>
        </button>
      </div>
      {children}
    </li>
  );
}

function TextSubmissionItemEditor({
  disabled,
  item,
  itemLabel,
  messages,
  onChange,
  widgetId,
}: {
  disabled: boolean;
  item: SubmitWorkDraftTextItem;
  itemLabel: string;
  messages: ReturnType<typeof getSubmitWorkMessages>;
  onChange: (itemId: string, value: string) => void;
  widgetId: string;
}) {
  const requestTextID = `${widgetId}-${item.id}-text`;
  const requestTextHintID = `${widgetId}-${item.id}-text-hint`;
  const requestHint = messages.requestHint?.trim();

  return (
    <>
      <label className={FIELD_LABEL_CLASS} htmlFor={requestTextID}>
        {itemLabel}
      </label>
      <Textarea
        aria-describedby={requestHint ? requestTextHintID : undefined}
        className={DASHBOARD_BODY_TEXT_CLASS}
        disabled={disabled}
        id={requestTextID}
        onChange={(event) => onChange(item.id, event.target.value)}
        placeholder={messages.requestPlaceholder}
        value={item.text}
      />
      {requestHint ? (
        <p className={HELP_TEXT_CLASS} id={requestTextHintID}>
          {requestHint}
        </p>
      ) : null}
    </>
  );
}

function FileSubmissionItemEditorShell({
  disabled,
  item,
  messages,
  onStageFileItems,
  widgetId,
}: {
  disabled: boolean;
  item: SubmitWorkDraftFileItem;
  messages: ReturnType<typeof getSubmitWorkMessages>;
  onStageFileItems: (itemId: string, files: File[]) => void;
  widgetId: string;
}) {
  return (
    <FileSubmissionItemEditor
      disabled={disabled}
      fieldLabelClassName={FIELD_LABEL_CLASS}
      helpTextClassName={HELP_TEXT_CLASS}
      inputID={`${widgetId}-${item.id}-file`}
      item={item}
      messages={messages}
      onStageFileItems={(files: File[]) => onStageFileItems(item.id, files)}
      validationTextClassName={VALIDATION_TEXT_CLASS}
    />
  );
}

function itemTypeLabel(
  messages: ReturnType<typeof getSubmitWorkMessages>,
  itemType: SubmitWorkDraftItemType,
): string {
  return messages.addItemOptionLabel(itemType);
}
