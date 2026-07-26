import { Text } from "@you-agent-factory/components/primitives";
import { DashboardIconButtonShell } from "../../../components/ui/dashboard-icon-button-shell";
import { Textarea } from "../../../components/ui/textarea";
import { WorkContentItemShell } from "../../work-content/components/work-content-item-shell";
import { submitWorkItemRowTypeLabel } from "../lib/submit-work-item-type-label";
import type { getSubmitWorkMessages } from "../messages/submit-work";
import type {
  SubmitWorkDraft,
  SubmitWorkDraftFileItem,
  SubmitWorkDraftTextItem,
} from "./submit-work-card";
import { FileSubmissionItemEditor } from "./submit-work-file-input";

export function SubmissionItemsList({
  controlsDisabled,
  draft,
  locale,
  messages,
  onItemTextChange,
  onRemoveItem,
  onStageFileItems,
  submissionItemsID,
  widgetId,
}: {
  controlsDisabled: boolean;
  draft: SubmitWorkDraft;
  locale?: string;
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
        const typeLabel = submitWorkItemRowTypeLabel(item.type, locale);

        return (
          <WorkContentItemShell
            headerActions={
              <DashboardIconButtonShell
                aria-label={messages.removeItemLabel(typeLabel, index + 1)}
                disabled={controlsDisabled}
                onClick={() => {
                  if (controlsDisabled) {
                    return;
                  }
                  onRemoveItem(item.id);
                }}
                tone="dangerGhost"
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
            }
            itemTypeLabel={typeLabel}
            key={item.id}
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
                locale={locale}
                messages={messages}
                onStageFileItems={onStageFileItems}
                widgetId={widgetId}
              />
            )}
          </WorkContentItemShell>
        );
      })}
    </ol>
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
        disabled={disabled}
        id={requestTextID}
        onChange={(event) => onChange(item.id, event.target.value)}
        placeholder={messages.requestPlaceholder}
        value={item.text}
      />
      {requestHint ? (
        <Text
          className="max-w-xl leading-relaxed text-on-surface-variant"
          id={requestTextHintID}
          variant="supporting"
        >
          {requestHint}
        </Text>
      ) : null}
    </>
  );
}

function FileSubmissionItemEditorShell({
  disabled,
  item,
  locale,
  messages,
  onStageFileItems,
  widgetId,
}: {
  disabled: boolean;
  item: SubmitWorkDraftFileItem;
  locale?: string;
  messages: ReturnType<typeof getSubmitWorkMessages>;
  onStageFileItems: (itemId: string, files: File[]) => void;
  widgetId: string;
}) {
  return (
    <FileSubmissionItemEditor
      disabled={disabled}
      inputID={`${widgetId}-${item.id}-file`}
      item={item}
      locale={locale}
      messages={messages}
      onStageFileItems={(files: File[]) => onStageFileItems(item.id, files)}
    />
  );
}
