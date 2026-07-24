import { OptionalEnumSelect } from "@you-agent-factory/components/forms";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@you-agent-factory/components/overlays";
import { Plus } from "lucide-react";
import { type ReactNode, useState } from "react";
import {
  Button,
  DashboardIconButtonShell,
  FormError,
  Input,
  Label,
  Text,
} from "../../../components/ui";
import { DashboardWidgetFrame } from "../../bento/public";
import { getSubmitWorkMessages } from "../messages/submit-work";
import { SubmissionItemsList } from "./submit-work-items-list";
import { SubmitWorkStatusPanel } from "./submit-work-status-panel";

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
  const canAttemptSubmit =
    hasConfiguredWorkTypes && !hasIncompleteFileItems && !isSubmitting;
  const isFormReady =
    canAttemptSubmit && hasSelectedWorkType && hasValidRequestName;
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
        className="grid content-start gap-3"
        onSubmit={(event) => {
          event.preventDefault();
          onSubmit();
        }}
      >
        <div
          className="grid content-start gap-4"
          data-submit-work-primary-content=""
        >
          <div className="grid gap-2">
            <label htmlFor={requestNameID}>
              <Label>
                {messages.requestNameLabel}{" "}
                <Text
                  as="span"
                  className="text-on-error-container"
                  variant="supporting"
                >
                  ({messages.requestNameRequiredAffordance})
                </Text>
              </Label>
            </label>
            <Input
              aria-describedby={
                validationErrors?.requestName ? requestNameErrorID : undefined
              }
              aria-invalid={validationErrors?.requestName ? "true" : undefined}
              aria-required="true"
              disabled={controlsDisabled}
              id={requestNameID}
              onChange={(event) => onRequestNameChange(event.target.value)}
              placeholder={messages.requestNamePlaceholder}
              type="text"
              value={draft.requestName}
            />
            {validationErrors?.requestName ? (
              <FormError id={requestNameErrorID}>
                {validationErrors.requestName}
              </FormError>
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
              <Text className="text-on-error-container" variant="supporting">
                {validationErrors.submissionItems}
              </Text>
            ) : null}
          </div>
        </div>

        <div className="grid gap-3">
          {shouldRenderStatus ? (
            <SubmitWorkStatusPanel id={statusID} status={status} />
          ) : null}
          <Button
            aria-busy={isSubmitting ? "true" : undefined}
            className="w-full justify-center"
            disabled={!canAttemptSubmit}
            tone={isFormReady ? "default" : "outline"}
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
          {messages.workTypeLabel} ({messages.workTypeRequiredAffordance})
        </label>
        <OptionalEnumSelect
          aria-describedby={
            validationErrors?.workTypeName ? workTypeErrorID : undefined
          }
          aria-invalid={validationErrors?.workTypeName ? "true" : undefined}
          aria-required="true"
          className="min-h-9 py-2 text-xs"
          disabled={controlsDisabled}
          emptyOptionLabel={messages.selectWorkTypePlaceholder}
          id={workTypeID}
          onValueChange={(value) => onWorkTypeNameChange(value ?? "")}
          options={submitWorkTypeNames.map((submitWorkTypeName) => ({
            label: submitWorkTypeName,
            value: submitWorkTypeName,
          }))}
          value={workTypeName || null}
        />
        {validationErrors?.workTypeName ? (
          <FormError id={workTypeErrorID}>
            {validationErrors.workTypeName}
          </FormError>
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
        <Text
          className="text-on-surface-variant"
          id={menuDescriptionID}
          variant="supporting"
        >
          {messages.addItemMenuDescription}
        </Text>
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
