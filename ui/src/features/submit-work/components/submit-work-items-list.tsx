import type { ReactNode } from "react";

import {
  DashboardIconButtonShell,
  Textarea,
} from "../../../components/ui";
import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SUPPORTING_LABEL_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../../components/ui/dashboard-typography";
import { cn } from "../../../lib/cn";
import type { getSubmitWorkMessages } from "../messages/submit-work";
import type {
  SubmitWorkDraft,
  SubmitWorkDraftFileItem,
  SubmitWorkDraftItemType,
  SubmitWorkDraftTextItem,
} from "./submit-work-card";
import { FileSubmissionItemEditor } from "./submit-work-file-input";

const FIELD_LABEL_CLASS = DASHBOARD_SUPPORTING_LABEL_CLASS;
const HELP_TEXT_CLASS = cn(
  "max-w-xl leading-relaxed text-af-text-muted",
  DASHBOARD_SUPPORTING_TEXT_CLASS,
);
const VALIDATION_TEXT_CLASS = cn(
  "text-af-danger-text",
  DASHBOARD_SUPPORTING_TEXT_CLASS,
);
export function SubmissionItemsList({
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
            disabled={controlsDisabled}
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
  disabled = false,
  itemTypeLabel,
  onRemove,
  removeLabel,
}: {
  children: ReactNode;
  disabled?: boolean;
  itemTypeLabel: string;
  onRemove: () => void;
  removeLabel: string;
}) {
  return (
    <li className="grid gap-3 rounded-lg border-af-border border bg-af-panel p-3">
      <div className="flex items-start justify-between gap-3">
        <div className="grid gap-1">
          <span className={FIELD_LABEL_CLASS}>{itemTypeLabel}</span>
        </div>
        <DashboardIconButtonShell
          aria-label={removeLabel}
          className="text-af-text-subtle hover:border-af-danger-border hover:bg-af-danger-surface hover:text-af-danger-text"
          disabled={disabled}
          onClick={() => {
            if (disabled) {
              return;
            }
            onRemove();
          }}
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
        </DashboardIconButtonShell>
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
      <Textarea
        aria-label={itemLabel}
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
